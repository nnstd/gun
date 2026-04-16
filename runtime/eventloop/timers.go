package eventloop

import (
	"sync/atomic"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// scheduleTimeout schedules a callback after ms milliseconds on this event loop.
func (el *EventLoop) scheduleTimeout(ms int, fn func(), id int64) {
	fired := &atomic.Bool{}
	j := &activeJob{fired: fired}
	el.jobs.Store(id, j)
	el.jobCount.Add(1)

	timer := time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
		if fired.CompareAndSwap(false, true) {
			el.jobChan <- func() {
				defer func() { recover() }()
				fn()
			}
			el.jobCount.Add(-1)
			el.wake()
		}
	})
	j.stopFn = func() { timer.Stop() }
}

// scheduleInterval schedules a repeating callback every ms milliseconds on this event loop.
func (el *EventLoop) scheduleInterval(ms int, fn func(), id int64) {
	fired := &atomic.Bool{}
	stopCh := make(chan struct{})
	j := &activeJob{
		stopFn: func() { close(stopCh) },
		fired:  fired,
	}
	el.jobs.Store(id, j)
	el.jobCount.Add(1)

	go func() {
		ticker := time.NewTicker(time.Duration(ms) * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				el.jobChan <- func() {
					defer func() { recover() }()
					fn()
				}
			}
		}
	}()
}

// scheduleImmediate schedules a callback to fire on the next event loop tick.
func (el *EventLoop) scheduleImmediate(fn func()) {
	id := el.nextJobID.Add(1)
	fired := &atomic.Bool{}
	j := &activeJob{fired: fired}
	el.jobs.Store(id, j)
	el.jobCount.Add(1)

	el.jobChan <- func() {
		if fired.CompareAndSwap(false, true) {
			defer func() { recover() }()
			fn()
			el.jobCount.Add(-1)
			el.wake()
		}
	}
}

// cancelJob cancels a scheduled job by ID. Only decrements jobCount if the
// job hasn't fired yet (CAS-protected).
func (el *EventLoop) cancelJob(id int64) {
	if v, ok := el.jobs.LoadAndDelete(id); ok {
		j := v.(*activeJob)
		if j.stopFn != nil {
			j.stopFn()
		}
		if j.fired.CompareAndSwap(false, true) {
			el.jobCount.Add(-1)
			el.wake()
		}
	}
}

// --- Exported JSValue-wrapped functions (operate on Default) ---

// SetTimeout schedules a callback after delay milliseconds.
// JS: setTimeout(callback, delay, ...args)
func SetTimeout(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 {
		return jsvalue.NewUndefined()
	}
	callback := args[0]
	delay := 0
	if len(args) > 1 {
		delay = int(args[1].Number())
	}
	if delay < 1 {
		delay = 1
	}

	var passArgs []*jsvalue.JSValue
	if len(args) > 2 {
		passArgs = args[2:]
	}

	id := Default.nextJobID.Add(1)
	Default.scheduleTimeout(delay, func() { callback.Call(passArgs...) }, id)
	return jsvalue.NewNumber(float64(id))
}

// SetInterval schedules a repeating callback every delay milliseconds.
// JS: setInterval(callback, delay, ...args)
func SetInterval(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 {
		return jsvalue.NewUndefined()
	}
	callback := args[0]
	delay := 0
	if len(args) > 1 {
		delay = int(args[1].Number())
	}
	if delay < 1 {
		delay = 1
	}

	var passArgs []*jsvalue.JSValue
	if len(args) > 2 {
		passArgs = args[2:]
	}

	id := Default.nextJobID.Add(1)
	Default.scheduleInterval(delay, func() { callback.Call(passArgs...) }, id)
	return jsvalue.NewNumber(float64(id))
}

// SetImmediate schedules a callback to fire on the next event loop tick.
// JS: setImmediate(callback, ...args)
func SetImmediate(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 {
		return jsvalue.NewUndefined()
	}
	callback := args[0]

	var passArgs []*jsvalue.JSValue
	if len(args) > 1 {
		passArgs = args[1:]
	}

	Default.scheduleImmediate(func() { callback.Call(passArgs...) })
	return jsvalue.NewUndefined()
}

// ClearTimeout cancels a timeout.
// JS: clearTimeout(id)
func ClearTimeout(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) >= 1 {
		Default.cancelJob(int64(args[0].Number()))
	}
	return jsvalue.NewUndefined()
}

// ClearInterval cancels an interval.
// JS: clearInterval(id)
func ClearInterval(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) >= 1 {
		Default.cancelJob(int64(args[0].Number()))
	}
	return jsvalue.NewUndefined()
}

// ClearImmediate cancels an immediate.
// JS: clearImmediate(id)
func ClearImmediate(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) >= 1 {
		Default.cancelJob(int64(args[0].Number()))
	}
	return jsvalue.NewUndefined()
}
