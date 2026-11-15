package provider

import (
	"strings"

	"github.com/yi-nology/go-git-platform/model"
)

// SplitFullName splits "owner/repo" into (owner, repo).
// If the input doesn't contain "/", owner is empty.
func SplitFullName(fullName string) (owner, name string) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", fullName
}

// ExtractOwnerFromFullName returns just the owner portion of "owner/repo".
func ExtractOwnerFromFullName(fullName string) string {
	owner, _ := SplitFullName(fullName)
	return owner
}

// BuildEventRepo creates an EventRepo from a full name string.
func BuildEventRepo(fullName string) *model.EventRepo {
	owner, name := SplitFullName(fullName)
	return &model.EventRepo{
		FullName: fullName,
		Owner:    owner,
		Name:     name,
	}
}

// ResolveMRSHAs derives the (head, base, start) SHAs for a GitLab-style merge
// request from the raw webhook fields. This encodes the shared priority used by
// the GitLab, TencentCode (and future GitCode) backends:
//
//   - head: diff_refs.head_sha when present, otherwise last_commit.id
//   - base: merge_commit_sha when present, otherwise diff_refs.base_sha
//   - start: diff_refs.start_sha (may be empty)
//
// Keeping this in one place guarantees identical fallback semantics across the
// GitLab-family backends and avoids behavioural drift.
func ResolveMRSHAs(diffRefsHead, diffRefsBase, diffRefsStart, mergeCommitSHA, lastCommitID string) (head, base, start string) {
	head = lastCommitID
	if diffRefsHead != "" {
		head = diffRefsHead
	}
	base = mergeCommitSHA
	if base == "" {
		base = diffRefsBase
	}
	start = diffRefsStart
	return head, base, start
}
