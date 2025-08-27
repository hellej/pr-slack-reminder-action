[![CI](https://github.com/hellej/pr-slack-reminder-action/actions/workflows/ci.yml/badge.svg)](https://github.com/hellej/pr-slack-reminder-action/actions/workflows/ci.yml) [![Build](https://github.com/hellej/pr-slack-reminder-action/actions/workflows/build.yml/badge.svg)](https://github.com/hellej/pr-slack-reminder-action/actions/workflows/build.yml) [![E2E Test Run](https://github.com/hellej/pr-slack-reminder-action/actions/workflows/e2e-test.yml/badge.svg)](https://github.com/hellej/pr-slack-reminder-action/actions/workflows/e2e-test.yml) [![Coverage Status](https://coveralls.io/repos/github/hellej/pr-slack-reminder-action/badge.svg?branch=main)](https://coveralls.io/github/hellej/pr-slack-reminder-action?branch=main)

# PR Slack Reminder Action

This GitHub Action sends friendly Slack reminders about open Pull Requests, helping your team stay on top of code reviews and keep the development flow moving.

⚠️ **Note**: The action is still experimental and released as `v1-beta` - `v1` is expected to be released by October 2025 (breaking changes may still be released to `v1-beta` until then).

## Features

- 🔍 **Multi-repository support** - List open PRs across multiple repositories
- 🎯 **Configurable filtering** - Find the most important PRs by authors and labels
- ⏰ **Age highlighting** - Highlight old PRs that need attention
- 🏷️ **Repository prefixes** - Easily distinguish PRs from different repositories

## Getting Started

### Prerequisites

- A Slack bot token with permissions to post messages
- [GitHub token](#-github-token-setup) with read access to your repositories

### Basic Usage Examples

#### 1. Simple Setup (Single Repository)

This monitors open PRs in your current repository.

```yaml
name: PR Reminder

on:
  schedule:
    - cron: "0 9 * * MON-FRI" # 9 AM on weekdays

jobs:
  remind:
    runs-on: ubuntu-latest
    steps:
      - uses: hellej/pr-slack-reminder-action@v1-beta
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          slack-bot-token: ${{ secrets.SLACK_BOT_TOKEN }}
          slack-channel-name: "dev-team"
```

#### 2. Multiple Repositories

Monitor several repositories with user mentions and custom messaging.

```yaml
name: PR Reminder

on:
  schedule:
    - cron: "0 9 * * MON-FRI" # 9 AM on weekdays

jobs:
  remind:
    runs-on: ubuntu-latest
    steps:
      - uses: hellej/pr-slack-reminder-action@v1-beta
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          slack-bot-token: ${{ secrets.SLACK_BOT_TOKEN }}
          slack-channel-name: "code-reviews"
          github-repositories: |
            myorg/frontend
            myorg/backend
            myorg/mobile-app
          github-user-slack-user-id-mapping: |
            alice: U1234567890
            kronk: U2345678901
            charlie: U3456789012
          main-list-heading: "We have <pr_count> PRs waiting for review! 👀"
          no-prs-message: "🎉 All caught up! No PRs waiting for review."
          old-pr-threshold-hours: 48
```

#### 3. Advanced Setup with Filtering

Full-featured setup with repository-specific filters and prefixes.

```yaml
name: PR Reminder

on:
  schedule:
    - cron: "0 9 * * MON-FRI"
  workflow_dispatch: # Allow manual triggers

jobs:
  remind:
    runs-on: ubuntu-latest
    steps:
      - uses: hellej/pr-slack-reminder-action@v1-beta
        with:
          github-token: ${{ secrets.MULTI_REPO_GITHUB_TOKEN }}
          slack-bot-token: ${{ secrets.SLACK_BOT_TOKEN }}
          slack-channel-name: "code-reviews"
          github-repositories: |
            myorg/web-app
            myorg/api-service
            myorg/mobile-app
          github-user-slack-user-id-mapping: |
            alice: U1234567890
            kronk: U2345678901
          main-list-heading: "<pr_count> PRs need your attention!"
          no-prs-message: "No PRs pending! Happy coding!"
          old-pr-threshold-hours: 24
          repository-prefixes: |
            web-app: 🌐
            api-service: 📡
            mobile-app: 📞
          filters: |
            {
              "labels-ignore": ["draft", "wip"],
              "authors-ignore": ["dependabot[bot]"]
            }
          repository-filters: |
            api-service: {"labels": ["ready-for-review"], "authors-ignore": ["intern-bot"]}
            mobile-app: {}
```

## Input Parameters

| Name                                | Required | Example                                                              | Description                                                          |
| ----------------------------------- | -------- | -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `github-token`                      | ✅       | `${{ secrets.GITHUB_TOKEN }}`                                        | GitHub token for repository access                                   |
| `slack-bot-token`                   | ✅       | `${{ secrets.SLACK_BOT_TOKEN }}`                                     | Slack bot token for sending messages                                 |
| `slack-channel-name`                | ❌       | `dev-team`                                                           | Slack channel name (use this OR slack-channel-id)                    |
| `slack-channel-id`                  | ❌       | `C1234567890`                                                        | Slack channel ID (use this OR slack-channel-name)                    |
| `github-repositories`               | ❌       | `owner/repo1`<br>`owner/repo2`                                       | Repositories to monitor (defaults to current repo)                   |
| `github-user-slack-user-id-mapping` | ❌       | `alice: U1234567890`<br>`kronk: U2345678901`                         | Map of GitHub usernames to Slack user IDs                            |
| `main-list-heading`                 | ❌       | `There are <pr_count> open PRs 💫`                                   | Message heading (`<pr_count>` gets replaced)                         |
| `no-prs-message`                    | ❌       | `All caught up! 🎉`                                                  | Message when no PRs are found (if not set, no empty message is sent) |
| `old-pr-threshold-hours`            | ❌       | `48`                                                                 | Hours after which PRs are highlighted as old                         |
| `repository-prefixes`               | ❌       | `repo1: 🚀`<br>`repo2: 📦`                                           | Repository specific prefixes to display before PR titles             |
| `filters`                           | ❌       | `{"authors": ["alice"], "labels-ignore": ["wip"]}`                   | Global filters (JSON format)                                         |
| `repository-filters`                | ❌       | `repo1: {"labels": ["bug"]}`<br>`repo2: {"authors-ignore": ["bot"]}` | Repository-specific filters                                          |

### Filter Options

Both `filters` and `repository-filters` support:

- `authors` - Only include PRs by these users
- `authors-ignore` - Exclude PRs by these users
- `labels` - Only include PRs with these labels
- `labels-ignore` - Exclude PRs with these (overrides the above)

⚠️ **Note**: You cannot use both `authors` and `authors-ignore` in the same filter.

## 🔑 GitHub Token Setup

### Option 1: Default Token (Single Repository)

For monitoring just your current repository, the default `GITHUB_TOKEN` (available automatically) works perfectly:

```yaml
github-token: ${{ secrets.GITHUB_TOKEN }}
```

### Option 2: GitHub App (Recommended for Organizations)

For better security and granular permissions:

1. **Create a GitHub App** in your organization settings
2. **Install the app** on repositories you want to monitor
3. **Use a token generation action** like [actions/create-github-app-token](https://github.com/actions/create-github-app-token)

### Option 3: Personal Access Token (Multiple Repositories)

To monitor multiple repositories, you'll need a Personal Access Token:

1. **Go to GitHub Settings** → Developer settings → Personal access tokens → Fine-grained tokens
2. **Click "Generate new token"** → Select the repositories of interest and at least read access to PRs
3. **Add the token as a repository secret** named `PR_REMINDER_GITHUB_TOKEN`
4. **Use it in your workflow:**
   ```yaml
   github-token: ${{ secrets.PR_REMINDER_GITHUB_TOKEN }}
   ```

## 💡 Tips

- **Test with `workflow_dispatch`**: Allow manual testing for your workflow
- **Use cron scheduling**: Run reminders at times that work for your team (avoid weekends!)
- **Customize messages**: Make the reminders fit your team's culture
- **Highlight old PRs**: Set a reasonable `old-pr-threshold-hours` to highlight stale PRs (consider weekends too)

## 🤝 Contributing

Found a bug or have a feature request? We'd love your help! Feel free to open an issue.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
