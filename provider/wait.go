package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	oekiterrors "github.com/oekit/pkg/errors"
)

// Default polling parameters for WaitForCommitStatus.
const (
	DefaultWaitInterval = 5 * time.Second
	DefaultWaitTimeout  = 10 * time.Minute
	minWaitInterval     = 250 * time.Millisecond
)

// WaitOptions tunes the polling loop in WaitForCommitStatus.
type WaitOptions struct {
	// Interval is the poll cadence; defaults to DefaultWaitInterval and
	// is clamped up to minWaitInterval.
	Interval time.Duration
	// Timeout bounds the whole wait. Zero uses DefaultWaitTimeout; a
	// negative value disables the budget entirely (rely on your own
	// context deadline). Expiry surfaces as ErrWaitTimedOut carrying the
	// last observed combined state.
	Timeout time.Duration
	// Contexts, when non-empty, restricts the combined state to the
	// named status contexts (e.g. []string{"ci/lint", "ci/test"}).
	// Empty means every context reported on the commit.
	Contexts []string
	// InitialDelay defers the first poll; useful right after
	// CreateCommitStatus, whose result may not be queryable for a
	// moment. Defaults to no delay.
	InitialDelay time.Duration
}

// ErrWaitTimedOut is returned by WaitForCommitStatus when the commit
// stays non-terminal past WaitOptions.Timeout.
var ErrWaitTimedOut = errors.New("commit status did not reach a terminal state before the timeout")

// WaitForCommitStatus polls ListCommitStatuses until the commit's
// combined state is terminal (success/failure/error/canceled) and returns
// it. It is the CI-gate primitive for automation and agent flows: create
// your status (or push a commit), wait, then decide.
//
// The provider must implement CommitStatusManager; otherwise the error
// wraps ErrNotImplemented. Context cancellation is honored between and
// during polls. A combined state of pending/running past the timeout
// fails with ErrWaitTimedOut (so a red build is never confused with "gave
// up waiting"): distinguish them via errors.Is.
//
// States are combined with CombineCommitStatusStates semantics — any
// error beats failure beats canceled; any pending/running keeps waiting;
// an all-success set succeeds. With WaitOptions.Contexts set, only the
// named contexts are folded; contexts that have not reported yet are
// simply absent, so the wait continues until each named context lands a
// terminal state.
func WaitForCommitStatus(ctx context.Context, p Provider, owner, repo, sha string, opts WaitOptions) (CommitStatusState, error) {
	csm, ok := p.(CommitStatusManager)
	if !ok {
		// no p.Platform() here: callers may pass a partially-implemented
		// (embedded-interface) provider whose Platform() would panic.
		return "", oekiterrors.Wrap(ErrNotImplemented, "provider does not implement CommitStatusManager")
	}

	interval, budget, want := initWait(opts)

	if opts.InitialDelay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-budget:
			return "", ErrWaitTimedOut
		case <-time.After(opts.InitialDelay):
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var last CombinedCommitStatus
	for {
		statuses, err := csm.ListCommitStatuses(ctx, owner, repo, sha)
		if err != nil {
			// A definitive miss is a diagnosis, not a reason to wait:
			// a wrong SHA (or lost permissions) would otherwise burn the
			// whole budget and surface as a misleading ErrWaitTimedOut.
			if errors.Is(err, ErrNotFound) {
				return "", err
			}
			// Transient read errors (5xx, rate limits already retried by
			// the transport) are not fatal: keep polling.
		} else {
			snap, done := foldTerminalStates(statuses, want)
			if done {
				return snap.State, nil
			}
			last = snap
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-budget:
			return "", oekiterrors.Wrap(ErrWaitTimedOut, "wait budget exhausted", "last_state", last.String())
		case <-ticker.C:
		}
	}
}

// CombinedCommitStatus is the fold of a commit's statuses with a snapshot
// of the per-context states it was derived from.
type CombinedCommitStatus struct {
	State CommitStatusState `json:"state"`
	// Contexts maps each contributing context to its latest state.
	Contexts map[string]CommitStatusState `json:"contexts,omitempty"`
}

func (c CombinedCommitStatus) String() string {
	if len(c.Contexts) == 0 {
		return string(c.State)
	}
	return fmt.Sprintf("%s %v", c.State, c.Contexts)
}

// LatestCommitStatuses collapses a commit's status history to one status
// per context, keeping the FIRST occurrence of each context. All platform
// list endpoints return statuses newest-first (GitHub documents reverse
// chronological order; Gitea/GitLab/Gitee/GitCode/Forgejo/Tencent Code
// sort descending by creation), so first-seen is the most recent report
// of that context — the same entry GitHub's combined status would use.
// CI re-runs append history instead of replacing it, so folding the raw
// list would let a stale failure (or pending) outvote the latest result.
func LatestCommitStatuses(statuses []CommitStatus) []CommitStatus {
	seen := make(map[string]bool, len(statuses))
	out := make([]CommitStatus, 0, len(statuses))
	for _, s := range statuses {
		if seen[s.Context] {
			continue
		}
		seen[s.Context] = true
		out = append(out, s)
	}
	return out
}

// initWait normalizes the wait parameters: it clamps the poll interval,
// arms the wait budget, and builds the wanted-contexts set. The budget
// is a plain timer channel, not context.WithTimeout: our own expiry must
// surface as ErrWaitTimedOut while a caller's context cancellation
// surfaces as ctx.Err() — the two are different diagnoses and callers
// check for them differently.
func initWait(opts WaitOptions) (interval time.Duration, budget <-chan time.Time, want map[string]bool) {
	interval = opts.Interval
	if interval <= 0 {
		interval = DefaultWaitInterval
	}
	if interval < minWaitInterval {
		interval = minWaitInterval
	}
	switch {
	case opts.Timeout == 0:
		opts.Timeout = DefaultWaitTimeout
		fallthrough
	case opts.Timeout > 0:
		// The timer is intentionally never stopped: the budget must stay
		// armed for the whole (unbounded) wait loop, and an unstopped
		// timer keeps firing regardless of when the wrapper is collected.
		t := time.NewTimer(opts.Timeout)
		budget = t.C
	}
	want = make(map[string]bool, len(opts.Contexts))
	for _, c := range opts.Contexts {
		want[c] = true
	}
	return interval, budget, want
}

// foldTerminalStates combines the LATEST status per context (filtered to
// the wanted contexts, when any) and reports whether the combined state
// is final. A wanted context that has not reported at all keeps the fold
// pending — absence of a report must never be voted in by a duplicate of
// another context.
func foldTerminalStates(statuses []CommitStatus, want map[string]bool) (CombinedCommitStatus, bool) {
	latest := LatestCommitStatuses(statuses)
	contexts := make(map[string]CommitStatusState, len(latest))
	states := make([]CommitStatusState, 0, len(latest))
	reported := 0
	for _, s := range latest {
		if len(want) > 0 {
			if !want[s.Context] {
				continue
			}
			reported++
		}
		contexts[s.Context] = s.State
		states = append(states, s.State)
	}
	if len(want) > 0 && reported < len(want) {
		return CombinedCommitStatus{State: CommitStatusPending}, false
	}
	combined := CombineCommitStatusStates(states)
	return CombinedCommitStatus{State: combined, Contexts: contexts}, combined.Terminal()
}
