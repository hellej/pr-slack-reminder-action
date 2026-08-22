# models

Shared value types (`Repository`, `PullRequestRef`) used across the pipeline.

## Behaviour

- `Repository{Owner, Name}` identifies a GitHub repository; `GetPath()`/`String()` render it as `"owner/name"`, `GetPullsURL()` as `https://github.com/owner/name/pulls`
- `ParseRepository(s)` parses a `"owner/name"` string, erroring unless it splits into exactly two non-empty parts
- `PullRequestRef{Repository, Number}` identifies a specific PR; used in persisted state

## Doesn't Do

- `ParseRepository` doesn't validate owner/name against GitHub's actual naming rules (allowed characters, length) — only checks for a single `/` and non-empty parts
