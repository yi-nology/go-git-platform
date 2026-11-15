// Package runtime is the machine layer of go-git-platform: the
// user-facing and automation-facing helpers that drive the contract
// seam instead of declaring it — platform detection (DetectPlatform),
// idempotent Ensure machines (EnsureWebhook, EnsureBranchProtection,
// EnsureDeployKey, EnsureIssueState, EnsureIssueLabels), commit-status
// polling (WaitForCommitStatus), webhook signature validation
// (ValidateWebhookWithRegistry and the WebhookValidator family), the
// bounded-concurrency batch engine (GetFileContents/GetCRs/
// SetCommitStatuses), the page-walking iterators (Each/Collect),
// the token cache and its background janitor (Manager), due_on
// time parsing, and the shared name/event helpers
// (SplitFullName/BuildEventRepo/ResolveMRSHAs).
//
// # Layering
//
// runtime sits directly above the contract seam: it may import
// provider (interfaces, options, errors) and model (entities), plus
// standard library and the whitelisted oekit modules — and nothing
// else from this repository. It must not import adapter, contracttest,
// transport or gitbackend, and no layer below it may import runtime
// back (spec §4.1 edge table: provider only looks down at model;
// provider → runtime is a forbidden edge). Signatures here take
// provider's manager interfaces and return model entities, so callers
// wire an adapter-built provider.Provider into a machine without either layer
// knowing the other's package path.
//
// Only this file carries the package comment; per-file package
// comments would concatenate in godoc.
package runtime
