// Package contracttest provides a reusable test harness for verifying that
// platform backends satisfy the behavioral contracts defined by the
// provider.Provider interface.
//
// Each backend's test suite imports this package and calls Run with a
// backend-specific Harness. This ensures that list/pagination, error
// classification, retry behavior, webhook validation, and context cancellation
// are consistent across every platform the SDK supports.
package contracttest

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oekit/pkg/webhook"

	"github.com/yi-nology/go-git-platform/provider"
)

// Harness bundles the inputs needed to run the contract suite against a
// backend.
type Harness struct {
	// Name is the human-readable platform identifier (e.g. "GitHub").
	Name string
	// Platform is the provider.Platform constant for this backend.
	Platform provider.Platform
	// NewProvider builds a provider.Provider from the given config. The harness
	// fills in BaseURL (and, for the retry subtest, RetryConfig) before calling,
	// so the function should forward any supplied RetryConfig/Hooks rather than
	// discarding them.
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	// EmptyListResponse is the JSON body the mock returns for empty lists.
	EmptyListResponse string
	// NonEmptyListResponse is the JSON body the mock returns for a non-empty
	// list, with at least one item that maps to a valid repo.
	NonEmptyListResponse string
	// Labels, when non-nil, auto-mounts the label-management suite inside
	// Run. Run enforces both directions: a platform declaring
	// Capabilities().Labels must provide this config, and a config must not
	// be provided by a platform that does not declare the capability.
	Labels *LabelsHarnessConfig
	// Issues, when non-nil, auto-mounts the issue-management suite inside
	// Run, with the same bidirectional drift checks as Labels.
	Issues *IssuesHarnessConfig
	// Reviews, when non-nil, auto-mounts the review-management suite inside
	// Run, with the same bidirectional drift checks as Labels and Issues.
	Reviews *ReviewsHarnessConfig
	// Milestones, when non-nil, auto-mounts the milestone-management suite
	// inside Run, with the same bidirectional drift checks as Labels,
	// Issues, and Reviews.
	Milestones *MilestonesHarnessConfig
	// Search, when non-nil, auto-mounts the search suite inside Run, with
	// the same bidirectional drift checks as Labels, Issues, Reviews, and
	// Milestones.
	Search *SearchHarnessConfig
	// CommitStatus, when non-nil, auto-mounts the commit-status suite inside
	// Run, with the same bidirectional drift checks as Labels, Issues,
	// Reviews, Milestones, and Search.
	CommitStatus *CommitStatusHarnessConfig
	// Notifications, when non-nil, auto-mounts the notification suite inside
	// Run, with the same bidirectional drift checks.
	Notifications *NotificationsHarnessConfig
	// Reactions, when non-nil, auto-mounts the reaction suite inside
	// Run, with the same bidirectional drift checks.
	Reactions *ReactionsHarnessConfig
	// BranchProtections, when non-nil, auto-mounts the branch-protection
	// suite inside Run, with the same bidirectional drift checks.
	BranchProtections *BranchProtectionsHarnessConfig
	// Collaborators, when non-nil, auto-mounts the collaborator suite.
	Collaborators *CollaboratorsHarnessConfig
	// WebhookConfig, when non-nil, auto-mounts the single-webhook
	// get/update suite (WebhookConfigManager — all seven backends).
	WebhookConfig *WebhookConfigHarnessConfig
	// Notes, when non-nil, auto-mounts the CR-note (merge-request note)
	// read/edit suite — GitLab-family platforms where MR notes live in a
	// separate iid namespace (GitHub-family: covered by IssueManager, no
	// config expected).
	Notes *NotesHarnessConfig
	// DeployKeys, when non-nil, auto-mounts the deploy-key suite.
	DeployKeys *DeployKeysHarnessConfig
	// RepoStats, when non-nil, auto-mounts the repo-stats suite.
	RepoStats *RepoStatsHarnessConfig
	// Users, when non-nil, auto-mounts the user-manager suite.
	Users *UsersHarnessConfig
	// CILogs, when non-nil, auto-mounts the CI failure-diagnostics suite.
	CILogs *CILogsHarnessConfig
	// RepoTemplates, when non-nil, auto-mounts the template-repo suite.
	RepoTemplates *RepoTemplatesHarnessConfig
	// Releases auto-mounts the release-management suite inside Run. Unlike
	// the fields above, ReleaseManager is a core interface composed into
	// provider.Provider (every backend implements it), so there is no
	// capability-declaration drift to check — the config is mandatory and
	// the suite fails the run when it is missing.
	Releases *ReleasesHarnessConfig
	// CRs auto-mounts the change-request suite inside Run (issue #63).
	// ChangeRequestManager is a core interface: the config is mandatory and
	// the suite fails the run when it is missing.
	CRs *CRHarnessConfig
	// Branches / Commits / Diffs / Files auto-mount the remaining core
	// manager suites (issue #63). Configs are mandatory; individual fixture
	// fields may be empty (their subtest then skips with a named message).
	Branches *BranchHarnessConfig
	Commits  *CommitHarnessConfig
	Diffs    *DiffHarnessConfig
	Files    *FileHarnessConfig
}

// Run executes the full contract suite against h. Each subtest is independent.
func Run(t *testing.T, h Harness) {
	t.Run("Platform", func(t *testing.T) { testPlatform(t, h) })
	t.Run("ListRepos_Empty", func(t *testing.T) { testListReposEmpty(t, h) })
	t.Run("ListRepos_NonEmpty", func(t *testing.T) { testListReposNonEmpty(t, h) })
	t.Run("IsNotFound", func(t *testing.T) { testIsNotFound(t, h) })
	t.Run("Pagination_Normalized", func(t *testing.T) { testPagination(t, h) })
	t.Run("Retry_On5xx", func(t *testing.T) { testRetry(t, h) })
	t.Run("TokenSource_Rotation", func(t *testing.T) { testTokenSourceRotation(t, h) })
	t.Run("ConditionalRequests", func(t *testing.T) { testConditionalRequests(t, h) })
	t.Run("Webhook_ValidateSignature", func(t *testing.T) { testWebhookSignature(t, h) })
	t.Run("Context_Cancel", func(t *testing.T) { testContextCancel(t, h) })
	t.Run("Capabilities_Consistency", func(t *testing.T) { testCapabilities(t, h) })
	t.Run("DivergenceSuite", func(t *testing.T) { testDivergenceSuite(t, h) })
	t.Run("LabelsSuite", func(t *testing.T) { testLabelsSuite(t, h) })
	t.Run("IssuesSuite", func(t *testing.T) { testIssuesSuite(t, h) })
	t.Run("ReviewsSuite", func(t *testing.T) { testReviewsSuite(t, h) })
	t.Run("MilestonesSuite", func(t *testing.T) { testMilestonesSuite(t, h) })
	t.Run("SearchSuite", func(t *testing.T) { testSearchSuite(t, h) })
	t.Run("ReleaseSuite", func(t *testing.T) { testReleaseSuite(t, h) })
	t.Run("CRSuite", func(t *testing.T) { testCRSuite(t, h) })
	t.Run("BranchSuite", func(t *testing.T) { testBranchSuite(t, h) })
	t.Run("CommitSuite", func(t *testing.T) { testCommitSuite(t, h) })
	t.Run("DiffSuite", func(t *testing.T) { testDiffSuite(t, h) })
	t.Run("FileSuite", func(t *testing.T) { testFileSuite(t, h) })
	t.Run("CommitStatusSuite", func(t *testing.T) { testCommitStatusSuite(t, h) })
	t.Run("NotificationsSuite", func(t *testing.T) { testNotificationsSuite(t, h) })
	t.Run("ReactionsSuite", func(t *testing.T) { testReactionsSuite(t, h) })
	t.Run("BranchProtectionsSuite", func(t *testing.T) { testBranchProtectionsSuite(t, h) })
	t.Run("CollaboratorsSuite", func(t *testing.T) { testCollaboratorsSuite(t, h) })
	t.Run("WebhookConfigSuite", func(t *testing.T) { testWebhookConfigSuite(t, h) })
	t.Run("NotesSuite", func(t *testing.T) { testNotesSuite(t, h) })
	t.Run("DeployKeysSuite", func(t *testing.T) { testDeployKeysSuite(t, h) })
	t.Run("RepoStatsSuite", func(t *testing.T) { testRepoStatsSuite(t, h) })
	t.Run("UsersSuite", func(t *testing.T) { testUsersSuite(t, h) })
	t.Run("CILogsSuite", func(t *testing.T) { testCILogsSuite(t, h) })
	t.Run("RepoTemplatesSuite", func(t *testing.T) { testRepoTemplatesSuite(t, h) })
}

func baseCfg(h Harness, baseURL string) provider.Config {
	return provider.Config{Platform: h.Platform, BaseURL: baseURL, Token: "test"}
}

func testPlatform(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	if p.Platform() != h.Platform {
		t.Errorf("expected %s, got %s", h.Platform, p.Platform())
	}
}

func testListReposEmpty(t *testing.T, h Harness) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.EmptyListResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func testListReposNonEmpty(t *testing.T, h Harness) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.NonEmptyListResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) == 0 {
		t.Error("expected at least 1 repo")
	}
	if repos[0].Platform != h.Platform {
		t.Errorf("expected platform %s, got %s", h.Platform, repos[0].Platform)
	}
}

func testIsNotFound(t *testing.T, h Harness) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	_, err := p.GetRepo(context.Background(), "missing", "repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if !provider.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func testPagination(t *testing.T, h Harness) {
	// page=0 / perPage=0 should be normalized to defaults, and perPage > 100
	// should be capped. We don't assert exact query encoding (each SDK differs)
	// but verify the call doesn't panic and returns without error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.EmptyListResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	if _, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 0, PerPage: 0}); err != nil {
		t.Errorf("ListRepos with zero page/perPage: %v", err)
	}
	if _, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 1000}); err != nil {
		t.Errorf("ListRepos with huge perPage: %v", err)
	}
}

// testRetry verifies that, with a retry config supplied, a transient 503
// followed by 200 is retried to success. It also asserts the server observed
// more than one attempt, proving the backend actually wired retry through.
func testRetry(t *testing.T, h Harness) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.NonEmptyListResponse))
	}))
	defer srv.Close()

	cfg := baseCfg(h, srv.URL)
	cfg.RetryConfig = &provider.RetryConfig{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	p := h.NewProvider(t, cfg)

	repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("expected retry to recover from 503, got error: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("expected non-empty result after retry")
	}
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Errorf("expected at least 2 HTTP attempts (proving retry), got %d", got)
	}
}

// testWebhookSignature exercises the platform's registered webhook validator:
// a correctly signed request must pass, an empty secret must be rejected
// (forging with an empty HMAC key must not pass either), and a tampered body
// must fail.
func testWebhookSignature(t *testing.T, h Harness) {
	v := provider.DefaultWebhookRegistry().Get(h.Platform)
	if v == nil {
		t.Skipf("no webhook validator registered for %s", h.Platform)
	}
	const secret = "webhook-secret"
	body := []byte(`{"event":"push"}`)

	// Correct signature must validate.
	good := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	signRequest(good, v, body, secret)
	if err := v.Validate(good, body, secret); err != nil {
		t.Errorf("valid signature rejected for %s: %v", h.Platform, err)
	}

	// Empty secret must be rejected (guards signature forgery with an empty
	// HMAC key on predictable payloads).
	emptyReq := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	signRequest(emptyReq, v, body, "") // header present, but secret empty
	if err := v.Validate(emptyReq, body, ""); err == nil {
		t.Errorf("expected empty secret to be rejected for %s", h.Platform)
	}

	// Tampered body must fail validation — but only for validators that sign
	// the body (HMAC). Static-token validators only compare a header token, so
	// altering the body is not expected to change the result.
	if _, isHMAC := v.(provider.HMACSHA256Validator); isHMAC {
		tampered := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
		signRequest(tampered, v, body, secret)
		if err := v.Validate(tampered, append(body, '!'), secret); err == nil {
			t.Errorf("expected tampered body to fail validation for %s", h.Platform)
		}
	}
}

// signRequest sets the validator's signature/token header on req for the given
// body and secret, so the validator can be exercised end to end. It understands
// the two validator types the SDK ships (HMAC-SHA256 and static-token).
//
// HMAC 签名收编 webhook.SignHMAC（pkg#325/#328，issue #137）：发端与验签器
// 家族共享同一 HMACOption 语义面（前缀/编码同源），删除四块手算 HMAC——
// 发↔收单源，漂移面消除。唯一保留的手算分支是**空密钥负对照**：上游
// SignHMAC 对空 secret fail-closed 拒签（构造期 panic）,而「攻击者用空
// 密钥伪造可预测载荷的签名」恰恰要求算出签名再被验签器拒绝——这层测试
// 意图只能手算表达。
func signRequest(req *http.Request, v provider.WebhookValidator, body []byte, secret string) {
	switch vv := v.(type) {
	case provider.HMACSHA256Validator:
		req.Header.Set(vv.Header, signBody(body, secret, "sha256="))
	case provider.StaticTokenValidator:
		req.Header.Set(vv.Header, secret)
	case provider.ForgejoWebhookValidator:
		req.Header.Set("X-Forgejo-Signature", signBody(body, secret, "sha256="))
	case provider.GiteeWebhookValidator:
		// Gitee backend scheme: HMAC-SHA256 hex over the body in
		// X-Gitee-Token (no timestamp). The registry also accepts the
		// ts-based sign mode, but the corpus pins the backend's scheme.
		req.Header.Set("X-Gitee-Token", signBody(body, secret, ""))
	case provider.GitCodeWebhookValidator:
		req.Header.Set("X-GitCode-Signature", signBody(body, secret, ""))
	}
}

// signBody produces the platform's HMAC-SHA256 signature header value via
// webhook.SignHMAC: prefix 非空走 sha256= 前缀形,空串走裸 hex 形（与验证器
// 家族同一选项接线）。secret 为空时上游 fail-closed 拒签 ⇒ 空密钥负对照
// 夹具在此保留最小手算（见 signRequest 注释）。
func signBody(body []byte, secret, prefix string) string {
	if secret == "" {
		mac := hmac.New(sha256.New, nil)
		_, _ = mac.Write(body)
		return prefix + hex.EncodeToString(mac.Sum(nil))
	}
	opts := []webhook.HMACOption(nil)
	if prefix != "" {
		opts = []webhook.HMACOption{webhook.WithSignaturePrefix(prefix)}
	}
	return webhook.SignHMAC([]byte(secret), crypto.SHA256, body, opts...)
}

// ParseSigned signs req with the platform's registered webhook validator and
// secret, then parses the event through p.ParseWebhookEvent. Backend unit
// tests use it to exercise payload mapping without hand-rolling per-platform
// signature code — and since v0.77.0 an empty secret is rejected by every
// validator, so unsigned fixture requests no longer pass validation.
func ParseSigned(t testing.TB, p provider.Provider, req *http.Request, secret string) *provider.NormalizedEvent {
	t.Helper()
	v := provider.DefaultWebhookRegistry().Get(p.Platform())
	if v == nil {
		t.Fatalf("no webhook validator registered for %s", p.Platform())
	}
	body, err := provider.ReadAndRestoreBody(req)
	if err != nil {
		t.Fatalf("read webhook body: %v", err)
	}
	signRequest(req, v, body, secret)
	if err := v.Validate(req, body, secret); err != nil {
		t.Fatalf("sign-and-validate webhook for %s: %v", p.Platform(), err)
	}
	ne, err := p.ParseWebhookEvent(req, secret)
	if err != nil {
		t.Fatalf("ParseWebhookEvent for %s: %v", p.Platform(), err)
	}
	return ne
}

func testContextCancel(t *testing.T, h Harness) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := p.ListRepos(ctx, provider.ListRepoOptions{})
	if err == nil {
		// Returned before timeout; acceptable in a race. Only fail on no error
		// AND the context not being done.
		if ctx.Err() == nil {
			t.Error("expected an error or a cancelled context")
		}
	}
}

// stubHandler returns a handler that serves empty responses for every path.
func stubHandler(h Harness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.EmptyListResponse))
	}
}

// beyondFirstPage reports whether the request explicitly asks for a page
// after the first. Backends whose list methods have no provider-side paging
// exhaust the endpoint's pagination (backendutil.AllPages): they probe page
// 2 after a non-empty page 1, and mocks use this to serve an empty terminal
// page so the loop stops instead of grinding to the 50-page safety cap.
// Requests without a page parameter (SDKs that omit paging when unset)
// always count as first-page.
func beyondFirstPage(r *http.Request) bool {
	page := r.URL.Query().Get("page")
	return page != "" && page != "1"
}

// --- 2A 三态 wire 断言辅助（ClearExpect 期望驱动，issues/CR 两侧共用）---

// assertBodyLacks 断言自 after 起记录的每条请求 body 都不含任一 key——
// unset（全零 options）面：写请求一旦携带字段就是「空值也下发」的违约；
// 短路到 GET 的平台（gitea/forgejo 的 issues 全空短路）GET 无 body，
// 天然通过——只要求「产生写请求时 body 干净」。
func assertBodyLacks(t *testing.T, requests *[]recordedRequest, after int, keys ...string) {
	t.Helper()
	for i := after; i < len(*requests); i++ {
		r := (*requests)[i]
		for _, k := range keys {
			if v, ok := r.Body[k]; ok {
				t.Errorf("unset: %s %s body carries %q=%v — 全零 options 不得携带字段", r.Method, r.Path, k, v)
			}
		}
	}
}

// assertBodyHasEmpty 断言自 after 起第一条携带 field 的请求其值为空形态
// （null / "" / 空数组 / 空对象 / 0）；一条都不携带即失败——缺席正是三态
// 规格要消灭的静默 no-op。after 之前的置值请求不参与（其非空值不会污染
// 清空判定）。
func assertBodyHasEmpty(t *testing.T, requests *[]recordedRequest, after int, field string) {
	t.Helper()
	for i := after; i < len(*requests); i++ {
		r := (*requests)[i]
		v, ok := r.Body[field]
		if !ok {
			continue
		}
		if !isEmptyBodyValue(v) {
			t.Errorf("%s %s: %s = %#v, want empty — 清空必须落到 wire", r.Method, r.Path, field, v)
		}
		return
	}
	t.Errorf("index %d 之后没有任何请求携带 %q — 清空静默 no-op；requests=%v", after, field, (*requests)[after:])
}

// assertBodyHasNonEmpty 断言自 after 起第一条携带 field 的请求其值非空——
// 置值回归的跨平台形态：GitLab/工蜂 把用户名解析成数字 ID 上线
// （assignee_ids），用户名字面值不可跨平台比对，「键在场且非空」即置值
// 已落线（缺席 = 静默丢弃，正是本断言要钉的）。
func assertBodyHasNonEmpty(t *testing.T, requests *[]recordedRequest, after int, field string) {
	t.Helper()
	for i := after; i < len(*requests); i++ {
		r := (*requests)[i]
		v, ok := r.Body[field]
		if !ok {
			continue
		}
		if isEmptyBodyValue(v) {
			t.Errorf("%s %s: %s = %#v, want non-empty — 置值必须落到 wire", r.Method, r.Path, field, v)
		}
		return
	}
	t.Errorf("index %d 之后没有任何请求携带 %q — 置值被静默丢弃；requests=%v", after, field, (*requests)[after:])
}

// isEmptyBodyValue 判定解码后的 body 值是否为空形态：JSON null、""、
// 空数组、空对象或数值 0——各平台「清空」的 wire 词表全覆盖
// （""/[]/{}/null/0）。
func isEmptyBodyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case float64:
		return x == 0
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
}

// clearKey 取某字段清空/置值断言的 wire 键：平台在 ClearExpect.BodyField
// 声明的词表优先；未声明（或 B 级条目不带 BodyField）回落 fallback。
func clearKey(clears map[string]ClearExpect, field, fallback string) string {
	if ce, ok := clears[field]; ok && ce.BodyField != "" {
		return ce.BodyField
	}
	return fallback
}

// unsetKeys 汇总 unset 断言要排查的键：canonical 键 ∪ 各平台声明的 wire
// BodyField（词表差异：GitLab 的 description/assignee_ids 等），去重后
// 返回（GitHub 这类 canonical 与 BodyField 同名的平台不重复报错）。
func unsetKeys(clears map[string]ClearExpect, canonical ...string) []string {
	seen := make(map[string]bool, len(canonical)*2)
	keys := make([]string, 0, len(canonical)*2)
	add := func(k string) {
		if k == "" || seen[k] {
			return
		}
		seen[k] = true
		keys = append(keys, k)
	}
	for _, k := range canonical {
		add(k)
	}
	for _, k := range canonical {
		if ce, ok := clears[k]; ok {
			add(ce.BodyField)
		}
	}
	return keys
}

// dispatchClear 执行一次清空调用并按 ClearExpect 分派断言——issues 与 CR
// 两侧的同构单源（避免两份分派逻辑漂移）：
//
//   - Unsupported=true（B 级）：必须返回 ErrClearNotSupported 且零新请求；
//   - 否则：请求必须发出（静默 no-op 是三态要修的缺陷）、Method（"" =
//     PATCH/PUT 不约束）与 PathContains 命中、BodyField 在场为空值。
//
// before 是本次调用前的请求条数，用于圈定本次调用产生的请求。
func dispatchClear(t *testing.T, clears map[string]ClearExpect, field string, requests *[]recordedRequest, before int, do func() error) {
	t.Helper()
	want, ok := clears[field]
	if !ok {
		want = ClearExpect{} // 未声明 = 全 A 级：方法不约束、body 不断言
	}
	err := do()
	if want.Unsupported {
		if !errors.Is(err, provider.ErrClearNotSupported) {
			t.Errorf("%s clear: want ErrClearNotSupported, got err=%v", field, err)
		}
		if len(*requests) != before {
			t.Errorf("%s clear: B 级必须零请求，多出 %d 条", field, len(*requests)-before)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s clear: %v", field, err)
	}
	if len(*requests) == before {
		t.Fatalf("%s clear: 没有发出请求（静默 no-op 是三态要修的缺陷）", field)
	}
	last := (*requests)[len(*requests)-1]
	if want.Method != "" && last.Method != want.Method {
		t.Errorf("%s clear: method = %s, want %s", field, last.Method, want.Method)
	}
	if want.PathContains != "" && !strings.Contains(last.Path, want.PathContains) {
		t.Errorf("%s clear: path %q missing %q", field, last.Path, want.PathContains)
	}
	if want.BodyField != "" {
		assertBodyHasEmpty(t, requests, before, want.BodyField)
	}
}

// VersionProxy returns a test server that responds to /api/v1/version with
// versionBody and reverse-proxies every other path to baseURL. Gitea/Forgejo
// SDKs require the version endpoint at client init; this wrapper lets the
// contract suite target those backends with a plain mock server.
func VersionProxy(baseURL, versionBody string) *httptest.Server {
	target, _ := url.Parse(baseURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(versionBody))
	})
	mux.HandleFunc("/", proxy.ServeHTTP)
	return httptest.NewServer(mux)
}
