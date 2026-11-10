package github_test

// 批 2 回归 wire 测试(issue #71/#72/#73/#74/#75/#78/#79)。
// 每条用例都是「把修复拿掉就会变红」的形状:直接钉 wire 请求形态、
// 状态码门、错误分类与副作用范围。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	sdkgithub "github.com/google/go-github/v93/github"

	ghbackend "github.com/yi-nology/go-git-platform/backends/github"
	"github.com/yi-nology/go-git-platform/provider"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wireRecorder 记录 httptest 收到的每个请求(方法+路径+body)。
type wireRecorder struct {
	mu       sync.Mutex
	requests []recordedRequest
}

type recordedRequest struct {
	Method string
	Path   string
	Body   string
}

func (w *wireRecorder) record(r *http.Request, body string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.requests = append(w.requests, recordedRequest{Method: r.Method, Path: r.URL.Path, Body: body})
}

func (w *wireRecorder) all() []recordedRequest {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]recordedRequest(nil), w.requests...)
}

func readAllBody(t *testing.T, r *http.Request) string {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return string(raw)
}

// TestGetArchive_StatusGateAndCtx 归档下载跳的状态码门与 ctx 传导
// (issue #72):第二跳 500 的 HTML 错误页**不得**当归档字节返回;
// 已取消的 ctx 必须立刻报错。链路用 302(go-github 的 GetArchiveLink
// 只自动跟随 301;302 的 Location 由本后端二次跳转)。
func TestGetArchive_StatusGateAndCtx(t *testing.T) {
	var base string // 晚绑定:handler 运行时 NewServer 已返回
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/owner/repo/tarball/main", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"/codeload/bad.tar.gz", http.StatusFound)
	})
	mux.HandleFunc("/codeload/bad.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<html>oops</html>"))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()
	p := newTestProvider(t, srv.URL)

	t.Run("second_hop_500_is_error_not_bytes", func(t *testing.T) {
		data, err := p.GetArchive(context.Background(), "owner", "repo", "main", "tarball")
		require.Error(t, err, "错误页不得当成功归档返回")
		assert.Empty(t, data)
		var pe *provider.ProviderError
		require.ErrorAs(t, err, &pe)
		assert.Equal(t, 500, pe.StatusCode, "错误必须带上第二跳的状态码")
		assert.True(t, pe.IsServerError())
		assert.Contains(t, err.Error(), "oops", "错误信息保留平台原始细节")
	})

	t.Run("ctx_cancel_propagates", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := p.GetArchive(ctx, "owner", "repo", "main", "tarball")
		require.Error(t, err, "已取消的 ctx 必须传导为错误")
	})

	t.Run("success_returns_bytes", func(t *testing.T) {
		mux.HandleFunc("/codeload/ok.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("TAR-BYTES"))
		})
		mux.HandleFunc("/api/v3/repos/owner/repo2/tarball/main", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, base+"/codeload/ok.tar.gz", http.StatusFound)
		})
		data, err := p.GetArchive(context.Background(), "owner", "repo2", "main", "tarball")
		require.NoError(t, err)
		assert.Equal(t, "TAR-BYTES", string(data))
	})
}

// TestGetFileContent_RejectsErrorBodyAsContent raw 下载第二跳的非 2xx
// 错误响应体不得当文件内容返回(issue #71,go-github 契约明示的陷阱)。
func TestGetFileContent_RejectsErrorBodyAsContent(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	// 第一跳:contents 元数据(ref=main 正常、其余 404)。
	mux.HandleFunc("/api/v3/repos/owner/repo/contents/CHANGELOG.md", func(w http.ResponseWriter, r *http.Request) {
		if ref := r.URL.Query().Get("ref"); ref != "" && ref != "main" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(&sdkgithub.RepositoryContent{
			Type:        new("file"),
			DownloadURL: new(base + "/raw/CHANGELOG.md"),
		})
	})
	mux.HandleFunc("/raw/CHANGELOG.md", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("API rate limit exceeded"))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()
	p := newTestProvider(t, srv.URL)

	t.Run("raw_429_is_error_not_content", func(t *testing.T) {
		content, err := p.GetFileContent(context.Background(), "owner", "repo", "CHANGELOG.md", "main")
		require.Error(t, err, "错误页文本不得当文件内容返回")
		assert.Empty(t, content)
		var pe *provider.ProviderError
		require.ErrorAs(t, err, &pe)
		assert.Equal(t, http.StatusTooManyRequests, pe.StatusCode)
		assert.True(t, provider.IsRateLimited(err), "429 必须归类 ErrRateLimited")
	})

	t.Run("metadata_404_classifies_not_found", func(t *testing.T) {
		_, err := p.GetFileContent(context.Background(), "owner", "repo", "CHANGELOG.md", "other-branch")
		require.Error(t, err)
		assert.True(t, provider.IsNotFound(err), "元数据跳 404 必须归类 ErrNotFound")
	})

	t.Run("success_returns_content", func(t *testing.T) {
		mux.HandleFunc("/raw/ok.txt", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("hello world"))
		})
		mux.HandleFunc("/api/v3/repos/owner/repo/contents/ok.txt", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(&sdkgithub.RepositoryContent{
				Type:        new("file"),
				DownloadURL: new(base + "/raw/ok.txt"),
			})
		})
		content, err := p.GetFileContent(context.Background(), "owner", "repo", "ok.txt", "")
		require.NoError(t, err)
		assert.Equal(t, "hello world", content)
	})
}

// TestMarkNotificationsRead_InvalidLastReadAtRejected LastReadAt 非空但
// 解析失败必须报错且**不发任何请求**(issue #75):静默降级会把格式笔误
// 升级成「全部标已读」的不可逆全量副作用(go-github 零值 lastRead 语义)。
func TestMarkNotificationsRead_InvalidLastReadAtRejected(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL)

	for _, raw := range []string{"2026/11/11", "not-a-time", "2026-13-45T00:00:00Z"} {
		err := p.MarkNotificationsRead(context.Background(), provider.MarkNotificationsOptions{LastReadAt: raw})
		require.Error(t, err, "LastReadAt=%q 解析失败必须报错", raw)
		assert.True(t, provider.IsInvalidInput(err), "必须归类 ErrInvalidInput(实际=%v)", err)
		err = p.MarkRepoNotificationsRead(context.Background(), "owner", "repo", provider.MarkNotificationsOptions{LastReadAt: raw})
		require.Error(t, err, "LastReadAt=%q 解析失败必须报错(repo 形)", raw)
		assert.True(t, provider.IsInvalidInput(err))
	}
	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, requests, "解析失败绝不允许到达 wire(零值即 mark-all 的通道)")
}

// TestMarkNotificationsRead_ValidLastReadAtOnWire 合法 LastReadAt 照常落线,
// 空 LastReadAt 才是 mark-all 通道。
func TestMarkNotificationsRead_ValidLastReadAtOnWire(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = readAllBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL)
	require.NoError(t, p.MarkNotificationsRead(context.Background(),
		provider.MarkNotificationsOptions{LastReadAt: "2026-11-11T00:00:00Z"}))
	assert.Contains(t, got, "2026-11-11T00:00:00Z", "合法时间必须落线(实际 body=%q)", got)
}

// TestCreateBranchProtection_DirectPutNoGet create 路径必须直 PUT
// (issue #73):对未保护分支(GET 404)创建,旧实现委托 Update 的
// GET 前置必然 404 ⇒ create 死锁;EnsureCreated 分支不可达。
func TestCreateBranchProtection_DirectPutNoGet(t *testing.T) {
	rec := &wireRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/owner/repo/branches/feature/protection", func(w http.ResponseWriter, r *http.Request) {
		body := readAllBody(t, r)
		rec.record(r, body)
		if r.Method == http.MethodPut {
			_ = json.NewEncoder(w).Encode(putProtectionResponse())
		} else {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	p := newTestProvider(t, srv.URL)

	out, err := p.CreateBranchProtection(context.Background(), "owner", "repo",
		provider.CreateBranchProtectionOptions{
			BranchName:               "feature",
			RequiredApprovingReviews: 2,
			RequiredStatusChecks:     true,
			AllowForcePushes:         true,
		})
	require.NoError(t, err, "未保护分支上的 create 必须成功(GET 404 不得拦路)")
	require.NotNil(t, out)

	reqs := rec.all()
	require.Len(t, reqs, 1, "create 只应发一次 PUT,不发 GET")
	assert.Equal(t, http.MethodPut, reqs[0].Method)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(reqs[0].Body), &body))
	rpr, _ := body["required_pull_request_reviews"].(map[string]any)
	require.NotNil(t, rpr)
	assert.InEpsilon(t, 2, rpr["required_approving_review_count"], 0)
	sc, _ := body["required_status_checks"].(map[string]any)
	require.NotNil(t, sc, "RequiredStatusChecks=true 必须落 required_status_checks")
	assert.Equal(t, true, body["allow_force_pushes"])
}

// TestUpdateBranchProtection_NilFieldsLeftUnchanged 「nil 字段不变」逐字段
// 兑现(issue #74):只改评审数,其余字段必须从 current 回填——enforce_admins
// 不得硬编码 true、restrictions 不得清空、force-push 例外不得复位。
func TestUpdateBranchProtection_NilFieldsLeftUnchanged(t *testing.T) {
	rec := &wireRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/owner/repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		body := readAllBody(t, r)
		rec.record(r, body)
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(currentProtectionResponse())
			return
		}
		_ = json.NewEncoder(w).Encode(putProtectionResponse())
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	p := newTestProvider(t, srv.URL)

	two := 2
	_, err := p.UpdateBranchProtection(context.Background(), "owner", "repo", "main",
		provider.UpdateBranchProtectionOptions{RequiredApprovingReviews: &two})
	require.NoError(t, err)

	reqs := rec.all()
	require.Len(t, reqs, 2, "update = GET 回填 + PUT")
	put := reqs[1]
	require.Equal(t, http.MethodPut, put.Method)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(put.Body), &body))
	assert.Equal(t, false, body["enforce_admins"], "enforce_admins 必须透传 current(false),不得硬编码 true")
	rpr, _ := body["required_pull_request_reviews"].(map[string]any)
	require.NotNil(t, rpr)
	assert.InEpsilon(t, 2, rpr["required_approving_review_count"], 0, "caller 覆盖项要生效")
	assert.Equal(t, true, rpr["dismiss_stale_reviews"], "current 的其余 review 设置要保留")
	assert.Equal(t, true, body["allow_force_pushes"], "force-push 例外不得被全量替换复位")
	sc, _ := body["required_status_checks"].(map[string]any)
	require.NotNil(t, sc, "status checks 要从 current 保留")
	res, _ := body["restrictions"].(map[string]any)
	require.NotNil(t, res, "restrictions 必须从 current 回填,不得 wire 成 null")
	users, _ := res["users"].([]any)
	assert.Contains(t, users, "alice", "restrictions.users 要逐个保留")
	teams, _ := res["teams"].([]any)
	assert.Contains(t, teams, "release-eng", "restrictions.teams 要逐个保留")
	apps, _ := res["apps"].([]any)
	assert.Contains(t, apps, "ci-bot", "restrictions.apps 要逐个保留")
}

// TestUpdateBranchProtection_UnprotectedBranchIsNotFound 未保护分支上的
// update 返回 ErrNotFound(create 由 CreateBranchProtection/Ensure 承担)。
func TestUpdateBranchProtection_UnprotectedBranchIsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/owner/repo/branches/new/protection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(putProtectionResponse())
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	p := newTestProvider(t, srv.URL)
	_, err := p.UpdateBranchProtection(context.Background(), "owner", "repo", "new",
		provider.UpdateBranchProtectionOptions{})
	require.Error(t, err)
	assert.True(t, provider.IsNotFound(err), "update 对未保护分支必须归类 ErrNotFound(实际=%v)", err)
}

// TestListCRs_SourceBranchFiltering SourceBranch 双口径(issue #78):
// 纯分支名 ⇒ 不下发 head 过滤、客户端后过滤(fork PR 也命中);
// owner:branch ⇒ 服务端 head 精确过滤。
func TestListCRs_SourceBranchFiltering(t *testing.T) {
	forkPR := &sdkgithub.PullRequest{
		Number: new(int(7)), Title: new("forked feature"), State: new("open"),
		Head: &sdkgithub.PullRequestBranch{Ref: new("feature-x"), Label: new("forker:feature-x"), SHA: new("abc")},
		Base: &sdkgithub.PullRequestBranch{Ref: new("main"), SHA: new("def")},
	}
	otherPR := &sdkgithub.PullRequest{
		Number: new(int(8)), Title: new("other branch"), State: new("open"),
		Head: &sdkgithub.PullRequestBranch{Ref: new("chore"), Label: new("owner:chore"), SHA: new("aaa")},
		Base: &sdkgithub.PullRequestBranch{Ref: new("main"), SHA: new("def")},
	}

	t.Run("pure_branch_name_postfilters_fork_prs", func(t *testing.T) {
		var gotQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode([]*sdkgithub.PullRequest{forkPR, otherPR})
		}))
		defer srv.Close()
		p := newTestProvider(t, srv.URL)
		crs, _, err := p.ListCRs(context.Background(), provider.ListCROptions{
			Owner: "owner", Repo: "repo", SourceBranch: "feature-x",
		})
		require.NoError(t, err)
		assert.NotContains(t, gotQuery, "head=", "纯分支名不得下发 head 过滤(实际 query=%q)", gotQuery)
		require.Len(t, crs, 1, "fork 来源的 feature-x 必须命中,其余被后过滤")
		assert.Equal(t, "feature-x", crs[0].SourceBranch)
	})

	t.Run("owner_branch_pinned_server_side", func(t *testing.T) {
		var gotQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode([]*sdkgithub.PullRequest{forkPR})
		}))
		defer srv.Close()
		p := newTestProvider(t, srv.URL)
		_, _, err := p.ListCRs(context.Background(), provider.ListCROptions{
			Owner: "owner", Repo: "repo", SourceBranch: "forker:feature-x",
		})
		require.NoError(t, err)
		assert.Contains(t, gotQuery, "head=forker%3Afeature-x", "owner:branch 形态必须原样下发 head 过滤")
	})
}

// TestRequestTeamReviewers_TeamChannelWire team review request 落独立
// team_reviewers 通道(issue #79),不影响用户评审请求。
func TestRequestTeamReviewers_TeamChannelWire(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = readAllBody(t, r)
		_ = json.NewEncoder(w).Encode(&sdkgithub.PullRequest{})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL)

	tm, ok := provider.Provider(p).(provider.TeamReviewManager)
	require.True(t, ok, "github 必须实现 TeamReviewManager")
	require.NoError(t, tm.RequestTeamReviewers(context.Background(), "owner", "repo", "1", []string{"platform-team"}))
	assert.Contains(t, gotBody, `"team_reviewers":["platform-team"]`)
	assert.NotContains(t, gotBody, `"reviewers"`, "用户通道不得误带(实际 body=%q)", gotBody)
}

// --- 夹具 ---

// currentProtectionResponse 构造 GET 保护规则的 current:带 restrictions、
// force-push 例外、enforce_admins=false、dismiss_stale=true。
func currentProtectionResponse() *sdkgithub.Protection {
	return &sdkgithub.Protection{
		RequiredStatusChecks: &sdkgithub.RequiredStatusChecks{Strict: true},
		RequiredPullRequestReviews: &sdkgithub.PullRequestReviewsEnforcement{
			DismissStaleReviews:          true,
			RequireCodeOwnerReviews:      false,
			RequiredApprovingReviewCount: 1,
		},
		EnforceAdmins:    &sdkgithub.AdminEnforcement{Enabled: false},
		AllowForcePushes: &sdkgithub.AllowForcePushes{Enabled: true},
		AllowDeletions:   &sdkgithub.AllowDeletions{Enabled: false},
		Restrictions: &sdkgithub.BranchRestrictions{
			Users: []*sdkgithub.User{{Login: new("alice")}},
			Teams: []*sdkgithub.Team{{Slug: new("release-eng")}},
			Apps:  []*sdkgithub.App{{Slug: new("ci-bot")}},
		},
	}
}

// putProtectionResponse PUT 的回读(形状与 current 无关,测试只看 wire)。
func putProtectionResponse() *sdkgithub.Protection { return currentProtectionResponse() }

// 编译期防漂移:github 的团队评审能力面(指针接收者)。
var _ provider.TeamReviewManager = (*ghbackend.Provider)(nil)
