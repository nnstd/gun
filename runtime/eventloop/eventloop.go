package eventloop

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/nnstd/gun/runtime/profile"
)

// jsMu serializes all JS execution. Goroutines acquire this lock before
// running any JSValue operations, replacing channel-based scheduling for
// the common request path. Mutex fast path is an atomic CAS — orders of
// magnitude cheaper than pthread_cond_signal/wait round-trips.
var jsMu sync.Mutex

// activeJob tracks a scheduled timer/interval for cancellation.
// Stored in EventLoop.jobs as jobID -> *activeJob.
type activeJob struct {
	stopFn  func()       // timer.Stop() or stop-channel close
	fired   *atomic.Bool // shared: CAS ensures exactly one decrement path
	context *profile.ContextToken
}

// EventLoop implements a Node.js-style event loop that keeps the process
// alive while timers or I/O handles are active.
type EventLoop struct {
	jobCount    atomic.Int64  // active timers + immediates
	handleCount atomic.Int64  // active I/O handles
	jobChan     chan func()   // timer/immediate callbacks — buffered
	wakeupChan  chan struct{} // wake from select when counts change
	nextJobID   atomic.Int64
	jobs        sync.Map // jobID -> *activeJob
	activeCtx   atomic.Value // stores context.Context for OTel span propagation
}

// Default is the package-level singleton event loop.
var Default = &EventLoop{
	jobChan:    make(chan func(), 4096),
	wakeupChan: make(chan struct{}, 1),
}

// Run blocks while timers or I/O handles are active. Returns when idle.
func (el *EventLoop) Run() {
	for el.jobCount.Load()+el.handleCount.Load() > 0 {
		select {
		case job := <-el.jobChan:
			job()
		case <-el.wakeupChan:
			// counts changed, recheck condition
		}
	}
}

// wake sends a non-blocking signal to wakeupChan.
func (el *EventLoop) wake() {
	select {
	case el.wakeupChan <- struct{}{}:
	default:
	}
}

// RegisterHandle increments the I/O handle count, keeping the loop alive.
func (el *EventLoop) RegisterHandle() {
	el.handleCount.Add(1)
	el.wake()
}

// UnregisterHandle decrements the I/O handle count and wakes the loop.
func (el *EventLoop) UnregisterHandle() {
	el.handleCount.Add(-1)
	el.wake()
}

// RegisterServer is a backward-compatible alias for RegisterHandle.
func (el *EventLoop) RegisterServer() {
	el.RegisterHandle()
}

// SetActiveContext stores a context.Context for OTel span propagation.
func (el *EventLoop) SetActiveContext(ctx context.Context) {
	el.activeCtx.Store(&ctx)
}

// ClearActiveContext removes the stored span context.
func (el *EventLoop) ClearActiveContext() {
	el.activeCtx.Store((*context.Context)(nil))
}

// ActiveContext retrieves the stored context, or context.Background() if none.
func (el *EventLoop) ActiveContext() context.Context {
	if v := el.activeCtx.Load(); v != nil {
		if p, ok := v.(*context.Context); ok && p != nil {
			return *p
		}
	}
	return context.Background()
}

// UnregisterServer is a backward-compatible alias for UnregisterHandle.
func (el *EventLoop) UnregisterServer() {
	el.UnregisterHandle()
}

// TrackPromise increments the job count to keep the event loop alive while a promise is pending.
func (el *EventLoop) TrackPromise() {
	el.jobCount.Add(1)
}

// SettlePromise decrements the job count for a settled promise and wakes the loop.
func (el *EventLoop) SettlePromise() {
	el.jobCount.Add(-1)
	el.wake()
}

// ScheduleMicrotask enqueues fn to run on the event loop goroutine as a microtask.
// All JS-visible code (JSValue .Call(), .Get(), .Set(), .MethodCall()) MUST run via
// this method or ScheduleCallback. I/O goroutines must never call JSValue methods
// directly — they should capture Go-native data, schedule fn here, and block on a
// channel until fn completes. Used by Promise resolution to dispatch handlers.
func (el *EventLoop) ScheduleMicrotask(fn func()) {
	el.scheduleCallback(profile.CaptureContext(), fn)
}

// ScheduleCallback enqueues fn to run on the event loop goroutine.
// All JS-visible code (JSValue .Call(), .Get(), .Set(), .MethodCall()) MUST run via
// this method or ScheduleMicrotask. I/O goroutines must never call JSValue methods
// directly — they should capture Go-native data, schedule fn here, and block on a
// Go channel until fn completes.
func (el *EventLoop) ScheduleCallback(fn func()) {
	el.scheduleCallback(profile.CaptureContext(), fn)
}

func (el *EventLoop) scheduleCallback(ctx *profile.ContextToken, fn func()) {
	el.jobCount.Add(1)
	if ctx == nil {
		el.jobChan <- func() {
			jsMu.Lock()
			defer jsMu.Unlock()
			defer func() { recover() }()
			fn()
			el.jobCount.Add(-1)
			el.wake()
		}
	} else {
		el.jobChan <- func() {
			jsMu.Lock()
			defer jsMu.Unlock()
			defer func() { recover() }()
			profile.WithContext(ctx, fn)
			el.jobCount.Add(-1)
			el.wake()
		}
	}
}

// RunSync locks the JS execution mutex, runs fn, and unlocks.
// Use for the common request path where a goroutine needs to run JS code
// without the overhead of channel-based scheduling. The mutex ensures
// only one goroutine executes JS at a time (single-threaded JS semantics).
func (el *EventLoop) RunSync(fn func()) {
	jsMu.Lock()
	defer jsMu.Unlock()
	fn()
}

// Pump starts a background goroutine that drains the event loop's job channel.
// Unlike Run(), Pump never exits and does not manage handle/job lifecycle.
// It is intended for use in tests where the process terminates after all tests complete.
func (el *EventLoop) Pump() {
	go func() {
		for fn := range el.jobChan {
			fn()
		}
	}()
}
