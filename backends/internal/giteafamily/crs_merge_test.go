package giteafamily

import (
	"context"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

type fakePRPort struct {
	mergeStatus int
	mergeErr    error
	pr          *FamilyPullRequest
}

func (f *fakePRPort) CreatePullRequest(context.Context, string, string, FamilyCreatePullRequestOption) (*FamilyPullRequest, error) {
	return f.pr, nil
}
func (f *fakePRPort) GetPullRequest(context.Context, string, string, int64) (*FamilyPullRequest, error) {
	return f.pr, nil
}
func (f *fakePRPort) ListPullRequests(context.Context, string, string, FamilyListPullRequestsOptions) ([]*FamilyPullRequest, int, error) {
	return nil, 0, nil
}
func (f *fakePRPort) MergePullRequest(context.Context, string, string, int64, FamilyMergePullRequestOption) (int, error) {
	return f.mergeStatus, f.mergeErr
}
func (f *fakePRPort) EditPullRequest(context.Context, string, string, int64, FamilyEditPullRequestOption) (*FamilyPullRequest, error) {
	return f.pr, nil
}
func (f *fakePRPort) ListPullRequestCommits(context.Context, string, string, int64, ListOptions) ([]*Commit, error) {
	return nil, nil
}

func TestMergeCR_Non2xxWithoutErrorIsFailure(t *testing.T) {
	// go-gitea 对空 diff PR 的 405 不置 err——若只看 err 会把"未合并"当成功，
	// 上层状态机谎报 MERGED（实测 bug）。非 2xx 必须转错误。
	fam := &Family{Platform: provider.PlatformGitea}
	port := &fakePRPort{
		mergeStatus: 405,
		mergeErr:    nil,
		pr:          &FamilyPullRequest{Index: 1, State: "open"},
	}
	cr, err := MergeCR(context.Background(), *fam, port, "o", "r", "1", provider.MergeCROptions{})
	if err == nil {
		t.Fatalf("expected error for HTTP 405 merge, got success cr=%+v", cr)
	}
}

func TestMergeCR_405AlreadyMergedRecovers(t *testing.T) {
	fam := &Family{Platform: provider.PlatformGitea}
	port := &fakePRPort{
		mergeStatus: 405,
		mergeErr:    nil,
		pr:          &FamilyPullRequest{Index: 1, State: "closed", HasMerged: true},
	}
	cr, err := MergeCR(context.Background(), *fam, port, "o", "r", "1", provider.MergeCROptions{})
	if err != nil {
		t.Fatalf("405 on already-merged PR should recover, got err=%v", err)
	}
	if cr == nil {
		t.Fatal("expected CR on recovery")
	}
}

func TestMergeCR_2xxSucceeds(t *testing.T) {
	fam := &Family{Platform: provider.PlatformGitea}
	port := &fakePRPort{
		mergeStatus: 200,
		pr:          &FamilyPullRequest{Index: 1, State: "closed", HasMerged: true},
	}
	if _, err := MergeCR(context.Background(), *fam, port, "o", "r", "1", provider.MergeCROptions{}); err != nil {
		t.Fatalf("200 merge should succeed, got %v", err)
	}
}
