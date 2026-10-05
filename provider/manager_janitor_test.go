package provider

import (
	"context"
	"testing"
	"time"
)

// TestStartJanitor_RestartAfterContextCancel guards against the janitor
// goroutine dying via ctx cancellation and leaving janitorStop non-nil,
// which made every subsequent StartJanitor a silent no-op ("already
// running") for the lifetime of the Manager.
func TestStartJanitor_RestartAfterContextCancel(t *testing.T) {
	m := NewManager(time.Minute)

	ctx1, cancel1 := context.WithCancel(context.Background())
	m.StartJanitor(ctx1, time.Hour) // interval never fires; lifecycle only

	m.jan.mu.Lock()
	done1 := m.jan.done
	stop1 := m.jan.stop
	m.jan.mu.Unlock()
	if stop1 == nil || done1 == nil {
		t.Fatal("expected janitor state to be set while running")
	}

	// Cancel and wait for the goroutine to actually exit.
	cancel1()
	select {
	case <-done1:
	case <-time.After(2 * time.Second):
		t.Fatal("janitor goroutine did not exit after ctx cancellation")
	}

	// Exit must have reset the state (before closing done).
	m.jan.mu.Lock()
	stopAfter := m.jan.stop
	doneAfter := m.jan.done
	m.jan.mu.Unlock()
	if stopAfter != nil || doneAfter != nil {
		t.Errorf("expected janitor state reset after exit, stop=%v done=%v", stopAfter, doneAfter)
	}

	// Restart must launch a brand-new goroutine, not silently no-op.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	m.StartJanitor(ctx2, time.Hour)

	m.jan.mu.Lock()
	stop2 := m.jan.stop
	done2 := m.jan.done
	m.jan.mu.Unlock()
	if stop2 == nil || done2 == nil {
		t.Fatal("expected StartJanitor to relaunch after a ctx-cancelled run")
	}
	if done2 == done1 {
		t.Error("expected a fresh done channel for the restarted janitor")
	}

	// Stop must work on the restarted janitor, be idempotent, and leave no
	// stale state behind.
	m.Stop()
	m.jan.mu.Lock()
	stopFinal := m.jan.stop
	doneFinal := m.jan.done
	m.jan.mu.Unlock()
	if stopFinal != nil || doneFinal != nil {
		t.Errorf("expected Stop to clear janitor state, stop=%v done=%v", stopFinal, doneFinal)
	}
	m.Stop() // double Stop must stay safe

	// Double Start while genuinely running must remain a no-op.
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel3()
	m.StartJanitor(ctx3, time.Hour)
	m.jan.mu.Lock()
	done3 := m.jan.done
	m.jan.mu.Unlock()
	m.StartJanitor(ctx3, time.Hour)
	m.jan.mu.Lock()
	done3b := m.jan.done
	m.jan.mu.Unlock()
	if done3b != done3 {
		t.Error("second StartJanitor while running must not replace the goroutine")
	}
	m.Stop()
}

// TestStopJanitor_ConcurrentWithContextCancel races Stop against a ctx
// cancellation to shake out state-reset races under -race.
func TestStopJanitor_ConcurrentWithContextCancel(t *testing.T) {
	for i := 0; i < 20; i++ {
		m := NewManager(time.Minute)
		ctx, cancel := context.WithCancel(context.Background())
		m.StartJanitor(ctx, time.Hour)

		done := make(chan struct{})
		go func() {
			defer close(done)
			cancel()
		}()
		m.Stop() // may run before, during, or after the goroutine's exit
		<-done
		<-ctx.Done()

		// Whatever the interleaving, the manager must be reusable.
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()
		m.StartJanitor(ctx2, time.Hour)
		m.jan.mu.Lock()
		running := m.jan.stop != nil
		m.jan.mu.Unlock()
		if !running {
			t.Fatal("expected janitor to be running after restart")
		}
		m.Stop()
	}
}
