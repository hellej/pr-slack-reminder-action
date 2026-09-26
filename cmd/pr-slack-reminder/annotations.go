package main

import (
	"log"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const actionNameInActionYML = "PR Slack Reminder"

// See docs/third-party-facts.md § A workflow command's message escapes `%`, CR and LF as `%25`, `%0D` and `%0A`
var annotationMessageEscaper = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")

func logWarning(message string) {
	log.Print(annotationLine("warning", message))
}

// One annotation per joined part: the run page shows an escaped newline as a space. See run.spec.md § Behaviour
func logError(err error) {
	for _, part := range splitJoinedError(err) {
		log.Print(annotationLine("error", part.Error()))
	}
}

func annotationLine(annotationType, message string) string {
	return "::" + annotationType + " title=" + actionNameInActionYML + "::" + annotationMessageEscaper.Replace(message)
}

// A multi-%w fmt.Errorf also unwraps to a slice. See run.spec.md § Behaviour
func splitJoinedError(err error) []error {
	if err == nil {
		return nil
	}
	multiError, unwrapsToSlice := err.(interface{ Unwrap() []error })
	if !unwrapsToSlice {
		return []error{err}
	}
	nonNilParts := utilities.Filter(multiError.Unwrap(), func(part error) bool { return part != nil })
	isErrorsJoin := err.Error() == strings.Join(utilities.Map(nonNilParts, error.Error), "\n")
	if !isErrorsJoin {
		return []error{err}
	}
	return utilities.FlatMap(utilities.Map(nonNilParts, splitJoinedError))
}
