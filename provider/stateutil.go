package provider

import "github.com/yi-nology/go-git-platform/model"

// MapStateToCR maps common string state representations to CRState.
// mergedFn is called when state is "closed" to determine if it was merged.
// For platforms that use a separate "merged" field (Gitea, Forgejo),
// pass a non-nil mergedFn. For platforms where "merged" is a distinct
// state string (GitLab, Tencent Code), pass nil.
func MapStateToCR(state string, mergedFn func() bool) model.CRState {
	switch state {
	case "merged":
		return model.CRStateMerged
	case "closed":
		if mergedFn != nil && mergedFn() {
			return model.CRStateMerged
		}
		return model.CRStateClosed
	case "opened", "open":
		return model.CRStateOpened
	default:
		return model.CRStateOpened
	}
}

// MapMRStateToCR is a convenience for platforms with explicit "merged" state (GitLab, Tencent Code).
func MapMRStateToCR(state string) model.CRState {
	return MapStateToCR(state, nil)
}

// MapBoolStateToCR is a convenience for platforms with a separate merged boolean (Gitea, Forgejo).
func MapBoolStateToCR(state string, merged bool) model.CRState {
	return MapStateToCR(state, func() bool { return merged })
}
