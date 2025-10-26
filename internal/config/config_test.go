package config_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

// ConfigTestHelpers provides helper functions for setting up test environments
// similar to the ones in testhelpers/confighelpers.go but focused on config package testing
type ConfigTestHelpers struct {
	t *testing.T
}

func newConfigTestHelpers(t *testing.T) *ConfigTestHelpers {
	return &ConfigTestHelpers{t: t}
}

func (h *ConfigTestHelpers) setEnv(key, value string) {
	h.t.Setenv(key, value)
}

func (h *ConfigTestHelpers) setInput(inputName, value string) {
	envName := h.inputNameAsEnv(inputName)
	h.t.Setenv(envName, value)
}

func (h *ConfigTestHelpers) setInputInt(inputName string, value int) {
	h.setInput(inputName, strconv.Itoa(value))
}

func (h *ConfigTestHelpers) setInputMapping(inputName string, mapping map[string]string) {
	if mapping == nil {
		return
	}
	var parts []string
	for key, value := range mapping {
		parts = append(parts, key+":"+value)
	}
	h.setInput(inputName, strings.Join(parts, ";"))
}

func (h *ConfigTestHelpers) setInputList(inputName string, list []string) {
	if list == nil {
		return
	}
	h.setInput(inputName, strings.Join(list, ";"))
}

func (h *ConfigTestHelpers) inputNameAsEnv(name string) string {
	e := strings.ReplaceAll(name, " ", "_")
	e = strings.ToUpper(e)
	return "INPUT_" + e
}

// createRepositories creates a slice of Repository structs from a slice of "owner/name" strings for testing
func (h *ConfigTestHelpers) createRepositories(repoPaths ...string) []models.Repository {
	repos := make([]models.Repository, len(repoPaths))
	for i, path := range repoPaths {
		repos[i] = h.createRepository(path)
	}
	return repos
}

// createRepository creates a Repository struct from a "owner/name" string for testing
func (h *ConfigTestHelpers) createRepository(repoPath string) models.Repository {
	parts := strings.Split(repoPath, "/")
	if len(parts) != 2 {
		h.t.Fatalf("Invalid repository path format: %s (expected owner/name)", repoPath)
	}
	if parts[0] == "" || parts[1] == "" {
		h.t.Fatalf("Owner and repository name cannot be empty in: %s", repoPath)
	}
	return models.Repository{
		Owner: parts[0],
		Name:  parts[1],
	}
}

// MinimalConfigOptions allows selectively disabling certain inputs in setupMinimalValidConfig
type MinimalConfigOptions struct {
	SkipGithubRepository bool // Skip setting GITHUB_REPOSITORY
	SkipGithubToken      bool // Skip setting github-token
	SkipSlackBotToken    bool // Skip setting slack-bot-token
	SkipSlackChannelName bool // Skip setting slack-channel-name
	SkipPRListHeading    bool // Skip setting main-list-heading
}

func (h *ConfigTestHelpers) setupMinimalValidConfig(opts ...MinimalConfigOptions) {
	var options MinimalConfigOptions
	if len(opts) > 1 {
		h.t.Fatalf("setupMinimalValidConfig accepts at most one MinimalConfigOptions argument, got %d", len(opts))
	}
	if len(opts) > 0 {
		options = opts[0]
	}

	if !options.SkipGithubRepository {
		h.setEnv(config.EnvGithubRepository, "test-org/test-repo")
	} else {
		h.setEnv(config.EnvGithubRepository, "") // Explicitly clear if skipping
	}
	if !options.SkipGithubToken {
		h.setInput(config.InputGithubToken, "gh_token_123")
	}
	if !options.SkipSlackBotToken {
		h.setInput(config.InputSlackBotToken, "xoxb-slack-token")
	}
	if !options.SkipSlackChannelName {
		h.setInput(config.InputSlackChannelName, "test-channel")
	}
	if !options.SkipPRListHeading {
		h.setInput(config.InputPRListHeading, "PRs needing attention")
	}
}

func (h *ConfigTestHelpers) setupFullValidConfig() {
	h.setupMinimalValidConfig()
	h.setInput(config.InputSlackChannelID, "C1234567890")
	h.setInputInt(config.InputOldPRThresholdHours, 24)
	h.setInput(config.InputNoPRsMessage, "No PRs found!")
	h.setInputMapping(config.InputSlackUserIdByGitHubUsername, map[string]string{
		"alice": "U1234567890",
		"bob":   "U2234567890",
	})
	h.setInputList(config.InputGithubRepositories, []string{
		"test-org/repo1",
		"test-org/repo2",
	})
	h.setInput(config.InputGlobalFilters, `{"authors": ["alice"], "labels": ["feature"]}`)
	h.setInput(config.InputRepositoryFilters, `repo1: {"labels-ignore": ["wip"]}`)
	h.setInputMapping(config.InputPRLinkRepoPrefixes, map[string]string{
		"repo1": "R1",
		"repo2": "R2",
	})
}

func TestGetConfig_MinimalValid(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig()

	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.GithubToken != "gh_token_123" {
		t.Errorf("Expected GithubToken 'gh_token_123', got '%s'", cfg.GithubToken)
	}
	if cfg.SlackBotToken != "xoxb-slack-token" {
		t.Errorf("Expected SlackBotToken 'xoxb-slack-token', got '%s'", cfg.SlackBotToken)
	}
	if cfg.SlackChannelName != "test-channel" {
		t.Errorf("Expected SlackChannelName 'test-channel', got '%s'", cfg.SlackChannelName)
	}
	if cfg.ContentInputs.PRListHeading != "PRs needing attention" {
		t.Errorf("Expected PRListHeading 'PRs needing attention', got '%s'", cfg.ContentInputs.PRListHeading)
	}

	if len(cfg.Repositories) != 1 {
		t.Fatalf("Expected 1 repository, got %d", len(cfg.Repositories))
	}
	if cfg.Repositories[0].GetPath() != "test-org/test-repo" {
		t.Errorf("Expected repository path 'test-org/test-repo', got '%s'", cfg.Repositories[0].GetPath())
	}
	if cfg.Repositories[0].Owner != "test-org" {
		t.Errorf("Expected repository owner 'test-org', got '%s'", cfg.Repositories[0].Owner)
	}
	if cfg.Repositories[0].Name != "test-repo" {
		t.Errorf("Expected repository name 'test-repo', got '%s'", cfg.Repositories[0].Name)
	}

	// Verify optional fields have default values
	if cfg.ContentInputs.OldPRThresholdHours != 0 {
		t.Errorf("Expected OldPRThresholdHours 0, got %d", cfg.ContentInputs.OldPRThresholdHours)
	}
	if cfg.ContentInputs.NoPRsMessage != "" {
		t.Errorf("Expected empty NoPRsMessage, got '%s'", cfg.ContentInputs.NoPRsMessage)
	}
}

func TestGetConfig_FullValid(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupFullValidConfig()

	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.SlackChannelID != "C1234567890" {
		t.Errorf("Expected SlackChannelID 'C1234567890', got '%s'", cfg.SlackChannelID)
	}
	if cfg.ContentInputs.OldPRThresholdHours != 24 {
		t.Errorf("Expected OldPRThresholdHours 24, got %d", cfg.ContentInputs.OldPRThresholdHours)
	}
	if cfg.ContentInputs.NoPRsMessage != "No PRs found!" {
		t.Errorf("Expected NoPRsMessage 'No PRs found!', got '%s'", cfg.ContentInputs.NoPRsMessage)
	}

	expectedUsers := map[string]string{
		"alice": "U1234567890",
		"bob":   "U2234567890",
	}
	for username, expectedSlackID := range expectedUsers {
		if slackID, exists := cfg.ContentInputs.SlackUserIdByGitHubUsername[username]; !exists {
			t.Errorf("Expected user mapping for '%s' to exist", username)
		} else if slackID != expectedSlackID {
			t.Errorf("Expected slack ID '%s' for user '%s', got '%s'", expectedSlackID, username, slackID)
		}
	}

	if len(cfg.Repositories) != 2 {
		t.Fatalf("Expected 2 repositories, got %d", len(cfg.Repositories))
	}
	expectedRepos := []string{"test-org/repo1", "test-org/repo2"}
	for i, expectedRepo := range expectedRepos {
		if cfg.Repositories[i].GetPath() != expectedRepo {
			t.Errorf("Expected repository %d path '%s', got '%s'", i, expectedRepo, cfg.Repositories[i].GetPath())
		}
	}

	expectedPrefixes := map[string]string{
		"test-org/repo1": "R1",
		"test-org/repo2": "R2",
	}
	for repoPath, expectedPrefix := range expectedPrefixes {
		repo := h.createRepository(repoPath)
		if prefix := cfg.ContentInputs.GetPRLinkRepoPrefix(repo); prefix == "" {
			t.Errorf("Expected repository prefix for '%s' to exist", repo)
		} else if prefix != expectedPrefix {
			t.Errorf("Expected prefix '%s' for repo '%s', got '%s'", expectedPrefix, repo, prefix)
		}
	}

	if len(cfg.GlobalFilters.Authors) != 1 || cfg.GlobalFilters.Authors[0] != "alice" {
		t.Errorf("Expected global authors filter ['alice'], got %v", cfg.GlobalFilters.Authors)
	}
	if len(cfg.GlobalFilters.Labels) != 1 || cfg.GlobalFilters.Labels[0] != "feature" {
		t.Errorf("Expected global labels filter ['feature'], got %v", cfg.GlobalFilters.Labels)
	}

	if len(cfg.RepositoryFilters) != 1 {
		t.Fatalf("Expected 1 repository filter, got %d", len(cfg.RepositoryFilters))
	}
	repo1Filters, exists := cfg.RepositoryFilters["repo1"]
	if !exists {
		t.Fatalf("Expected repository filters for 'repo1' to exist")
	}
	if len(repo1Filters.LabelsIgnore) != 1 || repo1Filters.LabelsIgnore[0] != "wip" {
		t.Errorf("Expected repo1 labels-ignore filter ['wip'], got %v", repo1Filters.LabelsIgnore)
	}
}

func TestGetConfig_MissingRequiredInputs(t *testing.T) {
	testCases := []struct {
		name           string
		setupOptions   MinimalConfigOptions
		expectedErrMsg string
	}{
		{
			name: "missing github repository",
			setupOptions: MinimalConfigOptions{
				SkipGithubRepository: true,
			},
			expectedErrMsg: "required input GITHUB_REPOSITORY is not set",
		},
		{
			name: "missing github token",
			setupOptions: MinimalConfigOptions{
				SkipGithubToken: true,
			},
			expectedErrMsg: "required input github-token is not set",
		},
		{
			name: "missing slack bot token",
			setupOptions: MinimalConfigOptions{
				SkipSlackBotToken: true,
			},
			expectedErrMsg: "required input slack-bot-token is not set",
		},
		{
			name: "missing PR list heading",
			setupOptions: MinimalConfigOptions{
				SkipPRListHeading: true,
			},
			expectedErrMsg: "main-list-heading is required when group-by-repository is false",
		},
		{
			name: "missing slack channel",
			setupOptions: MinimalConfigOptions{
				SkipSlackChannelName: true,
			},
			expectedErrMsg: "either slack-channel-id or slack-channel-name must be set",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigTestHelpers(t)
			h.setupMinimalValidConfig(tc.setupOptions)

			_, err := config.GetConfig()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.expectedErrMsg) {
				t.Errorf("Expected error to contain '%s', got '%s'", tc.expectedErrMsg, err.Error())
			}
		})
	}
}

func TestMultipleRepositories(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig()

	repositories := []string{"org1/repo1", "org2/repo2", "org3/repo3"}
	h.setInputList(config.InputGithubRepositories, repositories)

	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedRepos := h.createRepositories(repositories...)

	if len(cfg.Repositories) != len(expectedRepos) {
		t.Fatalf("Expected %d repositories, got %d", len(expectedRepos), len(cfg.Repositories))
	}

	for i, expectedRepo := range expectedRepos {
		repo := cfg.Repositories[i]
		if repo.GetPath() != expectedRepo.GetPath() {
			t.Errorf("Repository %d: expected path '%s', got '%s'", i, expectedRepo.GetPath(), repo.GetPath())
		}
		if repo.Owner != expectedRepo.Owner {
			t.Errorf("Repository %d: expected owner '%s', got '%s'", i, expectedRepo.Owner, repo.Owner)
		}
		if repo.Name != expectedRepo.Name {
			t.Errorf("Repository %d: expected name '%s', got '%s'", i, expectedRepo.Name, repo.Name)
		}
	}
}

func TestMultipleRepositories_WithInvalid(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig()
	h.setInputList(config.InputGithubRepositories, []string{
		"org1/repo1",
		"invalid-repo", // This should cause an error
		"org3/repo3",
	})

	_, err := config.GetConfig()
	if err == nil {
		t.Fatalf("Expected error due to invalid repository, got nil")
	}
	expectedErrMsg := "invalid repositories input: invalid owner/repository format: invalid-repo"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedErrMsg, err.Error())
	}
}

func TestRepositoriesFallbackToDefault(t *testing.T) {
	// When github-repositories is not set, should fall back to GITHUB_REPOSITORY
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig(MinimalConfigOptions{SkipGithubRepository: true})
	h.setEnv(config.EnvGithubRepository, "fallback-org/fallback-repo")

	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(cfg.Repositories) != 1 {
		t.Fatalf("Expected 1 repository, got %d", len(cfg.Repositories))
	}

	repo := cfg.Repositories[0]
	if repo.GetPath() != "fallback-org/fallback-repo" {
		t.Errorf("Expected repository path 'fallback-org/fallback-repo', got '%s'", repo.GetPath())
	}
	if repo.Owner != "fallback-org" {
		t.Errorf("Expected repository owner 'fallback-org', got '%s'", repo.Owner)
	}
	if repo.Name != "fallback-repo" {
		t.Errorf("Expected repository name 'fallback-repo', got '%s'", repo.Name)
	}
}

func TestGetConfig_InvalidRepository(t *testing.T) {
	testCases := []struct {
		name           string
		repository     string
		expectedErrMsg string
	}{
		{
			name:           "too many slashes",
			repository:     "org/repo/extra",
			expectedErrMsg: "invalid owner/repository format: org/repo/extra",
		},
		{
			name:           "missing repository name",
			repository:     "org/",
			expectedErrMsg: "owner or repository name cannot be empty in: org/",
		},
		{
			name:           "missing owner",
			repository:     "/repo",
			expectedErrMsg: "owner or repository name cannot be empty in: /repo",
		},
		{
			name:           "no slash",
			repository:     "invalid",
			expectedErrMsg: "invalid owner/repository format: invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigTestHelpers(t)
			h.setupMinimalValidConfig()
			h.setEnv(config.EnvGithubRepository, tc.repository)

			_, err := config.GetConfig()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.expectedErrMsg) {
				t.Errorf("Expected error to contain '%s', got '%s'", tc.expectedErrMsg, err.Error())
			}
		})
	}
}

func TestGetConfig_InvalidFilters(t *testing.T) {
	testCases := []struct {
		name           string
		setupFilters   func(*ConfigTestHelpers)
		expectedErrMsg string
	}{
		{
			name: "invalid global filters JSON",
			setupFilters: func(h *ConfigTestHelpers) {
				h.setInput(config.InputGlobalFilters, `{"invalid": "json"}`)
			},
			expectedErrMsg: "error reading input filters: unable to parse filters from",
		},
		{
			name: "conflicting global authors filters",
			setupFilters: func(h *ConfigTestHelpers) {
				h.setInput(config.InputGlobalFilters, `{"authors": ["alice"], "authors-ignore": ["bob"]}`)
			},
			expectedErrMsg: "cannot use both authors and authors-ignore filters at the same time",
		},
		{
			name: "conflicting global labels filters",
			setupFilters: func(h *ConfigTestHelpers) {
				h.setInput(config.InputGlobalFilters, `{"labels": ["feature"], "labels-ignore": ["feature"]}`)
			},
			expectedErrMsg: "labels filter cannot contain labels that are in labels-ignore filter",
		},
		{
			name: "invalid repository filters format",
			setupFilters: func(h *ConfigTestHelpers) {
				h.setInput(config.InputRepositoryFilters, "invalid-format")
			},
			expectedErrMsg: "error reading input repository-filters: invalid mapping format",
		},
		{
			name: "invalid repository filters JSON",
			setupFilters: func(h *ConfigTestHelpers) {
				h.setInput(config.InputRepositoryFilters, `repo1: {"invalid": "json"}`)
			},
			expectedErrMsg: "error parsing filters for repository repo1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigTestHelpers(t)
			h.setupMinimalValidConfig()
			tc.setupFilters(h)

			_, err := config.GetConfig()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.expectedErrMsg) {
				t.Errorf("Expected error to contain '%s', got '%s'", tc.expectedErrMsg, err.Error())
			}
		})
	}
}

func TestGetConfig_InvalidOldPRThreshold(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig()
	h.setInput(config.InputOldPRThresholdHours, "not-a-number")

	_, err := config.GetConfig()
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	expectedErrMsg := "error parsing input old-pr-threshold-hours as integer"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedErrMsg, err.Error())
	}
}

func TestGetConfig_Validation(t *testing.T) {
	testCases := []struct {
		name           string
		setupConfig    func(*ConfigTestHelpers)
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "valid config with channel name",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
			},
			expectError: false,
		},
		{
			name: "valid config with channel ID",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig(MinimalConfigOptions{SkipSlackChannelName: true})
				h.setInput(config.InputSlackChannelID, "C1234567890")
			},
			expectError: false,
		},
		{
			name: "invalid config - no slack channel",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig(MinimalConfigOptions{SkipSlackChannelName: true})
			},
			expectError:    true,
			expectedErrMsg: "either slack-channel-id or slack-channel-name must be set",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigTestHelpers(t)
			tc.setupConfig(h)

			cfg, err := config.GetConfig()

			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if err.Error() != tc.expectedErrMsg {
					t.Errorf("Expected error '%s', got '%s'", tc.expectedErrMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				// Verify we got a valid config back
				if cfg.GithubToken == "" {
					t.Error("Expected GithubToken to be set")
				}
			}
		})
	}
}

func TestGetConfig_RepositoryValidation(t *testing.T) {
	testCases := []struct {
		name           string
		setupConfig    func(*ConfigTestHelpers)
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "valid - repository filters and prefixes match existing repositories",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{"org/repo1", "org/repo2"})
				h.setInput(
					config.InputRepositoryFilters,
					`repo1: {"authors": ["alice"]}; org/repo2: {"labels": ["bug"]}`,
				)
				h.setInputMapping(config.InputPRLinkRepoPrefixes, map[string]string{
					"repo1": "🚀",
					"repo2": "📦",
				})
			},
			expectError: false,
		},
		{
			name: "valid - empty filters and prefixes",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
			},
			expectError: false,
		},
		{
			name: "valid - no duplicates with different organizations",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{
					"org1/repo1", "org1/repo2", "org2/repo1",
				})
			},
			expectError: false,
		},
		{
			name: "invalid - filter for non-existent repository",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{"org/repo1"})
				h.setInput(config.InputRepositoryFilters, `repo2: {"authors": ["alice"]}`)
			},
			expectError:    true,
			expectedErrMsg: "repository-filters contains entry for 'repo2' which does not match any repository",
		},
		{
			name: "invalid - prefix for non-existent repository",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{"org/repo1"})
				h.setInputMapping(config.InputPRLinkRepoPrefixes, map[string]string{
					"repo2": "📦",
				})
			},
			expectError:    true,
			expectedErrMsg: "pr-link-repo-prefixes contains entry for 'repo2' which does not match any repository",
		},
		{
			name: "invalid - multiple non-existent repository names in filters & prefixes",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{"org/repo1"})
				h.setInput(config.InputRepositoryFilters, `repo2: {"authors": ["alice"]}; repo3: {"labels": ["bug"]}`)
				h.setInputMapping(config.InputPRLinkRepoPrefixes, map[string]string{
					"repo4": "🔧",
				})
			},
			expectError:    true,
			expectedErrMsg: "contains entry for", // Map iteration order is not deterministic
		},
		{
			name: "invalid - ambiguous identifier for repository in filters",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{"org1/same-repo", "org2/same-repo"})
				h.setInput(config.InputRepositoryFilters, `same-repo: {"authors": ["alice"]}`)
			},
			expectError:    true,
			expectedErrMsg: "repository-filters contains ambiguous entry for 'same-repo' which matches multiple repositories (needs owner/repo format)",
		},
		{
			name: "invalid - ambiguous identifier for repository in prefixes",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{"org1/same-repo", "org2/same-repo"})
				h.setInputMapping(config.InputPRLinkRepoPrefixes, map[string]string{
					"same-repo": "🚀",
				})
			},
			expectError:    true,
			expectedErrMsg: "pr-link-repo-prefixes contains ambiguous entry for 'same-repo' which matches multiple repositories (needs owner/repo format)",
		},
		{
			name: "invalid - exact duplicate repositories",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{
					"org/repo1", "org/repo2", "org/repo1",
				})
			},
			expectError:    true,
			expectedErrMsg: "duplicate repository 'org/repo1' found in github-repositories",
		},
		{
			name: "invalid - multiple duplicates (reports first one)",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				h.setInputList(config.InputGithubRepositories, []string{
					"org/repo1", "org/repo2", "org/repo1", "org/repo2",
				})
			},
			expectError:    true,
			expectedErrMsg: "duplicate repository 'org/repo1' found in github-repositories",
		},
		{
			name: "valid - below repository limit",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				var repos []string
				for i := 1; i <= 5; i++ {
					repos = append(repos, fmt.Sprintf("org%d/repo%d", i, i))
				}
				h.setInputList(config.InputGithubRepositories, repos)
			},
			expectError: false,
		},
		{
			name: "valid - at repository limit",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				var repos []string
				for i := 1; i <= 30; i++ {
					repos = append(repos, fmt.Sprintf("org%d/repo%d", i, i))
				}
				h.setInputList(config.InputGithubRepositories, repos)
			},
			expectError: false,
		},
		{
			name: "invalid - exceeds repository limit",
			setupConfig: func(h *ConfigTestHelpers) {
				h.setupMinimalValidConfig()
				var repos []string
				for i := 1; i <= 31; i++ { // 31 exceeds the limit of 30
					repos = append(repos, fmt.Sprintf("org%d/repo%d", i, i))
				}
				h.setInputList(config.InputGithubRepositories, repos)
			},
			expectError:    true,
			expectedErrMsg: "too many repositories: maximum of 30 repositories allowed, got 31",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigTestHelpers(t)
			tc.setupConfig(h)

			cfg, err := config.GetConfig()

			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.expectedErrMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tc.expectedErrMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				// Verify we got a valid config back
				if cfg.GithubToken == "" {
					t.Error("Expected GithubToken to be set")
				}
			}
		})
	}
}

func TestConfigPrint(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig(MinimalConfigOptions{})

	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	cfg.Print() // should not panic
}

func TestGetConfig_RunMode_DefaultsToPost(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig()

	cfg, err := config.GetConfig()
	if err != nil {
		// Use Fatalf to ensure immediate test failure with context
		t.Fatalf("Expected no error, got: %v", err)
	}
	if cfg.RunMode != "post" {
		// Validate default when input absent
		t.Errorf("Expected RunMode 'post' by default, got '%s'", cfg.RunMode)
	}
}

func TestGetConfig_RunMode_ParsedCorrectly(t *testing.T) {
	testCases := []struct {
		name     string
		inputVal string
	}{
		{name: "post", inputVal: "post"},
		{name: "update", inputVal: "update"},
	}

	for _, tc := range testCases {
		// Table-driven scenarios for valid run modes
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigTestHelpers(t)
			h.setupMinimalValidConfig()
			h.setInput(config.InputRunMode, tc.inputVal)

			cfg, err := config.GetConfig()
			if err != nil {
				// Fail if parsing fails for valid modes
				t.Fatalf("Expected no error, got: %v", err)
			}
			if string(cfg.RunMode) != tc.inputVal {
				t.Errorf("Expected RunMode '%s', got '%s'", tc.inputVal, cfg.RunMode)
			}
		})
	}
}

func TestGetConfig_RunMode_Invalid(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig()
	h.setInput(config.InputRunMode, "invalid-mode")

	_, err := config.GetConfig()
	if err == nil {
		// Expect failure for unsupported mode values
		t.Fatalf("Expected error for invalid run mode, got nil")
	}
	if !strings.Contains(err.Error(), "invalid run mode") {
		// Confirm error message includes context
		t.Errorf("Expected error to mention 'invalid run mode', got '%s'", err.Error())
	}
}

func TestGetConfig_StateFilePath_Default(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig()

	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if cfg.StateFilePath != ".pr-slack-reminder/state.json" {
		t.Errorf("Expected default StateFilePath '.pr-slack-reminder/state.json', got '%s'", cfg.StateFilePath)
	}
}

func TestGetConfig_StateFilePath_Custom(t *testing.T) {
	h := newConfigTestHelpers(t)
	h.setupMinimalValidConfig()
	custom := "custom/path/state.json"
	h.setInput(config.InputStateFilePath, custom)

	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if cfg.StateFilePath != custom {
		t.Errorf("Expected custom StateFilePath '%s', got '%s'", custom, cfg.StateFilePath)
	}
}

func TestContentInputs_GetPRLinkRepoPrefix(t *testing.T) {
	testCases := []struct {
		name           string
		repositories   []string
		prefixes       map[string]string
		testRepository string
		expectedPrefix string
	}{
		{
			name:           "empty prefixes map",
			repositories:   []string{"test-org/test-repo"},
			prefixes:       map[string]string{},
			testRepository: "test-org/test-repo",
			expectedPrefix: "",
		},
		{
			name:           "no match found",
			repositories:   []string{"test-org/test-repo", "test-org/other-repo"},
			prefixes:       map[string]string{"other-repo": "OR"},
			testRepository: "test-org/test-repo",
			expectedPrefix: "",
		},
		{
			name:           "match by full path",
			repositories:   []string{"test-org/test-repo"},
			prefixes:       map[string]string{"test-org/test-repo": "TR"},
			testRepository: "test-org/test-repo",
			expectedPrefix: "TR",
		},
		{
			name:           "match by repository name only",
			repositories:   []string{"test-org/test-repo"},
			prefixes:       map[string]string{"test-repo": "TR"},
			testRepository: "test-org/test-repo",
			expectedPrefix: "TR",
		},
		{
			name:         "full path takes precedence over name",
			repositories: []string{"test-org/test-repo"},
			prefixes: map[string]string{
				"test-repo":          "NAME_MATCH",
				"test-org/test-repo": "FULL_PATH_MATCH",
			},
			testRepository: "test-org/test-repo",
			expectedPrefix: "FULL_PATH_MATCH",
		},
		{
			name:         "different organization same repo name - no ambiguous match",
			repositories: []string{"test-org/test-repo", "other-org/different-repo"},
			prefixes: map[string]string{
				"other-org/different-repo": "OTHER_ORG",
				"test-repo":                "NAME_MATCH",
			},
			testRepository: "test-org/test-repo",
			expectedPrefix: "NAME_MATCH",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigTestHelpers(t)
			h.setupMinimalValidConfig()
			h.setInputList(config.InputGithubRepositories, tc.repositories)
			h.setInputMapping(config.InputPRLinkRepoPrefixes, tc.prefixes)

			cfg, err := config.GetConfig()
			if err != nil {
				t.Fatalf("Expected no error creating config, got: %v", err)
			}

			repo := h.createRepository(tc.testRepository)
			actualPrefix := cfg.ContentInputs.GetPRLinkRepoPrefix(repo)
			if actualPrefix != tc.expectedPrefix {
				t.Errorf("Expected prefix '%s', got '%s'", tc.expectedPrefix, actualPrefix)
			}
		})
	}
}

func TestConfig_GetFiltersForRepository(t *testing.T) {
	testCases := []struct {
		name              string
		globalFilters     config.Filters
		repositoryFilters map[string]config.Filters
		repository        string
		expectedAuthors   []string
		expectedLabels    []string
	}{
		{
			name: "no repository-specific filters - returns global",
			globalFilters: config.Filters{
				Authors: []string{"global-author"},
				Labels:  []string{"global-label"},
			},
			repositoryFilters: map[string]config.Filters{},
			repository:        "test-org/test-repo",
			expectedAuthors:   []string{"global-author"},
			expectedLabels:    []string{"global-label"},
		},
		{
			name: "match by full path",
			globalFilters: config.Filters{
				Authors: []string{"global-author"},
			},
			repositoryFilters: map[string]config.Filters{
				"test-org/test-repo": {
					Authors: []string{"path-specific-author"},
					Labels:  []string{"path-specific-label"},
				},
			},
			repository:      "test-org/test-repo",
			expectedAuthors: []string{"path-specific-author"},
			expectedLabels:  []string{"path-specific-label"},
		},
		{
			name: "match by repository name only",
			globalFilters: config.Filters{
				Authors: []string{"global-author"},
			},
			repositoryFilters: map[string]config.Filters{
				"test-repo": {
					Authors: []string{"name-specific-author"},
					Labels:  []string{"name-specific-label"},
				},
			},
			repository:      "test-org/test-repo",
			expectedAuthors: []string{"name-specific-author"},
			expectedLabels:  []string{"name-specific-label"},
		},
		{
			name: "full path takes precedence over name",
			globalFilters: config.Filters{
				Authors: []string{"global-author"},
			},
			repositoryFilters: map[string]config.Filters{
				"test-repo": {
					Authors: []string{"name-match-author"},
				},
				"test-org/test-repo": {
					Authors: []string{"full-path-author"},
					Labels:  []string{"full-path-label"},
				},
			},
			repository:      "test-org/test-repo",
			expectedAuthors: []string{"full-path-author"},
			expectedLabels:  []string{"full-path-label"},
		},
		{
			name: "no match - fallback to global filters",
			globalFilters: config.Filters{
				Authors: []string{"fallback-author"},
				Labels:  []string{"fallback-label"},
			},
			repositoryFilters: map[string]config.Filters{
				"other-repo": {
					Authors: []string{"other-author"},
				},
			},
			repository:      "test-org/test-repo",
			expectedAuthors: []string{"fallback-author"},
			expectedLabels:  []string{"fallback-label"},
		},
		{
			name:              "empty filters everywhere",
			globalFilters:     config.Filters{},
			repositoryFilters: map[string]config.Filters{},
			repository:        "test-org/test-repo",
			expectedAuthors:   []string(nil),
			expectedLabels:    []string(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigTestHelpers(t)
			repo := h.createRepository(tc.repository)

			cfg := config.Config{
				GlobalFilters:     tc.globalFilters,
				RepositoryFilters: tc.repositoryFilters,
			}

			actualFilters := cfg.GetFiltersForRepository(repo)

			if len(actualFilters.Authors) != len(tc.expectedAuthors) {
				t.Errorf("Expected authors length %d, got %d", len(tc.expectedAuthors), len(actualFilters.Authors))
			}
			for i, expectedAuthor := range tc.expectedAuthors {
				if i >= len(actualFilters.Authors) || actualFilters.Authors[i] != expectedAuthor {
					t.Errorf("Expected author[%d] '%s', got '%s'", i, expectedAuthor, actualFilters.Authors[i])
				}
			}

			if len(actualFilters.Labels) != len(tc.expectedLabels) {
				t.Errorf("Expected labels length %d, got %d", len(tc.expectedLabels), len(actualFilters.Labels))
			}
			for i, expectedLabel := range tc.expectedLabels {
				if i >= len(actualFilters.Labels) || actualFilters.Labels[i] != expectedLabel {
					t.Errorf("Expected label[%d] '%s', got '%s'", i, expectedLabel, actualFilters.Labels[i])
				}
			}
		})
	}
}
