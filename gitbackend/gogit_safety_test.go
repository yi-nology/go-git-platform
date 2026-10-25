package gitbackend

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestGoGitBackend 的夹具见同包的 gogit_branch_test.go。
// 本文件钉住 gogit 后端**会破坏仓库**的几处行为(与 native 后端的语义对齐)。

// TestGoGit_Checkout_DirtyWorktree_KeepsHEAD 脏工作区切分支必须**整体失败**。
//
// 回归背景(P0):go-git 的 Checkout 先把 HEAD 切到目标分支、再 Reset 工作区
// ⇒ 工作区脏时 Reset 失败,但 **HEAD 已经切走** ⇒ 调用方拿到错误却面对
// "半切换"的仓库(HEAD=新分支、文件还是旧分支的内容)⇒ 后续 Commit 会把
// 旧分支的内容提交到新分支。
//
// 这条用例是**反向验证**:去掉 Checkout 里的脏预检,本用例变红(HEAD 被切走)。
func TestGoGit_Checkout_DirtyWorktree_KeepsHEAD(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	commitFile(t, repo, "a.txt", "base", "base")
	gitOutput(t, repo, "checkout", "-b", "feature")
	commitFile(t, repo, "f.txt", "feature", "feature commit")
	gitOutput(t, repo, "checkout", "-")

	before := currentBranch(t, repo)

	// 弄脏工作区(未暂存的改动)
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatalf("dirty file: %v", err)
	}

	err := b.Checkout(context.Background(), repo, "feature")
	if err == nil {
		t.Fatal("脏工作区切分支必须失败")
	}
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Errorf("期望 ErrDirtyWorktree,实际: %v", err)
	}
	if got := currentBranch(t, repo); got != before {
		t.Errorf("HEAD 必须在失败后保持不变(期望 %q,实际 %q)—— 半切换的仓库比报错更糟", before, got)
	}
}

// TestGoGit_DeleteBranch_CurrentBranch_Refused 不能删当前分支。
//
// 回归背景(P0):以前直接删 ref ⇒ HEAD 悬空 ⇒ 仓库不可用
// (`git rev-parse HEAD` 直接失败);native 报 "cannot delete branch used by worktree"。
func TestGoGit_DeleteBranch_CurrentBranch_Refused(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "x", "c")

	cur := currentBranch(t, repo)
	err := b.DeleteBranch(context.Background(), repo, cur)
	if err == nil {
		t.Fatal("删除当前分支必须被拒")
	}
	if !errors.Is(err, ErrBranchInUse) {
		t.Errorf("期望 ErrBranchInUse,实际: %v", err)
	}
	// 仓库必须仍然可用
	_ = headHash(t, repo)
}

// TestGoGit_RenameBranch_ExistingName_Refused 重命名不能占用已存在的名字。
//
// 回归背景(P0):以前直接 SetReference ⇒ **静默把别人的分支指针改掉**;
// native 报 "already exists"。
func TestGoGit_RenameBranch_ExistingName_Refused(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "x", "c")
	gitOutput(t, repo, "branch", "other")

	before := gitOutput(t, repo, "rev-parse", "refs/heads/other")
	// 目标名取**已存在**的当前分支名(夹具的默认分支名不一定是 main)
	taken := currentBranch(t, repo)

	err := b.RenameBranch(context.Background(), repo, "other", taken)
	if err == nil {
		t.Fatal("重命名到已存在的分支名必须被拒")
	}
	if !errors.Is(err, ErrBranchExists) {
		t.Errorf("期望 ErrBranchExists,实际: %v", err)
	}
	if after := gitOutput(t, repo, "rev-parse", "refs/heads/other"); after != before {
		t.Errorf("被占用的分支指针不得被改动(%q → %q)", before, after)
	}
}

// TestGoGit_RenameBranch_CurrentBranch_KeepsRepoUsable 重命名当前分支后仓库仍可用。
//
// 回归背景(P0):以前只改 refs/heads/<new> 并删掉旧 ref ⇒ HEAD 仍指向已删除的
// refs/heads/<old> ⇒ 仓库不可用(`git rev-parse HEAD` 失败)。
func TestGoGit_RenameBranch_CurrentBranch_KeepsRepoUsable(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "x", "c")

	before := currentBranch(t, repo)
	if err := b.RenameBranch(context.Background(), repo, before, before+"-renamed"); err != nil {
		t.Fatalf("RenameBranch: %v", err)
	}
	if got := currentBranch(t, repo); got != before+"-renamed" {
		t.Errorf("当前分支应跟着重命名(期望 %q,实际 %q)", before+"-renamed", got)
	}
	_ = headHash(t, repo) // 仓库必须仍然可用
}

// TestGoGit_Merge_FastForward_UpdatesWorktree 快进合并必须同步工作区。
//
// 回归背景(P0):以前只把分支 ref 指到新提交 ⇒ **工作区/索引仍是旧内容**
// ⇒ HEAD 与磁盘不一致(下一次 Add/Commit 会把"删除"当成待提交变更);
// native 的 git merge 会更新工作区。
func TestGoGit_Merge_FastForward_UpdatesWorktree(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	commitFile(t, repo, "a.txt", "base", "base")
	gitOutput(t, repo, "checkout", "-b", "feature")
	commitFile(t, repo, "new.txt", "new", "feature commit")
	gitOutput(t, repo, "checkout", "-")

	if err := b.Merge(context.Background(), repo, "feature", MergeOptions{}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// 工作区必须已经有新文件
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); err != nil {
		t.Errorf("快进后工作区必须出现新文件: %v", err)
	}
	// 且状态干净(没有"待提交的删除")
	st := gitOutput(t, repo, "status", "--porcelain")
	if st != "" {
		t.Errorf("快进后工作区应当干净,实际 status=%q", st)
	}
}

// TestGoGit_CherryPick_DirtyWorktree_Refused 脏工作区不许 pick。
//
// 回归背景(P0):gogit 的 pick 是"按路径覆盖"式实现 ⇒ 调用方**未提交的修改**
// 会被直接覆盖并**提交成功**(本地工作静默丢失);native 的 git cherry-pick
// 会拒绝("Your local changes … would be overwritten")。
func TestGoGit_CherryPick_DirtyWorktree_Refused(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	commitFile(t, repo, "a.txt", "base", "base")
	head := commitFile(t, repo, "b.txt", "picked", "to be picked")
	// 回退到父提交,让 head 成为"待 pick 的提交"
	gitOutput(t, repo, "reset", "--hard", "HEAD~1")

	// 弄脏工作区
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("dirty-local"), 0o644); err != nil {
		t.Fatalf("dirty file: %v", err)
	}

	err := b.CherryPick(context.Background(), repo, head)
	if err == nil {
		t.Fatal("脏工作区 pick 必须被拒")
	}
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Errorf("期望 ErrDirtyWorktree,实际: %v", err)
	}
	// 本地改动必须还在
	data, rerr := os.ReadFile(filepath.Join(repo, "a.txt"))
	if rerr != nil {
		t.Fatalf("read a.txt: %v", rerr)
	}
	if string(data) != "dirty-local" {
		t.Errorf("本地未提交的改动不得被覆盖,实际内容=%q", string(data))
	}
}

// TestGoGit_Clone_Submodules_ClonesThem 克隆必须带上子模块。
//
// 回归背景(P1):gogit 完全忽略 CloneOptions.Submodules ⇒ 生产的 gogit 后端下
// "递归同步子模块"这个开关**静默无效**(clone 出来 submodule 目录是空的,
// 而 .gitmodules 在);native 走 --recurse-submodules。
func TestGoGit_Clone_Submodules_ClonesThem(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git binary")
	}
	// 构造一个含子模块的源仓库
	src := t.TempDir()
	sub := filepath.Join(src, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	gitOutput(t, sub, "init")
	commitFile(t, sub, "s.txt", "sub-content", "sub commit")

	gitOutput(t, src, "init")
	commitFile(t, src, "main.txt", "main", "main commit")
	// file:// URL:go-git 的 file transport 认它(裸路径不行)
	subURL := "file://" + sub
	gitOutput(t, src, "-c", "protocol.file.allow=always", "submodule", "add", subURL, "submod")
	// submodule add 只是**暂存**(.gitmodules + gitlink)⇒ 必须提交,否则源仓里没有子模块
	gitOutput(t, src, "commit", "-m", "add submodule")

	dst := filepath.Join(t.TempDir(), "clone")
	b := newTestGoGitBackend(t)
	if err := b.Clone(context.Background(), CloneOptions{
		URL: src, Path: dst, Submodules: true,
	}); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// .gitmodules 必须在(浅克隆也会带它)
	if _, err := os.Stat(filepath.Join(dst, ".gitmodules")); err != nil {
		t.Errorf("克隆结果应含 .gitmodules: %v", err)
	}
	// 子模块目录必须有内容
	entries, err := os.ReadDir(filepath.Join(dst, "submod"))
	if err != nil {
		t.Fatalf("read submodule dir: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == "s.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("子模块内容未被克隆(Submodules:true 被忽略)—— 目录=%v", entries)
	}
}

// TestGoGit_GetBranchSyncInfo_UnrelatedHistories 无共同祖先时不能报 (0,0)。
//
// 回归背景(P1,漂移检测会被漏判):两侧历史无关(目标仓被别人强推改写过)时,
// 旧实现直接 `return 0, 0, nil` ⇒ 上层(漂移检测 / 同步状态)**误判成"已同步"**
// ⇒ 目标仓被改写历史后漂移再也检测不到。
// native 走 `git rev-list --left-right --count A...B`,会给出两侧各自的提交数。
//
// 这条用例是**反向验证**:把"无共同祖先 ⇒ 返回 (0,0)"改回去,本用例变红。
func TestGoGit_GetBranchSyncInfo_UnrelatedHistories(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	// 基线分支(夹具的默认分支名不一定是 main)
	base := currentBranch(t, repo)
	// 基线分支:一个提交
	commitFile(t, repo, "a.txt", "a", "base commit")
	// orphan:与基线分支无共同祖先的另一个历史
	gitOutput(t, repo, "checkout", "--orphan", "orphan")
	gitOutput(t, repo, "rm", "-rf", ".")
	commitFile(t, repo, "b.txt", "b", "orphan commit")
	gitOutput(t, repo, "checkout", base)

	// 造一个远端跟踪 ref 指向 orphan 历史
	gitOutput(t, repo, "update-ref", "refs/remotes/origin/"+base, "refs/heads/orphan")

	ahead, behind, err := b.GetBranchSyncInfo(context.Background(), repo, base, "origin/"+base)
	require.NoError(t, err)
	assert.NotZero(t, ahead+behind,
		"两侧历史无关时不能报 (0,0) —— 那会让上层误判成\"已同步\",漂移再也检测不到")
}
