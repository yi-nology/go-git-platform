package provider

import (
	"context"
	"sync"
	"time"

	"github.com/oekit/concurrency/async"
)

// janitor is the reusable background-cleanup runner behind Manager: it
// invokes a cleanup callback on a ticker until the context is cancelled or
// stop is called. Extracted from Manager so the goroutine lifecycle lives in
// one small component with explicit state instead of three fields folded
// into the manager struct.
type janitor struct {
	cleanup func()

	mu   sync.Mutex
	stop chan struct{} // non-nil while running
	done chan struct{} // closed when the goroutine exits
}

// start launches the janitor goroutine. Calling start more than once without
// an intervening stop (or without the previous goroutine having exited) is a
// no-op.
func (j *janitor) start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.stop != nil {
		return // already running
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	j.stop = stopCh
	j.done = doneCh
	// async.GoSafe recovers a panicking cleanup: the defers below still run
	// (done closed, state cleared), so the janitor stops cleanly instead of
	// crashing the host process, and start can relaunch it.
	async.GoSafe(func() {
		defer close(doneCh)
		// Reset the janitor state before signalling exit so start can
		// relaunch after this goroutine dies (e.g. via ctx cancellation).
		// Runs before close(doneCh): once doneCh is closed, the state is
		// already cleared. stop() may have cleared the fields already; the
		// identity checks keep that case a no-op.
		defer func() {
			j.mu.Lock()
			defer j.mu.Unlock()
			if j.stop == stopCh {
				j.stop = nil
			}
			if j.done == doneCh {
				j.done = nil
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				j.cleanup()
			}
		}
	})
}

// stopAndWait halts the janitor and blocks until it has exited.
func (j *janitor) stopAndWait() {
	j.mu.Lock()
	if j.stop == nil {
		j.mu.Unlock()
		return
	}
	close(j.stop)
	j.stop = nil
	done := j.done
	j.mu.Unlock()
	if done != nil {
		<-done
	}
	j.mu.Lock()
	j.done = nil
	j.mu.Unlock()
}
