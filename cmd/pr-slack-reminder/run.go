package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const prFetchTimeout = 60 * time.Second

func Run(
	getGitHubClient func(token, tokenForState string) githubclient.Client,
	getSlackClient func(token string) slackclient.Client,
) error {
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}
	cfg.Print()
	githubClient := getGitHubClient(cfg.GithubToken, cfg.GithubTokenForState)
	slackClient := getSlackClient(cfg.SlackBotToken)

	if cfg.SlackChannelID == "" {
		log.Println("Slack channel ID is not set, resolving it by name")
		channelID, err := slackClient.GetChannelIDByName(cfg.SlackChannelName)
		if err != nil {
			return fmt.Errorf("error getting channel ID by name: %v", err)
		}
		cfg.SlackChannelID = channelID
	}

	sentMessageHandler := getSentMessageHandler(cfg)

	// One "now" for the merged PR window and the canvas footer.
	generatedAt := time.Now().UTC()
	// A message edited or deleted over a fetch outage is one no later run can rebuild.
	openPRs, err := findOpenPRs(githubClient, cfg, githubclient.PRFetchOptions{
		IncludeDrafts: cfg.CanvasEnabled(),
	})
	if err != nil {
		return err
	}
	// A failed merged fetch costs the message and the canvas their merged rows, so the run
	// carries on without them. It also leaves update mode unable to tell an empty day from an
	// outage, which is why it takes the error along.
	mergedPRs, mergedPRsErr := findRecentlyMergedPRs(githubClient, cfg, generatedAt)
	if mergedPRsErr != nil {
		log.Printf("Failed to fetch recently merged PRs: %v", mergedPRsErr)
	}

	// The message path and the canvas refresh are independent attempts: a failing reminder
	// says nothing about whether the canvas can be written, and a stale canvas is what this
	// feature exists to prevent. Their errors are collected instead of short-circuited.
	var messageErr, canvasErr, stateErr error
	var stateToSave *state.State

	switch cfg.RunMode {
	case config.RunModePost:
		stateToSave, messageErr = runPostMode(
			slackClient, cfg, openPRs, mergedPRs, generatedAt, sentMessageHandler,
		)
	case config.RunModeUpdate:
		stateToSave, messageErr = runUpdateMode(
			githubClient, slackClient, cfg, openPRs, mergedPRs, mergedPRsErr, generatedAt,
			sentMessageHandler,
		)
	default:
		return fmt.Errorf("unsupported run mode: %s", cfg.RunMode)
	}

	var canvasContentHash string
	if stateToSave != nil {
		canvasContentHash = stateToSave.CanvasContentHash
	}
	if cfg.CanvasEnabled() {
		canvasContentHash, canvasErr = refreshPRTrackerCanvas(
			slackClient, cfg, openPRs, mergedPRs, mergedPRsErr, generatedAt, canvasContentHash,
		)
	}
	if canvasErr != nil {
		canvasErr = fmt.Errorf("PR tracker canvas refresh failed: %w", canvasErr)
	}
	if stateToSave != nil {
		stateToSave.CanvasContentHash = canvasContentHash
		stateErr = state.Save(cfg.StateFilePath, *stateToSave)
	}
	return errors.Join(messageErr, canvasErr, stateErr)
}

// Returns the state to save, which is nil on every path that leaves no message to track, so
// that nothing is written.
func runPostMode(
	slackClient slackclient.Client,
	cfg config.Config,
	openPRs githubclient.OpenPRsResult,
	mergedPRs []githubclient.PR,
	generatedAt time.Time,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) (*state.State, error) {
	prViews := buildNonDraftPRViews(openPRs, cfg)
	content := messagecontent.GetContent(
		prViews,
		nil,
		prview.BuildPRViews(mergedPRs, cfg.ContentInputs),
		generatedAt,
		cfg.ContentInputs,
	)
	if !content.HasPRs() && content.NoOpenPRsText == "" {
		log.Println("No PRs found and no message configured for this case, exiting")
		return nil, nil
	}
	message, summaryText := messagebuilder.BuildMessage(content)

	sentMessageInfo, err := slackClient.SendMessage(cfg.SlackChannelID, message, summaryText)
	if err != nil {
		return nil, err
	}

	postState := state.NewPostState(prViews, sentMessageInfo)
	return &postState, sentMessageHandler(sentMessageInfo)
}

// Drafts are dropped by the same predicate that keeps them out of the fetch when the canvas is
// off, and before anything else sees the PRs.
func buildNonDraftPRViews(openPRs githubclient.OpenPRsResult, cfg config.Config) []prview.PR {
	nonDraftPRs := utilities.Filter(openPRs.PRs, func(pr githubclient.PR) bool {
		return !pr.GetDraft()
	})
	return prview.BuildPRViews(nonDraftPRs, cfg.ContentInputs)
}

func runUpdateMode(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	cfg config.Config,
	openPRs githubclient.OpenPRsResult,
	mergedPRs []githubclient.PR,
	mergedPRsErr error,
	generatedAt time.Time,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) (*state.State, error) {
	loadedState, err := state.Load(
		context.Background(),
		githubClient,
		cfg.CurrentRepository,
		cfg.StateArtifactName,
		cfg.StateFilePath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}
	trackedPRs, trackedPRsErr := resolveTrackedPRs(
		githubClient, cfg, loadedState.PullRequests, openPRs.PRs, mergedPRs,
	)
	if trackedPRsErr != nil {
		log.Printf("Failed to fetch the tracked PRs the run's own fetches left unresolved: %v", trackedPRsErr)
	}

	content := messagecontent.GetContent(
		buildNonDraftPRViews(openPRs, cfg),
		prview.BuildPRViews(trackedPRs, cfg.ContentInputs),
		prview.BuildPRViews(mergedPRs, cfg.ContentInputs),
		generatedAt,
		cfg.ContentInputs,
	)

	if !content.HasPRs() && content.NoOpenPRsText == "" {
		// Deleting takes proof that nothing is left to show, and a failed fetch is not proof:
		// the message stands until a run that can rebuild it says otherwise.
		if mergedPRsErr != nil || trackedPRsErr != nil {
			log.Println("Keeping the Slack message: nothing to show, but a PR fetch failed")
			return loadedState, nil
		}
		log.Println("Nothing left to show: no open PRs and no merged ones")
		log.Println("Deleting Slack message as no-prs-message input is not set")
		if err := slackClient.DeleteMessage(
			loadedState.SlackMessage.ChannelID,
			loadedState.SlackMessage.MessageTS,
		); err != nil {
			log.Printf("Warning: failed to delete message: %v", err)
		}
		return loadedState, nil
	}
	if !content.HasPRs() {
		log.Printf("Updating Slack message with no-prs-message: %s", content.NoOpenPRsText)
	}

	message, summaryText := messagebuilder.BuildMessage(content)

	sentMessageInfo, err := slackClient.UpdateMessage(
		loadedState.SlackMessage.ChannelID,
		loadedState.SlackMessage.MessageTS,
		message,
		summaryText,
	)
	if err != nil {
		return loadedState, err
	}
	return loadedState, sentMessageHandler(sentMessageInfo)
}

// The PRs the message was posted with, in whatever state they are now: the Merged section reads
// the merged ones. The run's own fetches answer for most of them, so only the rest are fetched.
// A ref the open fetch returned is open right now and reaches no section, and a ref the merged
// fetch returned arrives with the reviewers GetPRs would give it. Absence from a fetch means
// unresolved, never gone: the residue is every other ref, merged outside the fetch's window or
// past its cap, closed, filtered out, turned into a draft, or past the open fetch's cap.
//
// A tracked PR resolved off the merged fetch is returned as tracked, so the merged section's cap
// on untracked PRs cannot drop it.
func resolveTrackedPRs(
	githubClient githubclient.Client,
	cfg config.Config,
	references []models.PullRequestRef,
	openPRs []githubclient.PR,
	mergedPRs []githubclient.PR,
) ([]githubclient.PR, error) {
	trackedMergedPRs := utilities.Filter(mergedPRs, func(pr githubclient.PR) bool {
		return slices.Contains(references, refOf(pr))
	})
	resolvedReferences := utilities.Map(slices.Concat(openPRs, mergedPRs), refOf)
	unresolvedReferences := utilities.Filter(references, func(ref models.PullRequestRef) bool {
		return !slices.Contains(resolvedReferences, ref)
	})
	fetchedPRs, err := fetchUnresolvedPRs(githubClient, cfg, unresolvedReferences)
	if err != nil {
		return trackedMergedPRs, err
	}
	return slices.Concat(trackedMergedPRs, fetchedPRs), nil
}

// An empty ref slice makes GetPRs log a fetch it never sends, so that case skips the call.
func fetchUnresolvedPRs(
	githubClient githubclient.Client,
	cfg config.Config,
	references []models.PullRequestRef,
) ([]githubclient.PR, error) {
	if len(references) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	return githubClient.GetPRs(ctx, references, cfg.GetFiltersForRepository)
}

func refOf(pr githubclient.PR) models.PullRequestRef {
	return models.PullRequestRef{Repository: pr.Repository, Number: pr.GetNumber()}
}

func findOpenPRs(
	githubClient githubclient.Client,
	cfg config.Config,
	fetchOptions githubclient.PRFetchOptions,
) (githubclient.OpenPRsResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	return githubClient.FindOpenPRs(ctx, cfg.Repositories, cfg.GetFiltersForRepository, fetchOptions)
}

func findRecentlyMergedPRs(
	githubClient githubclient.Client, cfg config.Config, generatedAt time.Time,
) ([]githubclient.PR, error) {
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	return githubClient.FindRecentlyMergedPRs(
		ctx,
		cfg.Repositories,
		cfg.GetFiltersForRepository,
		generatedAt.Add(-githubclient.RecentlyMergedWindow),
	)
}

// Returns a handler function that saves the sent Slack message blocks as a JSON file.
// This is useful in both dry-run mode of the action (TODO) and in integration tests.
func getSentMessageHandler(config config.Config) func(slackclient.SentMessageInfo) error {
	return func(sentMessageInfo slackclient.SentMessageInfo) error {
		if err := state.SaveSentSlackBlocksToFile(
			config.SentSlackBlocksFilePath, sentMessageInfo.JSONBlocks,
		); err != nil {
			return err
		}
		return nil
	}
}
