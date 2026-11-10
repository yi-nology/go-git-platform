package provider

import (
	"context"
	"sync"
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

// TestStopAndWait_StartDuringStopWindowKeepsReplacementDone reproduces
// issue #69: while a real stopAndWait is blocked on the old goroutine's done
// channel, a concurrent start may launch a replacement janitor and store its
// own done channel. The pre-fix tail cleared j.done unconditionally, wiping
// the replacement's state, so the next Stop returned without waiting for the
// replacement to exit. Each iteration drives that interleaving against a
// real stopAndWait call; 200 iterations make the microsecond window reliably
// reproducible under any scheduler.
func TestStopAndWait_StartDuringStopWindowKeepsReplacementDone(t *testing.T) {
	for iter := 0; iter < 200; iter++ {
		j := &janitor{cleanup: func() {}}
		ctx1, cancel1 := context.WithCancel(context.Background())
		j.start(ctx1, time.Hour)

		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			j.stopAndWait()
		}()

		// Wait until stopAndWait has cleared the stop channel — it is now
		// blocked on the old goroutine's done channel, the exact window from
		// issue #69.
		deadline := time.Now().Add(2 * time.Second)
		for {
			j.mu.Lock()
			cleared := j.stop == nil
			j.mu.Unlock()
			if cleared {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("stopAndWait never entered its wait window")
			}
		}

		// A concurrent start inside the window sees stop==nil and launches a
		// replacement janitor.
		ctx2, cancel2 := context.WithCancel(context.Background())
		j.start(ctx2, time.Hour)
		j.mu.Lock()
		done2 := j.done
		j.mu.Unlock()
		if done2 == nil {
			t.Fatal("expected the window-time start to launch a replacement janitor")
		}

		// Let the old goroutine finish so the blocked stopAndWait resumes.
		cancel1()
		<-stopped

		// The replacement must still be intact: stop set and its own done
		// channel preserved. The pre-fix tail wiped done to nil here.
		j.mu.Lock()
		stopAfter, doneAfter := j.stop, j.done
		j.mu.Unlock()
		if stopAfter == nil {
			t.Fatalf("iteration %d: replacement janitor's stop channel lost", iter)
		}
		if doneAfter != done2 {
			t.Fatalf("iteration %d: stopAndWait wiped the replacement's done channel: got %v, want %v (issue #69)", iter, doneAfter, done2)
		}

		// The replacement must still honor the blocking-Stop contract: this
		// stopAndWait waits for the goroutine to actually exit, so done2 is
		// closed by the time it returns. Under the pre-fix wipe the channel
		// is lost and Stop returns before the goroutine exits.
		j.stopAndWait()
		select {
		case <-done2:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: Stop returned without waiting for the replacement janitor to exit", iter)
		}
		j.mu.Lock()
		stop, done := j.stop, j.done
		j.mu.Unlock()
		if stop != nil || done != nil {
			t.Fatalf("iteration %d: expected clean state after Stop, stop=%v done=%v", iter, stop, done)
		}
		cancel1()
		cancel2()
	}
}

// TestStopAndWait_ConcurrentStopStartCycles hammers start/stopAndWait from
// multiple goroutines under -race; the final phase pins the observable
// contract: after a Stop returns, the janitor's goroutine has actually
// exited (its done channel is closed) — impossible if any racing stopAndWait
// wiped a live janitor's done channel.
func TestStopAndWait_ConcurrentStopStartCycles(t *testing.T) {
	for iter := 0; iter < 50; iter++ {
		j := &janitor{cleanup: func() {}}
		ctx, cancel := context.WithCancel(context.Background())
		j.start(ctx, time.Hour)

		var wg sync.WaitGroup
		wg.Add(4)
		for g := 0; g < 2; g++ {
			go func() {
				defer wg.Done()
				for k := 0; k < 5; k++ {
					j.stopAndWait()
				}
			}()
		}
		for g := 0; g < 2; g++ {
			go func() {
				defer wg.Done()
				for k := 0; k < 5; k++ {
					j.start(ctx, time.Hour)
				}
			}()
		}
		wg.Wait()

		// Drain whatever is running, then pin the contract on a fresh run.
		j.stopAndWait()
		j.start(ctx, time.Hour)
		j.mu.Lock()
		done := j.done
		j.mu.Unlock()
		j.stopAndWait()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Stop returned without waiting for the janitor goroutine to exit (done channel lost)")
		}
		j.mu.Lock()
		stop, doneAfter := j.stop, j.done
		j.mu.Unlock()
		if stop != nil || doneAfter != nil {
			t.Fatalf("expected clean state after Stop, stop=%v done=%v", stop, doneAfter)
		}
		cancel()
	}
}
