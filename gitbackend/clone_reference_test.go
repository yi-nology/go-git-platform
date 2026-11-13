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

// bareSourceRepo 建一个带提交的 workdir 源仓 + 它的 bare 镜像（--reference
// 的典型消费形态：Clone Cache 的共享 bare 仓）。返回 (源仓 workdir 路径,
// bare 路径)。
func bareSourceRepo(t *testing.T) (string, string) {
	t.Helper()
	src := createTestRepo(t)
	commitFile(t, src, "hello.txt", "shared object", "seed commit")
	bare := filepath.Join(t.TempDir(), "cache.git")
	gitOutput(t, src, "clone", "--bare", src, bare)
	return src, bare
}

func alternatesPath(dst string) string {
	return filepath.Join(dst, ".git", "objects", "info", "alternates")
}

// TestNative_Clone_Reference_SetsAlternates 回归（issue #148）：native 后端
// 的 --reference 直通——克隆后 workdir 的 objects/info/alternates 指向共享
// bare 仓的 objects 目录（git 原生行为实测：--reference 落 alternates 文件，
// 内容为共享仓 objects 的绝对路径）。
func TestNative_Clone_Reference_SetsAlternates(t *testing.T) {
	b := newTestNativeBackend(t)
	src, bare := bareSourceRepo(t)
	dst := filepath.Join(t.TempDir(), "work")

	if err := b.Clone(context.Background(), CloneOptions{URL: src, Path: dst, Reference: bare}); err != nil {
		t.Fatalf("reference clone: %v", err)
	}
	data, err := os.ReadFile(alternatesPath(dst))
	if err != nil {
		t.Fatalf("alternates file must exist after --reference clone: %v", err)
	}
	// git 把 reference 路径解析成真实绝对路径落盘（macOS 的 /var → /private/var
	// 符号链接），断言按解析后的路径对齐。
	want, err := filepath.EvalSymlinks(filepath.Join(bare, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(string(data), want) {
		t.Fatalf("alternates must point at the shared store %q, got %q", want, string(data))
	}
	// workdir 可用：HEAD 提交 checkout 完成
	if _, err := os.Stat(filepath.Join(dst, "hello.txt")); err != nil {
		t.Fatalf("cloned workdir must have the checked-out file: %v", err)
	}
}

// containsLine 断言 body 的某一行恰好等于 want（alternates 是行式文件）。
func containsLine(body, want string) bool {
	for _, line := range strings.Split(body, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

// TestNative_Clone_ReferenceDissociate_SelfContained 回归（issue #148）：
// --dissociate 克隆后断开借用——alternates 文件消失、对象已复制进自身，
// 产物自包含（共享仓此时即可删）。
func TestNative_Clone_ReferenceDissociate_SelfContained(t *testing.T) {
	b := newTestNativeBackend(t)
	src, bare := bareSourceRepo(t)
	seedSHA := gitOutput(t, bare, "rev-parse", "HEAD")
	dst := filepath.Join(t.TempDir(), "work")
	// 反面对照仓:同夹具、不带 --dissociate 的纯借用克隆。
	dstRef := filepath.Join(t.TempDir(), "work-borrowed")

	// file:// 强制传输路径=生产形态真借用(纯本地路径会走硬链优化把对象
	// 全部自有化,反面对照就失效了——探针实测 own=4 vs own=1)。
	srcURL := "file://" + src
	if err := b.Clone(context.Background(), CloneOptions{URL: srcURL, Path: dst, Reference: bare, Dissociate: true}); err != nil {
		t.Fatalf("reference+dissociate clone: %v", err)
	}
	if _, err := os.Stat(alternatesPath(dst)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("alternates must be gone after --dissociate (self-contained output), got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "hello.txt")); err != nil {
		t.Fatalf("cloned workdir must have the checked-out file: %v", err)
	}
	if err := b.Clone(context.Background(), CloneOptions{URL: srcURL, Path: dstRef, Reference: bare}); err != nil {
		t.Fatalf("borrowed control clone: %v", err)
	}

	// #154 收口:自包含的真义——共享仓删掉后照样可读、fsck 干净。
	// (只断言 alternates 文件消失,挡不住「没真正复制对象」的回归。)
	if err := os.RemoveAll(bare); err != nil {
		t.Fatalf("remove shared repo: %v", err)
	}
	gitOutput(t, dst, "cat-file", "-e", seedSHA)
	gitOutput(t, dst, "fsck", "--full")

	// 反面对照:未 dissociate 的借用仓在共享仓删除后读取必须失败——
	// 两相对照才证明上面的成功来自对象复制,而不是巧合。
	if !gitCommandFails(t, dstRef, "cat-file", "-e", seedSHA) {
		t.Fatal("negative control: borrowed-only reads must fail after shared repo deletion")
	}
}

// gitCommandFails 运行 git 命令并返回是否失败(期望失败的断言用)。
func gitCommandFails(t *testing.T, dir string, args ...string) bool {
	t.Helper()
	return exec.Command("git", append([]string{"-C", dir}, args...)...).Run() != nil
}

// TestNative_Clone_ReferenceEmpty_Unchanged 回归（issue #148 行为要求 1）：
// Reference 空 ⇒ 与现状完全一致——不传 --reference、不写 alternates。
func TestNative_Clone_ReferenceEmpty_Unchanged(t *testing.T) {
	b := newTestNativeBackend(t)
	src, bare := bareSourceRepo(t)
	dst := filepath.Join(t.TempDir(), "work")

	if err := b.Clone(context.Background(), CloneOptions{URL: src, Path: dst}); err != nil {
		t.Fatalf("plain clone: %v", err)
	}
	if _, err := os.Stat(alternatesPath(dst)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plain clone must not create alternates: %v", err)
	}
	// 同一夹具下 reference 形态仍生效（护栏：不是全链路退化）
	dst2 := filepath.Join(t.TempDir(), "work2")
	if err := b.Clone(context.Background(), CloneOptions{URL: src, Path: dst2, Reference: bare}); err != nil {
		t.Fatalf("reference clone control: %v", err)
	}
	if _, err := os.Stat(alternatesPath(dst2)); err != nil {
		t.Fatalf("reference clone control must write alternates: %v", err)
	}
}

// TestNative_Clone_ReferenceSanitizerAccepts 直通参数必须过 sanitizeGitArgs
// 的 exec 边界（形态参照 TestCloneOptions_FilterArgs）。
func TestNative_Clone_ReferenceSanitizerAccepts(t *testing.T) {
	args := []string{"clone", "--reference", "/srv/cache/bare.git", "--dissociate", "https://x/y.git", "/tmp/z"}
	if err := sanitizeGitArgs(args); err != nil {
		t.Fatalf("sanitize rejected reference clone args: %v", err)
	}
	args = []string{"clone", "--reference", "/srv/cache/bare.git", "https://x/y.git", "/tmp/z"}
	if err := sanitizeGitArgs(args); err != nil {
		t.Fatalf("sanitize rejected bare reference clone args: %v", err)
	}
}

// TestCloneReference_ValidationFailsFast 回归（issue #148 行为要求 3，
// 审计 #152/#153 收口）：Reference 指向不存在/非 git 仓的路径 ⇒ 明确报错
// （ErrNotAGitRepo）——包括「伪造 HEAD 的裸形状空壳」与「gitfile 指向
// 已删除 gitdir」两种穿透形态；Dissociate 脱离 Reference 单独为真 ⇒
// ErrInvalidGitArg。表对两后端都跑（校验器先于任何后端动作），错误同形：
// errors.Is(哨兵) 与 errors.As(*GitError)（newGitError 契约）都必须成立。
func TestCloneReference_ValidationFailsFast(t *testing.T) {
	src, _ := bareSourceRepo(t)

	// 伪造 HEAD 空壳:只有 HEAD 文件,无 objects/refs——审计探针实证曾穿透
	// 浅探针后报 git 自己的错(丢哨兵)。
	strayHead := t.TempDir()
	if err := os.WriteFile(filepath.Join(strayHead, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// gitfile 指向已删除的 gitdir(共享仓被掏空的真实残骸形态)。
	brokenWorktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(brokenWorktree, ".git"), []byte("gitdir: /nonexistent-gitdir\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		opts    CloneOptions
		wantErr error
	}{
		{
			name:    "missing reference path",
			opts:    CloneOptions{URL: src, Reference: filepath.Join(t.TempDir(), "nope")},
			wantErr: ErrNotAGitRepo,
		},
		{
			name:    "non-git directory",
			opts:    CloneOptions{URL: src, Reference: t.TempDir()},
			wantErr: ErrNotAGitRepo,
		},
		{
			name:    "stray HEAD bare-shape shell",
			opts:    CloneOptions{URL: src, Reference: strayHead},
			wantErr: ErrNotAGitRepo,
		},
		{
			name:    "gitfile pointing at deleted gitdir",
			opts:    CloneOptions{URL: src, Reference: brokenWorktree},
			wantErr: ErrNotAGitRepo,
		},
		{
			name:    "dissociate without reference",
			opts:    CloneOptions{URL: src, Dissociate: true},
			wantErr: ErrInvalidGitArg,
		},
	}
	backends := map[string]GitBackend{
		"native": newTestNativeBackend(t),
		"gogit":  newTestGoGitBackend(t),
	}
	for bname, bk := range backends {
		for _, tc := range cases {
			t.Run(bname+"/"+tc.name, func(t *testing.T) {
				dst := filepath.Join(t.TempDir(), "work")
				tc.opts.Path = dst
				err := bk.Clone(context.Background(), tc.opts)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want errors.Is(%v)", err, tc.wantErr)
				}
				// newGitError 契约:两后端错误都可结构化为 *GitError(#152)。
				var ge *GitError
				if !errors.As(err, &ge) {
					t.Fatalf("err must be *GitError per newGitError contract, got %T: %v", err, err)
				}
				if _, statErr := os.Stat(dst); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("failed validation must not clone: dest exists (%v)", statErr)
				}
			})
		}
	}
}

// TestGoGit_Clone_ReferenceUnsupported 回归（issue #148 gogit 边界）：gogit
// 后端收到 Reference ⇒ fail-fast 报 ErrReferenceUnsupported（文案指明走
// native）。夹具用手工 HEAD 文件通过共享校验器——本测试不依赖 git 二进制。
func TestGoGit_Clone_ReferenceUnsupported(t *testing.T) {
	b := newTestGoGitBackend(t)
	// bare 形状夹具(HEAD+objects+refs)——过深化的共享校验器,走到 gogit
	// 后端自己的 ErrReferenceUnsupported 边界(#153 后浅 HEAD 空壳会被
	// 校验器先拒,不再能到达这里)。
	fakeRef := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeRef, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"objects", "refs"} {
		if err := os.Mkdir(filepath.Join(fakeRef, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	dst := filepath.Join(t.TempDir(), "work")

	err := b.Clone(context.Background(), CloneOptions{
		URL: "https://example.com/o/r.git", Path: dst, Reference: fakeRef,
	})
	if !errors.Is(err, ErrReferenceUnsupported) {
		t.Fatalf("err = %v, want errors.Is(ErrReferenceUnsupported)", err)
	}

	// Dissociate 脱离 Reference：校验器口径（ErrInvalidGitArg），不是
	// ErrReferenceUnsupported——错误语义按层分离。
	err = b.Clone(context.Background(), CloneOptions{
		URL: "https://example.com/o/r.git", Path: dst, Dissociate: true,
	})
	if !errors.Is(err, ErrInvalidGitArg) {
		t.Fatalf("dissociate alone: err = %v, want errors.Is(ErrInvalidGitArg)", err)
	}
}

// TestNative_Clone_ReferenceFetchAfterShared 回归（issue #148 生命周期面）：
// 借用共享仓的 workdir 在共享仓仍在时可以正常 fetch——源仓新增提交后
// FetchAll 能拉到增量（Clone Cache 的常规运维路径）。
func TestNative_Clone_ReferenceFetchAfterShared(t *testing.T) {
	b := newTestNativeBackend(t)
	src, bare := bareSourceRepo(t)
	dst := filepath.Join(t.TempDir(), "work")

	if err := b.Clone(context.Background(), CloneOptions{URL: src, Path: dst, Reference: bare}); err != nil {
		t.Fatalf("reference clone: %v", err)
	}
	newSHA := commitFile(t, src, "more.txt", "incremental", "second commit")
	if err := b.FetchAll(context.Background(), dst, AuthConfig{}); err != nil {
		t.Fatalf("fetch from shared alternates workdir: %v", err)
	}
	// #151 收口:钉真实可达性——新提交对象必须能读到(只断言非空是重言式,
	// fetch 空转/错 refspec 也能过)。夹具确定 3 个提交(init+seed+本次)。
	gitOutput(t, dst, "cat-file", "-e", newSHA)
	if out := gitOutput(t, dst, "rev-list", "--all", "--count"); out != "3" {
		t.Fatalf("rev-list count = %q, want 3 (init+seed+incremental)", out)
	}
}

// TestNative_Clone_ReferenceLifecycleAfterSharedDeleted 回归（审计 #150，
// 探针实测 2026-11-13）：共享仓被删后的三个事实——
//  1. 借来的旧对象读取失败（unable-to-normalize），这是真正的失效面；
//  2. fetch **不失败**（exit 0）——所以不能拿 fetch 当共享仓健康信号；
//  3. fetch 会把整段历史重新下载进 workdir 自有对象库，读取随后恢复。
func TestNative_Clone_ReferenceLifecycleAfterSharedDeleted(t *testing.T) {
	b := newTestNativeBackend(t)
	src, bare := bareSourceRepo(t)
	seedSHA := gitOutput(t, bare, "rev-parse", "HEAD")
	dst := filepath.Join(t.TempDir(), "work")

	// 必须用 file:// URL:纯本地路径克隆走 git 的 local 硬链优化,对象全部
	// 自有(own=4,探针实测),「借来的对象」不存在、删共享仓读取照样成功;
	// file:// 强制传输路径才是生产形态的真借用(own=1)。
	srcURL := "file://" + src
	if err := b.Clone(context.Background(), CloneOptions{URL: srcURL, Path: dst, Reference: bare}); err != nil {
		t.Fatalf("reference clone: %v", err)
	}
	if err := os.RemoveAll(bare); err != nil {
		t.Fatalf("remove shared repo: %v", err)
	}

	// 事实①:共享仓消失后,只存在于共享仓的对象读取失败。
	if !gitCommandFails(t, dst, "cat-file", "-e", seedSHA) {
		t.Fatal("borrowed-only object must fail to read after shared repo deletion")
	}
	// 事实②:fetch 照样成功(exit 0)——文档修正(7d9a524)的依据。
	if err := b.FetchAll(context.Background(), dst, AuthConfig{}); err != nil {
		t.Fatalf("fetch must still succeed with dangling alternates (probe: exit 0), got %v", err)
	}
	// 事实③:历史已重下进自有对象库,读取恢复。
	gitOutput(t, dst, "cat-file", "-e", seedSHA)
}
