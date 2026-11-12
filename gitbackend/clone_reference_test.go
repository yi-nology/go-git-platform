package gitbackend

import (
	"context"
	"errors"
	"os"
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
	dst := filepath.Join(t.TempDir(), "work")

	if err := b.Clone(context.Background(), CloneOptions{URL: src, Path: dst, Reference: bare, Dissociate: true}); err != nil {
		t.Fatalf("reference+dissociate clone: %v", err)
	}
	if _, err := os.Stat(alternatesPath(dst)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("alternates must be gone after --dissociate (self-contained output), got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "hello.txt")); err != nil {
		t.Fatalf("cloned workdir must have the checked-out file: %v", err)
	}
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

// TestCloneReference_ValidationFailsFast 回归（issue #148 行为要求 3）：
// Reference 指向不存在/非 git 仓的路径 ⇒ 明确报错（ErrNotAGitRepo），
// 绝不静默退化成普通克隆（dest 不会被创建）；Dissociate 脱离 Reference
// 单独为真 ⇒ ErrInvalidGitArg。校验器两后端共享，错误同形。
func TestCloneReference_ValidationFailsFast(t *testing.T) {
	b := newTestNativeBackend(t)
	src, _ := bareSourceRepo(t)

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
			name:    "dissociate without reference",
			opts:    CloneOptions{URL: src, Dissociate: true},
			wantErr: ErrInvalidGitArg,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "work")
			tc.opts.Path = dst
			err := b.Clone(context.Background(), tc.opts)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want errors.Is(%v)", err, tc.wantErr)
			}
			if _, statErr := os.Stat(dst); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("failed validation must not clone: dest exists (%v)", statErr)
			}
		})
	}
}

// TestGoGit_Clone_ReferenceUnsupported 回归（issue #148 gogit 边界）：gogit
// 后端收到 Reference ⇒ fail-fast 报 ErrReferenceUnsupported（文案指明走
// native）。夹具用手工 HEAD 文件通过共享校验器——本测试不依赖 git 二进制。
func TestGoGit_Clone_ReferenceUnsupported(t *testing.T) {
	b := newTestGoGitBackend(t)
	fakeRef := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeRef, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
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
	commitFile(t, src, "more.txt", "incremental", "second commit")
	if err := b.FetchAll(context.Background(), dst, AuthConfig{}); err != nil {
		t.Fatalf("fetch from shared alternates workdir: %v", err)
	}
	// git 输出实测路径，验证增量对象可达
	out := gitOutput(t, dst, "rev-list", "--all", "--count")
	if out == "" {
		t.Fatal("rev-list returned nothing after incremental fetch")
	}
}
