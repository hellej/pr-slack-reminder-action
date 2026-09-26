package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockgithubclient"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockslackclient"
)

func TestLogWarningWritesOneWarningAnnotation(t *testing.T) {
	testCases := []struct {
		name         string
		message      string
		expectedLine string
	}{
		{
			name:         "plain message",
			message:      "Failed to fetch recently merged PRs: search failed",
			expectedLine: "::warning title=PR Slack Reminder::Failed to fetch recently merged PRs: search failed\n",
		},
		{
			name:         "percent sign",
			message:      "100% of PRs",
			expectedLine: "::warning title=PR Slack Reminder::100%25 of PRs\n",
		},
		{
			name:         "an already escaped newline stays literal text",
			message:      "literal %0A in a title",
			expectedLine: "::warning title=PR Slack Reminder::literal %250A in a title\n",
		},
		{
			name:         "carriage return and line feed",
			message:      "line one\r\nline two",
			expectedLine: "::warning title=PR Slack Reminder::line one%0D%0Aline two\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logOutput := testhelpers.CaptureLog(t)

			logWarning(tc.message)

			if logOutput.String() != tc.expectedLine {
				t.Errorf("Expected %q, got %q", tc.expectedLine, logOutput.String())
			}
		})
	}
}

type joinKeepingNilParts struct{ parts []error }

func (joined joinKeepingNilParts) Error() string {
	nonNilParts := slices.DeleteFunc(slices.Clone(joined.parts), func(part error) bool { return part == nil })
	return errors.Join(nonNilParts...).Error()
}

func (joined joinKeepingNilParts) Unwrap() []error { return joined.parts }

func TestLogErrorWritesOneErrorAnnotationPerJoinedPart(t *testing.T) {
	testCases := []struct {
		name          string
		err           error
		expectedLines string
	}{
		{
			name:          "nil error",
			err:           nil,
			expectedLines: "",
		},
		{
			name:          "a single error",
			err:           errors.New("failed to send Slack message: invalid_auth"),
			expectedLines: "::error title=PR Slack Reminder::failed to send Slack message: invalid_auth\n",
		},
		{
			name:          "a single error with a newline",
			err:           errors.New("first line\nsecond line"),
			expectedLines: "::error title=PR Slack Reminder::first line%0Asecond line\n",
		},
		{
			name: "a nested join",
			err: errors.Join(
				errors.New("message failed"),
				errors.Join(errors.New("canvas write failed"), errors.New("merged fetch failed")),
			),
			expectedLines: "::error title=PR Slack Reminder::message failed\n" +
				"::error title=PR Slack Reminder::canvas write failed\n" +
				"::error title=PR Slack Reminder::merged fetch failed\n",
		},
		{
			name:          "nil parts",
			err:           joinKeepingNilParts{parts: []error{nil, errors.New("state save failed"), nil}},
			expectedLines: "::error title=PR Slack Reminder::state save failed\n",
		},
		{
			name: "fmt.Errorf with two %w, whose text is not its parts' texts",
			err: fmt.Errorf(
				"failed to update Slack message: %w: %w", errors.New("message cannot be edited"), errors.New("cant_update_message"),
			),
			expectedLines: "::error title=PR Slack Reminder::failed to update Slack message: message cannot be edited: cant_update_message\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logOutput := testhelpers.CaptureLog(t)

			logError(tc.err)

			if logOutput.String() != tc.expectedLines {
				t.Errorf("Expected\n%q\ngot\n%q", tc.expectedLines, logOutput.String())
			}
		})
	}
}

func TestLogErrorWritesOneErrorAnnotationPerFailedCanvasPart(t *testing.T) {
	testCases := []struct {
		name                 string
		mergedPRsSearchError error
		expectedErrorLines   []string
	}{
		{
			name: "the canvas write fails",
			expectedErrorLines: []string{
				"::error title=PR Slack Reminder::PR tracker canvas refresh failed: canvas update failed: canvas_not_found",
			},
		},
		{
			name:                 "the canvas write and the merged PR fetch fail",
			mergedPRsSearchError: errors.New("search failed"),
			expectedErrorLines: []string{
				"::error title=PR Slack Reminder::PR tracker canvas refresh failed: canvas update failed: canvas_not_found",
				"::error title=PR Slack Reminder::PR tracker canvas refresh failed: error fetching merged pull requests: GraphQL request failed with status 500: unexpected response: {\"message\":\"search failed\"}",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
				config.InputPRTrackerCanvasLink: "https://hellej.slack.com/docs/T08SGDGNB2B/F0BMEPVR1DL",
			})
			runErr := Run(
				mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
					MergedPRsSearchError: tc.mergedPRsSearchError,
				}),
				mockslackclient.MakeSlackClientGetter(mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
					ReplaceCanvasError: errors.New("canvas_not_found"),
				})),
			)
			logOutput := testhelpers.CaptureLog(t)

			logError(runErr)

			expectedLogOutput := strings.Join(tc.expectedErrorLines, "\n") + "\n"
			if logOutput.String() != expectedLogOutput {
				t.Errorf("Expected\n%s\ngot\n%s", expectedLogOutput, logOutput.String())
			}
		})
	}
}
