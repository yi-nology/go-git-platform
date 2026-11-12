package gitbackend

import (
	"context"
	"fmt"
	"strings"
)

// --- Status and diff ---

func (b *NativeGitBackend) GetStatus(ctx context.Context, repoPath string) (*RepoStatus, error) {
	status := &RepoStatus{}

	// Get current branch
	branch, err := b.GetCurrentBranch(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	status.Branch = branch

	// Get ahead/behind
	ahead, behind, _ := b.getAheadBehind(ctx, repoPath)
	status.Ahead = ahead
	status.Behind = behind

	// Parse porcelain status。-z 形态按 NUL 分隔、路径字节原样输出，
	// 不受 core.quotePath 的 C 引用影响（issue #104）
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"status", "--porcelain=v1", "-z"}, AuthConfig{})
	if err != nil {
		return nil, newGitError("GetStatus", repoPath, stderr, err)
	}
	parseStatusPorcelainZ(stdout, status)

	status.IsClean = len(status.Staged) == 0 && len(status.Unstaged) == 0 && len(status.Untracked) == 0
	return status, nil
}

// parseStatusPorcelainZ 解析 `git status --porcelain=v1 -z` 的输出。
//
// -z 形态：每条记录 NUL 结尾，路径按字节原样给出（不含 C 引号/转义——中文、
// 引号、空格文件名不再被 quotePath 改写成 `"\"\\346\\226\\207\""` 形态），
// 路径本身保留首尾空格（旧实现的 TrimSpace 会吃掉它们）。rename/copy 条目
// 占两条记录：`XY <dst>` 后随 `<orig>`——旧实现不解引用，Path 是
// "old -> new" 整串。
func parseStatusPorcelainZ(stdout string, status *RepoStatus) {
	fields := strings.Split(stdout, "\x00")
	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		// "XY <path>" 至少 4 字节；末尾残段（空串/换行）跳过
		if len(rec) < 4 {
			continue
		}
		staging := StatusCode(rec[0])
		worktree := StatusCode(rec[1])
		path := rec[3:]

		// rename/copy 条目后随一条 orig-path 记录，消费掉
		if staging == StatusRenamed || staging == StatusCopied ||
			worktree == StatusRenamed || worktree == StatusCopied {
			i++
		}

		if staging == '?' && worktree == '?' {
			status.Untracked = append(status.Untracked, path)
			continue
		}
		fs := FileStatus{Path: path, Staging: staging, Worktree: worktree}
		if staging != ' ' && staging != '?' {
			status.Staged = append(status.Staged, fs)
		}
		if worktree != ' ' && worktree != '?' {
			status.Unstaged = append(status.Unstaged, fs)
		}
	}
}

func (b *NativeGitBackend) getAheadBehind(ctx context.Context, repoPath string) (int, int, error) {
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"rev-list", "--left-right", "--count", "HEAD...@{upstream}"}, AuthConfig{})
	if err != nil {
		return 0, 0, newGitError("getAheadBehind", repoPath, stderr, err)
	}
	parts := strings.Fields(strings.TrimSpace(stdout))
	if len(parts) != 2 {
		return 0, 0, nil
	}
	ahead, behind := 0, 0
	_, _ = fmt.Sscanf(parts[0], "%d", &ahead)
	_, _ = fmt.Sscanf(parts[1], "%d", &behind)
	return ahead, behind, nil
}

func (b *NativeGitBackend) Diff(ctx context.Context, repoPath string, opts DiffOptions) (string, error) {
	if err := checkNameArgs("Diff", opts.From, opts.To); err != nil {
		return "", err
	}
	var args []string
	switch {
	case opts.From == "" && opts.To == "":
		// 契约口径：工作区 vs HEAD。裸 `git diff` 是「工作区 vs index」，
		// staged 变更完全不可见——与契约和 gogit 后端三方分裂（issue #110）
		args = []string{"diff", "HEAD"}
	case opts.From != "" && opts.To != "":
		args = []string{"diff", opts.From, opts.To}
	default:
		// 单边 From/To 的意图不可判（from..worktree? worktree..to?），显式报错
		return "", newGitError("Diff", repoPath, "", fmt.Errorf(
			"both From and To must be given together (From=%q To=%q)", opts.From, opts.To))
	}
	args = append(args, "--")
	args = append(args, opts.Paths...)

	stdout, stderr, err := b.runGit(ctx, repoPath, args, AuthConfig{})
	if err != nil {
		return "", newGitError("Diff", repoPath, stderr, err)
	}
	return stdout, nil
}

// --- Advanced diff operations ---

func (b *NativeGitBackend) DiffNames(ctx context.Context, repoPath, from, to string) ([]string, error) {
	// diff 系命令的 `--` 之后是 pathspec——rev 只能放在它前面，
	// 位置参数反转靠 checkNameArgs 挡（issue #109）
	if err := checkNameArgs("DiffNames", from, to); err != nil {
		return nil, err
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"diff", "--name-only", from, to}, AuthConfig{})
	if err != nil {
		return nil, newGitError("DiffNames", repoPath, stderr, err)
	}
	var result []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

func (b *NativeGitBackend) DeletedFiles(ctx context.Context, repoPath, from, to string) ([]string, error) {
	if err := checkNameArgs("DeletedFiles", from, to); err != nil {
		return nil, err
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"diff", "--name-only", "--diff-filter=D", from, to}, AuthConfig{})
	if err != nil {
		return nil, newGitError("DeletedFiles", repoPath, stderr, err)
	}
	var result []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

// --- Revision parsing ---

func (b *NativeGitBackend) RevParse(ctx context.Context, repoPath, ref string) (string, error) {
	if err := checkNameArg("RevParse", ref); err != nil {
		return "", err
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"rev-parse", ref}, AuthConfig{})
	if err != nil {
		return "", newGitError("RevParse", repoPath, stderr, err)
	}
	return strings.TrimSpace(stdout), nil
}

func (b *NativeGitBackend) MergeBase(ctx context.Context, repoPath, a, other string) (string, error) {
	if err := checkNameArgs("MergeBase", a, other); err != nil {
		return "", err
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"merge-base", "--", a, other}, AuthConfig{})
	if err != nil {
		return "", newGitError("MergeBase", repoPath, stderr, err)
	}
	return strings.TrimSpace(stdout), nil
}
