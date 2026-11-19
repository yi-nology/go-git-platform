package github_test

// issue #171 扩展字段(Contexts / RequireCodeOwnerReviews / EnforceAdmins)
// 读写双侧的 wire 钉子。读侧:convertProtection 必须回填三字段,且指针形态
// 区分「平台未返回」(nil) 与显式 false;写侧:Create 直 PUT 映射三字段,
// Update 的 #74 保值族扩展到三字段——nil 走 current 回填,显式值覆盖,
// Contexts 显式空切片 = 清空(不是复位整个 checks)。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ghbackend "github.com/yi-nology/go-git-platform/adapter/github"
	"github.com/yi-nology/go-git-platform/provider"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extCurrentJSON 是带三扩展字段的保护规则 GET 载荷。
const extCurrentJSON = `{
  "required_status_checks": {"strict": true, "contexts": ["ci/build", "ci/test"]},
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": true,
    "required_approving_review_count": 1
  },
  "enforce_admins": {"enabled": true},
  "allow_force_pushes": {"enabled": true},
  "allow_deletions": {"enabled": false}
}`

// extBareJSON 是不携带三扩展字段对象的载荷(指针 nil 语义的负面对照):
// required_pull_request_reviews / enforce_admins 对象整体缺席,
// required_status_checks 不带 contexts 键。
const extBareJSON = `{
  "required_status_checks": {"strict": true},
  "allow_force_pushes": {"enabled": false},
  "allow_deletions": {"enabled": false}
}`

// extMux 构造 GET=extCurrentJSON / PUT=extCurrentJSON 的保护端点 mux。
func extMux(t *testing.T, rec *wireRecorder, getJSON string) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/owner/repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		body := readAllBody(t, r)
		rec.record(r, body)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(getJSON))
		case http.MethodPut:
			_, _ = w.Write([]byte(getJSON))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func extProvider(t *testing.T, mux *http.ServeMux) *ghbackend.Provider {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return newTestProvider(t, srv.URL)
}

func putBody(t *testing.T, rec *wireRecorder) map[string]any {
	t.Helper()
	reqs := rec.all()
	require.NotEmpty(t, reqs, "expected at least one request")
	put := reqs[len(reqs)-1]
	require.Equal(t, http.MethodPut, put.Method)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(put.Body), &body))
	return body
}

// TestGetBranchProtection_ExtendedFieldsReadBack 读侧回填(issue #171):
// contexts/code-owner/enforce-admins 三个对象都要映射进 model;平台未返回
// 对应对象时指针保持 nil(区分「未返回」与显式 false)。
func TestGetBranchProtection_ExtendedFieldsReadBack(t *testing.T) {
	rec := &wireRecorder{}
	p := extProvider(t, extMux(t, rec, extCurrentJSON))

	bp, err := p.GetBranchProtection(context.Background(), "owner", "repo", "main")
	require.NoError(t, err)
	require.NotNil(t, bp)
	assert.Equal(t, []string{"ci/build", "ci/test"}, bp.Contexts)
	require.NotNil(t, bp.RequireCodeOwnerReviews, "require_code_owner_reviews 对象在场,指针不得为 nil")
	assert.True(t, *bp.RequireCodeOwnerReviews)
	require.NotNil(t, bp.EnforceAdmins, "enforce_admins 对象在场,指针不得为 nil")
	assert.True(t, *bp.EnforceAdmins)
	assert.True(t, bp.RequiredStatusChecks)

	rec2 := &wireRecorder{}
	p2 := extProvider(t, extMux(t, rec2, extBareJSON))
	bare, err := p2.GetBranchProtection(context.Background(), "owner", "repo", "main")
	require.NoError(t, err)
	assert.Nil(t, bare.Contexts, "无 contexts 键时保持 nil 而非空切片")
	assert.Nil(t, bare.RequireCodeOwnerReviews, "平台未返回 code-owner 对象时指针为 nil")
	assert.Nil(t, bare.EnforceAdmins, "平台未返回 enforce_admins 对象时指针为 nil")
}

// TestCreateBranchProtection_ExtendedFieldsWire create 写侧三字段落线:
// Contexts 蕴含启用 required_status_checks,RequireCodeOwnerReviews 与
// EnforceAdmins 各归其位——单 PUT 直出(#73 形状不破)。
func TestCreateBranchProtection_ExtendedFieldsWire(t *testing.T) {
	rec := &wireRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/owner/repo/branches/feature/protection", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r, readAllBody(t, r))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(extCurrentJSON))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	p := extProvider(t, mux)

	codeOwner := true
	enforce := true
	out, err := p.CreateBranchProtection(context.Background(), "owner", "repo",
		provider.CreateBranchProtectionOptions{
			BranchName:               "feature",
			RequiredApprovingReviews: 1,
			Contexts:                 []string{"ci/build", "ci/test"},
			RequireCodeOwnerReviews:  &codeOwner,
			EnforceAdmins:            &enforce,
		})
	require.NoError(t, err)
	require.NotNil(t, out)

	reqs := rec.all()
	require.Len(t, reqs, 1, "create 只应发一次 PUT")
	body := putBody(t, rec)
	sc, ok := body["required_status_checks"].(map[string]any)
	require.True(t, ok, "Contexts 非空必须出线 required_status_checks")
	assert.Equal(t, true, sc["strict"])
	assert.Equal(t, []any{"ci/build", "ci/test"}, sc["contexts"])
	rpr, ok := body["required_pull_request_reviews"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, rpr["require_code_owner_reviews"])
	assert.Equal(t, true, body["enforce_admins"], "EnforceAdmins 显式 true 必须落线")
}

// TestCreateBranchProtection_ContextsWinOverExplicitFalse 冲突处置(issue
// #171,options.go 注释同钉):Create 时 Contexts 非空但
// RequiredStatusChecks 显式 false——以 contexts 为准(强制启用),不报错。
func TestCreateBranchProtection_ContextsWinOverExplicitFalse(t *testing.T) {
	rec := &wireRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/owner/repo/branches/feature/protection", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r, readAllBody(t, r))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(extCurrentJSON))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	p := extProvider(t, mux)

	_, err := p.CreateBranchProtection(context.Background(), "owner", "repo",
		provider.CreateBranchProtectionOptions{
			BranchName:           "feature",
			RequiredStatusChecks: false,
			Contexts:             []string{"ci/lint"},
		})
	require.NoError(t, err, "冲突处置选择「以 contexts 为准」,不得报错")
	body := putBody(t, rec)
	sc, ok := body["required_status_checks"].(map[string]any)
	require.True(t, ok, "contexts 非空必须蕴含启用 required_status_checks(以 contexts 为准)")
	assert.Equal(t, []any{"ci/lint"}, sc["contexts"])
}

// TestUpdateBranchProtection_ExtendedNilFieldsCarried #74 保值族扩展到三
// 字段:只改评审数时,current 的 contexts / code-owner / enforce-admins
// 必须原样回填,不得被全量替换语义复位。
func TestUpdateBranchProtection_ExtendedNilFieldsCarried(t *testing.T) {
	rec := &wireRecorder{}
	p := extProvider(t, extMux(t, rec, extCurrentJSON))

	two := 2
	_, err := p.UpdateBranchProtection(context.Background(), "owner", "repo", "main",
		provider.UpdateBranchProtectionOptions{RequiredApprovingReviews: &two})
	require.NoError(t, err)

	body := putBody(t, rec)
	sc, ok := body["required_status_checks"].(map[string]any)
	require.True(t, ok, "current 的 status checks(含 contexts)必须保留")
	assert.Equal(t, []any{"ci/build", "ci/test"}, sc["contexts"], "contexts 不得被复位")
	rpr, ok := body["required_pull_request_reviews"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, rpr["require_code_owner_reviews"], "code-owner 评审设置要从 current 回填")
	assert.InEpsilon(t, 2, rpr["required_approving_review_count"], 0)
	assert.Equal(t, true, body["enforce_admins"], "enforce_admins 要透传 current(true),不得复位为 false")
}

// TestUpdateBranchProtection_ExtendedFieldsExplicitOverride 显式值覆盖
// current:三个字段都显式给出时按显式值落线(Enable=false 也要能关掉)。
func TestUpdateBranchProtection_ExtendedFieldsExplicitOverride(t *testing.T) {
	rec := &wireRecorder{}
	p := extProvider(t, extMux(t, rec, extCurrentJSON))

	lintOnly := []string{"ci/lint"}
	codeOwner := false
	enforce := false
	_, err := p.UpdateBranchProtection(context.Background(), "owner", "repo", "main",
		provider.UpdateBranchProtectionOptions{
			Contexts:                &lintOnly,
			RequireCodeOwnerReviews: &codeOwner,
			EnforceAdmins:           &enforce,
		})
	require.NoError(t, err)

	body := putBody(t, rec)
	sc, ok := body["required_status_checks"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"ci/lint"}, sc["contexts"], "显式 Contexts 覆盖 current")
	rpr, ok := body["required_pull_request_reviews"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, rpr["require_code_owner_reviews"], "显式 false 必须落线(关掉 code-owner 评审)")
	assert.Equal(t, false, body["enforce_admins"], "显式 false 必须落线(关掉管理员强制)")
}

// TestUpdateBranchProtection_ContextsExplicitEmptyClears 显式空切片 =
// 清空 contexts(checks 保持启用,strict 保留),与「nil = 不动」区分。
func TestUpdateBranchProtection_ContextsExplicitEmptyClears(t *testing.T) {
	rec := &wireRecorder{}
	p := extProvider(t, extMux(t, rec, extCurrentJSON))

	empty := []string{}
	_, err := p.UpdateBranchProtection(context.Background(), "owner", "repo", "main",
		provider.UpdateBranchProtectionOptions{Contexts: &empty})
	require.NoError(t, err)

	body := putBody(t, rec)
	sc, ok := body["required_status_checks"].(map[string]any)
	require.True(t, ok, "清空 contexts 不等于关掉 checks——required_status_checks 必须仍在")
	assert.Equal(t, []any{}, sc["contexts"], "显式空切片必须落线 contexts:[]")
}
