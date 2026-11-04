# retry

## Behaviour

- `retry.sh <command> [args...]` runs the command up to 3 times, waiting 10 s after the first failure and 30 s after the second
- Exits 0 on the first success. After the third failure, exits with that attempt's exit code
- Logs each failed attempt and the final give-up to stderr, naming the command
- Release workflow callers, each a `git push` that is safe to repeat:
  - The binaries commit push in `.github/actions/build/action.yml`
  - The new version tag push in `create-release-tag.sh`
  - The forced major version tag move (`v3`, or `v1-beta` on `v0`) in `create-release-tag.sh`

## Doesn't Do

- Doesn't tell a transient failure from a permanent one: it retries every failure
- Doesn't retry `gh release create` in `create-draft-release.sh`: a draft created despite an error would be created twice
- Doesn't retry the `git ls-remote` latest tag lookups. One failing fails the job before anything is pushed

## Oddities

- A permanent failure costs 40 s of waiting before the job fails, such as a binaries push rejected because `main` moved during the build
- A push that landed despite an error reports success on the retry, since the remote is already up to date
- If the version tag push lands but every attempt reports an error, the job fails with the tag on the remote and the major version tag not moved. A re-run then bumps the version again, since `create-release-tag.sh` reads the latest tag from the remote
- Unverified: whether a retry clears an SSH `Permission denied (publickey)`, as seen once on the binaries push while the same key had fetched the repository moments earlier
- Callers run it as `./retry.sh`, so they must run from the repository root
- The log lines print the full command, so a secret passed as an argument would show in the job log. No caller passes one
