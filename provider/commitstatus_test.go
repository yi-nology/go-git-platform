package provider

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNormalizeCommitStatusState(t *testing.T) {
	tests := []struct {
		in   string
		want CommitStatusState
	}{
		{"success", CommitStatusSuccess},
		{"SUCCESS", CommitStatusSuccess},
		{"warning", CommitStatusSuccess},
		{"neutral", CommitStatusSuccess},
		{"failure", CommitStatusFailure},
		{"failed", CommitStatusFailure},
		{"error", CommitStatusError},
		{"canceled", CommitStatusCanceled},
		{"cancelled", CommitStatusCanceled},
		{"running", CommitStatusRunning},
		{"created", CommitStatusRunning},
		{"waiting_for_resource", CommitStatusRunning},
		{"pending", CommitStatusPending},
		{"", CommitStatusPending},
		{"some-new-verb", CommitStatusPending},
	}
	for _, tt := range tests {
		if got := NormalizeCommitStatusState(tt.in); got != tt.want {
			t.Errorf("NormalizeCommitStatusState(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCombineCommitStatusStates(t *testing.T) {
	p, r, s, e, c := CommitStatusPending, CommitStatusRunning, CommitStatusSuccess, CommitStatusError, CommitStatusCanceled
	tests := []struct {
		name   string
		states []CommitStatusState
		want   CommitStatusState
	}{
		{"empty is pending", nil, CommitStatusPending},
		{"all success", []CommitStatusState{s, s}, CommitStatusSuccess},
		{"success then pending stays pending", []CommitStatusState{s, p}, CommitStatusPending},
		{"pending then running stays pending", []CommitStatusState{p, r}, CommitStatusPending},
		{"failure dominates others", []CommitStatusState{s, c, CommitStatusFailure, p}, CommitStatusFailure},
		{"error dominates all", []CommitStatusState{s, c, p, e}, CommitStatusError},
		{"canceled dominates running", []CommitStatusState{c, p, r}, CommitStatusCanceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CombineCommitStatusStates(tt.states); got != tt.want {
				t.Errorf("CombineCommitStatusStates(%v) = %q, want %q", tt.states, got, tt.want)
			}
		})
	}
}

// waitFakeProvider embeds the nil Provider interface so only Platform and
// the CommitStatusManager method set are real. Everything is
// mutex-guarded: the wait loop polls from its own goroutine.
type waitFakeProvider struct {
	Provider
	mu       sync.Mutex
	statuses []CommitStatus
	err      error
	calls    int
}

func (f *waitFakeProvider) Platform() Platform { return PlatformGitHub }
func (f *waitFakeProvider) ListCommitStatuses(ctx context.Context, owner, repo, sha string) ([]CommitStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.statuses, f.err
}
func (f *waitFakeProvider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts CommitStatusOptions) error {
	return nil
}

func (f *waitFakeProvider) set(statuses []CommitStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses, f.err = statuses, nil
}

func TestWaitForCommitStatusNotImplemented(t *testing.T) {
	p := struct{ Provider }{}
	_, err := WaitForCommitStatus(context.Background(), p, "o", "r", "sha", WaitOptions{Interval: time.Millisecond})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", err)
	}
}

func TestWaitForCommitStatusConverges(t *testing.T) {
	fake := &waitFakeProvider{}
	fake.set([]CommitStatus{{State: CommitStatusRunning, Context: "ci"}})

	// flip to success (with an unrelated failing context) after a beat
	go func() {
		time.Sleep(20 * time.Millisecond)
		fake.set([]CommitStatus{
			{State: CommitStatusSuccess, Context: "ci"},
			{State: CommitStatusFailure, Context: "other"},
		})
	}()

	state, err := WaitForCommitStatus(context.Background(), fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond, Timeout: 2 * time.Second, Contexts: []string{"ci"}})
	if err != nil {
		t.Fatalf("WaitForCommitStatus() err = %v", err)
	}
	if state != CommitStatusSuccess {
		t.Fatalf("state = %q, want success (other-context failure must be excluded by the filter)", state)
	}
}

func TestWaitForCommitStatusContextFilterWaitsForAllContexts(t *testing.T) {
	fake := &waitFakeProvider{}
	fake.set([]CommitStatus{{State: CommitStatusSuccess, Context: "ci/lint"}})

	_, err := WaitForCommitStatus(context.Background(), fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond, Timeout: 80 * time.Millisecond, Contexts: []string{"ci/lint", "ci/test"}})
	if !errors.Is(err, ErrWaitTimedOut) {
		t.Fatalf("err = %v, want ErrWaitTimedOut (ci/test never reported)", err)
	}
}

func TestWaitForCommitStatusFailureIsTerminal(t *testing.T) {
	fake := &waitFakeProvider{}
	fake.set([]CommitStatus{
		{State: CommitStatusSuccess, Context: "ci/lint"},
		{State: CommitStatusFailure, Context: "ci/test"},
	})

	state, err := WaitForCommitStatus(context.Background(), fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("WaitForCommitStatus() err = %v", err)
	}
	if state != CommitStatusFailure {
		t.Fatalf("state = %q, want failure", state)
	}
}

func TestWaitForCommitStatusTimeout(t *testing.T) {
	fake := &waitFakeProvider{}
	fake.set([]CommitStatus{{State: CommitStatusPending, Context: "ci"}})

	_, err := WaitForCommitStatus(context.Background(), fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond, Timeout: 80 * time.Millisecond})
	if !errors.Is(err, ErrWaitTimedOut) {
		t.Fatalf("err = %v, want ErrWaitTimedOut", err)
	}
}

func TestWaitForCommitStatusReadErrorsKeepPolling(t *testing.T) {
	fake := &waitFakeProvider{err: errors.New("transient")}
	go func() {
		time.Sleep(20 * time.Millisecond)
		fake.set([]CommitStatus{{State: CommitStatusSuccess, Context: "ci"}})
	}()

	state, err := WaitForCommitStatus(context.Background(), fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("WaitForCommitStatus() err = %v", err)
	}
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if state != CommitStatusSuccess || calls < 2 {
		t.Fatalf("state = %q calls = %d, want success after retrying past read errors", state, calls)
	}
}

func TestWaitForCommitStatusContextCancellation(t *testing.T) {
	fake := &waitFakeProvider{}
	fake.set([]CommitStatus{{State: CommitStatusPending, Context: "ci"}})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := WaitForCommitStatus(ctx, fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond, Timeout: 5 * time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestWaitForCommitStatusNegativeTimeoutNoBudget(t *testing.T) {
	// Timeout: -1 disables the budget entirely (WaitOptions): the wait
	// must keep polling until the caller's context ends instead of
	// failing fast with ErrWaitTimedOut. Deleting the timeout<0 branch
	// in WaitForCommitStatus turns this red.
	fake := &waitFakeProvider{}
	fake.set([]CommitStatus{{State: CommitStatusPending, Context: "ci"}})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := WaitForCommitStatus(ctx, fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond, Timeout: -1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled (negative Timeout must keep polling until ctx ends)", err)
	}
	if errors.Is(err, ErrWaitTimedOut) {
		t.Fatalf("err = %v, must not be ErrWaitTimedOut (Timeout: -1 disables the budget)", err)
	}
}

func TestWaitForCommitStatusRerunHistoryTakesLatest(t *testing.T) {
	// CI re-runs append history: ci/test failed once, then succeeded.
	// The newest report must win (GitHub combined-status semantics) —
	// folding the raw list would keep waiting on the stale failure.
	fake := &waitFakeProvider{}
	fake.set([]CommitStatus{
		{State: CommitStatusSuccess, Context: "ci/test"},
		{State: CommitStatusFailure, Context: "ci/test"},
	})

	state, err := WaitForCommitStatus(context.Background(), fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("WaitForCommitStatus() err = %v", err)
	}
	if state != CommitStatusSuccess {
		t.Fatalf("state = %q, want success (latest per context wins)", state)
	}
}

func TestWaitForCommitStatusDuplicateMustNotVoteForMissingContext(t *testing.T) {
	// ci/lint reported twice but ci/test never did: the missing context
	// must keep the wait pending instead of passing the gate.
	fake := &waitFakeProvider{}
	fake.set([]CommitStatus{
		{State: CommitStatusSuccess, Context: "ci/lint"},
		{State: CommitStatusSuccess, Context: "ci/lint"},
	})

	_, err := WaitForCommitStatus(context.Background(), fake, "o", "r", "sha",
		WaitOptions{Interval: 5 * time.Millisecond, Timeout: 80 * time.Millisecond, Contexts: []string{"ci/lint", "ci/test"}})
	if !errors.Is(err, ErrWaitTimedOut) {
		t.Fatalf("err = %v, want ErrWaitTimedOut (ci/test never reported)", err)
	}
}

func TestWaitForCommitStatusNotFoundReturnsImmediately(t *testing.T) {
	fake := &waitFakeProvider{}
	notFound := New(PlatformGitHub, "ListCommitStatuses", 404, "no such commit")
	fake.mu.Lock()
	fake.err = notFound
	fake.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := WaitForCommitStatus(context.Background(), fake, "o", "r", "sha",
			WaitOptions{Interval: 5 * time.Millisecond, Timeout: 5 * time.Second})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound propagated immediately", err)
		}
	}()
	select {
	case <-done:
		// returned before any poll backoff — fast-fail works
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitForCommitStatus kept polling on ErrNotFound")
	}
}

func TestInitWaitTimeoutSemantics(t *testing.T) {
	interval, budget, timeout, _ := initWait(WaitOptions{})
	if interval != DefaultWaitInterval {
		t.Fatalf("interval = %v, want default", interval)
	}
	if budget == nil {
		t.Fatal("zero Timeout must arm the DefaultWaitTimeout budget, got nil")
	}
	select {
	case <-budget:
		t.Fatal("budget fired immediately")
	default:
	}
	if timeout != DefaultWaitTimeout {
		t.Fatalf("timeout = %v, want DefaultWaitTimeout after zero-value backfill", timeout)
	}
	if _, budget2, timeout2, _ := initWait(WaitOptions{Timeout: -1}); budget2 != nil {
		t.Fatal("negative Timeout must disable the budget")
	} else if timeout2 >= 0 {
		t.Fatalf("timeout = %v, want negative Timeout passed through", timeout2)
	}
	if _, budget3, _, w3 := initWait(WaitOptions{Contexts: []string{"a", "a", "b"}}); len(w3) != 2 {
		t.Fatalf("want = %v, want deduped contexts", w3)
	} else if budget3 == nil {
		t.Fatal("explicit Timeout must arm the budget")
	}
}

func TestLatestCommitStatuses(t *testing.T) {
	in := []CommitStatus{
		{State: CommitStatusSuccess, Context: "b"},
		{State: CommitStatusFailure, Context: "a"},
		{State: CommitStatusPending, Context: "b"},
	}
	out := LatestCommitStatuses(in)
	if len(out) != 2 || out[0].Context != "b" || out[0].State != CommitStatusSuccess ||
		out[1].Context != "a" {
		t.Fatalf("LatestCommitStatuses() = %v, want first-occurrence-per-context", out)
	}
}
