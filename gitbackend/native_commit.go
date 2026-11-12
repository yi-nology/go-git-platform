package gitbackend

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// --- Commit operations ---

// commitLogFormat 是 git log 的输出格式。每条记录以 %x00（NUL）开头、字段以
// NUL 分隔、记录之间是 git 默认插入的 "\n"（落在记录末位的 %B 里，随其尾部
// 换行一起被 TrimRight 吸收）：
//   - %B = 完整提交消息（可多行），置于记录末位吸收任意分隔符。旧实现用
//     %s 只有主题行，与 gogit 后端的全文口径分裂（issue #103）；
//   - %aI = author date 的 RFC3339/ISO-strict 形态。旧实现 %ai 是空格分隔
//     偏移（"2005-04-07 22:13:13 +0200"），time.Parse(RFC3339) 必败；
//   - 消息内含 NUL 属病理形态，git 的 porcelain 类输出对它本无定义。
const commitLogFormat = "--pretty=format:%x00%H%x00%an%x00%aI%x00%B"

// parseCommitLines 解析 commitLogFormat 的输出：整流按 NUL 切分，每 4 段
// 一条记录（format 前导 NUL 产生首段空段）。消息在记录末位，其尾部自身的
// 换行 + 记录分隔符一起 TrimRight，与 gogit 后端 TrimRight(c.Message, "\n")
// 同口径。
func parseCommitLines(stdout string) []CommitInfo {
	fields := strings.Split(stdout, "\x00")
	var commits []CommitInfo
	for i := 1; i+3 < len(fields); i += 4 {
		commits = append(commits, CommitInfo{
			Hash:    fields[i],
			Message: strings.TrimRight(fields[i+3], "\n"),
			Author:  fields[i+1],
			Date:    fields[i+2],
		})
	}
	return commits
}

func (b *NativeGitBackend) GetCommitsBetween(ctx context.Context, repoPath, from, to string) ([]CommitInfo, error) {
	if err := checkNameArgs("GetCommitsBetween", from, to); err != nil {
		return nil, err
	}
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
	if err := checkNameArgs("IsAncestor", ancestor, descendant); err != nil {
		return false, err
	}
	_, stderr, err := b.runGit(ctx, repoPath, []string{
		"merge-base", "--is-ancestor", "--", ancestor, descendant,
	}, AuthConfig{})
	if err == nil {
		return true, nil
	}
	// `git merge-base --is-ancestor` 的退出码语义：1 = 非祖先（正常结论），
	// 其余（128 等）= 致命错误（ref 不存在/非 git 仓库/ctx 取消）。只有 1
	// 才映射 (false, nil)；一切错误透传——否则调用方把「无效 ref」当
	// 「未合入」继续执行（跳过 cherry-pick、误判漂移、放行合并）。
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, newGitError("IsAncestor", repoPath, stderr, err)
}

func (b *NativeGitBackend) Merge(ctx context.Context, repoPath, branch string, opts MergeOptions) error {
	if err := checkNameArg("Merge", branch); err != nil {
		return err
	}
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
	if err := checkNameArg("CherryPick", commitHash); err != nil {
		return err
	}
	args := []string{"cherry-pick", "--", commitHash}
	stdout, stderr, err := b.runGit(ctx, repoPath, args, AuthConfig{})
	if err != nil {
		if isConflictOutput(stdout, stderr) {
			return newGitError("CherryPick", repoPath, stderr, ErrMergeConflict)
		}
		return newGitError("CherryPick", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) Rebase(ctx context.Context, repoPath, onto string) error {
	if err := checkNameArg("Rebase", onto); err != nil {
		return err
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"rebase", "--", onto}, AuthConfig{})
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
	if err := checkNameArg("GetCommit", hashStr); err != nil {
		return nil, err
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{
		"log", "-1", commitLogFormat, hashStr,
	}, AuthConfig{})
	if err != nil {
		return nil, newGitError("GetCommit", repoPath, stderr, err)
	}
	commits := parseCommitLines(stdout)
	if len(commits) == 0 {
		return nil, newGitError("GetCommit", repoPath, "", fmt.Errorf("unexpected log format"))
	}
	return &commits[0], nil
}

func (b *NativeGitBackend) Add(ctx context.Context, repoPath string, files []string) error {
	// `--` 后接 pathspec：以 "-" 开头的文件名不会被解析成 flag
	args := append([]string{"add", "--"}, files...)
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
