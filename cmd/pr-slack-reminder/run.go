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
	"github.com/hellej/pr-slack-reminder-action/internal/mergestats"
	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// An open-PR fetch is a listing then one wave of enrichment batches, each call retried for up to 52s.
const prFetchTimeout = 2 * time.Minute

// Monday's stats cover the whole work week before it.
func isMondayInUTC(runStart time.Time) bool {
	return runStart.UTC().Weekday() == time.Monday
}

// See run.spec.md for this function's full behaviour.
func Run(
	getGitHubClient func(token, tokenForState string) githubclient.Client,
	getSlackClient func(token string) slackclient.Client,
	isMergeStatsDay bool,
) error {
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	cfg.Print()
	githubClient := getGitHubClient(cfg.GithubToken, cfg.GithubTokenForState)
	slackClient := getSlackClient(cfg.SlackBotToken)

	if cfg.SlackChannelID == "" {
		log.Println("Slack channel ID is not set, resolving it by name")
		channelID, err := slackClient.GetChannelIDByName(cfg.SlackChannelName)
		if err != nil {
			return fmt.Errorf("error getting channel ID by name: %w", err)
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
	loadedState, loadStateErr := loadPreviousState(githubClient, cfg)
	mergedPRsShownAnywhere := cfg.ContentInputs.ShowMergedPRsInMessage || cfg.CanvasEnabled()
	var mergedPRs []githubclient.PR
	var mergedPRsErr error
	if mergedPRsShownAnywhere {
		mergedPRs, mergedPRsErr = findRecentlyMergedPRs(
			githubClient, cfg, generatedAt, mergedPRCacheToReuse(cfg.RunMode, loadedState),
		)
	}
	if mergedPRsErr != nil {
		logWarning(fmt.Sprintf("Failed to fetch recently merged PRs: %v", mergedPRsErr))
	}

	var messageErr, canvasErr, stateErr error
	var stateToSave *state.State

	switch cfg.RunMode {
	case config.RunModePost:
		previousPostState := previousPostStateOrLogWhyMissing(loadedState, loadStateErr)
		mergeStats := mergeStatsForThisPost(githubClient, cfg, previousPostState, isMergeStatsDay, generatedAt)
		stateToSave, messageErr = runPostMode(
			slackClient, cfg, previousPostState, openPRs, mergedPRs, mergeStats, generatedAt, sentMessageHandler,
		)
	case config.RunModeUpdate:
		if loadStateErr != nil {
			messageErr = fmt.Errorf("failed to load state: %w", loadStateErr)
			break
		}
		stateToSave, messageErr = runUpdateMode(
			githubClient, slackClient, cfg, *loadedState, openPRs, mergedPRs, mergedPRsErr, generatedAt,
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
	slackClient slackclient.Client,
	cfg config.Config,
	previousPostState *state.State,
	openPRs githubclient.OpenPRsResult,
	mergedPRs []githubclient.PR,
	mergeStats *mergestats.MergeStats,
	generatedAt time.Time,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) (*state.State, error) {
	prViews := buildNonDraftPRViews(openPRs, cfg)
	recentMergeStatsOfPreviousPost := recentMergeStatsOf(previousPostState)
	content := messagecontent.GetContent(
		prViews,
		nil,
		prview.BuildPRViews(mergedPRs, cfg.ContentInputs),
		mergeStats,
		recentMergeStatsOfPreviousPost.WeekBefore(mergeStats),
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

	postState := state.NewPostState(prViews, sentMessageInfo, summaryText, generatedAt, mergeStats)
	postState = state.WithMergedPRCache(postState, mergedPRs)
	postState.RecentMergeStats = recentMergeStatsOfPreviousPost.WithAdded(mergeStats, generatedAt)
	var markAsStaleErr error
	if previousPostState != nil {
		markAsStaleErr = markPreviousMessageStale(slackClient, *previousPostState, sentMessageInfo.ChannelID)
	}
	return &postState, errors.Join(sentMessageHandler(sentMessageInfo), markAsStaleErr)
}

// This run's own state is uploaded after it ends, so the load finds the previous post's.
func previousPostStateOrLogWhyMissing(loadedState *state.State, loadStateErr error) *state.State {
	if errors.Is(loadStateErr, githubclient.ErrNoArtifactFound) {
		log.Printf("Not marking the previous message stale, there is no previous state: %v", loadStateErr)
		return nil
	}
	if loadStateErr != nil {
		logWarning(fmt.Sprintf("Not marking the previous message stale, its state did not load: %v", loadStateErr))
		return nil
	}
	return loadedState
}

func recentMergeStatsOf(previousPostState *state.State) state.RecentMergeStats {
	if previousPostState == nil {
		return nil
	}
	return previousPostState.RecentMergeStats
}

// Nil when this post computes none, or fails to.
func mergeStatsForThisPost(
	githubClient githubclient.Client,
	cfg config.Config,
	previousPostState *state.State,
	isMergeStatsDay bool,
	generatedAt time.Time,
) *mergestats.MergeStats {
	if !shouldComputeMergeStats(cfg.ContentInputs.ShowMergeStatsInMessage, isMergeStatsDay) {
		return nil
	}
	return computeMergeStats(githubClient, cfg, mergeStatsWindowStart(previousPostState, generatedAt), generatedAt)
}

func shouldComputeMergeStats(showMergeStatsInMessage *bool, isMergeStatsDay bool) bool {
	if showMergeStatsInMessage != nil {
		return *showMergeStatsInMessage
	}
	return isMergeStatsDay
}

func mergeStatsWindowStart(previousPostState *state.State, generatedAt time.Time) time.Time {
	longestWindowStart := generatedAt.Add(-githubclient.RecentlyMergedWindow)
	previousStatsComputedAt := recentMergeStatsOf(previousPostState).LastComputedAt()
	if previousStatsComputedAt != nil && previousStatsComputedAt.After(longestWindowStart) {
		return *previousStatsComputedAt
	}
	return longestWindowStart
}

// Channel check: see run.spec.md § Behaviour.
func markPreviousMessageStale(
	slackClient slackclient.Client,
	previousState state.State,
	newMessageChannelID string,
) error {
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

func loadPreviousState(githubClient githubclient.Client, cfg config.Config) (*state.State, error) {
	return state.Load(
		context.Background(),
		githubClient,
		cfg.CurrentRepository,
		cfg.StateArtifactName,
		cfg.StateFilePath,
	)
}

func mergedPRCacheToReuse(runMode config.RunMode, loadedState *state.State) []githubclient.PR {
	if runMode != config.RunModeUpdate || loadedState == nil {
		return nil
	}
	return loadedState.MergedPRCache
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
	loadedState state.State,
	openPRs githubclient.OpenPRsResult,
	mergedPRs []githubclient.PR,
	mergedPRsErr error,
	generatedAt time.Time,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) (*state.State, error) {
	var trackedPRs []githubclient.PR
	var trackedPRsErr error
	if cfg.ContentInputs.ShowMergedPRsInMessage {
		trackedPRs, trackedPRsErr = resolveTrackedPRs(
			githubClient, cfg, loadedState.TrackedPRs, openPRs.PRs, mergedPRs, loadedState.MergedPRCache,
		)
	}
	if trackedPRsErr != nil {
		logWarning(fmt.Sprintf("Failed to fetch the tracked PRs the run's own fetches left unresolved: %v", trackedPRsErr))
	}

	listedOpenPRViews := buildNonDraftPRViews(openPRs, cfg)
	content := messagecontent.GetContent(
		listedOpenPRViews,
		prview.BuildPRViews(trackedPRs, cfg.ContentInputs),
		prview.BuildPRViews(mergedPRs, cfg.ContentInputs),
		loadedState.MergeStats,
		loadedState.RecentMergeStats.WeekBefore(loadedState.MergeStats),
		loadedState.MessagePostedAt,
		generatedAt,
		cfg.ContentInputs,
	)

	if !content.HasPRs() && content.NoOpenPRsText == "" {
		mergedSectionFetchFailed := mergedPRsErr != nil || trackedPRsErr != nil
		if cfg.ContentInputs.ShowMergedPRsInMessage && mergedSectionFetchFailed {
			log.Println("Keeping the Slack message: nothing to show, but a PR fetch failed")
			return &loadedState, nil
		}
		log.Println("Nothing left to show: no open PRs and no merged ones")
		log.Println("Deleting Slack message as no-prs-message is set to empty")
		if err := slackClient.DeleteMessage(
			loadedState.MessageRef.ChannelID,
			loadedState.MessageRef.MessageTS,
		); err != nil {
			logWarning(fmt.Sprintf("Keeping the Slack message with nothing left to show, its delete failed: %v", err))
		}
		return &loadedState, nil
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
		return &loadedState, err
	}
	editedState := state.WithMergedPRCache(
		state.WithTrackedPRsAdded(
			state.WithLastWrittenMessage(loadedState, sentMessageInfo, summaryText, generatedAt),
			listedOpenPRViews,
		),
		slices.Concat(mergedPRs, trackedPRs),
	)
	return &editedState, sentMessageHandler(sentMessageInfo)
}

// Resolves each tracked PR ref the run's own open and merged fetches didn't already answer for.
// See run.spec.md for what counts as resolved and why an already-resolved merged PR still
// counts as tracked.
func resolveTrackedPRs(
	githubClient githubclient.Client,
	cfg config.Config,
	trackedRefs []models.PullRequestRef,
	openPRs []githubclient.PR,
	mergedPRs []githubclient.PR,
	mergedPRCache []githubclient.PR,
) ([]githubclient.PR, error) {
	refsNotListedOpen := refsMissingFrom(trackedRefs, openPRs)
	fromMergedFetch := prsWithRefIn(mergedPRs, refsNotListedOpen)
	fromCache := prsWithRefIn(mergedPRCache, refsMissingFrom(refsNotListedOpen, fromMergedFetch))
	refsToFetch := refsMissingFrom(refsNotListedOpen, slices.Concat(fromMergedFetch, fromCache))
	fetchedPRs, err := fetchPRsByRef(githubClient, cfg, refsToFetch)
	return slices.Concat(fromMergedFetch, fromCache, fetchedPRs), err
}

func prsWithRefIn(prs []githubclient.PR, refs []models.PullRequestRef) []githubclient.PR {
	return utilities.Filter(prs, func(pr githubclient.PR) bool {
		return slices.Contains(refs, pr.GetPullRequestRef())
	})
}

func refsMissingFrom(refs []models.PullRequestRef, prs []githubclient.PR) []models.PullRequestRef {
	refsOfPRs := utilities.Map(prs, githubclient.PR.GetPullRequestRef)
	return utilities.Filter(refs, func(ref models.PullRequestRef) bool {
		return !slices.Contains(refsOfPRs, ref)
	})
}

// An empty ref slice makes GetPRs log a fetch it never sends, so that case skips the call.
func fetchPRsByRef(
	githubClient githubclient.Client,
	cfg config.Config,
	refs []models.PullRequestRef,
) ([]githubclient.PR, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	return githubClient.GetPRs(ctx, refs, cfg.GetFiltersForRepository)
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
	githubClient githubclient.Client,
	cfg config.Config,
	generatedAt time.Time,
	mergedPRCache []githubclient.PR,
) ([]githubclient.PR, error) {
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	return githubClient.FindRecentlyMergedPRs(
		ctx,
		cfg.Repositories,
		cfg.GetFiltersForRepository,
		generatedAt.Add(-githubclient.RecentlyMergedWindow),
		mergedPRCache,
	)
}

func computeMergeStats(
	githubClient githubclient.Client, cfg config.Config, mergedSince, generatedAt time.Time,
) *mergestats.MergeStats {
	mergedPRs, err := findPRsMergedSince(githubClient, cfg, mergedSince)
	if err != nil {
		logWarning(fmt.Sprintf("Leaving the merge stats out, their merged PR fetch failed: %v", err))
		return nil
	}
	for _, incompletePR := range utilities.Filter(mergedPRs, hasIncompleteReviewerInfo) {
		logWarning(fmt.Sprintf(
			"Leaving a merged PR out of the merge stats medians, it came without complete reviewer info: %s#%d",
			incompletePR.Repository.GetPath(), incompletePR.GetNumber(),
		))
	}
	mergeStats := mergestats.FromMergedPRs(mergedPRs, mergedSince, generatedAt)
	log.Printf(
		"Merge stats since %s: %d merged, median %s to first review, %s to merge",
		mergedSince.Format(time.RFC3339),
		mergeStats.MergedCount,
		durationLogText(mergeStats.MedianTimeToFirstReview),
		durationLogText(mergeStats.MedianTimeToMerge),
	)
	return &mergeStats
}

func hasIncompleteReviewerInfo(pr githubclient.PR) bool {
	return !pr.HasCompleteReviewerInfo
}

func durationLogText(duration *time.Duration) string {
	if duration == nil {
		return "none"
	}
	return duration.String()
}

func findPRsMergedSince(
	githubClient githubclient.Client, cfg config.Config, mergedSince time.Time,
) ([]githubclient.PR, error) {
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	return githubClient.FindPRsMergedSince(ctx, cfg.Repositories, cfg.GetFiltersForRepository, mergedSince)
}

// Returns a handler function that saves the sent Slack message blocks as a JSON file.
// This is useful in both dry-run mode of the action (TODO) and in integration tests.
func getSentMessageHandler(config config.Config) func(slackclient.SentMessageInfo) error {
	return func(sentMessageInfo slackclient.SentMessageInfo) error {
		return state.SaveSentSlackBlocksToFile(config.SentSlackBlocksFilePath, sentMessageInfo.BlocksAsSent)
	}
}
