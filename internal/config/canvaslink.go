package config

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

var canvasIDPattern = regexp.MustCompile(`^F[A-Z0-9]+$`)

// getCanvasIDFromLink extracts the canvas ID from a Slack canvas link of the shape
// https://<workspace>.slack.com/docs/<TEAM_ID>/<CANVAS_ID>. An empty link is valid
// and means the PR tracker canvas is disabled.
func getCanvasIDFromLink(link string) (string, error) {
	if link == "" {
		return "", nil
	}
	parsedLink, err := url.Parse(link)
	if err != nil || parsedLink.Host == "" {
		return "", invalidCanvasLinkError(link)
	}

	pathSegments := strings.Split(parsedLink.Path, "/")
	docsIndex := slices.Index(pathSegments, "docs")
	if docsIndex == -1 {
		return "", invalidCanvasLinkError(link)
	}

	canvasID, found := utilities.Find(pathSegments[docsIndex+1:], canvasIDPattern.MatchString)
	if !found {
		return "", invalidCanvasLinkError(link)
	}
	return canvasID, nil
}

func invalidCanvasLinkError(link string) error {
	return fmt.Errorf(
		"invalid %s '%s': expected a link like "+
			"https://<workspace>.slack.com/docs/<TEAM_ID>/<CANVAS_ID>, "+
			"copied from the canvas in Slack via ⋮ → Copy link",
		InputPRTrackerCanvasLink, link,
	)
}
