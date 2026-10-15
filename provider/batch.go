package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/oekit/concurrency/async"
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
// including items blocked waiting for a concurrency slot, since semaphore
// acquisition is itself a cancellable select.
func batch[T, R any](ctx context.Context, items []T, opts BatchOptions, key func(T) string, fn func(context.Context, T) (R, error)) []BatchResult[R] {
	results := make([]BatchResult[R], len(items))
	if len(items) == 0 {
		return results
	}

	sem := make(chan struct{}, opts.normalized())
	var wg sync.WaitGroup
	for i, item := range items {
		results[i].Key = key(item)
		if err := ctx.Err(); err != nil {
			results[i].Err = err
			continue
		}
		// Acquiring a slot must be interruptible: without the ctx.Done
		// branch a cancellation during contention (all slots busy) would
		// block the loop until an in-flight item happens to finish.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			results[i].Err = ctx.Err()
			continue
		}
		wg.Add(1)
		// async.GoSafe recovers a panicking fn at the outermost layer; the
		// inner recover below runs first and converts the panic into the
		// item's Err, so one bad item neither crashes the process nor goes
		// silently missing from the results.
		async.GoSafe(func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					results[i].Err = fmt.Errorf("batch: fn panicked: %v", r)
				}
			}()
			v, err := fn(ctx, item)
			results[i].Value = v
			results[i].Err = err
		})
	}
	wg.Wait()
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
				return struct{}{}, fmt.Errorf("%w: provider does not implement CommitStatusManager", ErrNotImplemented)
			}
			return struct{}{}, csm.CreateCommitStatus(ctx, owner, repo, sha, s)
		})
}
