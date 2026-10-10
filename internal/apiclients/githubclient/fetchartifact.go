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
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/retry"
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
	res, err := retry.TransientFailures(ctx, client.graphql.retryPolicy, "artifact list", func(attemptCtx context.Context) retry.AttemptResult[*github.ArtifactList] {
		return client.listArtifacts(attemptCtx, owner, repo, opts)
	})
	if err != nil {
		return err
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

	zipBytes, err := retry.TransientFailures(ctx, client.graphql.retryPolicy, "artifact download", func(attemptCtx context.Context) retry.AttemptResult[[]byte] {
		return client.downloadArtifactZip(attemptCtx, owner, repo, artifactID)
	})
	if err != nil {
		return err
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

func (client *client) listArtifacts(
	ctx context.Context, owner, repo string, opts *github.ListArtifactsOptions,
) retry.AttemptResult[*github.ArtifactList] {
	artifacts, resp, err := client.actionsService.ListArtifacts(ctx, owner, repo, opts)
	if err != nil {
		statusText := ""
		if resp != nil && resp.Status != "" {
			statusText = " status=" + resp.Status
		}
		return retry.AttemptResult[*github.ArtifactList]{
			Err:       fmt.Errorf("failed to list artifacts: %w%s", err, statusText),
			Transient: isTransientGitHubFailure(resp),
		}
	}
	return retry.AttemptResult[*github.ArtifactList]{Value: artifacts}
}

// Gets a fresh download URL, since one expires after a minute, and reads the whole zip under the
// same ctx. See docs/third-party-facts.md § `go-github` v78 `DownloadArtifact` returns a plain error on a non-302, with the `*Response`
func (client *client) downloadArtifactZip(
	ctx context.Context, owner, repo string, artifactID int64,
) retry.AttemptResult[[]byte] {
	downloadURL, resp, err := client.actionsService.DownloadArtifact(ctx, owner, repo, artifactID, 1)
	if err != nil {
		return retry.AttemptResult[[]byte]{
			Err:       fmt.Errorf("get artifact download URL: %w", err),
			Transient: isTransientGitHubFailure(resp),
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL.String(), nil)
	if err != nil {
		return retry.AttemptResult[[]byte]{Err: fmt.Errorf("build artifact zip request: %w", err)}
	}
	httpResp, err := client.http.Do(request)
	if err != nil {
		return retry.AttemptResult[[]byte]{Err: fmt.Errorf("download artifact zip: %w", err), Transient: true}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return retry.AttemptResult[[]byte]{
			Err:       fmt.Errorf("unexpected status code %d when downloading artifact", httpResp.StatusCode),
			Transient: httpResp.StatusCode >= http.StatusInternalServerError,
		}
	}

	zipBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return retry.AttemptResult[[]byte]{Err: fmt.Errorf("read artifact zip: %w", err), Transient: true}
	}
	return retry.AttemptResult[[]byte]{Value: zipBytes}
}

// The status is read off the *Response, since DownloadArtifact reports a failed status as a
// plain error. A 2xx with an error is, on the list call, a body read or decode that failed, as
// when the attempt deadline cuts it off. See docs/third-party-facts.md § `go-github` v78 returns the 2xx `*Response` with the error when reading or decoding the body fails
// On DownloadArtifact it is a 2xx where a 302 was expected, retried as well.
func isTransientGitHubFailure(resp *github.Response) bool {
	gotNoResponse := resp == nil || resp.Response == nil
	if gotNoResponse {
		return true
	}
	answeredSuccessWithError := resp.StatusCode >= 200 && resp.StatusCode < 300
	return answeredSuccessWithError || resp.StatusCode >= http.StatusInternalServerError
}
