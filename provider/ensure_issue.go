package provider

import (
	"context"

	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/yi-nology/go-git-platform/model"
)

// EnsureIssueState 收敛 issue 状态（2B spec）：GET 现状，已达 want 零写入
// 返 Unchanged；否则 Close/Reopen 恰一次返 Updated。写撞 409 返
// (OutcomeConflict, 写前快照, err)，其余写错与 GET 错误一律 ("", nil, err)。
//
// 形态与既有 Ensure* 不同（取 Provider 而非 IssueManager）：IssueManager
// 不在 Provider 组成里，内部断言让调用方免二次断言，缺能力统一
// ErrNotImplemented。
func EnsureIssueState(ctx context.Context, p Provider, owner, repo, number string, want model.IssueState) (MutationOutcome, *model.Issue, error) {
	if want != model.IssueStateOpen && want != model.IssueStateClosed {
		return "", nil, oekiterrors.Wrap(ErrInvalidInput, "EnsureIssueState: want must be open or closed", "want", string(want))
	}
	im, ok := p.(IssueManager)
	if !ok {
		return "", nil, oekiterrors.Wrap(ErrNotImplemented, "EnsureIssueState: provider lacks IssueManager")
	}
	cur, err := im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		return "", nil, err
	}
	if cur.State == want {
		return OutcomeUnchanged, cur, nil
	}
	var out *model.Issue
	if want == model.IssueStateClosed {
		out, err = im.CloseIssue(ctx, owner, repo, number)
	} else {
		out, err = im.ReopenIssue(ctx, owner, repo, number)
	}
	if err != nil {
		if IsConflict(err) {
			return OutcomeConflict, cur, err
		}
		return "", nil, err
	}
	return OutcomeUpdated, out, nil
}

// EnsureIssueLabels 收敛 issue 标签（2B spec）：GET 现状 labels，集合相等
// （map 归一，顺序与重复无关）零写入返 Unchanged（返 current 原样）；否则
// 按差集写入——toAdd = want∖current（保序去重）批量 AddIssueLabels、
// toRemove = current∖want 批量 RemoveIssueLabels（各非空才调一次，
// #163 循环收口 SDK 单点）——返 Updated。写撞 409 返
// (OutcomeConflict, 写前快照, err)，其余写错与 GET 错误一律
// ("", nil, err)。
//
// 全集替换语义：want 即目标全集，current 中多余标签一律删除，want 缺失者
// 补齐。返回值为本地计算的收敛目标（want 去重保序），非再读回——并发窗口
// 内平台侧可能已再变动，以 err / 下次 GET 为准。
//
// 形态与既有 Ensure* 不同（取 Provider 而非 IssueManager）：IssueManager
// 不在 Provider 组成里，内部断言让调用方免二次断言，缺能力统一
// ErrNotImplemented。
func EnsureIssueLabels(ctx context.Context, p Provider, owner, repo, number string, want []string) (MutationOutcome, []string, error) {
	im, ok := p.(IssueManager)
	if !ok {
		return "", nil, oekiterrors.Wrap(ErrNotImplemented, "EnsureIssueLabels: provider lacks IssueManager")
	}
	cur, err := im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		return "", nil, err
	}
	current := cur.Labels
	if labelSetEqual(current, want) {
		return OutcomeUnchanged, current, nil
	}
	target := dedupLabels(want)
	toAdd, toRemove := labelDiff(current, target)
	if len(toAdd) > 0 {
		if err := im.AddIssueLabels(ctx, owner, repo, number, toAdd); err != nil {
			if IsConflict(err) {
				return OutcomeConflict, current, err
			}
			return "", nil, err
		}
	}
	if len(toRemove) > 0 {
		if err := im.RemoveIssueLabels(ctx, owner, repo, number, toRemove); err != nil {
			if IsConflict(err) {
				return OutcomeConflict, current, err
			}
			return "", nil, err
		}
	}
	return OutcomeUpdated, target, nil
}

// labelDiff 计算标签收敛差集：toAdd = target∖current、toRemove =
// current∖target（current 侧保序去重）。target 由调用方 dedupLabels 预先
// 归一——dedup 单点（EnsureIssueLabels 的收敛目标与差集共用同一份，
// 不重复计算）；从 EnsureIssueLabels 抽出（gocyclo），行为与原内联逐字
// 一致；集合相等由 labelSetEqual 先判，本 helper 只在需要写入时调用。
func labelDiff(current, target []string) (toAdd, toRemove []string) {
	curSet := make(map[string]struct{}, len(current))
	for _, l := range current {
		curSet[l] = struct{}{}
	}
	toAdd = make([]string, 0, len(target))
	for _, l := range target {
		if _, ok := curSet[l]; !ok {
			toAdd = append(toAdd, l)
		}
	}
	wantSet := make(map[string]struct{}, len(target))
	for _, l := range target {
		wantSet[l] = struct{}{}
	}
	curDedup := dedupLabels(current)
	toRemove = make([]string, 0, len(curDedup))
	for _, l := range curDedup {
		if _, ok := wantSet[l]; !ok {
			toRemove = append(toRemove, l)
		}
	}
	return toAdd, toRemove
}

// labelSetEqual 集合相等判定：顺序无关、两侧重复各自折叠（map 归一）。
func labelSetEqual(a, b []string) bool {
	set := make(map[string]struct{}, len(a))
	for _, l := range a {
		set[l] = struct{}{}
	}
	seen := make(map[string]struct{}, len(b))
	for _, l := range b {
		if _, ok := set[l]; !ok {
			return false
		}
		seen[l] = struct{}{}
	}
	return len(seen) == len(set)
}

// dedupLabels 保序去重（首见保留），产出可作收敛目标的稳定切片。
func dedupLabels(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, l := range in {
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
}
