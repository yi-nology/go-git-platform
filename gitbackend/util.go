package gitbackend

import "strings"

// isCommitSHA reports whether s looks like a full 40-character git object SHA.
// It is shared by both backends.
func isCommitSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// mergeInsecure returns a copy of auth with InsecureSkipTLS set when either the
// AuthConfig itself or the explicit per-call flag requests skipping TLS
// verification. Lets Fetch/Push/Clone honor both opt paths (opts.InsecureSkipTLS
// and opts.Auth.InsecureSkipTLS) in one place.
func mergeInsecure(auth AuthConfig, insecure bool) AuthConfig {
	if insecure {
		auth.InsecureSkipTLS = true
	}
	return auth
}

// diffFetchRefs is the single ref-classification engine shared by the native
// and gogit backends: it classifies a before/after pair of ref snapshots into
// a FetchResult. Refs created by
// the fetch go to FetchedRefs + NewBranches/NewTags, refs that moved go to
// FetchedRefs + UpdatedBranch, and pruned remote-tracking refs go to
// DeletedBranch. NewBranches/UpdatedBranch/DeletedBranch carry short names
// (after the refs/remotes/<remote>/ or refs/tags/ prefix); FetchedRefs keeps
// the full refname of every ref the fetch created or moved.
func diffFetchRefs(remote string, before, after map[string]string) *FetchResult {
	remotePrefix := "refs/remotes/" + remote + "/"
	tagsPrefix := "refs/tags/"

	result := &FetchResult{}

	for ref, hash := range after {
		oldHash, existed := before[ref]
		switch {
		case !existed:
			// New ref after fetch.
			result.FetchedRefs = append(result.FetchedRefs, ref)
			switch {
			case strings.HasPrefix(ref, remotePrefix):
				result.NewBranches = append(result.NewBranches, strings.TrimPrefix(ref, remotePrefix))
			case strings.HasPrefix(ref, tagsPrefix):
				result.NewTags = append(result.NewTags, strings.TrimPrefix(ref, tagsPrefix))
			}
		case oldHash != hash:
			// Existing ref moved to a different commit.
			result.FetchedRefs = append(result.FetchedRefs, ref)
			if strings.HasPrefix(ref, remotePrefix) {
				result.UpdatedBranch = append(result.UpdatedBranch, strings.TrimPrefix(ref, remotePrefix))
			}
		}
	}

	for ref := range before {
		// Only remote-tracking refs can be pruned away by a fetch; tag refs
		// are never pruned.
		if _, exists := after[ref]; !exists && strings.HasPrefix(ref, remotePrefix) {
			result.DeletedBranch = append(result.DeletedBranch, strings.TrimPrefix(ref, remotePrefix))
		}
	}

	return result
}
