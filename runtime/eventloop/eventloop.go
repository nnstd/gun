package eventloop

import (
	"sync"
	"sync/atomic"
)

// activeJob tracks a scheduled timer/interval for cancellation.
// Stored in EventLoop.jobs as jobID -> *activeJob.
type activeJob struct {
	stopFn   func()       // timer.Stop() or stop-channel close
	fired    *atomic.Bool // shared: CAS ensures exactly one decrement path
}

// EventLoop implements a Node.js-style event loop that keeps the process
// alive while timers or I/O handles are active.
type EventLoop struct {
	jobCount    atomic.Int64  // active timers + immediates
	handleCount atomic.Int64  // active I/O (servers)
	jobChan     chan func()   // timer/immediate callbacks — buffered
	wakeupChan  chan struct{} // wake from select when counts change
	nextJobID   atomic.Int64
	jobs        sync.Map // jobID -> *activeJob
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

// RegisterServer increments the I/O handle count, keeping the loop alive.
func (el *EventLoop) RegisterServer() {
	el.handleCount.Add(1)
}

// UnregisterServer decrements the I/O handle count and wakes the loop.
func (el *EventLoop) UnregisterServer() {
	el.handleCount.Add(-1)
	el.wake()
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

// ScheduleMicrotask enqueues fn as a microtask on the event loop.
// The microtask increments jobCount on schedule and decrements it on completion.
// Used by Promise resolution to dispatch .then()/.catch() handlers asynchronously.
func (el *EventLoop) ScheduleMicrotask(fn func()) {
	el.jobCount.Add(1)
	el.jobChan <- func() {
		defer func() { recover() }()
		fn()
		el.jobCount.Add(-1)
		el.wake()
	}
}
