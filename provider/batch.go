package provider

import (
	"context"

	"github.com/oekit/concurrency/syncx"
	oekiterrors "github.com/oekit/pkg/errors"
)

// DefaultBatchConcurrency is the number of in-flight operations used by
// the batch helpers when BatchOptions.Concurrency is unset.
const DefaultBatchConcurrency = 4

// MaxBatchConcurrency caps the caller-configurable concurrency so a
// misconfigured batch cannot hammer a platform into rate limiting.
const MaxBatchConcurrency = 16

// BatchOptions tunes the batch helpers' concurrency. Zero fields take
// the defaults.
type BatchOptions struct {
	// Concurrency is the maximum number of in-flight operations;
	// defaults to DefaultBatchConcurrency, capped at
	// MaxBatchConcurrency.
	Concurrency int
}

func (o BatchOptions) normalized() int {
	if o.Concurrency <= 0 {
		return DefaultBatchConcurrency
	}
	if o.Concurrency > MaxBatchConcurrency {
		return MaxBatchConcurrency
	}
	return o.Concurrency
}

// BatchResult pairs one batch item's outcome with its input key so
// callers can attribute per-item failures without re-deriving order.
// A per-item Err never aborts the whole batch.
type BatchResult[T any] struct {
	// Key is the input selector: the file path, CR number, or the
	// status context.
	Key string
	// Value carries the operation result when Err is nil.
	Value T
	// Err is the per-item failure, already wrapped as a provider error
	// by the underlying call.
	Err error
}

// batch runs fn over items with bounded concurrency and returns results
// in input order. The context is honored: after cancellation, remaining
// items fail fast with the context error instead of being started —
// the engine checks ctx before running each item, so an already-cancelled
// batch drains in one pass.
//
// The engine is syncx.FanInLimitCtx (oekit/concurrency#27): at most
// opts.normalized() calls to fn are in flight, results land by index so
// order follows input, and a panicking fn becomes that item's Err as a
// typed *async.PanicError (carrying the stack) while the other items
// continue — one bad item neither crashes the process nor starves the rest.
func batch[T, R any](ctx context.Context, items []T, opts BatchOptions, key func(T) string, fn func(context.Context, T) (R, error)) []BatchResult[R] {
	results := make([]BatchResult[R], len(items))
	if len(items) == 0 {
		return results
	}
	for i, item := range items {
		results[i].Key = key(item)
	}

	slots := syncx.FanInLimitCtx(ctx, len(items), opts.normalized(),
		func(ctx context.Context, i int) (R, error) {
			return fn(ctx, items[i])
		})
	for i, s := range slots {
		results[i].Value = s.Val
		results[i].Err = s.Err
	}
	return results
}

// GetFileContents reads many files from one repository with bounded
// concurrency. ref may be empty to use the repository's default branch
// (a single ref applies to the whole batch; per-path refs are
// expressible by calling the helper once per ref).
//
// It is the context-economy read primitive for agent flows: fetch the
// handful of files that matter in one call, then trim each payload with
// projection if needed.
func GetFileContents(ctx context.Context, p Provider, owner, repo, ref string, paths []string, batchOpts ...BatchOptions) []BatchResult[string] {
	var opts BatchOptions
	if len(batchOpts) > 0 {
		opts = batchOpts[0]
	}
	// FileManager is a core sub-interface of Provider: no assertion needed.
	return batch(ctx, paths, opts,
		func(path string) string { return path },
		func(ctx context.Context, path string) (string, error) {
			return p.GetFileContent(ctx, owner, repo, path, ref)
		})
}

// GetCRs fetches many change requests from one repository with bounded
// concurrency, in input order. A lookup failure for one number fails
// only that item.
func GetCRs(ctx context.Context, p Provider, owner, repo string, numbers []string, batchOpts ...BatchOptions) []BatchResult[*ChangeRequest] {
	var opts BatchOptions
	if len(batchOpts) > 0 {
		opts = batchOpts[0]
	}
	return batch(ctx, numbers, opts,
		func(n string) string { return n },
		func(ctx context.Context, number string) (*ChangeRequest, error) {
			return p.GetCR(ctx, owner, repo, number)
		})
}

// SetCommitStatuses reports many statuses on one commit with bounded
// concurrency (the usual "CI publishes lint+test+build at once" shape).
// The provider must implement CommitStatusManager; a provider without it
// yields every item failed with ErrNotImplemented.
func SetCommitStatuses(ctx context.Context, p Provider, owner, repo, sha string, statuses []CommitStatusOptions, batchOpts ...BatchOptions) []BatchResult[struct{}] {
	var opts BatchOptions
	if len(batchOpts) > 0 {
		opts = batchOpts[0]
	}
	csm, ok := p.(CommitStatusManager)
	return batch(ctx, statuses, opts,
		func(s CommitStatusOptions) string { return s.Context },
		func(ctx context.Context, s CommitStatusOptions) (struct{}, error) {
			if !ok {
				// no p.Platform() here: callers may pass a partially
				// implemented (embedded-interface) provider whose
				// Platform() would panic.
				return struct{}{}, oekiterrors.Wrap(ErrNotImplemented, "provider does not implement CommitStatusManager")
			}
			return struct{}{}, csm.CreateCommitStatus(ctx, owner, repo, sha, s)
		})
}
