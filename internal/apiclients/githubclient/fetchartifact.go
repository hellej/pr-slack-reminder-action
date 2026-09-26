package githubclient

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/google/go-github/v78/github"
)

// GitHub answers a name no artifact has with an empty list, not an error. See
// docs/third-party-facts.md § GitHub's "List artifacts" with a `name` filter returns 200 and an empty list when nothing matches
var ErrNoArtifactFound = errors.New("no artifacts found")

// FetchLatestArtifactByName downloads the most recent GitHub Actions artifact by name,
// extracts a JSON file from the zip archive, and unmarshals it into the provided struct.
// The target parameter should be a pointer to the target struct for JSON deserialization.
func (client *client) FetchLatestArtifactByName(
	ctx context.Context,
	owner, repo, artifactName, jsonFilePath string,
	target any,
) error {
	opts := &github.ListArtifactsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		Name:        &artifactName,
	}
	res, resp, err := client.actionsService.ListArtifacts(ctx, owner, repo, opts)
	if err != nil {
		statusText := ""
		if resp != nil && resp.Status != "" {
			statusText = " status=" + resp.Status
		}
		return fmt.Errorf("failed to list artifacts: %w%s", err, statusText)
	}
	log.Printf("Found %d artifacts with name %q", res.GetTotalCount(), artifactName)

	artifacts := res.Artifacts
	if len(artifacts) == 0 {
		return fmt.Errorf("%w with name %q", ErrNoArtifactFound, artifactName)
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].GetCreatedAt().Time.After(artifacts[j].GetCreatedAt().Time)
	})

	latest := artifacts[0]
	artifactID := latest.GetID()
	log.Printf(
		"Downloading artifact %q (ID: %d) created at %s",
		artifactName, artifactID, latest.GetCreatedAt(),
	)

	downloadURL, _, err := client.actionsService.DownloadArtifact(ctx, owner, repo, artifactID, 1)
	if err != nil {
		return fmt.Errorf("get artifact download URL: %w", err)
	}

	httpResp, err := client.http.Get(downloadURL.String())
	if err != nil {
		return fmt.Errorf("download artifact zip: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d when downloading artifact", httpResp.StatusCode)
	}

	zipBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("read artifact zip: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	found := false
	for _, f := range zr.File {
		if filepath.Base(f.Name) == filepath.Base(jsonFilePath) {
			found = true
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open file inside zip: %w", err)
			}
			dec := json.NewDecoder(rc)
			if err := dec.Decode(target); err != nil {
				_ = rc.Close()
				return fmt.Errorf("decode json %q: %w", jsonFilePath, err)
			}
			_ = rc.Close()
			break
		}
	}

	if !found {
		return fmt.Errorf("json file %q not found inside artifact zip", jsonFilePath)
	}

	return nil
}
