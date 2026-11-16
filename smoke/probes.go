// Package smoke: 只读巡检 probe 注册表（spec 3期 §3.2）。
package smoke

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ProbeFunc 是一个只读探针家族；实现内禁止调用任何写方法
// （创建/更新/删除/标记）——lint 与评审共同把守（spec §2 红线）。
type ProbeFunc func(ctx context.Context, p provider.Provider, opts Options) error

// Options 沿用现有 smoke 入口的 owner/repo/target 参数；Token 与 Logf 是
// 3 期新增：errors 家族用 Token 构造「坏凭据」只读复核（401/403 分类哨兵，
// 真平台面），Logf 承接原入口的 t.Logf 观测行（nil = 静默，probe 对
// testing 零依赖）。Platform 一律取 p.Platform()，不进 Options。
type Options struct {
	Owner, Repo, URL string
	Token            string
	Logf             func(format string, args ...any)
}

// logf 是 nil 安全的日志出口。
func (o Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

// Probes 是六家族注册表——家族名即 workflow families input 的取值域。
// 只读红线（spec §2）：每个实现体内零写方法调用；新增家族必须同步
// smoke/probes_test.go 的 TestProbesRegistry_SixFamiliesReadOnly（长度
// 断言会红，强制有意识地登记）。
var Probes = map[string]ProbeFunc{
	"repo":       probeRepo,       // 现有 GetRepo+ListRepos 逻辑原样迁入
	"cr":         probeCR,         // GetCR+ListCRs 解析/状态词表/SourceBranch 过滤
	"webhook":    probeWebhook,    // ListWebhooks 解析（不创建不删除）
	"pagination": probePagination, // 同 endpoint 2 页 walk：页参生效、二页可空、无死循环
	"errors":     probeErrors,     // 不存在资源→ErrNotFound；无权→401/403 哨兵
	"connection": probeConnection, // TestConnection + 连通性
}

// ParseFamilies 解析 GIT_PLATFORM_SMOKE_FAMILIES：空→["repo"]（现状），
// "all"→全六，逗号分隔按序去空白。
func ParseFamilies(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{"repo"}
	}
	if strings.TrimSpace(s) == "all" {
		return []string{"repo", "cr", "webhook", "pagination", "errors", "connection"}
	}
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// probeRepo 是原 smoke_test.go 两探针的逐字迁移（行为回归锚）：
// 只读：GetRepo、ListRepos —— 零写方法（spec §2 红线）。
func probeRepo(ctx context.Context, p provider.Provider, opts Options) error {
	// 探针 1:GetRepo——单仓读路径、鉴权、错误分类全走一遍。
	got, err := p.GetRepo(ctx, opts.Owner, opts.Repo)
	if err != nil {
		return fmt.Errorf("GetRepo(%s/%s) against live %s: %w (wire drift? route/auth/vocabulary changes surface here first)", opts.Owner, opts.Repo, p.Platform(), err)
	}
	if got == nil || got.Name == "" {
		return fmt.Errorf("GetRepo returned an empty repo payload: %+v", got)
	}
	opts.logf("GetRepo ok: %s/%s (default branch %q)", got.Owner, got.Name, got.DefaultBranch)

	// 探针 2:ListRepos 一页——列表路径与分页词表。
	repos, err := p.ListRepos(ctx, provider.ListRepoOptions{Owner: opts.Owner, Page: 1, PerPage: 5})
	if err != nil {
		return fmt.Errorf("ListRepos(%s) against live %s: %w", opts.Owner, p.Platform(), err)
	}
	if len(repos) == 0 {
		return fmt.Errorf("ListRepos returned zero repos for %s — wire shape drift?", opts.Owner)
	}
	opts.logf("ListRepos ok: %d repo(s) on page 1", len(repos))
	return nil
}

// probeCR 只读：ListCRs、GetCR —— 零写方法（spec §2 红线）。
// 断言面（spec §3.2 cr 行）：状态词表 {opened,merged,closed}、分页字段
// （total 平台自报口径：nil=未知不猜、非 nil 必须非负）、SourceBranch
// 过滤口径（#78 纯分支名必须命中该 CR）。
func probeCR(ctx context.Context, p provider.Provider, opts Options) error {
	crs, total, err := p.ListCRs(ctx, provider.ListCROptions{
		Owner: opts.Owner, Repo: opts.Repo, Page: 1, PerPage: 5,
	})
	if err != nil {
		return fmt.Errorf("ListCRs(%s/%s) against live %s: %w", opts.Owner, opts.Repo, p.Platform(), err)
	}
	if total != nil && *total < 0 {
		return fmt.Errorf("ListCRs reported negative total %d — platform metadata drift?", *total)
	}
	// 页 1 条数不可能超过总数——自报 total 小于本页即分页字段漂移（spec §3.2 cr 行）。
	if total != nil && *total < len(crs) {
		return fmt.Errorf("ListCRs total %d smaller than page-1 count %d — pagination metadata drift", *total, len(crs))
	}
	opts.logf("ListCRs ok: %d CR(s) on page 1 (total=%s)", len(crs), totalStr(total))
	if err := checkCRVocab(crs); err != nil {
		return err
	}
	if len(crs) == 0 {
		opts.logf("no CRs on %s/%s — GetCR / SourceBranch legs skipped", opts.Owner, opts.Repo)
		return nil
	}

	// GetCR 回读：列表首条必须能按 Number 独立取回（读路径一致性）。
	first := crs[0]
	got, err := p.GetCR(ctx, opts.Owner, opts.Repo, first.Number)
	if err != nil {
		return fmt.Errorf("GetCR(#%s) against live %s: %w", first.Number, p.Platform(), err)
	}
	if got == nil || got.Number != first.Number {
		return fmt.Errorf("GetCR(#%s) returned a mismatching payload: %+v", first.Number, got)
	}
	return checkSourceBranchFilter(ctx, p, opts, first)
}

// checkCRVocab 断言 CR 解析面：状态词表与 Number 非空（spec §3.2 cr 行）。
func checkCRVocab(crs []*model.ChangeRequest) error {
	for _, cr := range crs {
		if cr == nil {
			return errors.New("ListCRs returned a nil CR entry")
		}
		if cr.Number == "" {
			return fmt.Errorf("ListCRs returned a CR with empty Number: %+v", cr)
		}
		switch cr.State {
		case model.CRStateOpened, model.CRStateMerged, model.CRStateClosed:
		default:
			return fmt.Errorf("CR #%s state %q outside unified vocabulary {opened,merged,closed}", cr.Number, cr.State)
		}
	}
	return nil
}

// checkSourceBranchFilter 复核 #78 口径：纯分支名过滤必须命中该 CR
// （fork 同名分支也命中——统一 SourceBranch 与命名空间无关）。
func checkSourceBranchFilter(ctx context.Context, p provider.Provider, opts Options, want *model.ChangeRequest) error {
	if want.SourceBranch == "" {
		opts.logf("CR #%s has no source branch — SourceBranch filter leg skipped", want.Number)
		return nil
	}
	filtered, _, err := p.ListCRs(ctx, provider.ListCROptions{
		Owner: opts.Owner, Repo: opts.Repo, SourceBranch: want.SourceBranch, Page: 1, PerPage: 20,
	})
	if err != nil {
		return fmt.Errorf("ListCRs(SourceBranch=%q) against live %s: %w", want.SourceBranch, p.Platform(), err)
	}
	for _, cr := range filtered {
		if cr != nil && cr.Number == want.Number {
			opts.logf("SourceBranch filter ok: %q matched %d CR(s)", want.SourceBranch, len(filtered))
			return nil
		}
	}
	return fmt.Errorf("CR #%s (source branch %q) absent from SourceBranch-filtered ListCRs (%d hit(s)) — #78 contract drift?", want.Number, want.SourceBranch, len(filtered))
}

// probeWebhook 只读：ListWebhooks —— 不创建不删除（spec §3.2 webhook 行）。
// 断言面：列表解析（ID/URL 非空），0 条也是合法结果（仓库可以没有 webhook）。
func probeWebhook(ctx context.Context, p provider.Provider, opts Options) error {
	hooks, err := p.ListWebhooks(ctx, opts.Owner, opts.Repo)
	if err != nil {
		return fmt.Errorf("ListWebhooks(%s/%s) against live %s: %w", opts.Owner, opts.Repo, p.Platform(), err)
	}
	for _, h := range hooks {
		if h == nil {
			return errors.New("ListWebhooks returned a nil webhook entry")
		}
		if h.ID == 0 {
			return fmt.Errorf("webhook with zero ID (parse drift?): %+v", h)
		}
		if h.URL == "" {
			return fmt.Errorf("webhook #%d has empty URL (parse drift?): %+v", h.ID, h)
		}
	}
	opts.logf("ListWebhooks ok: %d webhook(s)", len(hooks))
	return nil
}

// probePagination 只读：ListRepos 连续两页 walk（spec §3.2 pagination 行）。
// caller-paged 族步数上界 = 2，结构上无死循环；断言页参生效（两页首条
// 必须不同——忽略 page 参数的伪翻页在此现形）、第二页可空（合法）。
func probePagination(ctx context.Context, p provider.Provider, opts Options) error {
	p1, err := p.ListRepos(ctx, provider.ListRepoOptions{Owner: opts.Owner, Page: 1, PerPage: 1})
	if err != nil {
		return fmt.Errorf("ListRepos page 1 for %s: %w", opts.Owner, err)
	}
	if len(p1) == 0 {
		return fmt.Errorf("ListRepos page 1 empty for %s — owner list drift? (page-2 walk not meaningful)", opts.Owner)
	}
	p2, err := p.ListRepos(ctx, provider.ListRepoOptions{Owner: opts.Owner, Page: 2, PerPage: 1})
	if err != nil {
		return fmt.Errorf("ListRepos page 2 for %s: %w", opts.Owner, err)
	}
	if len(p2) > 0 && p1[0].FullName == p2[0].FullName {
		return fmt.Errorf("page parameter ignored: page1[0] and page2[0] are both %q (%d vs %d) — pseudo-pagination", p1[0].FullName, p1[0].ID, p2[0].ID)
	}
	opts.logf("pagination ok: page1=%d repo(s), page2=%d repo(s) (empty page 2 is legal)", len(p1), len(p2))
	return nil
}

// probeErrors 只读：两次 GetRepo（不存在资源 + 坏凭据第二 provider）——
// 零写方法（spec §2 红线）。断言面（spec §3.2 errors 行）：不存在资源 →
// ErrNotFound；无权/私有 → 401/403 分类哨兵（一律 errors.Is 判定，
// 规则二禁止匹配错误消息文本）。
func probeErrors(ctx context.Context, p provider.Provider, opts Options) error {
	// 腿 1：不存在资源 → ErrNotFound（随机名保证不存在）。
	ghost, err := ghostRepoName()
	if err != nil {
		return fmt.Errorf("generate nonexistent repo name: %w", err)
	}
	_, err = p.GetRepo(ctx, opts.Owner, ghost)
	if err == nil {
		return fmt.Errorf("GetRepo(%s/%s) succeeded for a nonexistent repo — classification drift?", opts.Owner, ghost)
	}
	if !errors.Is(err, provider.ErrNotFound) {
		return fmt.Errorf("GetRepo(nonexistent %s/%s) classified %q, want errors.Is(ErrNotFound): %w", opts.Owner, ghost, err, err)
	}
	opts.logf("ErrNotFound ok: nonexistent %s/%s", opts.Owner, ghost)

	// 腿 2：坏凭据 → 401/403 分类哨兵（同 platform/URL、token 载坏的
	// 第二 provider；只发 GET，仍属只读巡检面）。
	if opts.Token == "" {
		opts.logf("bad-credential leg skipped: no token in Options")
		return nil
	}
	bad, err := provider.NewProvider(provider.Config{
		Platform: p.Platform(),
		BaseURL:  opts.URL,
		Token:    opts.Token + "-invalid",
	})
	if err != nil {
		return fmt.Errorf("new bad-credential provider for %s: %w", p.Platform(), err)
	}
	_, err = bad.GetRepo(ctx, opts.Owner, opts.Repo)
	if err == nil {
		return fmt.Errorf("bad-credential GetRepo(%s/%s) unexpectedly succeeded", opts.Owner, opts.Repo)
	}
	if !errors.Is(err, provider.ErrAuthentication) && !errors.Is(err, provider.ErrForbidden) {
		return fmt.Errorf("bad-credential GetRepo classified %q, want ErrAuthentication/ErrForbidden: %w", err, err)
	}
	opts.logf("401/403 ok: bad-credential GetRepo → %v", err)
	return nil
}

// ghostRepoName 生成一个随机的、保证不存在的仓库名（crypto/rand，gosec 面）。
func ghostRepoName() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "smoke-no-such-repo-" + hex.EncodeToString(b[:]), nil
}

// probeConnection 只读：TestConnection 两次（静态 token + TokenSource
// 通路）——零写方法（spec §2 红线）。断言面（spec §3.2 connection 行）：
// 连通性 Connected=true；TokenSource 轮转型凭据（GitHub App 安装令牌等）
// 走 Config.TokenSource 逐请求取 token 的通路冒烟。
// 局限说明：本 probe 用同一静态 token 接线 TokenSource 通路（StaticTokenSource），
// 只验「逐请求取 token」的通路本身，非真轮转（单一凭证无多 token 可转）——
// spec §3.2 connection 行「TokenSource 轮转冒烟」的最小合理实现，真轮转面
// 归消费方凭据实现（GitHub App 安装令牌等）自行保证。
func probeConnection(ctx context.Context, p provider.Provider, opts Options) error {
	res, err := p.TestConnection(ctx)
	if err != nil {
		return fmt.Errorf("TestConnection against live %s: %w", p.Platform(), err)
	}
	if res == nil || !res.Connected {
		return fmt.Errorf("TestConnection not connected: %+v", res)
	}
	opts.logf("TestConnection ok: platform=%s user=%q canListRepos=%v", res.Platform, res.UserName, res.CanListRepos)

	if opts.Token == "" {
		opts.logf("TokenSource leg skipped: no token in Options")
		return nil
	}
	ts, err := provider.NewProvider(provider.Config{
		Platform:    p.Platform(),
		BaseURL:     opts.URL,
		TokenSource: provider.StaticTokenSource(opts.Token),
	})
	if err != nil {
		return fmt.Errorf("new TokenSource provider for %s: %w", p.Platform(), err)
	}
	res2, err := ts.TestConnection(ctx)
	if err != nil {
		return fmt.Errorf("TestConnection via TokenSource against live %s: %w", p.Platform(), err)
	}
	if res2 == nil || !res2.Connected {
		return fmt.Errorf("TestConnection via TokenSource not connected: %+v", res2)
	}
	opts.logf("TokenSource path ok: user=%q (per-request token consultation — rotation surface)", res2.UserName)
	return nil
}

// totalStr 渲染 ListCRs 的 total：nil = 平台未给权威计数（未知即未知）。
func totalStr(total *int) string {
	if total == nil {
		return "unknown"
	}
	return strconv.Itoa(*total)
}
