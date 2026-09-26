# inputhelpers

Reads GitHub Action inputs and environment variables, and parses them into strings, numbers, booleans, lists and mappings. Which inputs exist belongs to [internal/config](../config.spec.md).

## Behaviour

- Input `name` is read from env var `INPUT_<NAME>`: spaces become `_`, letters are uppercased, hyphens are kept (`github-token` → `INPUT_GITHUB-TOKEN`)
- `GetInput`: the input's value with surrounding whitespace trimmed, `""` when unset
- `GetInputOr`: the input's value, or the default only when the env var is unset. An input set to empty or whitespace yields `""`
- `GetInputRequired`: errors with `required input <name> is not set` when the value is empty
- `GetInputInt`, `GetInputBool`: parse the value, returning `0` / `false` without error when it is empty, and an error naming the input when it doesn't parse. Booleans accept Go's `strconv.ParseBool` forms (`true`, `TRUE`, `1`, `false`, `0`, …)
- `GetInputList`: splits the value into trimmed items, on `;` when the value contains one, otherwise on newlines. An empty value yields an empty slice
- `GetInputMapping`: parses `key: value` lines into a map, split on `;` or newlines by the same rule as lists. Blank lines and lines starting with `#` are skipped. Splits on the first `:`, so a value may contain `:`. Errors when a line has no `:` or an empty key or value
- `GetEnv`, `GetEnvRequired`: read a plain env var by its exact name, untrimmed

## Doesn't Do

- No escaping: a list item or mapping value can't contain the separator
- `GetInputList` doesn't drop duplicates

## Oddities

- One `;` anywhere switches the whole value to `;` separation, so a newline-separated value with a `;` in an item splits on `;` only, with newlines left inside items
- `GetInputList` keeps an empty item between two adjacent separators, while `GetInputMapping` skips it
- `GetInputMapping` doesn't reject a repeated key: the last value wins
- `GetEnvRequired`'s error says `required input`, though it reads a plain env var
