# utilities

Generic slice helpers (filter/map/find/unique/flatten/intersperse) used throughout the pipeline in place of manual loops.

## Behaviour

- `Filter`, `Map`, `Find`, `FlatMap`: standard slice transformations
- `MapWithError`: maps a slice, stopping at the first error and returning it along with the results collected so far
- `Intersperse`: places a separator between each adjacent pair of items, as its own element
- `UniqueFunc`: dedupes using a caller-supplied equality function, preserving first-occurrence order

## Doesn't Do

- No sorting, grouping, or reduce/fold helpers
- `Filter`/`Map`/`FlatMap` always process the full input slice; there's no early-exit for the slice-returning variants

## Oddities

- `Filter`, `Map`, `FlatMap`, `MapWithError` and `UniqueFunc` return `nil`, not an empty slice, when there is nothing to return
- `MapWithError`'s partial results (everything mapped before the failing element) are still returned alongside the error, not discarded. A caller that ignores the error risks acting on an incomplete slice
- `UniqueFunc`'s equality check is quadratic in the input size, since it lacks a hashable key to dedupe by directly
