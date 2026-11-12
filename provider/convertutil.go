package provider

import (
	"regexp"
	"strings"
)

// mentionRe matches @username patterns that are NOT preceded by a word
// character (to avoid matching email addresses like user@domain.com).
// Valid usernames contain letters, digits, underscores, hyphens, and dots
// (the last to cover platforms like GitLab that use @first.last).
var mentionRe = regexp.MustCompile(`(?:^|[\s\p{P}])@([\w.-]+)`)

// urlContextRe matches URL-like prefixes that indicate a @mention is inside
// a URL rather than a real mention: "](", "://...", or "mailto:...".
// Each pattern is anchored to the end of the prefix so it only matches when
// the @ is immediately inside the URL context.
var urlContextRe = regexp.MustCompile(`\]\([^)]*$|\w+://\S*$|mailto:\S*$`)

// ExtractMentions returns the deduplicated list of @usernames found in body.
// Email addresses (foo@bar.com) are excluded by the leading non-word-char
// guard. Mentions inside URLs (e.g. `[@user](https://example.com/@user)`)
// are also excluded. The returned order follows first occurrence.
//
// 尾部标点剥离（issue #145）：用户名字符类含 '.'/'-'（GitLab 的 first.last
// 形态），正文最常见的 "cc @alice." 会把句号吞进用户名——解析出 "alice."
// 而非合法账号 "alice"，真实用户被漏掉。合法账号名不以 '.'/'-' 结尾
// （GitHub/GitLab 注册规则同此），只裁尾部、保留内部点号（@first.last
// 不受影响）；"@alice..." 三连句号裁成 "alice"。
func ExtractMentions(body string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, m := range mentionRe.FindAllStringSubmatchIndex(body, -1) {
		// m[0] = full match start, m[2] = group 1 start, m[3] = group 1 end
		if m[2] < 0 || m[3] < 0 {
			continue
		}
		name := body[m[2]:m[3]]
		// Check if the @ is inside a URL context.
		// The @ character is at position m[0]+1 (after the leading non-word char)
		// or at m[0] if the match starts at the beginning of the string.
		atPos := m[0]
		if atPos < len(body) && body[atPos] != '@' {
			atPos++
		}
		if isInsideURL(body, atPos) {
			continue
		}
		name = strings.TrimRight(name, ".-")
		if name == "" {
			continue // 字符类只命中标点本身（如 "@..."），不是用户名
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			result = append(result, name)
		}
	}
	return result
}

// isInsideURL reports whether the @ at position pos in body is inside a URL
// context (a markdown link target or a bare URL).
func isInsideURL(body string, pos int) bool {
	// Look at the text before the @ to see if we're inside a URL.
	prefix := body[:pos]
	return urlContextRe.MatchString(prefix)
}

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
func BuildEventRepo(fullName string) *EventRepo {
	owner, name := SplitFullName(fullName)
	return &EventRepo{
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
