package contracttest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/runtime"
)

// testFaultSuite 钉 spec §3.3 异常注入三形态（3期 T3）：5xx 故障响应体、
// 截断 JSON、伪翻页止损。capability 无关 ⇒ 七后端无条件跑（skip 只在
// IssueManager 缺席时，见 faultIssueManager）；每子测自带 httptest，
// 形态同 ErrorClassSuite。
//
// Step 1 现场核证（2026-11-16，七后端实测）：
//
//   - 5xx：classifyStatusCode（provider/errors.go，5xx → ErrServerSide）与
//     body 形态无关——空体同样命中，分类不依赖文本。但**平台错误详情会进
//     err 文本**：provider.Wrap 保留 cause 链（其 doc 明示 "the Error()
//     message retains the platform's original detail"），各 SDK 的 Error()
//     拼接其解析出的 message 字段 / 原始 body。⇒ spec §3.3 的「body 不进
//     错误文本」在 JSON message 形态下七后端一致不成立——红线内只报告 +
//     另立单，测试按现实钉（见 Fault_5xxBodyForms 内注释）。
//
//   - 截断 JSON：encoding/json 先整段语法校验（checkValid），未闭合 / 非法
//     UTF-8 尾 → "unexpected EOF" / "unexpected end of JSON input"；七后端
//     全部返回 err，无静默空集、无 panic（gitea/forgejo 额外把 200 状态贴
//     进错误链，仍非 nil）。
//
//   - 伪翻页：runtime/pageiter.go EachBounded —— 第 2 页与第 1 页逐值相同
//     （DeepEqual）→ return nil（伪翻页 = 列表已到尾，每页只计一次，**不是**
//     预算错误）；页页不同且永非空 → page > maxPages → ErrPageBudgetExceeded
//     （哨兵名现场核于 pageiter.go）。adapter 的 backendutil.AllPages 即委托
//     CollectBounded(maxAllPages=50)——本套件用 CollectBounded 直驱 ListIssues
//     做 wire 级钉。
func testFaultSuite(t *testing.T, h Harness) {
	t.Helper()
	t.Run("Fault_5xxBodyForms", func(t *testing.T) { testFault5xxBodyForms(t, h) })
	t.Run("TruncatedJSON", func(t *testing.T) { testTruncatedJSON(t, h) })
	t.Run("FakePagination_Stops", func(t *testing.T) { testFakePaginationStops(t, h) })
}

// faultIssueManager builds a provider against baseURL and asserts it exposes
// the seven-backend-universal read method (same skip guard as ErrorClassSuite).
func faultIssueManager(t *testing.T, h Harness, baseURL string) provider.IssueManager {
	t.Helper()
	p := h.NewProvider(t, baseCfg(h, baseURL))
	im, ok := p.(provider.IssueManager)
	if !ok {
		t.Skipf("%s lacks IssueManager — no wire method to drive", h.Name)
	}
	return im
}

// faultListIssues drives ListIssues with a caller-driven page (the caller-
// paged family: Page > 0 = exactly one request for that page).
func faultListIssues(ctx context.Context, im provider.IssueManager, page int) ([]*model.Issue, error) {
	issues, _, err := im.ListIssues(ctx, provider.ListIssuesOptions{
		Owner: "o", Repo: "r", Page: page, PerPage: 20,
	})
	return issues, err
}

// faultIssueList returns the harness's own issue-list wire fixture as a JSON
// array — every backend's ListResponse is guaranteed to parse on that backend
// (gitee numbers are strings, gitlab keys by iid; a hand-rolled body is not
// portable). The fault suite serves it verbatim (same page) or doubled (page
// alternation) as the fake-pagination stub body.
func faultIssueList(t *testing.T, h Harness) []json.RawMessage {
	t.Helper()
	if h.Issues == nil {
		t.Fatalf("%s: Harness.Issues not wired — the fault suite needs an issue-list fixture", h.Name)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(h.Issues.ListResponse), &arr); err != nil {
		t.Fatalf("%s: Issues.ListResponse is not a JSON array: %v", h.Name, err)
	}
	if len(arr) == 0 {
		t.Fatalf("%s: Issues.ListResponse is empty — need at least 1 issue", h.Name)
	}
	return arr
}

// testFault5xxBodyForms: 5xx + 三种 body 形态，逐形态驱动 ListIssues。
//
// 状态码用 501 而非 500：gitlab client-go 对 500-range（501 除外）内置
// retryablehttp 指数退避——ErrorClassSuite 的 500 子测单次实测 ~11.9s 即
// 此因；三形态 ×12s 白烧 36s。501 同属 5xx，classifyStatusCode 同判
// ErrServerSide，分类契约零差异（Retry_On5xx 已单独钉重试，本套件不管）。
//
// Canarity 裁定（Step 1 现场核证，spec §3.3 vs 现实）：
//   - json_object（{"message":"<canary>"}）：七后端一致 —— SDK 解析 message
//     字段并拼进 Error()（provider.Wrap doc 明示保留平台原始细节）⇒ canary
//     **在场**，断言钉现实。spec 期望的 absence 不成立 → 另立单裁定是否在
//     provider 层做错误文本脱敏；若脱敏落地，本断言翻转为 absence
//     （flip-together）。
//   - plain_text：跨后端发散 —— github/gitee JSON 解析失败即丢弃 body
//     （不在场）；gitlab/gitea/forgejo/gitcode/tencentcode 拼接原始 body
//     （在场）。无统一真相可钉 ⇒ 只断言分类，不作 canary 断言。
//   - empty：体为空，无可泄漏内容 ⇒ 只断言分类。
func testFault5xxBodyForms(t *testing.T, h Harness) {
	const canary = "FAULT5XX-CANARY-2f7c6fbc"
	forms := []struct {
		name        string
		body        string
		contentType string
		// checkCanary / wantCanaryPresent：仅在跨后端行为一致的形态上钉
		// canary（见函数文档）。plain_text 发散、empty 无内容 → 不检查。
		checkCanary       bool
		wantCanaryPresent bool
	}{
		{
			name: "json_object", body: `{"message":"` + canary + `"}`,
			contentType: "application/json", checkCanary: true, wantCanaryPresent: true,
		},
		{name: "plain_text", body: canary, contentType: "text/plain; charset=utf-8"},
		{name: "empty", body: "", contentType: "application/json"},
	}
	for _, f := range forms {
		t.Run(f.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", f.contentType)
				w.WriteHeader(http.StatusNotImplemented)
				if f.body != "" {
					_, _ = w.Write([]byte(f.body))
				}
			}))
			defer srv.Close()
			im := faultIssueManager(t, h, srv.URL)
			_, _, err := im.ListIssues(context.Background(), provider.ListIssuesOptions{Owner: "o", Repo: "r"})
			if err == nil {
				t.Fatalf("%s: 5xx must return an error, got nil", f.name)
			}
			// 分类只走哨兵（规则二）：与 body 文本无关——空体形态同样命中即证。
			if !errors.Is(err, provider.ErrServerSide) {
				t.Fatalf("%s: want errors.Is(ErrServerSide), got %v", f.name, err)
			}
			if f.checkCanary {
				// absence/presence 检查（金丝雀在场性），非消息匹配分类。
				if got := strings.Contains(err.Error(), canary); got != f.wantCanaryPresent {
					t.Errorf("%s: canary in err text = %v, want %v — 错误文本承载平台详情的现实变了，翻转本断言并同步 CHANGELOG/另立单",
						f.name, got, f.wantCanaryPresent)
				}
			}
		})
	}
}

// testTruncatedJSON: 200 + 腰斩 JSON，钉「返回 err 而非静默空集、绝不 panic」。
// 三种腰斩形态现场实测七后端全部返回 decode err（Step 1 记录见文件头），
// 故断言 err != nil；err == nil 且带数据/空数据都是危险形态（静默成功）。
func testTruncatedJSON(t *testing.T, h Harness) {
	full := `[{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},{"number":2,"title":"second","state":"open","user":{"login":"dev"},"html_url":"https://example.com/2","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`
	cases := []struct {
		name string
		body []byte
	}{
		// 半截数组：在第二个元素的时间戳值中间腰斩。
		{"truncated_array", []byte(full[:len(full)-40])},
		// 半截对象：body 根本不是数组，且自身未闭合。
		{"truncated_object", []byte(`{"number":1,"title":"bug","state":"ope`)},
		// 非法 UTF-8 尾：多字节字符（附 = E9 99 84）只写出首字节即被腰斩——
		// 真实连接中断发生在 rune 中间，尾部是非法 UTF-8 + 未闭合字符串。
		{"invalid_utf8_tail", append([]byte(`[{"number":1,"title":"`), 0xE9)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(c.body)
			}))
			defer srv.Close()
			im := faultIssueManager(t, h, srv.URL)
			issues, err := faultListIssues(context.Background(), im, 1)
			if err == nil {
				// 静默空集/静默成功是最危险形态：腰斩的 200 绝不能当成功。
				t.Fatalf("%s: truncated 200 body must return an error, got nil with %d issues", c.name, len(issues))
			}
		})
	}
}

// testFakePaginationStops: stub 忽略 page 参数（spec §3.3.3 的 Link next 指向
// 恒不推进的服务器），钉 pageiter 两条停止语义（#285 wire 级）：
//
//   - same_page_no_progress：每页恒返同一批数据 → DeepEqual 命中 → return nil
//     （列表已到尾），每页只计一次、不重复计入、第 2 页收手（预算 50 远未到）。
//   - endless_distinct_budget_error：页页不同且永非空 → 无进展检测不触发 →
//     预算耗尽返回 errors.Is(ErrPageBudgetExceeded)（哨兵现场核名）而非丢数据。
func testFakePaginationStops(t *testing.T, h Harness) {
	arr := faultIssueList(t, h)
	onePage := h.Issues.ListResponse

	t.Run("same_page_no_progress", func(t *testing.T) {
		var reqs int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&reqs, 1)
			w.Header().Set("Content-Type", "application/json")
			// 服务器宣称有下一页，但忽略 page 参数恒返同页——伪翻页 wire 形态。
			w.Header().Set("Link", `</api/v1/repos/o/r/issues?page=2>; rel="next"`)
			_, _ = w.Write([]byte(onePage))
		}))
		defer srv.Close()
		im := faultIssueManager(t, h, srv.URL)
		ctx := context.Background()

		// 基线：单页调用的条数（夹具必须非空，否则本套件无意义）。
		single, err := faultListIssues(ctx, im, 1)
		if err != nil {
			t.Fatalf("baseline single page: %v", err)
		}
		if len(single) == 0 {
			t.Fatalf("issue fixture must yield at least 1 issue, got 0")
		}

		before := atomic.LoadInt32(&reqs)
		items, err := runtime.CollectBounded(ctx, func(ctx context.Context, page int) ([]*model.Issue, error) {
			return faultListIssues(ctx, im, page)
		}, 50)
		walkReqs := atomic.LoadInt32(&reqs) - before
		if err != nil {
			// 无进展止损的现实语义是 nil error（pageiter.go：同页重复 = 已到尾）。
			t.Fatalf("same-page walk must stop via no-progress detection with nil error, got %v", err)
		}
		if len(items) != len(single) {
			t.Errorf("no-progress stop must count each issue exactly once: walk got %d, single page has %d", len(items), len(single))
		}
		if walkReqs != 2 {
			t.Errorf("no-progress detection must halt at page 2 (budget is 50): walk made %d requests", walkReqs)
		}
	})

	t.Run("endless_distinct_budget_error", func(t *testing.T) {
		var reqs int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&reqs, 1)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Link", `</api/v1/repos/o/r/issues?page=2>; rel="next"`)
			// 奇数页恒返原夹具、偶数页翻倍：每页与上一页逐值不同（长度即不同）
			// 且永非空 → 无进展检测不触发，只能靠预算收手。元素本身是后端原生
			// 夹具（gitee 的 string number / gitlab 的 iid 均原样保留），无需
			// 手写跨后端 wire 形状。
			page := r.URL.Query().Get("page")
			n, convErr := strconv.Atoi(page)
			body := onePage
			if convErr == nil && n%2 == 0 && n > 0 {
				doubled := make([]json.RawMessage, 0, len(arr)*2)
				doubled = append(doubled, arr...)
				doubled = append(doubled, arr...)
				marshaled, mErr := json.Marshal(doubled)
				if mErr != nil {
					t.Errorf("marshal doubled page: %v", mErr)
					return
				}
				body = string(marshaled)
			}
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()
		im := faultIssueManager(t, h, srv.URL)

		const budget = 3
		before := atomic.LoadInt32(&reqs)
		_, err := runtime.CollectBounded(context.Background(), func(ctx context.Context, page int) ([]*model.Issue, error) {
			return faultListIssues(ctx, im, page)
		}, budget)
		walkReqs := atomic.LoadInt32(&reqs) - before
		if !errors.Is(err, runtime.ErrPageBudgetExceeded) {
			t.Fatalf("endless distinct pages must exhaust the budget: want ErrPageBudgetExceeded, got %v", err)
		}
		if walkReqs != budget {
			t.Errorf("budget exhaustion must stop after exactly %d fetches, made %d", budget, walkReqs)
		}
	})
}
