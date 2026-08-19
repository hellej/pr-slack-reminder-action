# utilities

Generic slice helpers (filter/map/find/unique/flatten) used throughout the pipeline in place of manual loops.

## Behaviour

- `Filter`, `Map`, `Find`, `FlatMap`: standard slice transformations, plus lazy iterator variants of each
- `MapWithError`: maps a slice, stopping at the first error and returning it along with the results collected so far
- `UniqueFunc`: dedupes using a caller-supplied equality function, preserving first-occurrence order

## Doesn't Do

- No sorting, grouping, or reduce/fold helpers
- `Filter`/`Map`/`FlatMap` always process the full input slice; there's no early-exit for the slice-returning variants

## Oddities

- `UniqueFunc` returns `nil`, not an empty slice, for empty input
- `MapWithError`'s partial results (everything mapped before the failing element) are still returned alongside the error, not discarded — a caller that ignores the error risks acting on an incomplete slice
- `UniqueFunc`'s equality check is quadratic in the input size, since it lacks a hashable key to dedupe by directly
