package gitlab

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListReviews implements provider.ReviewManager.
//
// Registered mapping (divergence ledger): GitLab has no per-review list — approvals
// are a single state on the merge request. ListReviews maps to
// MergeRequestApprovalsService.GetApprovalState(pid, mrIID) and synthesizes
// one summary review per approver: Review{ID: MR IID, User: approver
// username, State: approved}, taken from rules[].approved_by. There is no
// per-approval ID on the wire, so every synthesized review shares the MR IID
// as its ID, and an approver listed under several rules yields one review.
func (p *Provider) ListReviews(ctx context.Context, owner, repo, number string) ([]model.Review, error) {
	iid, err := backendutil.ParsePRNumber64(model.PlatformGitLab, "ListReviews", number)
	if err != nil {
		return nil, err
	}
	state, _, err := p.client.MergeRequestApprovals.GetApprovalState(pidOf(owner, repo), iid, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListReviews", err)
	}
	return approvalStateReviews(state, iid), nil
}

// GetReview implements provider.ReviewManager.
//
// Registered mapping (divergence ledger): the same GetApprovalState call as
// ListReviews. GitLab approvals expose no per-review IDs, so reviewID cannot
// be matched; this returns the first synthesized approver review as an
// approximation. When nobody has approved yet the call reports NotFound.
func (p *Provider) GetReview(ctx context.Context, owner, repo, number string, reviewID int64) (*model.Review, error) {
	iid, err := backendutil.ParsePRNumber64(model.PlatformGitLab, "GetReview", number)
	if err != nil {
		return nil, err
	}
	state, _, err := p.client.MergeRequestApprovals.GetApprovalState(pidOf(owner, repo), iid, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "GetReview", err)
	}
	reviews := approvalStateReviews(state, iid)
	if len(reviews) == 0 {
		return nil, provider.New(model.PlatformGitLab, "GetReview", http.StatusNotFound, "no approvals")
	}
	return &reviews[0], nil
}

// CreateReview implements provider.ReviewManager.
//
// Registered mapping (divergence ledger): GitLab has no native review object, so a
// review is expressed as a comment-style review — a merge-request note via
// Notes.CreateMergeRequestNote(pid, iid, {Body}), the same shape the
// pre-ReviewManager DiffManager.CreateReview used for its summary. Inline
// comments (opts.Comments) and verdicts (opts.Event, opts.CommitID) are not
// mapped: a note is neither an approval nor a commit report, so the created
// review is always in the commented state.
//
// v0.65.0 行内评论：opts.Comments 非空时改走 discussions 路径——此前实现丢弃
// Comments 只发纯文本 note，调用方（argus poster）的行内评论链路在 GitLab
// 后端从未生效。position 用 MR diff_refs 三 SHA + new_path/new_line（模型给
// 的是新文件行号）；单条 discussion 失败（行号不在 diff hunk 内等 422）按条
// 跳过不整单失败；总结 note 照发保留评审事件语义。
func (p *Provider) CreateReview(ctx context.Context, owner, repo, number string, opts provider.CreateReviewOptions) (*model.ReviewResult, error) {
	iid, err := backendutil.ParsePRNumber64(model.PlatformGitLab, "CreateReview", number)
	if err != nil {
		return nil, err
	}
	if len(opts.Comments) > 0 {
		if res, err := p.createReviewWithComments(ctx, owner, repo, iid, opts); err == nil {
			return res, nil
		}
		// 行内路径失败（拉不到 diff_refs 等）→ 降级纯评论 note，评审事件不丢
	}
	return p.createNoteReview(ctx, owner, repo, iid, opts)
}

// createReviewWithComments 逐条建 position discussion + 总结 note。
func (p *Provider) createReviewWithComments(ctx context.Context, owner, repo string, iid int64, opts provider.CreateReviewOptions) (*model.ReviewResult, error) {
	mr, _, err := p.client.MergeRequests.GetMergeRequest(pidOf(owner, repo), iid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CreateReview", err)
	}
	refs := mr.DiffRefs
	if refs.HeadSha == "" || refs.BaseSha == "" {
		p.logger.Warn("gitlab.CreateReview: diff_refs 缺失，行内评论降级纯评论",
			"base", refs.BaseSha, "head", refs.HeadSha, "start", refs.StartSha)
		return nil, provider.Wrap(model.PlatformGitLab, "CreateReview",
			errors.New("mr diff_refs 缺失，无法定位行内评论"))
	}
	p.logger.Info("gitlab.CreateReview: inline comments path", "comments", len(opts.Comments))
	// diff 感知定位（v0.66.0）：模型行号来自 get_file 全文（文件级行号），而
	// GitLab position 只接受 hunk 内的行——只传 new_line 遇上下文行/非 hunk 行
	// 会 400 "must be a valid line code"（v0.65 实测）。拉一次 diffs 构建
	// per-file 的 hunk 行集与 new→old 映射：
	//   行 ∈ hunk → 精确行级 position（new_line + 推导的 old_line）；
	//   文件在 diff 但行 ∉ hunk → file 级 position（评论仍挂在 diff 视图该文件下）；
	//   文件不在 diff → 跳过（调用方 poster 的文件级预校验已拦，双保险）。
	hunks := p.mrDiffHunks(ctx, owner, repo, iid)
	posType := "text"
	fileType := "file"
	for _, c := range opts.Comments {
		if c.Path == "" || c.Line <= 0 || c.Body == "" {
			continue
		}
		path, line, body := c.Path, int64(c.Line), c.Body
		position := &gitlab.PositionOptions{
			BaseSHA:      new(refs.BaseSha),
			HeadSHA:      new(refs.HeadSha),
			StartSHA:     new(refs.StartSha),
			PositionType: new(posType),
			NewPath:      new(path),
			OldPath:      new(path),
		}
		if h, ok := hunks[path]; ok {
			if old, in := h.newToOld[int(line)]; in {
				// hunk 内：行级 position。上下文行必须带 old_line（服务端据此
				// 生成 line_code）；纯新增行 old_line 置 0 不传。
				position.NewLine = new(line)
				if old > 0 {
					position.OldLine = new(int64(old))
				}
			} else {
				// 行不在 hunk：file 级 position（不 400，评论挂文件下）
				position.PositionType = new(fileType)
				position.NewLine = nil
				p.logger.Info("gitlab.CreateReview: line not in hunk, file-level position",
					"path", c.Path, "line", c.Line)
			}
		} else {
			// 文件不在 diff：跳过（错误行/未变更文件——不可定位也不该挂）
			p.logger.Warn("gitlab.CreateReview: file not in diff, skipped",
				"path", c.Path, "line", c.Line)
			continue
		}
		opt := &gitlab.CreateMergeRequestDiscussionOptions{
			Body:     new(body),
			Position: position,
		}
		// 单条失败 best-effort 跳过：行号不在 hunk / 文件二进制等，不该拖垮整轮评审；
		// 但必须留痕——否则调用方行内全丢还以为发出去了（v0.65.1 实测教训）
		if _, _, err := p.client.Discussions.CreateMergeRequestDiscussion(pidOf(owner, repo), iid, opt, gitlab.WithContext(ctx)); err != nil {
			p.logger.Warn("gitlab.CreateReview: inline discussion skipped",
				"path", c.Path, "line", c.Line, "error", err.Error())
		}
	}
	return p.createNoteReview(ctx, owner, repo, iid, opts)
}

// mrDiffHunk 单文件 diff 的行定位信息：hunk 内 new 行号 → old 行号映射
// （上下文行 old/new 同步推进；纯 + 行 old=0；纯 - 行不入映射）。
type mrDiffHunk struct {
	newToOld map[int]int
}

// mrDiffHunks 拉 MR 全部文件 diff 并解析 hunk 行映射（一次调用，供本任务
// 全部行内评论共用）。解析失败返回空 map——调用方对未知文件走 skip 分支。
func (p *Provider) mrDiffHunks(ctx context.Context, owner, repo string, iid int64) map[string]mrDiffHunk {
	out := map[string]mrDiffHunk{}
	diffs, _, err := p.client.MergeRequests.ListMergeRequestDiffs(pidOf(owner, repo), iid,
		&gitlab.ListMergeRequestDiffsOptions{ListOptions: gitlab.ListOptions{PerPage: 100}}, gitlab.WithContext(ctx))
	if err != nil {
		p.logger.Warn("gitlab.CreateReview: 拉取 MR diffs 失败，行内定位退化为保守模式",
			"error", err.Error())
		return out
	}
	for _, d := range diffs {
		out[d.NewPath] = parseDiffHunk(d.Diff)
	}
	return out
}

// parseDiffHunk 单文件 unified diff 解析（纯函数，单测钉住）：hunk 内
// new 行号 → old 行号映射。上下文行同步推进；纯 + 行 old=0；纯 - 行不占
// new 号；"\" 行（no newline 标记）忽略；hunk 间计数重置。
func parseDiffHunk(diff string) mrDiffHunk {
	h := mrDiffHunk{newToOld: map[int]int{}}
	oldLn, newLn := 0, 0
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
				inHunk = true
				oldLn, _ = strconv.Atoi(m[1])
				newLn, _ = strconv.Atoi(m[2])
				// hunk 头行号是首行行号，先回退一格让后续逐行 ++ 从首行开始
				oldLn--
				newLn--
			} else {
				inHunk = false
			}
		case !inHunk:
			// 文件头（---/+++/index）忽略
		case strings.HasPrefix(line, "+"):
			newLn++
			h.newToOld[newLn] = 0
		case strings.HasPrefix(line, "-"):
			oldLn++
		case strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file" 不计数
		default: // 上下文行
			newLn++
			oldLn++
			h.newToOld[newLn] = oldLn
		}
	}
	return h
}

// createNoteReview 纯评论评审（无行内时的原路径）。
func (p *Provider) createNoteReview(ctx context.Context, owner, repo string, iid int64, opts provider.CreateReviewOptions) (*model.ReviewResult, error) {
	note, _, err := p.client.Notes.CreateMergeRequestNote(pidOf(owner, repo), iid,
		&gitlab.CreateMergeRequestNoteOptions{Body: new(opts.Body)}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CreateReview", err)
	}
	return convertNoteReviewResult(note), nil
}

// RequestReviewers implements provider.ReviewManager. Reviewer usernames
// are resolved to user IDs via the Users API (cached) and written through
// UpdateMergeRequest's reviewer_ids.
func (p *Provider) RequestReviewers(ctx context.Context, owner, repo, number string, reviewers []string) error {
	iid, err := backendutil.ParsePRNumber64(model.PlatformGitLab, "RequestReviewers", number)
	if err != nil {
		return err
	}
	if len(reviewers) == 0 {
		return nil
	}
	ids, err := p.resolveUserIDs(ctx, "RequestReviewers", reviewers)
	if err != nil {
		return err
	}
	if _, _, err := p.client.MergeRequests.UpdateMergeRequest(pidOf(owner, repo), iid,
		&gitlab.UpdateMergeRequestOptions{ReviewerIDs: &ids}, gitlab.WithContext(ctx)); err != nil {
		return provider.Wrap(model.PlatformGitLab, "RequestReviewers", err)
	}
	return nil
}

// DismissReview implements provider.ReviewManager.
//
// Registered mapping (divergence ledger): UnapproveMergeRequest(pid, mrIID). GitLab
// approvals hang off the merge request as a whole (per user), not off
// individual review objects, so reviewID is not addressable and the
// dismissal message has no GitLab equivalent (ignored).
func (p *Provider) DismissReview(ctx context.Context, owner, repo, number string, reviewID int64, message string) error {
	iid, err := backendutil.ParsePRNumber64(model.PlatformGitLab, "DismissReview", number)
	if err != nil {
		return err
	}
	if _, err := p.client.MergeRequestApprovals.UnapproveMergeRequest(pidOf(owner, repo), iid, gitlab.WithContext(ctx)); err != nil {
		return provider.Wrap(model.PlatformGitLab, "DismissReview", err)
	}
	return nil
}

// approvalStateReviews synthesizes the summary reviews of a GitLab approval
// state: one approved review per distinct approver listed in
// rules[].approved_by, each keyed by the MR IID because GitLab approvals
// carry no per-approval IDs.
func approvalStateReviews(state *gitlab.MergeRequestApprovalState, mrIID int64) []model.Review {
	reviews := make([]model.Review, 0)
	if state == nil {
		return reviews
	}
	seen := make(map[string]bool)
	for _, rule := range state.Rules {
		if rule == nil {
			continue
		}
		for _, approver := range rule.ApprovedBy {
			if approver == nil || approver.Username == "" || seen[approver.Username] {
				continue
			}
			seen[approver.Username] = true
			reviews = append(reviews, model.Review{
				ID:    mrIID,
				User:  approver.Username,
				State: model.ReviewStateApproved,
			})
		}
	}
	return reviews
}

// convertNoteReviewResult maps the merge-request note backing a comment-style
// review to a model.ReviewResult (ID = note ID).
func convertNoteReviewResult(note *gitlab.Note) *model.ReviewResult {
	if note == nil {
		return nil
	}
	return &model.ReviewResult{
		ID:   strconv.FormatInt(note.ID, 10),
		Body: note.Body,
		User: convertNoteAuthor(note.Author),
	}
}

var _ provider.ReviewManager = (*Provider)(nil)

// hunkHeaderRe unified diff hunk 头：@@ -old[,n] +new[,n] @@（后缀函数上下文忽略）。
var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
