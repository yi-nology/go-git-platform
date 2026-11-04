package gitbackend

import (
	"context"
	"fmt"
	"strings"

	oekiterrors "github.com/oekit/pkg/errors"
)

// --- Commit operations ---

// commitLogFormat 是 git log 提交行的输出格式。字段分隔用 NUL（%x00）：
// 提交标题是任意文本，含 "|" 时旧的 `%H|%s|%an|%ai` 会把 Author/Date
// 解析错位（"fix: handle a | b" 这类标题实测复现）。哈希为十六进制、
// 日期为固定格式，均不含 NUL；%s 只输出单行标题，标题内不可能有 NUL。
const commitLogFormat = "--pretty=format:%H%x00%s%x00%an%x00%ai"

// parseCommitLines 解析 commitLogFormat 的输出（每提交一行，NUL 分字段）。
func parseCommitLines(stdout string) []CommitInfo {
	var commits []CommitInfo
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, CommitInfo{
			Hash: parts[0], Message: parts[1], Author: parts[2], Date: parts[3],
		})
	}
	return commits
}

func (b *NativeGitBackend) GetCommitsBetween(ctx context.Context, repoPath, from, to string) ([]CommitInfo, error) {
	var rangeArg string
	if from == "" {
		rangeArg = to
	} else {
		rangeArg = fmt.Sprintf("%s..%s", from, to)
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{
		"log", rangeArg, commitLogFormat,
	}, AuthConfig{})
	if err != nil {
		return nil, newGitError("GetCommitsBetween", repoPath, stderr, err)
	}
	return parseCommitLines(stdout), nil
}

func (b *NativeGitBackend) IsAncestor(ctx context.Context, repoPath, ancestor, descendant string) (bool, error) {
	_, _, err := b.runGit(ctx, repoPath, []string{
		"merge-base", "--is-ancestor", ancestor, descendant,
	}, AuthConfig{})
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (b *NativeGitBackend) Merge(ctx context.Context, repoPath, branch string, opts MergeOptions) error {
	args := []string{"merge"}
	if opts.Squash {
		args = append(args, "--squash")
	}
	if opts.FFOnly {
		args = append(args, "--ff-only")
	}
	if opts.NoCommit {
		args = append(args, "--no-commit")
	}
	if opts.AllowUnrelated {
		args = append(args, "--allow-unrelated-histories")
	}
	if opts.Message != "" {
		args = append(args, "-m", opts.Message)
	}
	args = append(args, branch)

	stdout, stderr, err := b.runGit(ctx, repoPath, args, AuthConfig{})
	if err != nil {
		if isConflictOutput(stdout, stderr) {
			return newGitError("Merge", repoPath, stderr, ErrMergeConflict)
		}
		return newGitError("Merge", repoPath, stderr, err)
	}
	return nil
}

// isConflictOutput reports whether git's output announces merge conflicts.
// Real conflicts always print the uppercase marker "CONFLICT (" (content/
// rename/delete/...) on stdout; stderr stays empty on a content conflict, so
// both streams are inspected. A bare lowercase "conflict" substring is NOT a
// marker: it also appears in unrelated failures (e.g. merging a nonexistent
// branch literally named "conflict-fix"), which used to be misclassified as
// ErrMergeConflict.
func isConflictOutput(stdout, stderr string) bool {
	return strings.Contains(stdout+stderr, "CONFLICT (")
}

func (b *NativeGitBackend) CherryPick(ctx context.Context, repoPath, commitHash string) error {
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"cherry-pick", commitHash}, AuthConfig{})
	if err != nil {
		if isConflictOutput(stdout, stderr) {
			return newGitError("CherryPick", repoPath, stderr, ErrMergeConflict)
		}
		return newGitError("CherryPick", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) Rebase(ctx context.Context, repoPath, onto string) error {
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"rebase", onto}, AuthConfig{})
	if err != nil {
		if isConflictOutput(stdout, stderr) {
			return newGitError("Rebase", repoPath, stderr, ErrMergeConflict)
		}
		return newGitError("Rebase", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) RebaseAbort(ctx context.Context, repoPath string) error {
	_, stderr, err := b.runGit(ctx, repoPath, []string{"rebase", "--abort"}, AuthConfig{})
	if err != nil {
		return newGitError("RebaseAbort", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) RebaseContinue(ctx context.Context, repoPath string) error {
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"rebase", "--continue"}, AuthConfig{})
	if err != nil {
		if isConflictOutput(stdout, stderr) {
			return newGitError("RebaseContinue", repoPath, stderr, ErrMergeConflict)
		}
		return newGitError("RebaseContinue", repoPath, stderr, err)
	}
	return nil
}

// --- Commit query and index operations ---

func (b *NativeGitBackend) GetCommit(ctx context.Context, repoPath, hashStr string) (*CommitInfo, error) {
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{
		"log", "-1", commitLogFormat, hashStr,
	}, AuthConfig{})
	if err != nil {
		return nil, newGitError("GetCommit", repoPath, stderr, err)
	}
	commits := parseCommitLines(stdout)
	if len(commits) == 0 {
		return nil, newGitError("GetCommit", repoPath, "", oekiterrors.Errorf("unexpected log format"))
	}
	return &commits[0], nil
}

func (b *NativeGitBackend) Add(ctx context.Context, repoPath string, files []string) error {
	args := append([]string{"add"}, files...)
	_, stderr, err := b.runGit(ctx, repoPath, args, AuthConfig{})
	if err != nil {
		return newGitError("Add", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) CommitWithIdentity(ctx context.Context, repoPath, name, email, message string) error {
	args := []string{
		"-c", fmt.Sprintf("user.name=%s", name),
		"-c", fmt.Sprintf("user.email=%s", email),
		"-c", "commit.gpgsign=false",
		"commit", "--allow-empty", "-m", message,
	}
	// 显式设 GIT_AUTHOR_*/GIT_COMMITTER_*:进程环境里的同名变量优先级高于
	// `git -c user.name`,不覆盖会导致调用方指定的身份被全局配置顶掉。
	env := []string{
		"GIT_AUTHOR_NAME=" + name,
		"GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + name,
		"GIT_COMMITTER_EMAIL=" + email,
	}
	_, stderr, err := b.runGitEnv(ctx, repoPath, args, AuthConfig{}, env)
	if err != nil {
		return newGitError("CommitWithIdentity", repoPath, stderr, err)
	}
	return nil
}
