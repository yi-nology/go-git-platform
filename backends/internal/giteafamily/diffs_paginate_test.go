package giteafamily

import (
	"context"
	"fmt"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// fakeDiffPort 记录 ListPullRequestFiles 的每次调用（page/size），按
// page→预置批次回放（issue #44：>PageSize 的 PR 必须翻页取全）。
type fakeDiffPort struct {
	pages map[int][]*FamilyChangedFile
	got   []ListOptions
}

func (f *fakeDiffPort) GetPullRequestDiff(context.Context, string, string, int64) ([]byte, error) {
	return nil, nil
}

func (f *fakeDiffPort) ListPullRequestFiles(_ context.Context, _, _ string, _ int64, opt ListPullRequestFilesOptions) ([]*FamilyChangedFile, error) {
	f.got = append(f.got, opt.ListOptions)
	return f.pages[opt.ListOptions.Page], nil
}

func (f *fakeDiffPort) CreateIssueComment(context.Context, string, string, int64, CreateIssueCommentOption) (*FamilyComment, int, error) {
	return nil, 0, nil
}

func (f *fakeDiffPort) DeleteIssueComment(context.Context, string, string, int64) (int, error) {
	return 0, nil
}

// TestGetCRFilesWalksAllPages：page=1 满 PageSize + page=2 短页 ⇒ 全量
// 聚合且逐页推进；此前固定 Page:1 只取一页（issue #44 静默截断）。
func TestGetCRFilesWalksAllPages(t *testing.T) {
	const pageSize = 50
	fake := &fakeDiffPort{pages: map[int][]*FamilyChangedFile{}}
	for i := 0; i < pageSize; i++ {
		fake.pages[1] = append(fake.pages[1], &FamilyChangedFile{Filename: fmt.Sprintf("p1-%d", i), Additions: 1})
	}
	for i := 0; i < 5; i++ {
		fake.pages[2] = append(fake.pages[2], &FamilyChangedFile{Filename: fmt.Sprintf("p2-%d", i), Additions: 1})
	}

	fam := Family{Platform: provider.PlatformGitea, PageSize: pageSize}
	files, err := GetCRFiles(context.Background(), fam, fake, "o", "r", "7")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != pageSize+5 {
		t.Fatalf("files = %d, want %d（两页全量）", len(files), pageSize+5)
	}
	// 行走器以「空页即尾」收口（pageiter.EachBounded）：短页后多一次空页
	// 探测是既定约定，不是 bug。
	if len(fake.got) != 3 {
		t.Fatalf("应发 3 次列表请求（含末尾空页探测）, got %d", len(fake.got))
	}
	if fake.got[0].Page != 1 || fake.got[1].Page != 2 || fake.got[2].Page != 3 {
		t.Fatalf("页序错: %v", fake.got)
	}
	if fake.got[0].PageSize != pageSize {
		t.Fatalf("per page: got %d, want %d", fake.got[0].PageSize, pageSize)
	}
}
