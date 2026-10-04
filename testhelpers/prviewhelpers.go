package testhelpers

import (
	"fmt"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// DescribeRows gives each row as one string, so a test pins a section's rows in one expectation.
func DescribeRows(rows []prview.Row) []string {
	return utilities.Map(rows, func(row prview.Row) string {
		switch row := row.(type) {
		case prview.PR:
			return fmt.Sprintf("#%d", row.GetNumber())
		case prview.CollapsedRow:
			numbers := utilities.Map(row.PRs, func(pr prview.PR) string { return fmt.Sprintf("#%d", pr.GetNumber()) })
			return row.AuthorLogin + ": " + strings.Join(numbers, " ")
		default:
			return fmt.Sprintf("unknown row %T", row)
		}
	})
}
