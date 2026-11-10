package gitbackend

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNative_PushCloneFetchAll(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	origin, _, clone := pushSetup(t)

	if _, err := b.Push(ctx, PushOptions{RepoPath: clone, Remote: "origin"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if got := gitOutput(t, origin, "log", "-1", "--pretty=%s"); got != "local commit" {
		t.Errorf("expected the local commit on origin, got %q", got)
	}

	dst := filepath.Join(t.TempDir(), "clone")
	if err := b.Clone(ctx, CloneOptions{URL: origin, Path: dst}); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		t.Errorf("expected the clone on disk: %v", err)
	}

	if err := b.FetchAll(ctx, dst, AuthConfig{}); err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
}

func TestNative_PullBehindOrigin(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	origin := bareOrigin(t)
	seed := t.TempDir()
	gitOutput(t, seed, "init", "--initial-branch=main")
	gitOutput(t, seed, "config", "user.email", "test@test.com")
	gitOutput(t, seed, "config", "user.name", "Test")
	gitOutput(t, seed, "remote", "add", "origin", origin)
	commitFile(t, seed, "a.txt", "one", "first")
	gitOutput(t, seed, "push", "-q", "origin", "main")

	clone := t.TempDir()
	cloneRepo(t, origin, clone)
	tip := commitFile(t, seed, "remote.txt", "remote work", "remote commit")
	gitOutput(t, seed, "push", "-q", "origin", "main")

	if err := b.Pull(ctx, clone, "origin", "main", AuthConfig{}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got := gitOutput(t, clone, "rev-parse", "HEAD"); got != tip {
		t.Errorf("expected clone HEAD at %s after pull, got %s", tip, got)
	}
}

func TestNative_RunRawAndTestConnection(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	origin, _, _ := pushSetup(t)

	stdout, _, err := b.RunRaw(ctx, origin, []string{"log", "-1", "--pretty=%s"})
	if err != nil || strings.TrimSpace(stdout) != "seed" {
		t.Errorf("RunRaw: got %q (%v)", stdout, err)
	}
	if err := b.TestConnection(ctx, origin, AuthConfig{}); err != nil {
		t.Errorf("TestConnection to a local origin: %v", err)
	}
	if err := b.TestConnection(ctx, filepath.Join(t.TempDir(), "missing"), AuthConfig{}); err == nil {
		t.Error("expected TestConnection to fail for a missing repo")
	}
}

func TestNative_FileQueriesAndCheckout(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "v2", "second touch")
	head := headHash(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "guide.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", ".")
	gitOutput(t, repo, "commit", "-m", "docs")
	tip := headHash(t, repo)

	commits, err := b.GetFileHistory(ctx, repo, "a.txt", 1)
	if err != nil || len(commits) != 1 || commits[0].Message != "second touch" {
		t.Errorf("GetFileHistory: got %+v (%v)", commits, err)
	}

	flat, err := b.GetTree(ctx, repo, "HEAD", "", false)
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}
	var docs *TreeEntry
	for i := range flat {
		if flat[i].Name == "docs" {
			docs = &flat[i]
		}
	}
	if docs == nil || docs.Type != TreeEntryDir || docs.Mode != "040000" {
		t.Errorf("expected a docs tree entry, got %+v", flat)
	}

	recursive, err := b.GetTree(ctx, repo, "HEAD", "docs", true)
	if err != nil || len(recursive) != 1 || recursive[0].Path != "docs/guide.md" {
		t.Errorf("GetTree (docs, recursive): got %+v (%v)", recursive, err)
	}

	text, err := b.GetBlob(ctx, repo, "HEAD", "a.txt")
	if err != nil || text.Encoding != EncodingUTF8 || text.Content != "v2" {
		t.Errorf("GetBlob (text): got %+v (%v)", text, err)
	}
	_, err = b.GetBlob(ctx, repo, "HEAD", "missing.txt")
	if err == nil || !IsNotFound(err) {
		t.Errorf("GetBlob (missing): expected NotFound, got %v", err)
	}

	// Rewind via ref, then restore a clobbered file by name.
	if err := b.CheckoutRef(ctx, repo, head); err != nil {
		t.Fatalf("CheckoutRef: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs")); !os.IsNotExist(err) {
		t.Errorf("expected docs gone at the older checkout, stat err = %v", err)
	}
	if err := b.CheckoutRef(ctx, repo, tip); err != nil {
		t.Fatalf("CheckoutRef (back): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("clobbered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.CheckoutFiles(ctx, repo, "HEAD", []string{"a.txt"}); err != nil {
		t.Fatalf("CheckoutFiles: %v", err)
	}
	if body, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(body) != "v2" {
		t.Errorf("expected a.txt restored, got %q", body)
	}
}

func TestNative_RemotesAndTags(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	origin, _, clone := pushSetup(t)

	url, err := b.GetRemoteURL(ctx, clone, "origin")
	if err != nil || url != origin {
		t.Errorf("GetRemoteURL: got %q (%v)", url, err)
	}
	names, err := b.GetRemotes(ctx, clone)
	if err != nil || !contains(names, "origin") {
		t.Errorf("GetRemotes: got %v (%v)", names, err)
	}

	if err := b.CreateTag(ctx, clone, "v1", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	tags, err := b.GetTagList(ctx, clone)
	if err != nil || len(tags) != 1 || tags[0].Name != "v1" || tags[0].Hash == "" {
		t.Errorf("GetTagList: got %+v (%v)", tags, err)
	}
	if err := b.PushTag(ctx, clone, "origin", "v1", AuthConfig{}); err != nil {
		t.Fatalf("PushTag: %v", err)
	}
	if got := gitOutput(t, origin, "tag"); got != "v1" {
		t.Errorf("expected v1 on origin, got %q", got)
	}
	if err := b.DeleteTag(ctx, clone, "v1"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if tags, _ := b.GetTagList(ctx, clone); len(tags) != 0 {
		t.Errorf("expected no tags after deletion, got %+v", tags)
	}
}

func TestNative_StashLifecycle(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)

	// git stash push only stashes tracked changes, so dirty a tracked file.
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.StashSave(ctx, repo, "native stash"); err != nil {
		t.Fatalf("StashSave: %v", err)
	}
	if body, _ := os.ReadFile(filepath.Join(repo, "README.md")); string(body) != "init" {
		t.Errorf("expected the tracked file stashed away, got %q", body)
	}
	entries, err := b.StashList(ctx, repo)
	if err != nil || len(entries) != 1 || !strings.Contains(entries[0].Message, "native stash") {
		t.Fatalf("StashList: got %+v (%v)", entries, err)
	}

	if err := b.StashApply(ctx, repo, 0); err != nil {
		t.Fatalf("StashApply: %v", err)
	}
	if body, _ := os.ReadFile(filepath.Join(repo, "README.md")); string(body) != "wip" {
		t.Errorf("expected the change restored, got %q", body)
	}

	// Drop clears the entry; clear on an empty stash is a no-op success.
	if err := b.StashDrop(ctx, repo, 0); err != nil {
		t.Fatalf("StashDrop: %v", err)
	}
	if entries, _ := b.StashList(ctx, repo); len(entries) != 0 {
		t.Errorf("expected no stash entries, got %+v", entries)
	}
	if err := b.StashClear(ctx, repo); err != nil {
		t.Fatalf("StashClear: %v", err)
	}
}

func TestNative_StashPop(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.StashSave(ctx, repo, ""); err != nil {
		t.Fatalf("StashSave: %v", err)
	}
	if err := b.StashPop(ctx, repo, 0); err != nil {
		t.Fatalf("StashPop: %v", err)
	}
	if body, _ := os.ReadFile(filepath.Join(repo, "README.md")); string(body) != "wip" {
		t.Errorf("expected the change restored by pop, got %q", body)
	}
	if entries, _ := b.StashList(ctx, repo); len(entries) != 0 {
		t.Errorf("expected pop to drop the entry, got %+v", entries)
	}
}

func TestNative_Config(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)

	if err := b.SetConfig(ctx, repo, "native.key", "value"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got, err := b.GetConfig(ctx, repo, "native.key"); err != nil || got != "value" {
		t.Errorf("GetConfig: got %q (%v)", got, err)
	}
	if _, err := b.GetConfig(ctx, repo, "native.missing"); err == nil {
		t.Error("expected an error for a missing config key")
	}
}

func TestNative_DiffStatusAndRevisionQueries(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "one", "first")
	base := headHash(t, repo)
	commitFile(t, repo, "b.txt", "two", "second")
	gitOutput(t, repo, "rm", "-q", "README.md")
	gitOutput(t, repo, "commit", "-m", "remove readme")
	tip := headHash(t, repo)

	if got, err := b.RevParse(ctx, repo, "HEAD"); err != nil || got != tip {
		t.Errorf("RevParse: got %q (%v)", got, err)
	}
	if got, err := b.MergeBase(ctx, repo, base, tip); err != nil || got != base {
		t.Errorf("MergeBase: got %q (%v)", got, err)
	}
	names, err := b.DiffNames(ctx, repo, base, tip)
	if err != nil || !contains(names, "b.txt") || !contains(names, "README.md") {
		t.Errorf("DiffNames: got %v (%v)", names, err)
	}
	deleted, err := b.DeletedFiles(ctx, repo, base, tip)
	if err != nil || len(deleted) != 1 || deleted[0] != "README.md" {
		t.Errorf("DeletedFiles: got %v (%v)", deleted, err)
	}
	diff, err := b.Diff(ctx, repo, DiffOptions{From: base, To: tip})
	if err != nil || !strings.Contains(diff, "b.txt") {
		t.Errorf("Diff: got %q (%v)", diff, err)
	}

	// RebaseContinue with no rebase in progress must error, not no-op.
	if err := b.RebaseContinue(ctx, repo); err == nil {
		t.Error("expected an error continuing with no rebase in progress")
	}
}

func TestNativeOutputParsers(t *testing.T) {
	// PushedRefs 是 ref 名（push 报告的 dst），不是 URL 与 old..new 区间
	pushed := parsePushRefs("To /repo\n   abc123..def456  main -> main")
	if len(pushed) != 1 || pushed[0] != "main" {
		t.Errorf("parsePushRefs: got %v, want [main]", pushed)
	}
	if got := parsePushRefs("nothing to do"); len(got) != 0 {
		t.Errorf("parsePushRefs(no-op): got %v", got)
	}

	if !isText(nil) || !isText([]byte("plain text")) {
		t.Error("expected empty and plain text to classify as text")
	}
	if isText([]byte{0x00, 0x01}) || isText([]byte{0xFF, 0xFE}) {
		t.Error("expected null bytes and invalid UTF-8 to classify as binary")
	}
}

// TestNative_Fetch_ClassifiesNewUpdatedDeletedAndTags verifies the native
// backend fills every FetchResult field from the before/after ref snapshots,
// with short branch/tag names, mirroring the gogit backend.
func TestNative_Fetch_ClassifiesNewUpdatedDeletedAndTags(t *testing.T) {
	b := newTestNativeBackend(t)
	ctx := context.Background()
	origin := bareOrigin(t)
	seed := t.TempDir()
	gitOutput(t, seed, "init", "--initial-branch=main")
	gitOutput(t, seed, "config", "user.email", "test@test.com")
	gitOutput(t, seed, "config", "user.name", "Test")
	gitOutput(t, seed, "remote", "add", "origin", origin)
	commitFile(t, seed, "a.txt", "one", "first")
	gitOutput(t, seed, "push", "-q", "origin", "main")

	clone := t.TempDir()
	cloneRepo(t, origin, clone)

	// Origin gains a branch, a tag, and a moved main.
	gitOutput(t, seed, "branch", "feature")
	gitOutput(t, seed, "tag", "v1")
	commitFile(t, seed, "a.txt", "two", "second")
	gitOutput(t, seed, "push", "-q", "origin", "main", "feature", "v1")

	res, err := b.Fetch(ctx, FetchOptions{RepoPath: clone, Tags: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !contains(res.NewBranches, "feature") {
		t.Errorf("expected NewBranches to report feature (short name), got %+v", res)
	}
	if !contains(res.UpdatedBranch, "main") {
		t.Errorf("expected UpdatedBranch to report main (short name), got %+v", res)
	}
	if !contains(res.NewTags, "v1") {
		t.Errorf("expected NewTags to report v1, got %+v", res)
	}
	if !contains(res.FetchedRefs, "refs/remotes/origin/feature") {
		t.Errorf("expected FetchedRefs to carry the full new remote-tracking ref, got %+v", res)
	}
	if len(res.DeletedBranch) != 0 {
		t.Errorf("expected no deletions yet, got %+v", res)
	}

	// Deleting feature on the remote must surface as a pruned short name,
	// even though the fetch did not ask for Prune (native always prunes).
	gitOutput(t, seed, "push", "-q", "origin", ":refs/heads/feature")
	res, err = b.Fetch(ctx, FetchOptions{RepoPath: clone})
	if err != nil {
		t.Fatalf("Fetch (after remote delete): %v", err)
	}
	if !contains(res.DeletedBranch, "feature") {
		t.Errorf("expected DeletedBranch to report feature (short name), got %+v", res)
	}
	if contains(res.NewBranches, "feature") || contains(res.UpdatedBranch, "feature") {
		t.Errorf("expected feature only in DeletedBranch, got %+v", res)
	}
}

// TestDiffFetchRefs unit-tests the classification rules in isolation,
// including the tag namespace and the moved-hash case.
func TestDiffFetchRefs(t *testing.T) {
	before := map[string]string{
		"refs/remotes/origin/main":   "aaaa",
		"refs/remotes/origin/gone":   "bbbb",
		"refs/tags/v1":               "cccc",
		"refs/tags/v1-unchanged":     "dddd",
		"refs/remotes/origin/stable": "eeee",
	}
	after := map[string]string{
		"refs/remotes/origin/main":   "1111",
		"refs/remotes/origin/newbr":  "2222",
		"refs/tags/v1":               "cccc", // unchanged tags are not reported
		"refs/tags/v2":               "3333",
		"refs/tags/v1-unchanged":     "dddd",
		"refs/remotes/origin/stable": "eeee",
	}

	res := diffFetchRefs("origin", before, after)
	if !contains(res.NewBranches, "newbr") || len(res.NewBranches) != 1 {
		t.Errorf("NewBranches: got %v", res.NewBranches)
	}
	if !contains(res.UpdatedBranch, "main") || len(res.UpdatedBranch) != 1 {
		t.Errorf("UpdatedBranch: got %v", res.UpdatedBranch)
	}
	if !contains(res.DeletedBranch, "gone") || len(res.DeletedBranch) != 1 {
		t.Errorf("DeletedBranch: got %v", res.DeletedBranch)
	}
	if !contains(res.NewTags, "v2") || len(res.NewTags) != 1 {
		t.Errorf("NewTags: got %v", res.NewTags)
	}
	if !contains(res.FetchedRefs, "refs/remotes/origin/main") ||
		!contains(res.FetchedRefs, "refs/remotes/origin/newbr") ||
		!contains(res.FetchedRefs, "refs/tags/v2") || len(res.FetchedRefs) != 3 {
		t.Errorf("FetchedRefs: got %v", res.FetchedRefs)
	}
}
