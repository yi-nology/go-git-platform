package forgejo_test

// issue #79 wire 回归:team review request 落独立 team_reviewers 通道。
// gitea/forgejo 的 review-request wire 同时携带 reviewers 与
// team_reviewers 两个字段;统一 ReviewManager 只表达用户半边,团队半边
// 走 TeamReviewManager 可选能力。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/go-git-platform/provider"
)

func TestRequestTeamReviewers_TeamChannelWire(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		writeJSON(w, map[string]any{})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	tm, ok := provider.Provider(p).(provider.TeamReviewManager)
	require.True(t, ok, "gitea 必须实现 TeamReviewManager")
	require.NoError(t, tm.RequestTeamReviewers(context.Background(), "owner", "repo", "3", []string{"platform-team"}))

	teams, _ := gotBody["team_reviewers"].([]any)
	require.Len(t, teams, 1)
	assert.Equal(t, "platform-team", teams[0], "team slug 必须落 team_reviewers 通道")
	// SDK 的 wire 形态 reviewers 无 omitempty(null 也算不带用户)
	revs, _ := gotBody["reviewers"].([]any)
	assert.Empty(t, revs, "用户通道不得误带(实际 body=%v)", gotBody)
}

// TestRequestTeamReviewers_BothChannelsIndependent 用户与团队两次请求互不覆盖。
func TestRequestTeamReviewers_BothChannelsIndependent(t *testing.T) {
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastBody = nil
		require.NoError(t, json.NewDecoder(r.Body).Decode(&lastBody))
		writeJSON(w, map[string]any{})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	rm := provider.Provider(p).(provider.ReviewManager)
	tm := provider.Provider(p).(provider.TeamReviewManager)
	require.NoError(t, rm.RequestReviewers(context.Background(), "owner", "repo", "3", []string{"alice"}))
	users, _ := lastBody["reviewers"].([]any)
	require.Len(t, users, 1)
	teams0, _ := lastBody["team_reviewers"].([]any)
	assert.Empty(t, teams0, "用户请求不得误带团队通道")

	require.NoError(t, tm.RequestTeamReviewers(context.Background(), "owner", "repo", "3", []string{"platform-team"}))
	teams, _ := lastBody["team_reviewers"].([]any)
	require.Len(t, teams, 1)
	revs2, _ := lastBody["reviewers"].([]any)
	assert.Empty(t, revs2, "团队请求不得误带用户通道")
}
