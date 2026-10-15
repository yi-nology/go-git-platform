# Contributing to go-git-platform

Thanks for your interest in contributing! This document covers the basics.

## Prerequisites

- Go (see the version pinned in `go.mod`).
- [`golangci-lint`](https://golangci-lint.run/) (the CI pins v2.12.2).

## Development loop

Common tasks are wired through the `Makefile`:

```sh
make fmt      # gofmt + goimports
make vet      # go vet
make lint     # golangci-lint run ./...
make test     # go test -race -coverprofile=coverage.out ./...
make check    # vet + lint + test (the CI gate)
make cover    # print the coverage summary
```

Run `make check` before pushing — it mirrors what CI runs.

## Versioning & compatibility commitment

The project follows [SemVer](https://semver.org/spec/v2.0.0.html) with the
0.x convention: breaking changes may land in a minor release, but must be
called out in the CHANGELOG. What counts as what:

**Non-breaking (fine in any release)**

- Adding a field to an exported struct (`options.go` types, `Error`,
  `NormalizedEvent`, ...). Callers using unkeyed struct literals is not a
  supported pattern.
- Adding a new optional capability: a new `iface_*.go` interface plus a
  `CapabilitySet` field. Existing `Capabilities()` declarations compile
  unchanged because the zero value means "not implemented".
- Adding a package, function, or method (including new helper functions in
  `provider`).
- New toolsets/tools in the MCP module; new tool parameters are additive
  JSON fields.

**Breaking (minor release + CHANGELOG entry; avoid where possible)**

- Removing or renaming an exported identifier (`provider.ListAllPages`
  removal in v0.73.0 is the precedent).
- Adding a method to an interface others implement — every `Provider`
  implementor breaks. Route new surface through optional capability
  interfaces instead.
- Changing a function signature or the meaning of an existing value
  (e.g. webhook event-type vocabulary renames in v0.72.0).

When a release contains breaking changes, the CHANGELOG entry must start
with the migration note (see v0.72.0's `cr.note` → `comment.created` for
the shape).

## What we look for in changes

- **Tests.** Add or update tests for behavior changes. Backends share a
  cross-platform contract suite in `backends/contracttest/`; when you add a
  capability that should be consistent across platforms, extend that suite
  rather than only testing one backend.
- **Race safety.** The test suite runs with `-race`; new concurrency code must
  be safe under it (see `pkg/credential/sshkey.go` for the mutex + double-check
  pattern used for shared maps).
- **No secrets in code or logs.** Never pass secrets through `argv` (see the
  removed `ssh-keygen` fallback). Keep credentials out of log output.
- **Follow existing patterns.** The `provider` package defines the abstraction;
  each `backends/<platform>/` implements it. Shared plumbing lives in
  `backends/internal/backendutil` — reuse it instead of copy-pasting.

## oekit-first conventions (required)

This SDK consumes the [oekit](https://oekit.github.io) ecosystem. Prefer its
bottom-layer modules over hand-rolled equivalents; the allowed dependency set
is exactly three modules (adding any other oekit module would drag transitive
dependencies onto every downstream consumer):

| Module | Use for | Entry points here |
|---|---|---|
| `github.com/oekit/pkg` | error wrapping | `oekiterrors.Wrap(err, msg, kv...)` |
| `github.com/oekit/concurrency` | panic-safe goroutines | `async.GoSafe`, `async.GoSafeWithErrChannel` |
| `github.com/oekit/logx` | logging facade | `provider.NewLogxLogger`, `transport.NewLogxLogger` |

Concrete rules:

- **Error wrapping.** Any wrap that must keep the cause chain uses
  `oekiterrors.Wrap` (nil-safe; put dynamic context in kv pairs, not in the
  message text). `fmt.Errorf` survives only for the two dual-`%w` sites in
  `provider/errors.go` (sentinel + cause double chain — Wrap is single-cause)
  and for fresh errors with no cause. Never wrap with `%v`, never discard
  errors with `_`.
- **Classification is sentinel-based.** Branch with `errors.Is` on
  `provider.ErrNotFound` & co. Never match error message text — since
  v0.87.0 dynamic values are kv suffixes and the text is unstable.
- **Logging.** Library code logs through the injected `provider.Logger` /
  `transport.Logger` (noop by default). To attach the consumer's oekit/logx
  stack use `NewLogxLogger(component)` (lazy `logx.Delegating` resolution).
  Deployable binaries (the MCP server) own a service name via
  `logx.WithService`. No new stdlib `log` or `fmt.Println` debug output.
- **Concurrency.** No bare `go func` — use `async.GoSafe` (panic-recovering);
  `GoSafeWithErrChannel` when the goroutine produces an error. Bounded fan-out
  follows the `provider/batch.go` semaphore + `BatchResult` pattern (a panicking
  item becomes that item's `Err`; it must not crash the process).
- **Explicitly out of scope** (settled — don't re-litigate): `BizError` /
  `mw.Fail` (HTTP-service-layer concepts; this SDK has no such layer), and
  replacing `transport/retry.go` with `oekit/concurrency/retry` (the former is
  HTTP-specialized: Retry-After handling, GitHub 403 secondary-rate detection).

`AGENTS.md` at the repo root is the authoritative version of these rules,
plus the release/CI discipline AI agents must follow.

## Adding a new platform backend

1. Create `backends/<platform>/` with a `New(cfg provider.Config) (provider.Provider, error)`
   constructor. Use `backends/internal/backendutil` for HTTP client / retry /
   hooks wiring so you don't reimplement it.
2. Implement only the capability interfaces the platform actually supports.
   `IssueManager` and `SearchManager` are **optional** — leave them out unless
   the platform genuinely offers them; consumers type-assert for optional caps.
3. Register the platform with `provider.Register` in an `init()`, and add a
   blank import in `backends/all/all.go`.
4. Add a `contract_test.go` that wires `contracttest.Run` with a `Harness`
   (see any existing backend's contract test).
5. Add the platform constant to `provider/provider.go` and the README platform
   list.

## Adding an optional capability

1. Define the interface in `provider/iface_<name>.go` and add a
   `CapabilitySet` field (e.g. `CommitStatuses`).
2. Implement it in every backend that supports it; declare the field in each
   backend's `Capabilities()`. Platforms that cannot serve the capability do
   not implement it at all — absence is expressed by the declaration, never
   by a stub.
3. Extend `backends/contracttest/capabilities.go` with the bidirectional
   assertion for the new field, and add a mounted suite
   (`backends/contracttest/<name>.go` + `Harness` field + `Run` wiring).
4. Register any partial divergences (per-method stubs, ignored fields,
   semantic mappings, raw detours) in the affected backends'
   `divergence.go`, then run `go generate ./...` to refresh
   `docs/divergence-ledger.md`.
5. Update this checklist if the coupling points change.

## Breaking changes

This project follows SemVer. Breaking changes to the public API require a new
major version. When you make one, document it under a `### ⚠️ Breaking changes`
heading in `CHANGELOG.md` and explain the migration.

### What counts as breaking (v1.0 contract)

| Change | Version |
|---|---|
| Adding a field to a unified type (`ChangeRequest`, `Issue`, …) | minor (non-breaking — structs are append-only for consumers) |
| Adding a method to `provider.Provider` or a core sub-interface | **major** (breaks every implementor, in-repo and external) |
| Adding an optional capability interface (`ReviewManager`, …) | minor (additive; nothing implements it before) |
| Adding a method to an existing optional capability interface | **major** (breaks implementors of that capability) |
| Changing a sentinel error, normalizing a state vocabulary, or renaming an option field | **major** |
| Widening behavior inside an existing method (pagination completeness, tighter validation) | patch/minor — behavior locked by the contract suites, so a suite change accompanying it must explain why the old wire shape was wrong |
| Swapping a wrapped platform SDK when the unified surface is unchanged | minor (the divergence ledger + contract suites are the compatibility proof) |

Two repo invariants back this contract:

1. **The contract suites are the spec.** Every behavioral promise above is
   enforced by `backends/contracttest/`; a PR that changes suite assertions
   must justify the change as a bug fix in the commit message.
2. **The divergence ledger is the fine print.** Platform gaps are declared
   (`provider.Divergences()`), generated into `docs/divergence-ledger.md`,
   and asserted against behavior by the drift check — never documented
  -only.
