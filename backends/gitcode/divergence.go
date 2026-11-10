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
}

// Divergences returns the registered divergence ledger for the GitCode backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
