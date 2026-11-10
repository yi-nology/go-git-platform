package gitee_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestCreateCommitStatus_PendingWritesInFlightForm issue #58 回归:
// 统一词表的 in-flight 态(pending/running)必须写 check-run 的
// in_progress 形态且**不带 conclusion**——写 completed+neutral 会被读侧
// conclusion 归一成 Success,「写 pending 读回 success」的静默反转把
// 合并门禁提前放行。终态动词照常落 completed+conclusion。
func TestCreateCommitStatus_PendingWritesInFlightForm(t *testing.T) {
	tests := []struct {
		name           string
		state          string
		wantStatus     string
		wantConclusion string // 空串 = wire 上不得出现 conclusion
	}{
		{name: "pending_is_in_progress_without_conclusion", state: "pending", wantStatus: "in_progress", wantConclusion: ""},
		{name: "running_is_in_progress_without_conclusion", state: "running", wantStatus: "in_progress", wantConclusion: ""},
		{name: "success_is_completed_conclusion", state: "success", wantStatus: "completed", wantConclusion: "success"},
		{name: "failure_is_completed_conclusion", state: "failure", wantStatus: "completed", wantConclusion: "failure"},
		{name: "error_is_completed_conclusion", state: "error", wantStatus: "completed", wantConclusion: "failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(&gitee.CheckRun{})
			}))
			defer srv.Close()
			p := newTestProvider(t, srv)

			require.NoError(t, p.CreateCommitStatus(context.Background(), "owner", "repo", "abc",
				provider.CommitStatusOptions{State: tt.state, Context: "ci/build"}))

			assert.Equal(t, tt.wantStatus, gotBody["status"], "check-run status 落线形态")
			if tt.wantConclusion == "" {
				// conclusion 要么缺席(omitempty)要么为空——不得出现 neutral
				conc, has := gotBody["conclusion"]
				assert.False(t, has && conc != "", "in-flight 态不得带 conclusion(实际=%v)", gotBody["conclusion"])
			} else {
				assert.Equal(t, tt.wantConclusion, gotBody["conclusion"])
			}
		})
	}
}

// TestListCommitStatuses_InFlightRunsStayNonTerminal 读侧 round-trip:
// in_progress 形态的 check run 必须归一回 Running(而非被 conclusion
// 缺席路径误判),completed+conclusion 才落终态——合并门禁语义闭环。
func TestListCommitStatuses_InFlightRunsStayNonTerminal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		inProgress := "in_progress"
		completed := "completed"
		conc := "success"
		_ = json.NewEncoder(w).Encode(&gitee.CheckRunList{
			CheckRuns: &[]*gitee.CheckRun{
				{Name: gitee.String("ci/in-flight"), Status: &inProgress},
				{Name: gitee.String("ci/done"), Status: &completed, Conclusion: &conc},
			},
			TotalCount: gitee.Int(2),
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	statuses, err := p.ListCommitStatuses(context.Background(), "owner", "repo", "abc")
	require.NoError(t, err)
	require.Len(t, statuses, 2)

	assert.Equal(t, provider.CommitStatusRunning, statuses[0].State, "in_progress 必须归一 Running")
	assert.False(t, statuses[0].State.Terminal(), "in-flight 态不得是终态(门禁不得放行)")
	assert.Equal(t, provider.CommitStatusSuccess, statuses[1].State, "completed+success 归一 Success")
}
