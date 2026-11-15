package model

import (
	"strings"
)

// CommitStatusState is the unified state vocabulary for commit statuses.
// It covers the union of the platforms' verbs: GitHub
// (error/failure/pending/success), GitLab (pending/running/success/
// failed/canceled and pipeline pre-states), Gitea/Forgejo
// (pending/success/error/failure/warning), and the GitHub-shaped
// verbs spoken by GitCode, Gitee (via check-run conclusions) and
// Tencent Code.
type CommitStatusState string

// Terminal reports whether the state is a final outcome: success,
// failure, error, or canceled — anything but the pending/running family
// and the empty state.
func (s CommitStatusState) Terminal() bool {
	return s == CommitStatusSuccess || s == CommitStatusFailure ||
		s == CommitStatusError || s == CommitStatusCanceled
}

const (
	// CommitStatusPending is the "not finished yet" state: the check is
	// queued or running. This is the only non-terminal state.
	CommitStatusPending CommitStatusState = "pending"
	// CommitStatusRunning is an alias of CommitStatusPending kept as a
	// distinct constant so callers can surface GitLab-style running
	// states verbatim; CombineCommitStatusStates treats it as pending.
	CommitStatusRunning CommitStatusState = "running"
	// CommitStatusSuccess is the terminal "passed" state.
	CommitStatusSuccess CommitStatusState = "success"
	// CommitStatusFailure is the terminal "failed" state.
	CommitStatusFailure CommitStatusState = "failure"
	// CommitStatusError is the terminal "errored before/without a run"
	// state. It is terminal like CommitStatusFailure but distinct so
	// infrastructure breakage stays tellable from test breakage.
	CommitStatusError CommitStatusState = "error"
	// CommitStatusCanceled is the terminal "canceled" state (GitLab).
	CommitStatusCanceled CommitStatusState = "canceled"
)

// NormalizeCommitStatusState maps a platform's raw status verb onto the
// unified CommitStatusState vocabulary. Unknown verbs normalize to
// CommitStatusPending so that unrecognised in-flight states keep callers
// waiting instead of concluding early; platform backends map their own
// verbs through this helper before returning CommitStatus values.
//
// Gitea/Forgejo "warning" (build passed with warnings) normalizes to
// success: it is a terminal passing outcome, and merge gates treat it as
// such on the platform itself.
func NormalizeCommitStatusState(state string) CommitStatusState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "success", "ok", "passed", "warning", "neutral":
		return CommitStatusSuccess
	case "failure", "failed":
		return CommitStatusFailure
	case "error":
		return CommitStatusError
	case "canceled", "cancelled":
		return CommitStatusCanceled
	case "startup_failure", "action_required":
		return CommitStatusError
	case "running", "created", "queued", "in_progress", "preparing",
		"waiting_for_resource", "requested":
		return CommitStatusRunning
	case "pending":
		return CommitStatusPending
	default:
		return CommitStatusPending
	}
}

// CombineCommitStatusStates folds per-context states into the single
// combined state of a commit, using the same precedence as GitHub's
// combined status: any error wins, then failure, then canceled; any
// pending/running keeps the commit pending; only an all-success set is
// success. An empty set is pending — a commit with no reported statuses
// has nothing to gate on yet.
func CombineCommitStatusStates(states []CommitStatusState) CommitStatusState {
	var hasError, hasFailure, hasCanceled, hasRunning, hasSuccess bool
	for _, s := range states {
		switch s {
		case CommitStatusError:
			hasError = true
		case CommitStatusFailure:
			hasFailure = true
		case CommitStatusCanceled:
			hasCanceled = true
		case CommitStatusPending, CommitStatusRunning:
			hasRunning = true
		case CommitStatusSuccess:
			hasSuccess = true
		}
	}
	switch {
	case hasError:
		return CommitStatusError
	case hasFailure:
		return CommitStatusFailure
	case hasCanceled:
		return CommitStatusCanceled
	case hasRunning:
		return CommitStatusPending
	case hasSuccess:
		return CommitStatusSuccess
	default:
		return CommitStatusPending
	}
}

// CommitStatus is one status reported on a commit, unified across
// platforms. State is normalized via NormalizeCommitStatusState.
type CommitStatus struct {
	State       CommitStatusState `json:"state"`
	Context     string            `json:"context"`
	Description string            `json:"description,omitempty"`
	TargetURL   string            `json:"target_url,omitempty"`
}
