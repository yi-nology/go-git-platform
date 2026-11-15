package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
)

// fakeIssueMgr 嵌入 nil Provider + IssueManager：只有 EnsureIssue* 会碰的
// 五个方法（GetIssue/CloseIssue/ReopenIssue/AddIssueLabels/RemoveIssueLabel）
// 是真实现并记录调用，其余方法在这些测试里不会被调用。
type fakeIssueMgr struct {
	Provider
	IssueManager

	getCalls        int
	getErr          error
	cur             *model.Issue
	writeErr        error // 非 nil 时写方法返回它
	closeCalls      int
	reopenCalls     int
	closed          *model.Issue // CloseIssue 返回体
	reopened        *model.Issue // ReopenIssue 返回体
	addCalls        [][]string
	removed         []string
	bulkRemoved     []string
	bulkRemoveCalls int
}

func (f *fakeIssueMgr) GetIssue(context.Context, string, string, string) (*model.Issue, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.cur, nil
}

func (f *fakeIssueMgr) CloseIssue(context.Context, string, string, string) (*model.Issue, error) {
	f.closeCalls++
	if f.writeErr != nil {
		return nil, f.writeErr
	}
	return f.closed, nil
}

func (f *fakeIssueMgr) ReopenIssue(context.Context, string, string, string) (*model.Issue, error) {
	f.reopenCalls++
	if f.writeErr != nil {
		return nil, f.writeErr
	}
	return f.reopened, nil
}

func (f *fakeIssueMgr) AddIssueLabels(_ context.Context, _, _, _ string, labels []string) error {
	f.addCalls = append(f.addCalls, slices.Clone(labels))
	return f.writeErr
}

func (f *fakeIssueMgr) RemoveIssueLabel(_ context.Context, _, _, _, name string) error {
	f.removed = append(f.removed, name)
	return f.writeErr
}

// RemoveIssueLabels 记录批量参数与调用次数（#163：Ensure 必须走批量形、
// 恰一次），内容进 bulkRemoved 供 wantRemove 断言。
func (f *fakeIssueMgr) RemoveIssueLabels(_ context.Context, _, _, _ string, names []string) error {
	f.bulkRemoved = append(f.bulkRemoved, names...)
	f.bulkRemoveCalls++
	return f.writeErr
}

var (
	_ Provider     = (*fakeIssueMgr)(nil)
	_ IssueManager = (*fakeIssueMgr)(nil)
)

// TestEnsureIssueState 用例表（2B T2 brief Step 1）：1 已达 want 零写 /
// 2 不符恰一写 / 3 非法 want 零 HTTP / 4 缺能力零 HTTP，另附 409 冲突、
// 非冲突写错与 GET 失败三条 outcome 契约钉子。
func TestEnsureIssueState(t *testing.T) {
	curOpen := &model.Issue{Number: "7", State: model.IssueStateOpen}
	outClosed := &model.Issue{Number: "7", State: model.IssueStateClosed}
	getBoom := errors.New("get boom")
	writeBoom := errors.New("write boom")

	tests := []struct {
		name        string
		p           Provider      // 非 nil 时替代 fake（缺能力桩）
		fake        *fakeIssueMgr // p 为 nil 时使用
		want        model.IssueState
		wantOutcome MutationOutcome
		wantIssue   *model.Issue // 指针身份比对
		wantErr     error        // nil = 期望成功；非 nil 走 errors.Is
		getCalls    int
		closeCalls  int
		reopenCalls int
	}{
		{
			name:        "1_已达want_Unchanged零写",
			fake:        &fakeIssueMgr{cur: curOpen},
			want:        model.IssueStateOpen,
			wantOutcome: OutcomeUnchanged,
			wantIssue:   curOpen,
			getCalls:    1,
		},
		{
			name:        "2_不符_Close恰一调用_Updated返返回体",
			fake:        &fakeIssueMgr{cur: curOpen, closed: outClosed},
			want:        model.IssueStateClosed,
			wantOutcome: OutcomeUpdated,
			wantIssue:   outClosed,
			getCalls:    1,
			closeCalls:  1,
		},
		{
			name:    "3_want非openclosed_InvalidInput零HTTP",
			fake:    &fakeIssueMgr{cur: curOpen},
			want:    model.IssueState("merged"),
			wantErr: ErrInvalidInput,
		},
		{
			name:    "4_缺IssueManager能力_NotImplemented零HTTP",
			p:       struct{ Provider }{},
			want:    model.IssueStateClosed,
			wantErr: ErrNotImplemented,
		},
		{
			name:        "写409_Conflict带写前快照",
			fake:        &fakeIssueMgr{cur: curOpen, writeErr: fmt.Errorf("close: %w", ErrConflict)},
			want:        model.IssueStateClosed,
			wantOutcome: OutcomeConflict,
			wantIssue:   curOpen,
			wantErr:     ErrConflict,
			getCalls:    1,
			closeCalls:  1,
		},
		{
			name:       "非409写错_零outcome",
			fake:       &fakeIssueMgr{cur: curOpen, writeErr: writeBoom},
			want:       model.IssueStateClosed,
			wantErr:    writeBoom,
			getCalls:   1,
			closeCalls: 1,
		},
		{
			name:     "GET失败_错误透传零outcome",
			fake:     &fakeIssueMgr{getErr: getBoom},
			want:     model.IssueStateClosed,
			wantErr:  getBoom,
			getCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p
			f := tt.fake
			if p == nil {
				p = f
			}
			outcome, issue, err := EnsureIssueState(context.Background(), p, "o", "r", "7", tt.want)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is(%v)", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("outcome = %q, want %q", outcome, tt.wantOutcome)
			}
			if issue != tt.wantIssue {
				t.Errorf("issue = %+v, want %+v（指针身份）", issue, tt.wantIssue)
			}
			if f == nil {
				return // 缺能力桩：无 manager 可调，零 HTTP 由错误断言保证
			}
			if f.getCalls != tt.getCalls {
				t.Errorf("GetIssue calls = %d, want %d", f.getCalls, tt.getCalls)
			}
			if f.closeCalls != tt.closeCalls {
				t.Errorf("CloseIssue calls = %d, want %d", f.closeCalls, tt.closeCalls)
			}
			if f.reopenCalls != tt.reopenCalls {
				t.Errorf("ReopenIssue calls = %d, want %d", f.reopenCalls, tt.reopenCalls)
			}
		})
	}
}

// TestEnsureIssueLabels 用例表（2B T2 brief Step 1）：5 集合相等顺序无关
// 零写 / 6 want⊃current 批量 Add 一次 / 7 want⊂current 逐个 Remove /
// 8 want=[] 全删返空 / 9 GET 失败透传，另附 409 冲突与 want 重复去重两条
// 契约钉子。
func TestEnsureIssueLabels(t *testing.T) {
	getBoom := errors.New("get boom")

	tests := []struct {
		name        string
		p           Provider
		fake        *fakeIssueMgr
		want        []string
		wantOutcome MutationOutcome
		wantLabels  []string // 期望返回值（slices.Equal）
		wantErr     error
		getCalls    int
		wantAdd     [][]string // 期望的 AddIssueLabels 批次（次数+参数）
		wantRemove  []string   // 期望的 RemoveIssueLabels 批量参数（恰一次调用）
	}{
		{
			name:        "5_集合相等顺序无关_Unchanged零写返现状原样",
			fake:        &fakeIssueMgr{cur: &model.Issue{Number: "7", Labels: []string{"b", "a"}}},
			want:        []string{"a", "b"},
			wantOutcome: OutcomeUnchanged,
			wantLabels:  []string{"b", "a"},
			getCalls:    1,
		},
		{
			name:        "6_want超集_批量Add恰一次差集_Updated",
			fake:        &fakeIssueMgr{cur: &model.Issue{Number: "7", Labels: []string{"a"}}},
			want:        []string{"b", "a", "c"},
			wantOutcome: OutcomeUpdated,
			wantLabels:  []string{"b", "a", "c"},
			getCalls:    1,
			wantAdd:     [][]string{{"b", "c"}},
		},
		{
			name:        "7_want子集_批量Remove差集恰一次_Updated",
			fake:        &fakeIssueMgr{cur: &model.Issue{Number: "7", Labels: []string{"a", "b", "c"}}},
			want:        []string{"a"},
			wantOutcome: OutcomeUpdated,
			wantLabels:  []string{"a"},
			getCalls:    1,
			wantRemove:  []string{"b", "c"},
		},
		{
			name:        "8_want空_current非空_批量Remove恰一次_Updated返空",
			fake:        &fakeIssueMgr{cur: &model.Issue{Number: "7", Labels: []string{"x", "y"}}},
			want:        []string{},
			wantOutcome: OutcomeUpdated,
			wantLabels:  []string{},
			getCalls:    1,
			wantRemove:  []string{"x", "y"},
		},
		{
			name: "Remove写409_Conflict带写前快照（批量形）",
			fake: &fakeIssueMgr{
				cur:      &model.Issue{Number: "7", Labels: []string{"a", "b"}},
				writeErr: fmt.Errorf("remove: %w", ErrConflict),
			},
			want:        []string{"a"},
			wantOutcome: OutcomeConflict,
			wantLabels:  []string{"a", "b"},
			wantErr:     ErrConflict,
			getCalls:    1,
			wantRemove:  []string{"b"},
		},
		{
			name:     "9_GET失败_错误透传零outcome",
			fake:     &fakeIssueMgr{getErr: getBoom},
			want:     []string{"a"},
			wantErr:  getBoom,
			getCalls: 1,
		},
		{
			name: "Add写409_Conflict带写前快照",
			fake: &fakeIssueMgr{
				cur:      &model.Issue{Number: "7", Labels: []string{"a"}},
				writeErr: fmt.Errorf("add: %w", ErrConflict),
			},
			want:        []string{"b"},
			wantOutcome: OutcomeConflict,
			wantLabels:  []string{"a"},
			wantErr:     ErrConflict,
			getCalls:    1,
			wantAdd:     [][]string{{"b"}},
		},
		{
			name:        "want含重复_toAdd与目标去重保序",
			fake:        &fakeIssueMgr{cur: &model.Issue{Number: "7", Labels: []string{"a"}}},
			want:        []string{"b", "b", "a", "c"},
			wantOutcome: OutcomeUpdated,
			wantLabels:  []string{"b", "a", "c"},
			getCalls:    1,
			wantAdd:     [][]string{{"b", "c"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p
			f := tt.fake
			if p == nil {
				p = f
			}
			outcome, labels, err := EnsureIssueLabels(context.Background(), p, "o", "r", "7", tt.want)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is(%v)", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("outcome = %q, want %q", outcome, tt.wantOutcome)
			}
			if !slices.Equal(labels, tt.wantLabels) {
				t.Errorf("labels = %v, want %v", labels, tt.wantLabels)
			}
			if f == nil {
				return
			}
			if f.getCalls != tt.getCalls {
				t.Errorf("GetIssue calls = %d, want %d", f.getCalls, tt.getCalls)
			}
			if len(f.addCalls) != len(tt.wantAdd) {
				t.Fatalf("AddIssueLabels calls = %v, want %v", f.addCalls, tt.wantAdd)
			}
			for i := range tt.wantAdd {
				if !slices.Equal(f.addCalls[i], tt.wantAdd[i]) {
					t.Errorf("AddIssueLabels[%d] = %v, want %v", i, f.addCalls[i], tt.wantAdd[i])
				}
			}
			if !slices.Equal(f.bulkRemoved, tt.wantRemove) {
				t.Errorf("RemoveIssueLabels args = %v, want %v", f.bulkRemoved, tt.wantRemove)
			}
			wantBulkCalls := 0
			if len(tt.wantRemove) > 0 {
				wantBulkCalls = 1
			}
			if f.bulkRemoveCalls != wantBulkCalls {
				t.Errorf("RemoveIssueLabels calls = %d, want %d（批量形恰一次）", f.bulkRemoveCalls, wantBulkCalls)
			}
			if len(f.removed) != 0 {
				t.Errorf("singular RemoveIssueLabel must not be called by Ensure, got %v", f.removed)
			}
		})
	}
}
