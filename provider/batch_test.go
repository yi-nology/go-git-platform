package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/oekit/concurrency/async"
)

func TestBatchResultOrderAndAttribution(t *testing.T) {
	// odd-length paths fail, even-length succeed — order must follow input
	paths := []string{"a", "bb", "ccc"}
	got := GetFileContents(context.Background(), batchFakeProvider{}, "o", "r", "", paths,
		BatchOptions{Concurrency: 3})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, r := range got {
		if r.Key != paths[i] {
			t.Fatalf("result %d key = %q, want %q", i, r.Key, paths[i])
		}
	}
	if got[0].Err == nil || got[2].Err == nil {
		t.Fatalf("odd items must fail: %+v", got)
	}
	if got[1].Err != nil || got[1].Value != "content:bb" {
		t.Fatalf("bb must succeed: %+v", got[1])
	}
}

func TestBatchConcurrencyBound(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	p := countingFakeProvider{hook: func() {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
	}}

	paths := make([]string, 8)
	for i := range paths {
		paths[i] = fmt.Sprintf("f%d", i)
	}
	_ = GetFileContents(context.Background(), p, "o", "r", "", paths, BatchOptions{Concurrency: 2})
	mu.Lock()
	defer mu.Unlock()
	if peak > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", peak)
	}
}

func TestBatchContextCancellationFailsRemaining(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := GetCRs(ctx, batchFakeProvider{}, "o", "r", []string{"1", "2", "3"})
	for i, r := range got {
		if !errors.Is(r.Err, context.Canceled) {
			t.Fatalf("result %d err = %v, want context.Canceled", i, r.Err)
		}
	}
}

// gatingFakeProvider's GetFileContent blocks until release is closed, so a
// batch item can hold a concurrency slot for as long as the test wants.
type gatingFakeProvider struct {
	Provider
	started chan struct{} // signaled (once) when a call is in flight
	release chan struct{} // closed to let in-flight calls finish
}

func (p gatingFakeProvider) Platform() Platform { return PlatformGitHub }

func (p gatingFakeProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	select {
	case p.started <- struct{}{}:
	default:
	}
	<-p.release
	return "content:" + path, nil
}

// TestBatchCancellationInterruptsSemaphoreAcquisition verifies that the loop
// waiting on a busy semaphore is woken by cancellation: with Concurrency 1
// and one in-flight item, canceling must mark the queued items as canceled
// instead of leaving them to run after the in-flight item releases its slot.
func TestBatchCancellationInterruptsSemaphoreAcquisition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := gatingFakeProvider{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}

	paths := []string{"first", "second", "third"}
	done := make(chan []BatchResult[string], 1)
	go func() {
		done <- GetFileContents(ctx, p, "o", "r", "", paths, BatchOptions{Concurrency: 1})
	}()

	// Wait until "first" holds the only semaphore slot.
	<-p.started
	// Let the loop reach (and block on) the semaphore acquisition for "second".
	time.Sleep(20 * time.Millisecond)

	cancel()
	// Release the in-flight item so the batch can return; the queued items
	// must already have been marked canceled by the cancellation above.
	close(p.release)

	select {
	case results := <-done:
		if results[0].Err != nil || results[0].Value != "content:first" {
			t.Fatalf("first = %+v, want in-flight item to succeed", results[0])
		}
		for i, want := range []string{"second", "third"} {
			r := results[i+1]
			if !errors.Is(r.Err, context.Canceled) {
				t.Fatalf("%s: err = %v, want context.Canceled", want, r.Err)
			}
			if r.Value != "" {
				t.Fatalf("%s: value = %q, want empty (item must not run after cancel)", want, r.Value)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("batch did not return after cancellation")
	}
}

// panickingFakeProvider panics on odd-length paths: the batch must convert
// the panic into that item's Err and leave the sibling items unaffected.
type panickingFakeProvider struct{ Provider }

func (panickingFakeProvider) Platform() Platform { return PlatformGitHub }
func (panickingFakeProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	if len(path)%2 == 1 {
		panic("boom: " + path)
	}
	return "content:" + path, nil
}

func TestBatchPanicBecomesItemError(t *testing.T) {
	paths := []string{"a", "bb"}
	got := GetFileContents(context.Background(), panickingFakeProvider{}, "o", "r", "", paths,
		BatchOptions{Concurrency: 2})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// The batch engine delegates panic conversion to async.RunSafeRet, so a
	// panicking item fails with a typed *async.PanicError carrying the
	// original value and the capture stack.
	var pe *async.PanicError
	if got[0].Err == nil || !errors.As(got[0].Err, &pe) {
		t.Fatalf("panicking item err = %v, want typed *async.PanicError", got[0].Err)
	}
	if pe.Stack == nil {
		t.Fatal("PanicError.Stack is empty")
	}
	if got[1].Err != nil || got[1].Value != "content:bb" {
		t.Fatalf("sibling item = %+v, want unaffected success", got[1])
	}
}

// notImplementedProvider satisfies Provider via nil-embedding but has no
// CommitStatusManager (an optional capability, not part of Provider).
type notImplementedProvider struct{ Provider }

func (notImplementedProvider) Platform() Platform { return PlatformGitHub }

func TestBatchNotImplementedCapability(t *testing.T) {
	p := notImplementedProvider{}
	got2 := SetCommitStatuses(context.Background(), p, "o", "r", "sha", []CommitStatusOptions{{Context: "ci"}})
	if len(got2) != 1 || !errors.Is(got2[0].Err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", got2[0].Err)
	}
}

func TestBatchSetCommitStatusesKeys(t *testing.T) {
	p := statusFakeProvider{}
	got := SetCommitStatuses(context.Background(), p, "o", "r", "sha", []CommitStatusOptions{
		{Context: "ci/lint"}, {Context: "ci/test"},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Key != "ci/lint" || got[1].Key != "ci/test" {
		t.Fatalf("keys = %q %q, want contexts", got[0].Key, got[1].Key)
	}
	if got[0].Err != nil || got[1].Err != nil {
		t.Fatalf("unexpected errs: %v %v", got[0].Err, got[1].Err)
	}
}

// --- fakes ---

type batchFakeProvider struct{ Provider }

func (batchFakeProvider) Platform() Platform { return PlatformGitHub }
func (batchFakeProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	if len(path)%2 == 1 {
		return "", New(PlatformGitHub, "GetFileContent", http.StatusNotFound, "no such file")
	}
	return "content:" + path, nil
}
func (batchFakeProvider) GetCR(ctx context.Context, owner, repo, number string) (*ChangeRequest, error) {
	return &ChangeRequest{Number: number}, nil
}

type countingFakeProvider struct {
	Provider
	hook func()
}

func (p countingFakeProvider) Platform() Platform { return PlatformGitHub }
func (p countingFakeProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	p.hook()
	return "", nil
}

type statusFakeProvider struct{ Provider }

func (statusFakeProvider) Platform() Platform { return PlatformGitHub }
func (statusFakeProvider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts CommitStatusOptions) error {
	return nil
}
func (statusFakeProvider) ListCommitStatuses(ctx context.Context, owner, repo, sha string) ([]CommitStatus, error) {
	return nil, nil
}
