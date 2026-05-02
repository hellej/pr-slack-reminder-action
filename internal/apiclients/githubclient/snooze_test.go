package githubclient

import (
	"testing"
	"time"

	"github.com/google/go-github/v78/github"
)

func TestParseSnoozeComment(t *testing.T) {
	baseTime := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		body     string
		expected *time.Time
	}{
		{
			name:     "snooze for N days",
			body:     "/snooze for 4 days",
			expected: timePtr(baseTime.Add(4 * 24 * time.Hour)),
		},
		{
			name:     "snooze for N d",
			body:     "/snooze for 4d",
			expected: timePtr(baseTime.Add(4 * 24 * time.Hour)),
		},
		{
			name:     "snooze for 1 day (singular)",
			body:     "/snooze for 1 day",
			expected: timePtr(baseTime.Add(1 * 24 * time.Hour)),
		},
		{
			name:     "snooze PR reminder for N days",
			body:     "/snooze PR reminder for 4 days",
			expected: timePtr(baseTime.Add(4 * 24 * time.Hour)),
		},
		{
			name:     "snooze pr-reminder for N d",
			body:     "/snooze pr-reminder for 4d",
			expected: timePtr(baseTime.Add(4 * 24 * time.Hour)),
		},
		{
			name:     "case insensitive",
			body:     "/SNOOZE PR REMINDER FOR 7 DAYS",
			expected: timePtr(baseTime.Add(7 * 24 * time.Hour)),
		},
		{
			name:     "zero days returns createdAt (effectively unsnooze)",
			body:     "/snooze for 0 days",
			expected: timePtr(baseTime),
		},
		{
			name:     "snooze with extra whitespace",
			body:     "/snooze   for   10   days",
			expected: timePtr(baseTime.Add(10 * 24 * time.Hour)),
		},
		{
			name:     "excessive days capped to 365",
			body:     "/snooze for 9999999 days",
			expected: timePtr(baseTime.Add(365 * 24 * time.Hour)),
		},
		{
			name:     "missing slash does not match",
			body:     "snooze for 4 days",
			expected: nil,
		},
		{
			name:     "missing for does not match",
			body:     "/snooze 4 days",
			expected: nil,
		},
		{
			name:     "missing unit does not match",
			body:     "/snooze for 4",
			expected: nil,
		},
		{
			name:     "bare number does not match",
			body:     "/snooze 4",
			expected: nil,
		},
		{
			name:     "non-matching comment",
			body:     "This is a regular comment",
			expected: nil,
		},
		{
			name:     "snooze in middle of text does not match",
			body:     "I think we should snooze for 4 days",
			expected: nil,
		},
		{
			name:     "empty body",
			body:     "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSnoozeComment(tt.body, baseTime)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("parseSnoozeComment(%q) = %v, expected nil", tt.body, result)
				}
			} else {
				if result == nil {
					t.Fatalf("parseSnoozeComment(%q) = nil, expected %v", tt.body, tt.expected)
				}
				if !result.Equal(*tt.expected) {
					t.Errorf("parseSnoozeComment(%q) = %v, expected %v", tt.body, result, tt.expected)
				}
			}
		})
	}
}

func TestFindActiveSnooze(t *testing.T) {
	now := time.Now()
	recentTime := now.Add(-1 * 24 * time.Hour) // 1 day ago
	olderTime := now.Add(-3 * 24 * time.Hour)  // 3 days ago

	tests := []struct {
		name             string
		timelineComments []*github.IssueComment
		expectSnoozed    bool
	}{
		{
			name:             "no comments",
			timelineComments: nil,
			expectSnoozed:    false,
		},
		{
			name: "no snooze comments",
			timelineComments: []*github.IssueComment{
				{Body: github.Ptr("looks good"), CreatedAt: &github.Timestamp{Time: recentTime}},
			},
			expectSnoozed: false,
		},
		{
			name: "active snooze comment",
			timelineComments: []*github.IssueComment{
				{Body: github.Ptr("/snooze for 5 days"), CreatedAt: &github.Timestamp{Time: recentTime}},
			},
			expectSnoozed: true,
		},
		{
			name: "expired snooze comment",
			timelineComments: []*github.IssueComment{
				{Body: github.Ptr("/snooze for 2 days"), CreatedAt: &github.Timestamp{Time: olderTime}},
			},
			expectSnoozed: false,
		},
		{
			name: "most recent snooze wins - active snooze after expired",
			timelineComments: []*github.IssueComment{
				{Body: github.Ptr("/snooze for 1 day"), CreatedAt: &github.Timestamp{Time: olderTime}},
				{Body: github.Ptr("/snooze for 7 days"), CreatedAt: &github.Timestamp{Time: recentTime}},
			},
			expectSnoozed: true,
		},
		{
			name: "most recent snooze wins - expired snooze after active",
			timelineComments: []*github.IssueComment{
				{Body: github.Ptr("/snooze for 30 days"), CreatedAt: &github.Timestamp{Time: olderTime}},
				{Body: github.Ptr("/snooze for 0 days"), CreatedAt: &github.Timestamp{Time: recentTime}},
			},
			expectSnoozed: false,
		},
		{
			name: "snooze comment mixed with regular comments",
			timelineComments: []*github.IssueComment{
				{Body: github.Ptr("regular comment"), CreatedAt: &github.Timestamp{Time: olderTime}},
				{Body: github.Ptr("/snooze PR reminder for 5 days"), CreatedAt: &github.Timestamp{Time: recentTime}},
				{Body: github.Ptr("another comment"), CreatedAt: &github.Timestamp{Time: recentTime}},
			},
			expectSnoozed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findActiveSnooze(tt.timelineComments)
			if tt.expectSnoozed && result == nil {
				t.Error("findActiveSnooze() = nil, expected active snooze")
			}
			if !tt.expectSnoozed && result != nil {
				t.Errorf("findActiveSnooze() = %v, expected nil", result)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
