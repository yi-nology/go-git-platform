package gitcode

import "github.com/yi-nology/go-git-platform/provider"

// divergences is the GitCode divergence ledger: the registered places where
// this backend's behavior departs from the unified provider semantics.
// See provider.Divergence and docs/divergence-ledger.md.
//
// go-gitcode v0.8.0 aligned the SDK with the documented v5 wire (milestone
// due_on omitempty, webhook password + per-family event flags, pagination
// on the issue-comment/label/hook lists), which retired the former
// raw-transport detours (milestone create/update, issue comment/label
// lists) and the "webhook events not clearable" gap; the entries below are
// what remains.
var divergences = []provider.Divergence{
	{Capability: "LabelManager", Method: "CreateLabel", Field: "opts.Description", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's label API has no description field."},
	{Capability: "LabelManager", Method: "UpdateLabel", Field: "opts.Description", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's label API has no description field."},
	{Capability: "IssueManager", Method: "ListIssues", Field: "opts.Sort, opts.Direction", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's issue-list endpoint takes no sort/direction; the filters are accepted but ignored (incremental backups must fall back themselves)."},
	{Capability: "CommitManager", Method: "ListCommits", Field: "opts.Since, opts.Until (valid values)", Kind: provider.DivergenceIgnore,
		Reason: "The gitcode SDK's commit-list options carry no time filters; values are validated against RFC3339 (issue #52 — unparsable input fails the call with ErrInvalidInput) but valid filters cannot be applied — incremental syncs must filter client-side."},
	{Capability: "ReleaseManager", Method: "GetArchive", Kind: provider.DivergenceStub,
		Reason: "GitCode serves no archive endpoint — /archive, /archive/{ref.ext}, /archive/status all 404 live (probe round 2026-11-03); the method fails with ErrNotImplemented and touches no wire."},
	{Capability: "NotificationManager", Method: "ListNotifications", Kind: provider.DivergenceStub,
		Reason: "GitCode serves no global notification inbox — /notifications 404s live (probe round 2026-11-03); only the repo-scoped endpoints exist, so the method fails with ErrNotImplemented."},
	{Capability: "NotificationManager", Method: "MarkNotificationRead", Kind: provider.DivergenceStub,
		Reason: "The thread-read route hangs off the global inbox GitCode does not serve (live 404, probe round 2026-11-03); the method fails with ErrNotImplemented."},
	{Capability: "NotificationManager", Method: "MarkNotificationsRead", Kind: provider.DivergenceStub,
		Reason: "The global mark-all route 404s live (probe round 2026-11-03); the method fails with ErrNotImplemented."},
	{Capability: "RepoManager", Method: "ForkRepo", Field: "opts.Organization", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's fork endpoint (POST /repos/{owner}/{repo}/forks) takes no organization: forks always land under the authenticated user's namespace; a non-empty opts.Organization is accepted and dropped (opts.Name is honored as the fork's new name)."},
	{Capability: "DiffManager", Method: "CreateDiscussion", Field: "opts.BaseSHA, opts.StartSHA, opts.StartNewLine", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's inline-comment wire takes path/line/side/commit_id only: FilePath/NewLine/OldLine/HeadSHA anchor the comment, but the base/start SHA pair and multi-line ranges have no fields and are dropped."},
	{Capability: "ChangeRequestManager", Method: "UpdateCR", Field: "opts.Assignees", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's pull-update wire has no assignee field, and its /pulls/{n}/assignees routes manage reviewers (Assign/UnassignPullRequestReviewers), not assignees; a non-empty opts.Assignees is accepted and dropped."},
	{Capability: "CILogManager", Method: "ListFailedJobLogs", Field: "", Kind: provider.DivergenceMapping,
		Reason: "The v8 wire has no head_sha filter on the runs list; the SDK's FindActionRunByHeadSHA scans newest-first and returns only the single newest run matching the SHA, so failed jobs in other workflows' runs for the same commit are not attributed (GitHub/Gitea collect the latest run per workflow). Job status vocabulary is asserted case-insensitively (SDK fixtures show UPPERCASE COMPLETED/RUNNING; the failure word is inferred as FAILED); live validation of the vocabulary and the download-log route is pending."},
	// 2A 三态 B 级（T6）：四处 patch 清空/置空在任何 HTTP 之前返回
	// provider.ErrClearNotSupported（整调用失败、零请求）——wire 结构
	// 不可表达且无清空侧端点。Kind 取 stub（能力缺席：错误 + 不碰
	// wire）而非 ignore（其 doc 是「调用成功且静默丢弃」，与哨兵失败
	// 形态相反）；field 级条目在 contracttest/divergence.go 的行为检查
	// 中按 Field != "" 豁免，ErrNotImplemented 口径由 Reason 点名
	// ErrClearNotSupported 覆盖。legacy 字段路径不受影响（旧字段
	// 置值照常上线、CR 指派人维持既有 ignore 的静默丢弃）。
	{Capability: "IssueManager", Method: "UpdateIssue", Field: "opts.BodyPatch", Kind: provider.DivergenceStub,
		Reason: "GitCode 的 issue-update wire 是 string+omitempty，空串不可表达且无 body 侧端点——patch 清空返回 provider.ErrClearNotSupported（非静默丢弃）。"},
	{Capability: "IssueManager", Method: "UpdateIssue", Field: "opts.MilestonePatch", Kind: provider.DivergenceStub,
		Reason: "GitCode 无 milestone detach 端点（int64+omitempty 丢 0）——patch 清空返回 provider.ErrClearNotSupported。"},
	{Capability: "ChangeRequestManager", Method: "UpdateCR", Field: "opts.DescriptionPatch", Kind: provider.DivergenceStub,
		Reason: "同 body 面：string+omitempty 丢空串——patch 清空返回 ErrClearNotSupported。"},
	{Capability: "ChangeRequestManager", Method: "UpdateCR", Field: "opts.AssigneesPatch", Kind: provider.DivergenceStub,
		Reason: "pull-update wire 无 assignee 字段（既有 ignore 针对旧字段静默丢弃）——patch 任意指定均返回 ErrClearNotSupported。"},
}

// Divergences returns the registered divergence ledger for the GitCode backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
