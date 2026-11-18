package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- fakes ---

type fakeWebhookManager struct {
	hooks       []*model.PlatformWebhook
	nextID      int64
	created     provider.CreateWebhookOptions
	updatedByID map[int64]string // webhook ID → secret patched in
}

func (f *fakeWebhookManager) ListWebhooks(context.Context, string, string) ([]*model.PlatformWebhook, error) {
	return f.hooks, nil
}

func (f *fakeWebhookManager) CreateWebhook(_ context.Context, opts provider.CreateWebhookOptions) (*model.PlatformWebhook, error) {
	f.nextID++
	events := opts.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request"}
	}
	f.created = opts
	h := &model.PlatformWebhook{ID: f.nextID, URL: opts.URL, Events: events}
	f.hooks = append(f.hooks, h)
	return h, nil
}

func (f *fakeWebhookManager) UpdateWebhook(_ context.Context, _, _ string, webhookID int64, opts provider.UpdateWebhookOptions) (*model.PlatformWebhook, error) {
	if f.updatedByID == nil {
		f.updatedByID = map[int64]string{}
	}
	if opts.Secret != nil {
		f.updatedByID[webhookID] = *opts.Secret
	}
	for _, h := range f.hooks {
		if h.ID == webhookID {
			return h, nil
		}
	}
	return nil, errors.New("no such hook")
}

func (f *fakeWebhookManager) GetWebhook(_ context.Context, _, _ string, webhookID int64) (*model.PlatformWebhook, error) {
	for _, h := range f.hooks {
		if h.ID == webhookID {
			return h, nil
		}
	}
	return nil, errors.New("no such hook")
}

func (f *fakeWebhookManager) DeleteWebhook(_ context.Context, _, _ string, id int64) error {
	for i, h := range f.hooks {
		if h.ID == id {
			f.hooks = append(f.hooks[:i], f.hooks[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeWebhookManager) ParseWebhookEvent(*http.Request, string) (*model.NormalizedEvent, error) {
	return nil, provider.ErrNotImplemented
}

func (f *fakeWebhookManager) ValidateWebhookSignature(*http.Request, string) error { return nil }

type fakeBranchProtection struct {
	rules    map[string]*model.BranchProtection
	getFails error
}

func (f *fakeBranchProtection) ListBranchProtections(context.Context, string, string) ([]*model.BranchProtection, error) {
	out := make([]*model.BranchProtection, 0, len(f.rules))
	for _, r := range f.rules {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeBranchProtection) GetBranchProtection(_ context.Context, _, _, branch string) (*model.BranchProtection, error) {
	if f.getFails != nil {
		return nil, f.getFails
	}
	r, ok := f.rules[branch]
	if !ok {
		return nil, &provider.ProviderError{Op: "GetBranchProtection", Cause: provider.ErrNotFound}
	}
	return r, nil
}

func (f *fakeBranchProtection) CreateBranchProtection(_ context.Context, _, _ string, opts provider.CreateBranchProtectionOptions) (*model.BranchProtection, error) {
	r := &model.BranchProtection{
		BranchName: opts.BranchName, RequiredApprovingReviews: opts.RequiredApprovingReviews,
		RequiredStatusChecks: opts.RequiredStatusChecks, AllowForcePushes: opts.AllowForcePushes,
		AllowDeletions: opts.AllowDeletions,
	}
	f.rules[opts.BranchName] = r
	return r, nil
}

func (f *fakeBranchProtection) UpdateBranchProtection(_ context.Context, _, _, branch string, opts provider.UpdateBranchProtectionOptions) (*model.BranchProtection, error) {
	r := f.rules[branch]
	if opts.RequiredApprovingReviews != nil {
		r.RequiredApprovingReviews = *opts.RequiredApprovingReviews
	}
	if opts.RequiredStatusChecks != nil {
		r.RequiredStatusChecks = *opts.RequiredStatusChecks
	}
	if opts.AllowForcePushes != nil {
		r.AllowForcePushes = *opts.AllowForcePushes
	}
	if opts.AllowDeletions != nil {
		r.AllowDeletions = *opts.AllowDeletions
	}
	return r, nil
}

func (f *fakeBranchProtection) DeleteBranchProtection(_ context.Context, _, _, branch string) error {
	delete(f.rules, branch)
	return nil
}

type fakeDeployKeys struct {
	keys  []*model.DeployKey
	next  int64
	added provider.AddDeployKeyOptions
}

func (f *fakeDeployKeys) ListDeployKeys(context.Context, string, string) ([]*model.DeployKey, error) {
	return f.keys, nil
}

func (f *fakeDeployKeys) AddDeployKey(_ context.Context, _, _ string, opts provider.AddDeployKeyOptions) (*model.DeployKey, error) {
	f.next++
	f.added = opts
	k := &model.DeployKey{ID: f.next, Title: opts.Title, Key: opts.Key, ReadOnly: opts.ReadOnly}
	f.keys = append(f.keys, k)
	return k, nil
}

func (f *fakeDeployKeys) DeleteDeployKey(_ context.Context, _, _ string, id int64) error {
	for i, k := range f.keys {
		if k.ID == id {
			f.keys = append(f.keys[:i], f.keys[i+1:]...)
			return nil
		}
	}
	return nil
}

// --- webhooks ---

func TestEnsureWebhookCreates(t *testing.T) {
	f := &fakeWebhookManager{}
	action, hook, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook"})
	if err != nil {
		t.Fatalf("EnsureWebhook: %v", err)
	}
	if action != EnsureCreated {
		t.Fatalf("action = %q, want created", action)
	}
	if hook.ID != 1 || len(f.hooks) != 1 {
		t.Fatalf("hook = %+v, hooks = %+v", hook, f.hooks)
	}
	// Default events applied.
	if len(f.created.Events) != 2 {
		t.Fatalf("created events = %v", f.created.Events)
	}
}

func TestEnsureWebhookUnchangedOnReRun(t *testing.T) {
	f := &fakeWebhookManager{}
	_, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Events: []string{"push", "pull_request"}})
	if err != nil {
		t.Fatal(err)
	}
	before := f.hooks[0].ID
	action, hook, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook"})
	if err != nil {
		t.Fatal(err)
	}
	if action != EnsureUnchanged || hook.ID != before {
		t.Fatalf("action = %q id = %d, want unchanged/%d", action, hook.ID, before)
	}
}

func TestEnsureWebhookReplacesOnEventDrift(t *testing.T) {
	f := &fakeWebhookManager{}
	if _, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook"}); err != nil {
		t.Fatal(err)
	}
	f.hooks[0].Events = []string{"push"} // drift
	action, hook, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook"})
	if err != nil {
		t.Fatal(err)
	}
	if action != EnsureUpdated {
		t.Fatalf("action = %q, want updated", action)
	}
	if len(f.hooks) != 1 || hook.ID != 2 {
		t.Fatalf("hooks = %+v, want single replaced hook with new ID", f.hooks)
	}
}

// TestCanonicalWebhookEventsDedupes pins issue #85's canonicalization half:
// aliases ("note" and "comment") collapse onto one canonical name, so a
// backend that reads one flag back under both names canonicalizes to a
// single-element set instead of a duplicated one.
func TestCanonicalWebhookEventsDedupes(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "alias pair collapses", in: []string{"comment", "note"}, want: []string{"note"}},
		{name: "alias pair mixed with others stays sorted and single", in: []string{"note", "push", "comment"}, want: []string{"note", "push"}},
		{name: "unregistered platform names pass through deduped", in: []string{"deployment", "deployment"}, want: []string{"deployment"}},
		{name: "empty stays empty", in: nil, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalWebhookEvents(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("canonicalWebhookEvents(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("canonicalWebhookEvents(%v) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}

// TestEnsureWebhookUnchangedDespiteAliasDuplication pins issue #85's
// convergence half: an existing hook whose read-back event list carries the
// same canonical family twice (["comment","note"] for one note_events flag)
// is unchanged against a desired ["note"], not delete+recreated.
func TestEnsureWebhookUnchangedDespiteAliasDuplication(t *testing.T) {
	f := &fakeWebhookManager{}
	if _, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Events: []string{"note"}}); err != nil {
		t.Fatal(err)
	}
	// Simulate a backend reading one flag back under two names (the
	// pre-#85 gitcode convertHook shape).
	f.hooks[0].Events = []string{"comment", "note"}
	before := f.hooks[0].ID
	action, hook, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Events: []string{"note"}})
	if err != nil {
		t.Fatal(err)
	}
	if action != EnsureUnchanged || hook.ID != before {
		t.Fatalf("action = %q id = %d, want unchanged/%d (aliases must collapse before comparison)", action, hook.ID, before)
	}
}

// provider.EnsureSecretAlwaysUpdate converges a rotated secret on a matching hook:
// in-place patch (hook ID unchanged), reported as updated.
func TestEnsureWebhookSecretAlwaysUpdatePatchesOnMatch(t *testing.T) {
	f := &fakeWebhookManager{}
	if _, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Secret: "old"}); err != nil {
		t.Fatal(err)
	}
	id := f.hooks[0].ID
	action, hook, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Secret: "new", EnsureSecretMode: provider.EnsureSecretAlwaysUpdate})
	if err != nil {
		t.Fatal(err)
	}
	if action != EnsureUpdated {
		t.Fatalf("action = %q, want updated", action)
	}
	if hook.ID != id {
		t.Fatalf("hook ID = %d, want in-place patch keeping %d", hook.ID, id)
	}
	if got := f.updatedByID[id]; got != "new" {
		t.Fatalf("patched secret = %q, want new", got)
	}
	if len(f.hooks) != 1 {
		t.Fatalf("hooks = %+v, want the single hook preserved", f.hooks)
	}
}

// Default mode keeps the pre-#34 behavior: a matching hook is untouched
// even when the secret differs (no patch goes out).
func TestEnsureWebhookSecretDefaultKeepsOnMatch(t *testing.T) {
	f := &fakeWebhookManager{}
	if _, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Secret: "old"}); err != nil {
		t.Fatal(err)
	}
	action, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Secret: "rotated"})
	if err != nil {
		t.Fatal(err)
	}
	if action != EnsureUnchanged {
		t.Fatalf("action = %q, want unchanged", action)
	}
	if len(f.updatedByID) != 0 {
		t.Fatalf("unexpected patches: %v", f.updatedByID)
	}
}

// always-update on an empty desired secret is a no-op: there is nothing to
// converge, and rewriting to empty is not a platform-portable operation.
func TestEnsureWebhookSecretAlwaysUpdateSkipsEmptySecret(t *testing.T) {
	f := &fakeWebhookManager{}
	if _, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Secret: "old"}); err != nil {
		t.Fatal(err)
	}
	action, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", EnsureSecretMode: provider.EnsureSecretAlwaysUpdate})
	if err != nil {
		t.Fatal(err)
	}
	if action != EnsureUnchanged {
		t.Fatalf("action = %q, want unchanged", action)
	}
	if len(f.updatedByID) != 0 {
		t.Fatalf("unexpected patches: %v", f.updatedByID)
	}
}

// bareWebhookManager implements provider.WebhookManager only (no UpdateWebhook) —
// the always-update mode must fail loudly on such a manager instead of
// silently keeping a possibly-stale secret.
type bareWebhookManager struct {
	hooks []*model.PlatformWebhook
}

func (f *bareWebhookManager) ListWebhooks(context.Context, string, string) ([]*model.PlatformWebhook, error) {
	return f.hooks, nil
}

func (f *bareWebhookManager) CreateWebhook(_ context.Context, opts provider.CreateWebhookOptions) (*model.PlatformWebhook, error) {
	h := &model.PlatformWebhook{ID: int64(len(f.hooks) + 1), URL: opts.URL, Events: opts.Events}
	f.hooks = append(f.hooks, h)
	return h, nil
}

func (f *bareWebhookManager) DeleteWebhook(_ context.Context, _, _ string, id int64) error {
	for i, h := range f.hooks {
		if h.ID == id {
			f.hooks = append(f.hooks[:i], f.hooks[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *bareWebhookManager) ParseWebhookEvent(*http.Request, string) (*model.NormalizedEvent, error) {
	return nil, provider.ErrNotImplemented
}

func (f *bareWebhookManager) ValidateWebhookSignature(*http.Request, string) error { return nil }

func TestEnsureWebhookSecretAlwaysUpdateRequiresConfigManager(t *testing.T) {
	f := &bareWebhookManager{}
	if _, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Secret: "old"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Secret: "new", EnsureSecretMode: provider.EnsureSecretAlwaysUpdate})
	if err == nil {
		t.Fatal("expected an error for a manager without WebhookConfigManager")
	}
}

// --- branch protection ---

func TestEnsureBranchProtectionLifecycle(t *testing.T) {
	f := &fakeBranchProtection{rules: map[string]*model.BranchProtection{}}
	opts := provider.CreateBranchProtectionOptions{BranchName: "main", RequiredApprovingReviews: 2, RequiredStatusChecks: true}

	action, _, err := EnsureBranchProtection(t.Context(), f, "o", "r", opts)
	if err != nil || action != EnsureCreated {
		t.Fatalf("create: action=%q err=%v", action, err)
	}
	action, _, err = EnsureBranchProtection(t.Context(), f, "o", "r", opts)
	if err != nil || action != EnsureUnchanged {
		t.Fatalf("re-run: action=%q err=%v", action, err)
	}
	opts.RequiredApprovingReviews = 1
	action, rule, err := EnsureBranchProtection(t.Context(), f, "o", "r", opts)
	if err != nil || action != EnsureUpdated {
		t.Fatalf("update: action=%q err=%v", action, err)
	}
	if rule.RequiredApprovingReviews != 1 {
		t.Fatalf("rule = %+v, want reviews=1", rule)
	}
}

func TestEnsureBranchProtectionPropagatesRealErrors(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeBranchProtection{getFails: boom}
	// 读路径错误（GET 非 404）不动：零 outcome + nil 实体，逐字上抛。
	outcome, entity, err := EnsureBranchProtection(t.Context(), f, "o", "r", provider.CreateBranchProtectionOptions{BranchName: "main"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if outcome != "" || entity != nil {
		t.Fatalf("outcome = %q entity = %+v, want zero outcome/nil（读路径非变更冲突）", outcome, entity)
	}
}

// --- deploy keys ---

func TestEnsureDeployKeyLifecycle(t *testing.T) {
	f := &fakeDeployKeys{}
	opts := provider.AddDeployKeyOptions{Title: "ci", Key: "ssh-ed25519 AAA", ReadOnly: true}

	action, _, err := EnsureDeployKey(t.Context(), f, "o", "r", opts)
	if err != nil || action != EnsureCreated {
		t.Fatalf("create: action=%q err=%v", action, err)
	}
	action, _, err = EnsureDeployKey(t.Context(), f, "o", "r", opts)
	if err != nil || action != EnsureUnchanged {
		t.Fatalf("re-run: action=%q err=%v", action, err)
	}
	opts.Key = "ssh-ed25519 BBB"
	action, key, err := EnsureDeployKey(t.Context(), f, "o", "r", opts)
	if err != nil || action != EnsureUpdated {
		t.Fatalf("replace: action=%q err=%v", action, err)
	}
	if len(f.keys) != 1 || key.Key != "ssh-ed25519 BBB" {
		t.Fatalf("keys = %+v", f.keys)
	}
}

// TestEnsureWebhook_VocabularyMismatchConverges 复刻 issue #40 的生产形态：
// GitLab/TencentCode 家族写侧接受 pull_request、读侧 convertHook 读回
// merge_request——归一缺失时 EnsureWebhook 永不收敛（每次 delete+recreate）。
// 归一后：读侧家族名 → 输入名，同族 ⇒ unchanged，钩子 ID 不翻动。
func TestEnsureWebhook_VocabularyMismatchConverges(t *testing.T) {
	f := &fakeWebhookManager{}
	if _, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Events: []string{"push", "pull_request"}}); err != nil {
		t.Fatal(err)
	}
	id := f.hooks[0].ID
	// 模拟 GitLab 列表输出：同一布尔位读回家族名（pull_request → merge_request）。
	f.hooks[0].Events = []string{"push", "merge_request"}
	action, hook, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Events: []string{"push", "pull_request"}})
	if err != nil {
		t.Fatal(err)
	}
	if action != EnsureUnchanged || hook.ID != id {
		t.Fatalf("action = %q id = %d, want unchanged/%d（词表归一缺失即永不收敛）", action, hook.ID, id)
	}

	// TencentCode 形态：读侧 merge_request + tag_push + issues + note vs
	// 输入 push + cr + tag + comment + issue——全部族归一后等价。
	f2 := &fakeWebhookManager{}
	if _, _, err := EnsureWebhook(t.Context(), f2, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Events: []string{"push", "cr", "tag", "comment", "issue"}}); err != nil {
		t.Fatal(err)
	}
	id2 := f2.hooks[0].ID
	f2.hooks[0].Events = []string{"push", "merge_request", "tag_push", "note", "issues"}
	action2, hook2, err := EnsureWebhook(t.Context(), f2, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Events: []string{"push", "cr", "tag", "comment", "issue"}})
	if err != nil {
		t.Fatal(err)
	}
	if action2 != EnsureUnchanged || hook2.ID != id2 {
		t.Fatalf("action = %q id = %d, want unchanged/%d", action2, hook2.ID, id2)
	}

	// 真漂移仍然报漂移：读侧多出一个未请求的族。
	f.hooks[0].Events = []string{"push", "merge_request", "note"}
	action3, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://cb.example/hook", Events: []string{"push", "pull_request"}})
	if err != nil {
		t.Fatal(err)
	}
	if action3 != EnsureUpdated {
		t.Fatalf("extra family must be drift, action = %q", action3)
	}
}

// --- conflict outcomes (2B T1) ---

// conflictWebhookMgr 是写操作撞 409 的桩：List 返回现场 hooks，写方法返回
// 包 provider.ErrConflict（409 分类）的错误——与 provider.Wrap 后上抛的形态一致。
// 嵌入 nil provider.WebhookManager 仅满足未覆写的方法（ParseWebhookEvent 等在这些
// 测试里不会被调用）。
type conflictWebhookMgr struct {
	provider.WebhookManager
	hooks  []*model.PlatformWebhook
	delErr error // 非 nil 时 DeleteWebhook 返回它（默认 409 冲突）
}

func (m conflictWebhookMgr) ListWebhooks(context.Context, string, string) ([]*model.PlatformWebhook, error) {
	return m.hooks, nil
}

func (m conflictWebhookMgr) DeleteWebhook(context.Context, string, string, int64) error {
	if m.delErr != nil {
		return m.delErr
	}
	return fmt.Errorf("delete: %w", provider.ErrConflict)
}

func (m conflictWebhookMgr) CreateWebhook(context.Context, provider.CreateWebhookOptions) (*model.PlatformWebhook, error) {
	return nil, fmt.Errorf("create: %w", provider.ErrConflict)
}

// GetWebhook/UpdateWebhook 让桩同时满足 provider.WebhookConfigManager（always-update
// 路径的类型断言）；实际只有 UpdateWebhook 会被调用。
func (m conflictWebhookMgr) GetWebhook(_ context.Context, _ string, _ string, id int64) (*model.PlatformWebhook, error) {
	for _, h := range m.hooks {
		if h.ID == id {
			return h, nil
		}
	}
	return nil, fmt.Errorf("get: %w", provider.ErrConflict)
}

func (m conflictWebhookMgr) UpdateWebhook(context.Context, string, string, int64, provider.UpdateWebhookOptions) (*model.PlatformWebhook, error) {
	return nil, fmt.Errorf("update: %w", provider.ErrConflict)
}

// TestEnsureWebhook_ConflictOutcome：无命中钩子（既有 URL 不同）→ 直接
// create 撞 409 → (OutcomeConflict, nil, err)——err 经 provider.IsConflict 可判，
// outcome 必须是 OutcomeConflict（唯一同时带 outcome 与非 nil error 的分支）。
func TestEnsureWebhook_ConflictOutcome(t *testing.T) {
	m := conflictWebhookMgr{hooks: []*model.PlatformWebhook{{ID: 1, URL: "https://a", Events: []string{"push"}}}}
	outcome, entity, err := EnsureWebhook(t.Context(), m,
		provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://b", Events: []string{"push"}})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", outcome)
	}
	if entity != nil {
		t.Fatalf("entity = %+v, want nil（无既有实体）", entity)
	}
}

// 删除路径：URL 命中但事件漂移 → delete 撞 409 → (OutcomeConflict, matches[0], err)。
func TestEnsureWebhook_DeleteConflictOutcome(t *testing.T) {
	m := conflictWebhookMgr{hooks: []*model.PlatformWebhook{{ID: 1, URL: "https://b", Events: []string{"push"}}}}
	outcome, entity, err := EnsureWebhook(t.Context(), m,
		provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://b", Events: []string{"push", "pull_request"}})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", outcome)
	}
	if entity == nil || entity.ID != 1 {
		t.Fatalf("entity = %+v, want matches[0] (ID 1)", entity)
	}
}

// secret 补写路径：URL+事件全匹配 + always-update → UpdateWebhook 撞 409 →
// (OutcomeConflict, 该钩子, err)。
func TestEnsureWebhook_SecretUpdateConflictOutcome(t *testing.T) {
	m := conflictWebhookMgr{hooks: []*model.PlatformWebhook{{ID: 7, URL: "https://b", Events: []string{"push", "pull_request"}}}}
	outcome, entity, err := EnsureWebhook(t.Context(), m, provider.CreateWebhookOptions{
		Owner: "o", Repo: "r", URL: "https://b", Events: []string{"push", "pull_request"},
		Secret: "rotated", EnsureSecretMode: provider.EnsureSecretAlwaysUpdate,
	})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", outcome)
	}
	if entity == nil || entity.ID != 7 {
		t.Fatalf("entity = %+v, want the patched hook (ID 7)", entity)
	}
}

// 只有 409 翻 outcome：非冲突写错误逐字保持 ("", nil, err)。
func TestEnsureWebhook_NonConflictWriteErrorKeepsZeroOutcome(t *testing.T) {
	boom := errors.New("boom")
	m := conflictWebhookMgr{
		hooks:  []*model.PlatformWebhook{{ID: 1, URL: "https://b", Events: []string{"push"}}},
		delErr: boom,
	}
	outcome, entity, err := EnsureWebhook(t.Context(), m,
		provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://b", Events: []string{"push", "pull_request"}})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if outcome != "" || entity != nil {
		t.Fatalf("outcome = %q entity = %+v, want zero outcome/nil（仅 409 翻 outcome）", outcome, entity)
	}
}

// conflictBranchProtection：Get 返回 cur（nil = 未找到，→ provider.ErrNotFound），
// 写操作一律撞 409。
type conflictBranchProtection struct {
	cur *model.BranchProtection
}

func (m conflictBranchProtection) ListBranchProtections(context.Context, string, string) ([]*model.BranchProtection, error) {
	return nil, nil
}

func (m conflictBranchProtection) GetBranchProtection(context.Context, string, string, string) (*model.BranchProtection, error) {
	if m.cur == nil {
		return nil, &provider.ProviderError{Op: "GetBranchProtection", Cause: provider.ErrNotFound}
	}
	return m.cur, nil
}

func (m conflictBranchProtection) CreateBranchProtection(context.Context, string, string, provider.CreateBranchProtectionOptions) (*model.BranchProtection, error) {
	return nil, fmt.Errorf("create: %w", provider.ErrConflict)
}

func (m conflictBranchProtection) UpdateBranchProtection(context.Context, string, string, string, provider.UpdateBranchProtectionOptions) (*model.BranchProtection, error) {
	return nil, fmt.Errorf("update: %w", provider.ErrConflict)
}

func (m conflictBranchProtection) DeleteBranchProtection(context.Context, string, string, string) error {
	return nil
}

// 更新路径：已有规则与期望不符 → Update 撞 409 → (OutcomeConflict, cur, err)。
func TestEnsureBranchProtection_UpdateConflictOutcome(t *testing.T) {
	cur := &model.BranchProtection{BranchName: "main", RequiredApprovingReviews: 1}
	m := conflictBranchProtection{cur: cur}
	outcome, entity, err := EnsureBranchProtection(t.Context(), m, "o", "r",
		provider.CreateBranchProtectionOptions{BranchName: "main", RequiredApprovingReviews: 2})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", outcome)
	}
	if entity != cur {
		t.Fatalf("entity = %+v, want cur %+v", entity, cur)
	}
}

// 创建路径：规则不存在 → Create 撞 409 → (OutcomeConflict, nil, err)（无既有实体）。
func TestEnsureBranchProtection_CreateConflictOutcome(t *testing.T) {
	m := conflictBranchProtection{}
	outcome, entity, err := EnsureBranchProtection(t.Context(), m, "o", "r",
		provider.CreateBranchProtectionOptions{BranchName: "main", RequiredApprovingReviews: 2})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", outcome)
	}
	if entity != nil {
		t.Fatalf("entity = %+v, want nil（无既有实体）", entity)
	}
}

// conflictDeployKeys：写操作按配置返回错误，deleteErr/addErr 为 nil 即成功。
type conflictDeployKeys struct {
	keys      []*model.DeployKey
	deleteErr error
	addErr    error
}

func (m conflictDeployKeys) ListDeployKeys(context.Context, string, string) ([]*model.DeployKey, error) {
	return m.keys, nil
}

func (m conflictDeployKeys) AddDeployKey(context.Context, string, string, provider.AddDeployKeyOptions) (*model.DeployKey, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	return &model.DeployKey{ID: 99, Title: "replaced"}, nil
}

func (m conflictDeployKeys) DeleteDeployKey(context.Context, string, string, int64) error {
	return m.deleteErr
}

// 删除路径：标题命中但材料不同 → Delete 撞 409 → (OutcomeConflict, 旧键 k, err)。
func TestEnsureDeployKey_DeleteConflictOutcome(t *testing.T) {
	k := &model.DeployKey{ID: 3, Title: "ci", Key: "ssh-old"}
	m := conflictDeployKeys{
		keys:      []*model.DeployKey{k},
		deleteErr: fmt.Errorf("delete: %w", provider.ErrConflict),
	}
	outcome, entity, err := EnsureDeployKey(t.Context(), m, "o", "r",
		provider.AddDeployKeyOptions{Title: "ci", Key: "ssh-new"})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", outcome)
	}
	if entity != k {
		t.Fatalf("entity = %+v, want old key %+v", entity, k)
	}
}

// 替换路径：删除成功后 re-add 撞 409 → (OutcomeConflict, 旧键 k, err)。
func TestEnsureDeployKey_ReplaceAddConflictOutcome(t *testing.T) {
	k := &model.DeployKey{ID: 3, Title: "ci", Key: "ssh-old"}
	m := conflictDeployKeys{
		keys:   []*model.DeployKey{k},
		addErr: fmt.Errorf("add: %w", provider.ErrConflict),
	}
	outcome, entity, err := EnsureDeployKey(t.Context(), m, "o", "r",
		provider.AddDeployKeyOptions{Title: "ci", Key: "ssh-new"})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", outcome)
	}
	if entity != k {
		t.Fatalf("entity = %+v, want old key %+v", entity, k)
	}
}

// 新建路径：无同名键 → Add 撞 409 → (OutcomeConflict, nil, err)（无既有实体）。
func TestEnsureDeployKey_CreateConflictOutcome(t *testing.T) {
	m := conflictDeployKeys{addErr: fmt.Errorf("add: %w", provider.ErrConflict)}
	outcome, entity, err := EnsureDeployKey(t.Context(), m, "o", "r",
		provider.AddDeployKeyOptions{Title: "ci", Key: "ssh-new"})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", outcome)
	}
	if entity != nil {
		t.Fatalf("entity = %+v, want nil（无既有实体）", entity)
	}
}

// nilGetBPM 违约模拟：Get 返回 (nil, nil)（iface 契约禁止的形态）。
type nilGetBPM struct {
	provider.BranchProtectionManager
	created, updated int
}

func (b *nilGetBPM) GetBranchProtection(context.Context, string, string, string) (*model.BranchProtection, error) {
	return nil, nil
}
func (b *nilGetBPM) CreateBranchProtection(_ context.Context, _, _ string, opts provider.CreateBranchProtectionOptions) (*model.BranchProtection, error) {
	b.created++
	return &model.BranchProtection{BranchName: opts.BranchName}, nil
}
func (b *nilGetBPM) UpdateBranchProtection(_ context.Context, _, _, branch string, _ provider.UpdateBranchProtectionOptions) (*model.BranchProtection, error) {
	b.updated++
	return &model.BranchProtection{BranchName: branch}, nil
}

// 违约 (nil,nil) 必须被防御为缺席→Create（#161）：绝不能误走 Update。
func TestEnsureBranchProtection_NilNilGetTreatedAsAbsent(t *testing.T) {
	b := &nilGetBPM{}
	action, entity, err := EnsureBranchProtection(t.Context(), b, "o", "r",
		provider.CreateBranchProtectionOptions{BranchName: "main", RequiredApprovingReviews: 1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if action != EnsureCreated {
		t.Fatalf("want EnsureCreated (nil,nil treated as absent), got %q", action)
	}
	if b.created != 1 || b.updated != 0 {
		t.Fatalf("want exactly 1 create / 0 update, got created=%d updated=%d", b.created, b.updated)
	}
	if entity == nil {
		t.Fatal("want created entity, got nil")
	}
}

// W2 收尾（2B defer）：List 命中同 URL 旧钩子、Delete 成功后 Create 撞 409——
// 冲突携带非 nil 写前快照（prior = matches[0]）。
func TestEnsureWebhook_RecreateConflictCarriesPriorSnapshot(t *testing.T) {
	hook := &model.PlatformWebhook{ID: 7, URL: "https://new", Events: []string{"push"}}
	m := conflictWebhookMgr{hooks: []*model.PlatformWebhook{hook}}
	action, entity, err := EnsureWebhook(context.Background(), m,
		provider.CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://new", Events: []string{"push", "pull_request"}})
	if !provider.IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	if action != OutcomeConflict {
		t.Fatalf("action = %q, want Conflict", action)
	}
	if entity == nil || entity.ID != 7 {
		t.Fatalf("want prior snapshot ID 7, got %+v", entity)
	}
}

// v1.0.6 修复钉子：缺能力错误带 provider.ErrNotImplemented 哨兵（对齐
// ensure_issue 侧惯例——旧实现裸 errors.New 无哨兵链，消费方无法用
// errors.Is 分类「缺能力勿重试」与传输错误）。fakeWebhookManager 自己就
// 实现了 Get/UpdateWebhook（即 WebhookConfigManager），断言走不进——需
// 只实现 WebhookManager 的裁剪 fake。
type webhookMgrNoConfig struct {
	hooks []*model.PlatformWebhook
}

func (f *webhookMgrNoConfig) ListWebhooks(context.Context, string, string) ([]*model.PlatformWebhook, error) {
	return f.hooks, nil
}

func (f *webhookMgrNoConfig) CreateWebhook(_ context.Context, opts provider.CreateWebhookOptions) (*model.PlatformWebhook, error) {
	// 存规范形 events（与真 provider 一致），否则第二次 EnsureWebhook 判
	// drift 走重建路径、到不了被钉的 secret 分支。
	h := &model.PlatformWebhook{ID: int64(len(f.hooks) + 1), URL: opts.URL, Events: canonicalWebhookEvents(opts.Events)}
	f.hooks = append(f.hooks, h)
	return h, nil
}

func (f *webhookMgrNoConfig) ParseWebhookEvent(*http.Request, string) (*model.NormalizedEvent, error) {
	return nil, nil
}

func (f *webhookMgrNoConfig) ValidateWebhookSignature(*http.Request, string) error { return nil }

func (f *webhookMgrNoConfig) DeleteWebhook(context.Context, string, string, int64) error {
	return nil
}

func TestEnsureWebhook_SecretModeMissingConfigManager_Sentinel(t *testing.T) {
	f := &webhookMgrNoConfig{}
	// 先建 hook：EnsureSecretAlwaysUpdate 只在「匹配到既有 hook 走 patch」
	// 分支触发 secret 处理（裸 Create 不经过 ensureWebhookSecret）。
	if _, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{
		Owner: "o", Repo: "r", URL: "https://cb.example/hook",
	}); err != nil {
		t.Fatal(err)
	}
	_, _, err := EnsureWebhook(t.Context(), f, provider.CreateWebhookOptions{
		Owner: "o", Repo: "r", URL: "https://cb.example/hook", Secret: "new",
		EnsureSecretMode: provider.EnsureSecretAlwaysUpdate,
	})
	if err == nil || !errors.Is(err, provider.ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented-classified error, got %v", err)
	}
}
