package gitbackend

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- issue #101：IsAncestor 退出码分流 ---

// TestNative_IsAncestor_InvalidRefIsError 回归：`git merge-base --is-ancestor`
// 退出码 1 = 非祖先（正常结论）、≥2（实测 128）= 致命错误（ref 不存在）。
// 旧实现把一切错误吞成 (false, nil)，无效 ref 与「确实不是祖先」不可区分；
// gogit 后端对不可解析 ref 返回 error——两后端行为分裂。
func TestNative_IsAncestor_InvalidRefIsError(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)
	base := headHash(t, repo)
	commitFile(t, repo, "a.txt", "v2", "second")
	tip := headHash(t, repo)

	cur := currentBranch(t, repo) // CI 的 git init 默认分支可能是 master
	cases := []struct {
		name       string
		ancestor   string
		descendant string
		wantOK     bool
		wantErr    bool
	}{
		{"ancestor of descendant", base, tip, true, false},
		{"not an ancestor", tip, base, false, false},
		{"invalid ancestor ref", "no-such-ref", tip, false, true},
		{"invalid descendant ref", base, "no-such-ref", false, true},
		{"both invalid", "no-a", "no-b", false, true},
		{"branch names instead of hashes", cur, "HEAD", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := b.IsAncestor(ctx, repo, tc.ancestor, tc.descendant)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("IsAncestor(%q, %q) = (false, nil), want error for unresolvable ref", tc.ancestor, tc.descendant)
				}
				return
			}
			if err != nil {
				t.Fatalf("IsAncestor(%q, %q): %v", tc.ancestor, tc.descendant, err)
			}
			if ok != tc.wantOK {
				t.Fatalf("IsAncestor(%q, %q) = %v, want %v", tc.ancestor, tc.descendant, ok, tc.wantOK)
			}
		})
	}

	// 非仓库目录必须报错而不是 (false, nil)
	notRepo := t.TempDir()
	if _, err := b.IsAncestor(ctx, notRepo, base, tip); err == nil {
		t.Fatal("IsAncestor on a non-repo must error")
	}
}

// --- issue #104：porcelain -z 解析 ---

// TestNative_GetStatus_SpecialNames 回归：-z 形态路径按字节原样输出——
// 中文文件名不被 core.quotePath 转义成 C 引号串、文件名首尾空格不被
// TrimSpace 吃掉。
func TestNative_GetStatus_SpecialNames(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)

	cjk := "文档一.md"
	spacey := "  leading and trailing  .txt"
	writeRepoFile(t, repo, cjk, "cjk content")
	writeRepoFile(t, repo, spacey, "spacey content")
	gitOutput(t, repo, "add", "--", cjk, spacey)

	st, err := b.GetStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, fs := range st.Staged {
		got = append(got, fs.Path)
	}
	if !contains(got, cjk) {
		t.Fatalf("CJK name must appear verbatim, got %v", got)
	}
	if !contains(got, spacey) {
		t.Fatalf("name with spaces must keep its edges, got %v", got)
	}
}

// TestNative_GetStatus_RenameSplit 回归：porcelain -z 的 rename 条目是
// 「XY <dst>\0<orig>\0」两条记录，Path 必须是 dst——旧实现不解引用，
// Path 是 "old -> new" 整串。
func TestNative_GetStatus_RenameSplit(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)

	writeRepoFile(t, repo, "old.txt", "content")
	gitOutput(t, repo, "add", "--", "old.txt")
	gitOutput(t, repo, "commit", "-m", "add old")
	gitOutput(t, repo, "mv", "old.txt", "new.txt")

	st, err := b.GetStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 1 {
		t.Fatalf("expected exactly one staged entry (the rename), got %+v", st.Staged)
	}
	fs := st.Staged[0]
	if fs.Path != "new.txt" {
		t.Fatalf("rename Path must be the new name, got %q", fs.Path)
	}
	if fs.Staging != StatusRenamed {
		t.Fatalf("rename Staging code = %q, want R", fs.Staging)
	}
}

// TestParseStatusPorcelainZ 单测解析器的记录布局（含 rename 双记录、
// 末尾残段、untracked）。
func TestParseStatusPorcelainZ(t *testing.T) {
	stdout := "R  new name.txt\x00old name.txt\x00" +
		"A  文档.md\x00" +
		" M modified.txt\x00" + // X=空格 Y=M：仅工作区修改 → Unstaged
		"?? untracked file.txt\x00" +
		"D  deleted.txt\x00" + // X=D Y=空格：仅暂存删除 → Staged
		"\n"
	status := &RepoStatus{}
	parseStatusPorcelainZ(stdout, status)

	if len(status.Staged) != 3 { // rename + CJK add + staged deletion
		t.Fatalf("staged = %+v, want 3 entries", status.Staged)
	}
	if status.Staged[0].Path != "new name.txt" {
		t.Fatalf("rename dst path = %q, want %q", status.Staged[0].Path, "new name.txt")
	}
	if status.Staged[1].Path != "文档.md" {
		t.Fatalf("CJK path misparsed: %q", status.Staged[1].Path)
	}
	if len(status.Unstaged) != 1 || status.Unstaged[0].Path != "modified.txt" {
		t.Fatalf("unstaged = %+v", status.Unstaged)
	}
	if len(status.Untracked) != 1 || status.Untracked[0] != "untracked file.txt" {
		t.Fatalf("untracked = %+v", status.Untracked)
	}
	// deleted.txt 只出现在 staged（D 空格）：worktree 列是空格，不应进 Unstaged
	for _, fs := range status.Unstaged {
		if fs.Path == "deleted.txt" {
			t.Fatalf("staged deletion must not appear in Unstaged: %+v", fs)
		}
	}
}

// --- issue #107：parsePushRefs ---

// TestParsePushRefs 钉 push 报告的逐形态解析（实测输出形态见函数注释）：
// dst 是 ref 名；URL 行、old..new 区间、rejected 行不进结果；
// new branch/new tag/deleted 各有归属。
func TestParsePushRefs(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   []string
	}{
		{
			"new branch",
			"To /repo.git\n * [new branch]      main -> main\n",
			[]string{"main"},
		},
		{
			"fast-forward update",
			"To /repo.git\n   abc123..def456  main -> main\n",
			[]string{"main"},
		},
		{
			"forced update",
			"To /repo.git\n + def456...abc123  main -> main\n",
			[]string{"main"},
		},
		{
			"new tag",
			"To /repo.git\n * [new tag]         v1 -> v1\n",
			[]string{"v1"},
		},
		{
			"deletion has no arrow",
			"To /repo.git\n - [deleted]         other\n",
			[]string{"other"},
		},
		{
			"rejected excluded",
			"To /repo.git\n ! [rejected]        main -> main (non-fast-forward)\nerror: failed to push some refs to '/repo.git'\n",
			nil,
		},
		{
			"up-to-date mixed with new",
			"To /repo.git\n * [new branch]      feat -> feat\n   abc..def  main -> main\n",
			[]string{"feat", "main"},
		},
		{
			"multi-ref report",
			"To /repo.git\n * [new branch]      main -> main\n * [new branch]      dev -> dev\n * [new tag]         v1 -> v1\n",
			[]string{"main", "dev", "v1"},
		},
		{
			"nothing to do",
			"Everything up-to-date\n",
			nil,
		},
		{
			"URL with embedded credentials must not leak in",
			"To https://user:pass@host.example.com/repo.git\n * [new branch]      main -> main\n",
			[]string{"main"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePushRefs(tc.output)
			if len(got) != len(tc.want) {
				t.Fatalf("parsePushRefs = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("parsePushRefs = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestNative_Push_PushedRefsAreRefNames 集成回归：真实 push（新分支 →
// 更新 → 删除）后 PushedRefs 都是 ref 名——旧实现返回 URL + 区间，且
// `* [new branch]` 行完全丢失。
func TestNative_Push_PushedRefsAreRefNames(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	origin := bareOrigin(t)
	repo := createTestRepo(t)
	cur := currentBranch(t, repo) // CI 的 git init 默认分支可能是 master
	gitOutput(t, repo, "remote", "add", "origin", origin)
	gitOutput(t, repo, "push", "-q", "origin", cur)

	// 新分支：旧行为 PushedRefs 为空
	gitOutput(t, repo, "branch", "feature")
	res, err := b.Push(ctx, PushOptions{RepoPath: repo, Remote: "origin", RefSpecs: []string{"feature"}})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.PushedRefs, "feature") {
		t.Fatalf("PushedRefs must contain the new branch name, got %+v", res.PushedRefs)
	}
	for _, ref := range res.PushedRefs {
		if strings.Contains(ref, "..") || strings.Contains(ref, "To ") || strings.Contains(ref, "@") {
			t.Fatalf("PushedRefs must be ref names, got %q", ref)
		}
	}

	// 更新已有分支
	commitFile(t, repo, "b.txt", "two", "second")
	res, err = b.Push(ctx, PushOptions{RepoPath: repo, Remote: "origin", RefSpecs: []string{cur}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.PushedRefs) != 1 || res.PushedRefs[0] != cur {
		t.Fatalf("PushedRefs = %+v, want [%s]", res.PushedRefs, cur)
	}

	// 删除远端分支（refspec 无箭头行形态）
	res, err = b.Push(ctx, PushOptions{RepoPath: repo, Remote: "origin", RefSpecs: []string{":refs/heads/feature"}})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.PushedRefs, "feature") {
		t.Fatalf("deletion must report the removed ref, got %+v", res.PushedRefs)
	}
}

// TestPushedRefsFromRefSpecs：refspec → dst 短名归一（gogit 后端与
// native 回填共用）。
func TestPushedRefsFromRefSpecs(t *testing.T) {
	cases := []struct {
		specs []string
		want  []string
	}{
		{[]string{"refs/heads/main:refs/heads/main"}, []string{"main"}},
		{[]string{"main"}, []string{"main"}},
		{[]string{"+refs/heads/feat:refs/heads/feat"}, []string{"feat"}},
		{[]string{"refs/tags/v1"}, []string{"v1"}},
		{[]string{"HEAD:refs/heads/main"}, []string{"main"}},
		{[]string{"refs/heads/*:refs/heads/*"}, []string{"refs/heads/*"}},
		{[]string{"main", "refs/tags/v1"}, []string{"main", "v1"}},
	}
	for _, tc := range cases {
		got := pushedRefsFromRefSpecs(tc.specs)
		if len(got) != len(tc.want) {
			t.Fatalf("pushedRefsFromRefSpecs(%v) = %v, want %v", tc.specs, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("pushedRefsFromRefSpecs(%v) = %v, want %v", tc.specs, got, tc.want)
			}
		}
	}
}

// --- issue #109：位置参数语义反转 ---

// TestNative_PositionalArgSemanticsFlip 回归：以 "-" 开头的名字/rev 会被
// git 的 interspersed 选项解析器当成 flag——CreateBranch("-D", victim) 旧
// 实现会真删掉 victim 并报成功。统一走 checkNameArg 显式拒绝
// （ErrInvalidGitArg）+ 关键位置 `--` 分隔双保险。
func TestNative_PositionalArgSemanticsFlip(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)
	gitOutput(t, repo, "branch", "victim")
	gitOutput(t, repo, "tag", "victitag")

	// CreateBranch("-D", "victim")：victim 必须原封不动，调用拿到错误
	err := b.CreateBranch(ctx, repo, "-D", "victim")
	if !errors.Is(err, ErrInvalidGitArg) {
		t.Fatalf("CreateBranch(-D, victim) err = %v, want ErrInvalidGitArg", err)
	}
	if !branchExists(t, repo, "victim") {
		t.Fatal("victim branch was deleted — positional args still reach git's option parser")
	}

	// 同族形态逐个显式拒绝
	flipCases := []struct {
		name string
		act  func() error
	}{
		{"CreateBranch(ref=-f)", func() error { return b.CreateBranch(ctx, repo, "nb", "-f") }},
		{"DeleteBranch(--delete)", func() error { return b.DeleteBranch(ctx, repo, "--delete") }},
		{"RenameBranch(-m)", func() error { return b.RenameBranch(ctx, repo, "-m", "x") }},
		{"CreateTag(-d)", func() error { return b.CreateTag(ctx, repo, "-d", "HEAD") }},
		{"CreateTag(ref=--force)", func() error { return b.CreateTag(ctx, repo, "t2", "--force") }},
		{"DeleteTag(victitag intact)", func() error { return b.DeleteTag(ctx, repo, "-v") }},
		{"Checkout(-b)", func() error { return b.Checkout(ctx, repo, "-b") }},
		{"CheckoutRef(-b)", func() error { return b.CheckoutRef(ctx, repo, "-b") }},
		{"CheckoutDetached(--detach)", func() error { return b.CheckoutDetached(ctx, repo, "--detach") }},
		{"CherryPick(--abort)", func() error { return b.CherryPick(ctx, repo, "--abort") }},
		{"Rebase(--continue)", func() error { return b.Rebase(ctx, repo, "--continue") }},
		{"Merge(--abort)", func() error { return b.Merge(ctx, repo, "--abort", MergeOptions{}) }},
		{"GetCommit(--output=…)", func() error { _, err := b.GetCommit(ctx, repo, "--output=/tmp/should-not-exist"); return err }},
		{"GetCommitsBetween(--output=…)", func() error {
			_, err := b.GetCommitsBetween(ctx, repo, "--output=/tmp/should-not-exist", "HEAD")
			return err
		}},
		{"RevParse(--flags)", func() error { _, err := b.RevParse(ctx, repo, "--git-dir"); return err }},
		{"DiffNames(--output=…)", func() error { _, err := b.DiffNames(ctx, repo, "--output=/tmp/should-not-exist", "HEAD"); return err }},
		{"AddRemote(name=-D)", func() error { return b.AddRemote(ctx, repo, "-D", "https://example.com/x.git") }},
		{"AddRemote(url=-D)", func() error { return b.AddRemote(ctx, repo, "r2", "-D") }},
		{"GetRemoteURL(--all)", func() error { _, err := b.GetRemoteURL(ctx, repo, "--all"); return err }},
		{"ListRemoteBranches(--list)", func() error { _, err := b.ListRemoteBranches(ctx, repo, "--list"); return err }},
		{"Fetch(--depth)", func() error { _, err := b.Fetch(ctx, FetchOptions{RepoPath: repo, Remote: "--depth"}); return err }},
		{"Push(remote=--delete)", func() error { _, err := b.Push(ctx, PushOptions{RepoPath: repo, Remote: "--delete"}); return err }},
		{"Push(refspec=--force)", func() error {
			_, err := b.Push(ctx, PushOptions{RepoPath: repo, Remote: "origin", RefSpecs: []string{"--force"}})
			return err
		}},
		{"Pull(remote=-q)", func() error { return b.Pull(ctx, repo, "-q", "main", AuthConfig{}) }},
		{"TestConnection(--upload-pack)", func() error { return b.TestConnection(ctx, "--upload-pack", AuthConfig{}) }},
		{"GetFileAtRevision(ref=--output=…)", func() error {
			_, err := b.GetFileAtRevision(ctx, repo, "README.md", "--output=/tmp/should-not-exist")
			return err
		}},
		{"GetTree(ref=--output=…)", func() error { _, err := b.GetTree(ctx, repo, "--output=/tmp/should-not-exist", "", false); return err }},
		{"GetBlob(ref=--output=…)", func() error {
			_, err := b.GetBlob(ctx, repo, "--output=/tmp/should-not-exist", "README.md")
			return err
		}},
	}
	for _, tc := range flipCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.act(); !errors.Is(err, ErrInvalidGitArg) {
				t.Fatalf("err = %v, want ErrInvalidGitArg", err)
			}
		})
	}

	// 攻击面收口验证：--output 文件没有被写出
	if _, err := os.Stat("/tmp/should-not-exist"); err == nil {
		_ = os.Remove("/tmp/should-not-exist") //nolint:gosec // 测试清理
		t.Fatal("--output reached git: arbitrary file write via rev argument")
	}
	// tag 也必须原封不动
	if out := gitOutput(t, repo, "tag", "-l", "victitag"); out != "victitag" {
		t.Fatalf("victim tag disturbed: %q", out)
	}

	// 正常名字不受影响（含 -- 分隔后的建分支/删分支/建删标签）
	if err := b.CreateBranch(ctx, repo, "plain-feat", "HEAD"); err != nil {
		t.Fatalf("normal CreateBranch: %v", err)
	}
	if err := b.DeleteBranch(ctx, repo, "plain-feat"); err != nil {
		t.Fatalf("normal DeleteBranch: %v", err)
	}
	if err := b.CreateTag(ctx, repo, "plain-tag", "HEAD"); err != nil {
		t.Fatalf("normal CreateTag: %v", err)
	}
	if err := b.DeleteTag(ctx, repo, "plain-tag"); err != nil {
		t.Fatalf("normal DeleteTag: %v", err)
	}
	if err := b.RenameBranch(ctx, repo, "victim", "victim2"); err != nil {
		t.Fatalf("normal RenameBranch: %v", err)
	}
}

// TestNative_FetchAll_PerRemoteSessions 回归（issue #106 的 FetchAll 面）：
// 多 remote 仓库上 FetchAll 不再走单条 `git fetch --all`——逐 remote 调用
// 让每个凭证会话只对当前 remote 的主机应答。本测试用两个本地 remote 钉
// 「逐 remote 均被 fetch 到」的功能语义（等价 `git fetch --all`）。
func TestNative_FetchAll_PerRemoteSessions(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()

	// 两个独立 bare remote，各自有不同分支
	origin := bareOrigin(t)
	second := bareOrigin(t)
	repo := createTestRepo(t)
	cur := currentBranch(t, repo) // CI 的 git init 默认分支可能是 master
	gitOutput(t, repo, "remote", "add", "origin", origin)
	gitOutput(t, repo, "remote", "add", "second", second)
	gitOutput(t, repo, "push", "-q", "origin", cur)
	gitOutput(t, repo, "branch", "onsecond")
	gitOutput(t, repo, "push", "-q", "second", "onsecond")

	// 清掉本地分支再全量拉取，验证两个 remote 的 ref 都到达
	gitOutput(t, repo, "branch", "-D", "onsecond")
	if err := b.FetchAll(ctx, repo, AuthConfig{}); err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	remotes := gitOutput(t, repo, "branch", "-r", "--format=%(refname:short)")
	if !strings.Contains(remotes, "origin/"+cur) || !strings.Contains(remotes, "second/onsecond") {
		t.Fatalf("FetchAll must reach every remote, tracking refs:\n%s", remotes)
	}
}

// --- 测试助手 ---

// writeRepoFile 在仓库里写一个文件（内容任意）。
func writeRepoFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, name)), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// branchExists 检查本地分支是否存在。
func branchExists(t *testing.T, repo, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return cmd.Run() == nil
}
