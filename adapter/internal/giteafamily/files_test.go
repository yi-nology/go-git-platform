package giteafamily

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// fakeFilePort records the FamilyFileOptions each write verb receives, which
// is the seam where provider.FileOptions.Content (plaintext) becomes the
// family SDK's base64 wire field.
type fakeFilePort struct {
	got                  FamilyFileOptions
	calls                int
	createErr, updateErr error
}

func (f *fakeFilePort) GetFile(context.Context, string, string, string, string) ([]byte, int, error) {
	return nil, 0, nil
}

func (f *fakeFilePort) CreateFile(_ context.Context, _, _, _ string, opt FamilyFileOptions) (*FamilyFileResult, error) {
	f.calls++
	f.got = opt
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &FamilyFileResult{CommitSHA: "c0ffee"}, nil
}

func (f *fakeFilePort) UpdateFile(_ context.Context, _, _, _ string, opt FamilyFileOptions) (*FamilyFileResult, error) {
	f.calls++
	f.got = opt
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &FamilyFileResult{CommitSHA: "c0ffee"}, nil
}

func (f *fakeFilePort) DeleteFile(context.Context, string, string, string, FamilyFileOptions) (*FamilyFileResult, int, error) {
	return &FamilyFileResult{CommitSHA: "c0ffee"}, 200, nil
}

// TestCreateFileBase64EncodesPlaintextContent pins issue #169: the gitea and
// forgejo SDKs declare their file `content` field "must be base64 encoded"
// (gitea.dev/sdk repo_file.go, forgejo-sdk repo_file.go). Before the fix the
// family forwarded the caller's plaintext verbatim, so the same
// provider.FileOptions that works on github/gitlab 4xx'd here. The family is
// the port boundary that owns the codec; callers keep passing plaintext.
func TestCreateFileBase64EncodesPlaintextContent(t *testing.T) {
	const plaintext = "hello world\n"
	port := &fakeFilePort{}
	fam := Family{Platform: model.PlatformGitea}

	res, err := CreateFile(context.Background(), fam, port, "owner", "repo", provider.FileOptions{
		Path:    "docs/hello.txt",
		Content: plaintext,
		Message: "add hello",
		Branch:  "main",
		Author:  "dev",
		Email:   "dev@example.com",
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	if res == nil || res.CommitSHA != "c0ffee" {
		t.Fatalf("result = %+v, want commit c0ffee", res)
	}
	want := base64.StdEncoding.EncodeToString([]byte(plaintext))
	if port.got.Content != want {
		t.Errorf("wire content = %q, want base64 %q (callers pass plaintext)", port.got.Content, want)
	}
	// The rest of the options are pass-through and must not be disturbed by
	// the codec.
	if port.got.Message != "add hello" || port.got.Branch != "main" || port.got.Author != "dev" || port.got.Email != "dev@example.com" {
		t.Errorf("options changed by the content codec: %+v", port.got)
	}
}

// TestUpdateFileBase64EncodesPlaintextContent is the update half: the same
// base64 contract applies to UpdateFile (the SHA is the blob SHA, unrelated
// to the content encoding).
func TestUpdateFileBase64EncodesPlaintextContent(t *testing.T) {
	const plaintext = "updated content\nwith two lines\n"
	port := &fakeFilePort{}
	fam := Family{Platform: model.PlatformForgejo}

	res, err := UpdateFile(context.Background(), fam, port, "owner", "repo", provider.FileOptions{
		Path:    "docs/hello.txt",
		Content: plaintext,
		Message: "update hello",
		Branch:  "main",
		SHA:     "abc123",
	})
	if err != nil {
		t.Fatalf("UpdateFile: %v", err)
	}
	if res == nil || res.CommitSHA != "c0ffee" {
		t.Fatalf("result = %+v, want commit c0ffee", res)
	}
	want := base64.StdEncoding.EncodeToString([]byte(plaintext))
	if port.got.Content != want {
		t.Errorf("wire content = %q, want base64 %q", port.got.Content, want)
	}
	if port.got.SHA != "abc123" || port.got.Message != "update hello" || port.got.Branch != "main" {
		t.Errorf("options changed by the content codec: %+v", port.got)
	}
}

// TestDeleteFileDoesNotTouchContent guards the delete path: it shares
// FamilyFileOptions with create/update, but there is no content to encode.
func TestDeleteFileDoesNotTouchContent(t *testing.T) {
	port := &fakeFilePort{}
	fam := Family{Platform: model.PlatformGitea}
	if _, err := DeleteFile(context.Background(), fam, port, "owner", "repo", provider.FileDeleteOptions{
		Path: "docs/hello.txt", Message: "delete hello", Branch: "main", SHA: "abc123",
	}); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if port.got.Content != "" {
		t.Errorf("delete carried a content field: %q", port.got.Content)
	}
}
