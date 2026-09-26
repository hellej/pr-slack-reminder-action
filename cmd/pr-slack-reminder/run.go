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

// See run.spec.md for this function's full behaviour.
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

	generatedAt := time.Now().UTC()
	openPRs, err := findOpenPRs(githubClient, cfg, githubclient.PRFetchOptions{
		IncludeDrafts: cfg.CanvasEnabled(),
	})
	if err != nil {
		return err
	}
	mergedPRs, mergedPRsErr := findRecentlyMergedPRs(githubClient, cfg, generatedAt)
	if mergedPRsErr != nil {
		logWarning(fmt.Sprintf("Failed to fetch recently merged PRs: %v", mergedPRsErr))
	}

	var messageErr, canvasErr, stateErr error
	var stateToSave *state.State

	switch cfg.RunMode {
	case config.RunModePost:
		stateToSave, messageErr = runPostMode(
			githubClient, slackClient, cfg, openPRs, mergedPRs, generatedAt, sentMessageHandler,
		)
	case config.RunModeUpdate:
		stateToSave, messageErr = runUpdateMode(
			githubClient, slackClient, cfg, openPRs, mergedPRs, mergedPRsErr, generatedAt,
			sentMessageHandler,
		)
	default:
		return fmt.Errorf("unsupported run mode: %s", cfg.RunMode)
	}

	var lastWrittenCanvasMarkdownHash string
	if stateToSave != nil {
		lastWrittenCanvasMarkdownHash = stateToSave.LastWrittenCanvasMarkdownHash
	}
	if cfg.CanvasEnabled() {
		lastWrittenCanvasMarkdownHash, canvasErr = refreshPRTrackerCanvas(
			slackClient, cfg, openPRs, mergedPRs, mergedPRsErr, generatedAt, lastWrittenCanvasMarkdownHash,
		)
	}
	canvasErr = errors.Join(utilities.Map(splitJoinedError(canvasErr), func(part error) error {
		return fmt.Errorf("PR tracker canvas refresh failed: %w", part)
	})...)
	if stateToSave != nil {
		stateToSave.LastWrittenCanvasMarkdownHash = lastWrittenCanvasMarkdownHash
		stateErr = state.Save(cfg.StateFilePath, *stateToSave)
	}
	return errors.Join(messageErr, canvasErr, stateErr)
}

// Returns a nil state when there is nothing to send, so callers write nothing.
func runPostMode(
	githubClient githubclient.Client,
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
		time.Time{},
		generatedAt,
		cfg.ContentInputs,
	)
	if !content.HasPRs() && content.NoOpenPRsText == "" {
		log.Println("No PRs found and no-prs-message is set to empty, exiting")
		return nil, nil
	}
	message, summaryText := messagebuilder.BuildMessageToPost(content)

	sentMessageInfo, err := slackClient.SendMessage(cfg.SlackChannelID, message, summaryText)
	if err != nil {
		return nil, err
	}

	postState := state.NewPostState(prViews, sentMessageInfo, summaryText, generatedAt)
	return &postState, errors.Join(
		sentMessageHandler(sentMessageInfo),
		markPreviousMessageStale(githubClient, slackClient, cfg, sentMessageInfo.ChannelID),
	)
}

// This run's own state is uploaded after it ends, so the load still finds the previous post's.
// Channel check: see run.spec.md § Behaviour.
func markPreviousMessageStale(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	cfg config.Config,
	newMessageChannelID string,
) error {
	previousState, err := state.Load(
		context.Background(),
		githubClient,
		cfg.CurrentRepository,
		cfg.StateArtifactName,
		cfg.StateFilePath,
	)
	if errors.Is(err, githubclient.ErrNoArtifactFound) {
		log.Printf("Not marking the previous message stale, there is no previous state: %v", err)
		return nil
	}
	if err != nil {
		logWarning(fmt.Sprintf("Not marking the previous message stale, its state did not load: %v", err))
		return nil
	}
	lastWrittenMessage := previousState.LastWrittenMessage
	if len(lastWrittenMessage.Blocks) == 0 {
		log.Println("Not marking the previous message stale, its state records no sent message")
		return nil
	}
	if previousState.MessageRef.ChannelID != newMessageChannelID {
		log.Printf(
			"Not marking the previous message stale, it is in channel %s, not in %s where this run posted",
			previousState.MessageRef.ChannelID, newMessageChannelID,
		)
		return nil
	}

	messageMarkedStale, err := messagebuilder.BuildMessageMarkedStale(lastWrittenMessage.Blocks, lastWrittenMessage.GeneratedAt)
	if err != nil {
		return fmt.Errorf("failed to mark the previous message stale: %w", err)
	}
	_, err = slackClient.UpdateMessage(
		previousState.MessageRef.ChannelID,
		previousState.MessageRef.MessageTS,
		messageMarkedStale,
		lastWrittenMessage.SummaryText,
	)
	if errors.Is(err, slackclient.ErrMessageNotEditable) {
		logWarning(fmt.Sprintf("Not marking the previous message stale: %v", err))
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to mark the previous message stale: %w", err)
	}
	log.Println("Marked the previous message stale")
	return nil
}

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
		logWarning(fmt.Sprintf("Failed to fetch the tracked PRs the run's own fetches left unresolved: %v", trackedPRsErr))
	}

	content := messagecontent.GetContent(
		buildNonDraftPRViews(openPRs, cfg),
		prview.BuildPRViews(trackedPRs, cfg.ContentInputs),
		prview.BuildPRViews(mergedPRs, cfg.ContentInputs),
		loadedState.MessagePostedAt,
		generatedAt,
		cfg.ContentInputs,
	)

	if !content.HasPRs() && content.NoOpenPRsText == "" {
		if mergedPRsErr != nil || trackedPRsErr != nil {
			log.Println("Keeping the Slack message: nothing to show, but a PR fetch failed")
			return loadedState, nil
		}
		log.Println("Nothing left to show: no open PRs and no merged ones")
		log.Println("Deleting Slack message as no-prs-message is set to empty")
		if err := slackClient.DeleteMessage(
			loadedState.MessageRef.ChannelID,
			loadedState.MessageRef.MessageTS,
		); err != nil {
			logWarning(fmt.Sprintf("Keeping the Slack message with nothing left to show, its delete failed: %v", err))
		}
		return loadedState, nil
	}
	if !content.HasPRs() {
		log.Printf("Updating Slack message with no-prs-message: %s", content.NoOpenPRsText)
	}

	message, summaryText := messagebuilder.BuildMessageWithUpdateTimeFooter(content)

	sentMessageInfo, err := slackClient.UpdateMessage(
		loadedState.MessageRef.ChannelID,
		loadedState.MessageRef.MessageTS,
		message,
		summaryText,
	)
	if err != nil {
		return loadedState, err
	}
	editedState := state.WithLastWrittenMessage(*loadedState, sentMessageInfo, summaryText, generatedAt)
	return &editedState, sentMessageHandler(sentMessageInfo)
}

// Resolves each tracked PR ref the run's own open and merged fetches didn't already answer for.
// See run.spec.md for what counts as resolved and why an already-resolved merged PR still
// counts as tracked.
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
			config.SentSlackBlocksFilePath, sentMessageInfo.BlocksAsSent,
		); err != nil {
			return err
		}
		return nil
	}
}
