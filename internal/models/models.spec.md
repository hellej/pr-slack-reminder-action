# models

Shared value types (`Repository`, `PullRequestRef`) used across the pipeline.

## Behaviour

- `Repository{Owner, Name}` identifies a GitHub repository; `GetPath()`/`String()` render it as `"owner/name"`
- `ParseRepository(s)` parses a `"owner/name"` string, erroring unless it splits into exactly two non-empty parts
- `NewRepository(owner, name)` builds a `Repository` directly from already-known parts, without parsing or validation
- `PullRequestRef{Repository, Number}` identifies a specific PR; used in persisted state

## Doesn't Do

- `ParseRepository` doesn't validate owner/name against GitHub's actual naming rules (allowed characters, length) — only checks for a single `/` and non-empty parts

## Oddities

- `NewRepository` skips the validation `ParseRepository` performs — an owner or name built this way that itself contains `/` produces a `Repository` whose path doesn't round-trip back through `ParseRepository`
