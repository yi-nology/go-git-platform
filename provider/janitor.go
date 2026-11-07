package provider

import (
	"context"
	"sync"
	"time"

	"github.com/oekit/concurrency/async"
	"github.com/oekit/concurrency/loop"
)

// janitor is the reusable background-cleanup runner behind Manager: it
// invokes a cleanup callback on a cadence until the context is cancelled or
// stop is called. Extracted from Manager so the goroutine lifecycle lives in
// one small component with explicit state instead of three fields folded
// into the manager struct.
//
// 轮次纪律委托 oekit loop.Run（concurrency v0.28，IntervalAfterCompletion
// 模式）：单轮 cleanup panic 被收编、循环存活（旧 GoSafeLabeled+ticker
// 形态下 panic 后循环静默死亡、清理永久停摆直到手动 start——正是上游
// 文档点名的「消费 worker 停摆=丢消息」失败形态）；间隔从「本轮结束」
// 起算，长清扫不重叠不双发；FirstDelay=interval 保持旧形态的首扫时机。
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

	runCtx, cancel := context.WithCancel(ctx)
	l := loop.Run(runCtx, loop.Config{
		Interval:     interval,
		IntervalMode: loop.IntervalAfterCompletion,
		FirstDelay:   interval,
		Name:         "janitor",
	}, func(context.Context) {
		j.cleanup()
	})
	async.GoSafeLabeled("janitor", func() {
		defer close(doneCh)
		// Reset the janitor state before signalling exit so start can
		// relaunch after this goroutine dies. Runs before close(doneCh):
		// once doneCh is closed, the state is already cleared. stop() may
		// have cleared the fields already; the identity checks keep that
		// case a no-op.
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
		// stopCh 或 ctx 任一触发即取消 runCtx：loop 拦下一轮、在飞轮次
		// 跑完（轮间等待可被取消打断），排水后再关 doneCh——stopAndWait
		// 的「等到退出」契约不变。
		select {
		case <-ctx.Done():
		case <-stopCh:
		}
		cancel()
		l.Wait()
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
