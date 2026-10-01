# Changelog

All notable changes to the `fxcl/anima` line are documented here.
The project follows [Keep a Changelog](https://keepachangelog.com/) conventions;
versions are tagged as they are released.

## Unreleased

### Fixed

- Pagination no longer splices COUNT SQL through string surgery past the dialect
  boundary; COUNT queries now go through the dialect and cannot be corrupted by
  column names that happen to contain keywords.
- The Converter registry no longer recurses without bound: a `FromDatabase`
  implementation that never lands on its own target type now reports an error
  instead of exhausting the stack.
- `Select[*Entity]` (pointer entity form) no longer panics on every scanned row;
  it returns `[]*Entity` as its signature advertises.
- Requesting a page beyond the last one now returns empty rows with page
  metadata that agrees with them.
- `ScalarSQL[*T]` maps SQL NULL to a nil pointer instead of erroring; NULL now
  round-trips through the raw-SQL path like it does through the entity path.
- `SaveBatch` rolls back the entire batch on any failure instead of silently
  committing the rows that ran before the failure.
- `OrderCol` rejects any direction outside `asc`/`desc` (case-insensitive)
  instead of splicing the raw string into `ORDER BY`;
  `JoinSpec.Order` is pinned to identifier-plus-optional-direction. Both were
  SQL-injection surfaces; `Order` on the top-level query remains the documented
  raw-expression escape hatch.

### Added

- `(*Anima).SetPool(maxOpen, maxIdle int) *Anima`: public connection-pool
  sizing. The previous silent default (`MaxIdleConns = 2`) tore down and
  re-dialed connections under any concurrency above two.
- Package documentation (`doc.go`) and compile-checked godoc examples for the
  core API surface.

### Changed

- Module path is now `github.com/fxcl/anima`; the importable package is
  the repository root (`anima/dialect` and the in-process test server moved up
  accordingly).
- The `go` directive is raised to 1.24 to match the testing API actually used.
- CI follows the Go version declared in `go.mod` and now also runs `go vet` and
  the race detector on every push.
- The in-process test server binds an ephemeral port instead of a fixed one, so
  repeated or concurrent test runs no longer collide.

### Security

- `govulncheck` reports zero reachable vulnerabilities across the dependency
  tree (import- and module-level advisories exist only in the test engine's
  transitive dependencies and are unreachable from this code).
