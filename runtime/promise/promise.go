package promise

import (
	"sync"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
)

const (
	statePending   = "pending"
	stateFulfilled = "fulfilled"
	stateRejected  = "rejected"
)

var Promise *jsvalue.JSValue
var (
	promiseStateKey   = jsvalue.PropertyKey(jsvalue.NewSymbol("Promise.state"))
	promiseValueKey   = jsvalue.PropertyKey(jsvalue.NewSymbol("Promise.value"))
	promiseFulfillKey = jsvalue.PropertyKey(jsvalue.NewSymbol("Promise.fulfill_handlers"))
	promiseRejectKey  = jsvalue.PropertyKey(jsvalue.NewSymbol("Promise.reject_handlers"))
)

// promiseInternal holds Go-native concurrency primitives for a promise.
// Stored in promiseInternals sync.Map keyed by *jsvalue.JSValue pointer.
type promiseInternal struct {
	mu      sync.Mutex
	settled chan struct{}
}

var promiseInternals sync.Map // *jsvalue.JSValue -> *promiseInternal

type hookRegistry struct {
	mu      sync.Mutex
	nextID  int
	init    map[int]*jsvalue.JSValue
	before  map[int]*jsvalue.JSValue
	after   map[int]*jsvalue.JSValue
	settled map[int]*jsvalue.JSValue
}

var hooks = hookRegistry{
	init:    map[int]*jsvalue.JSValue{},
	before:  map[int]*jsvalue.JSValue{},
	after:   map[int]*jsvalue.JSValue{},
	settled: map[int]*jsvalue.JSValue{},
}

func registerHook(store map[int]*jsvalue.JSValue, fn *jsvalue.JSValue) func() {
	hooks.mu.Lock()
	defer hooks.mu.Unlock()
	hooks.nextID++
	id := hooks.nextID
	store[id] = fn
	return func() {
		hooks.mu.Lock()
		delete(store, id)
		hooks.mu.Unlock()
	}
}

func RegisterInitHook(fn *jsvalue.JSValue) func()    { return registerHook(hooks.init, fn) }
func RegisterBeforeHook(fn *jsvalue.JSValue) func()  { return registerHook(hooks.before, fn) }
func RegisterAfterHook(fn *jsvalue.JSValue) func()   { return registerHook(hooks.after, fn) }
func RegisterSettledHook(fn *jsvalue.JSValue) func() { return registerHook(hooks.settled, fn) }

func emitHooks(store map[int]*jsvalue.JSValue, args ...*jsvalue.JSValue) {
	hooks.mu.Lock()
	callbacks := make([]*jsvalue.JSValue, 0, len(store))
	for _, fn := range store {
		callbacks = append(callbacks, fn)
	}
	hooks.mu.Unlock()
	for _, fn := range callbacks {
		if fn != nil {
			fn.Call(args...)
		}
	}
}

func newPromiseInternal(p *jsvalue.JSValue) *promiseInternal {
	pi := &promiseInternal{
		settled: make(chan struct{}),
	}
	promiseInternals.Store(p, pi)
	return pi
}

func getPromiseInternal(p *jsvalue.JSValue) *promiseInternal {
	if v, ok := promiseInternals.Load(p); ok {
		return v.(*promiseInternal)
	}
	return nil
}

func defineInternal(p *jsvalue.JSValue, key string, value *jsvalue.JSValue) {
	p.DefineProperty(key, &jsvalue.PropertyDescriptor{
		Value:        value,
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})
}

func getHandlers(p *jsvalue.JSValue, key string) *jsvalue.JSValue {
	list := p.Get(key)
	if !list.IsArray() {
		list = jsvalue.NewArray()
		defineInternal(p, key, list)
	}
	return list
}

func newPendingPromise() *jsvalue.JSValue {
	return newPendingPromiseWithParent(nil)
}

func newPendingPromiseWithParent(parent *jsvalue.JSValue) *jsvalue.JSValue {
	if parent == nil {
		return Promise.Call()
	}
	return Promise.Call(jsvalue.NewUndefined(), parent)
}

func getState(p *jsvalue.JSValue) string {
	if p == nil {
		return ""
	}
	return p.Get(promiseStateKey).String()
}

func IsPromise(v *jsvalue.JSValue) bool {
	return v != nil && jsvalue.InstanceOf(v, Promise).Bool()
}

// Await unwraps a resolved Promise, returning its value directly.
// If the value is not a Promise, returns it unchanged.
// If the Promise is pending, blocks the caller goroutine until settlement.
//
// MUST NOT be called from a microtask handler on the event loop goroutine (deadlock).
func Await(v *jsvalue.JSValue) *jsvalue.JSValue {
	if !IsPromise(v) {
		return v
	}
	state := getState(v)
	if state == stateFulfilled || state == stateRejected {
		return v.Get(promiseValueKey)
	}
	// Pending — block until settled
	pi := getPromiseInternal(v)
	if pi != nil {
		<-pi.settled
	}
	return v.Get(promiseValueKey)
}

// IsRejected reports whether v is a Promise settled in the rejected state.
func IsRejected(v *jsvalue.JSValue) bool {
	return IsPromise(v) && getState(v) == stateRejected
}

func thenMethod(v *jsvalue.JSValue) *jsvalue.JSValue {
	if v == nil {
		return nil
	}
	then := v.Get("then")
	if then == nil || then.TypeString() != "function" {
		return nil
	}
	return then
}

func isThenable(v *jsvalue.JSValue) bool {
	return thenMethod(v) != nil
}

func dispatchHandlers(p *jsvalue.JSValue) {
	if p == nil {
		return
	}
	value := p.Get(promiseValueKey)
	var handlers []*jsvalue.JSValue
	switch getState(p) {
	case stateFulfilled:
		handlers = getHandlers(p, promiseFulfillKey).Array()
	case stateRejected:
		handlers = getHandlers(p, promiseRejectKey).Array()
	}
	// Clear handler arrays immediately (handlers captured by closures)
	defineInternal(p, promiseFulfillKey, jsvalue.NewArray())
	defineInternal(p, promiseRejectKey, jsvalue.NewArray())
	// Schedule each handler as a separate microtask
	for _, h := range handlers {
		if h != nil {
			val := value
			handler := h
			eventloop.Default.ScheduleMicrotask(func() {
				emitHooks(hooks.before, p)
				handler.Call(val)
				emitHooks(hooks.after, p)
			})
		}
	}
}

func fulfill(p *jsvalue.JSValue, value *jsvalue.JSValue) *jsvalue.JSValue {
	if p == nil {
		return p
	}
	// Cycle detection — before mutex to avoid deadlock with reject
	if value == p {
		return reject(p, jsvalue.NewString("Chaining cycle detected for promise"))
	}

	pi := getPromiseInternal(p)
	if pi != nil {
		pi.mu.Lock()
	}
	if getState(p) != statePending {
		if pi != nil {
			pi.mu.Unlock()
		}
		return p
	}

	// Thenable: chain instead of resolving directly
	if isThenable(value) {
		if pi != nil {
			pi.mu.Unlock()
		}
		value.MethodCall("then",
			jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
				v := jsvalue.NewUndefined()
				if len(args) > 0 && args[0] != nil {
					v = args[0]
				}
				fulfill(p, v)
				return jsvalue.NewUndefined()
			}),
			jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
				v := jsvalue.NewUndefined()
				if len(args) > 0 && args[0] != nil {
					v = args[0]
				}
				reject(p, v)
				return jsvalue.NewUndefined()
			}),
		)
		return p
	}

	// State transition under mutex
	defineInternal(p, promiseStateKey, jsvalue.NewString(stateFulfilled))
	defineInternal(p, promiseValueKey, jsvalue.Nullish(value, jsvalue.NewUndefined()))
	// Close settled channel, clean up internals, release mutex
	if pi != nil {
		close(pi.settled)
		promiseInternals.Delete(p)
		pi.mu.Unlock()
	}

	// Dispatch handlers FIRST (increments jobCount via ScheduleMicrotask)
	// THEN decrement for promise settlement (prevents TOCTOU race)
	emitHooks(hooks.settled, p)
	dispatchHandlers(p)
	if pi != nil {
		eventloop.Default.SettlePromise()
	}

	return p
}

func reject(p *jsvalue.JSValue, reason *jsvalue.JSValue) *jsvalue.JSValue {
	if p == nil {
		return p
	}

	pi := getPromiseInternal(p)
	if pi != nil {
		pi.mu.Lock()
	}
	if getState(p) != statePending {
		if pi != nil {
			pi.mu.Unlock()
		}
		return p
	}

	defineInternal(p, promiseStateKey, jsvalue.NewString(stateRejected))
	defineInternal(p, promiseValueKey, jsvalue.Nullish(reason, jsvalue.NewUndefined()))
	// Close settled channel, clean up internals, release mutex
	if pi != nil {
		close(pi.settled)
		promiseInternals.Delete(p)
		pi.mu.Unlock()
	}

	// Dispatch handlers FIRST, THEN settle
	emitHooks(hooks.settled, p)
	dispatchHandlers(p)
	if pi != nil {
		eventloop.Default.SettlePromise()
	}

	return p
}

func init() {
	Promise = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var parent *jsvalue.JSValue
		if len(args) > 1 && args[1] != nil {
			parent = args[1]
		}
		defineInternal(this, promiseStateKey, jsvalue.NewString(statePending))
		defineInternal(this, promiseValueKey, jsvalue.NewUndefined())
		defineInternal(this, promiseFulfillKey, jsvalue.NewArray())
		defineInternal(this, promiseRejectKey, jsvalue.NewArray())

		// Initialize concurrency primitives
		newPromiseInternal(this)
		// Liveness: keep event loop alive while this promise is pending
		eventloop.Default.TrackPromise()
		emitHooks(hooks.init, this, jsvalue.Nullish(parent, jsvalue.NewUndefined()))

		if len(args) > 0 && args[0] != nil && args[0].TypeString() == "function" {
			resolve := jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				value := jsvalue.NewUndefined()
				if len(inner) > 0 && inner[0] != nil {
					value = inner[0]
				}
				return fulfill(this, value)
			})
			rejectFn := jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				value := jsvalue.NewUndefined()
				if len(inner) > 0 && inner[0] != nil {
					value = inner[0]
				}
				return reject(this, value)
			})
			func() {
				defer func() {
					if r := recover(); r != nil {
						reject(this, jsvalue.From(r))
					}
				}()
				args[0].Call(resolve, rejectFn)
			}()
		}
		return nil
	}, nil)

	Promise.Get("prototype").Set("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newPendingPromise()
		}
		self := args[0]
		var onFulfilled *jsvalue.JSValue
		var onRejected *jsvalue.JSValue
		if len(args) > 1 {
			onFulfilled = args[1]
		}
		if len(args) > 2 {
			onRejected = args[2]
		}
		next := newPendingPromiseWithParent(self)

		fulfillHandler := jsvalue.NewFunction(func(callArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			value := jsvalue.NewUndefined()
			if len(callArgs) > 0 && callArgs[0] != nil {
				value = callArgs[0]
			}
			if onFulfilled == nil || onFulfilled.TypeString() != "function" {
				fulfill(next, value)
				return jsvalue.NewUndefined()
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						reject(next, jsvalue.From(r))
					}
				}()
				fulfill(next, onFulfilled.Call(value))
			}()
			return jsvalue.NewUndefined()
		})

		rejectHandler := jsvalue.NewFunction(func(callArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			reason := jsvalue.NewUndefined()
			if len(callArgs) > 0 && callArgs[0] != nil {
				reason = callArgs[0]
			}
			if onRejected == nil || onRejected.TypeString() != "function" {
				reject(next, reason)
				return jsvalue.NewUndefined()
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						reject(next, jsvalue.From(r))
					}
				}()
				fulfill(next, onRejected.Call(reason))
			}()
			return jsvalue.NewUndefined()
		})

		// Atomically check state + push/schedule under promiseInternal
		// mutex.  Prevents TOCTOU race where fulfill() on another goroutine
		// transitions state and clears handler arrays between the getState
		// read and the handler push.
		pi := getPromiseInternal(self)
		if pi != nil {
			pi.mu.Lock()
		}
		switch getState(self) {
		case statePending:
			getHandlers(self, promiseFulfillKey).MethodCall("push", fulfillHandler)
			getHandlers(self, promiseRejectKey).MethodCall("push", rejectHandler)
		case stateFulfilled:
			val := self.Get(promiseValueKey)
			h := fulfillHandler
			eventloop.Default.ScheduleMicrotask(func() {
				emitHooks(hooks.before, self)
				h.Call(val)
				emitHooks(hooks.after, self)
			})
		case stateRejected:
			reason := self.Get(promiseValueKey)
			h := rejectHandler
			eventloop.Default.ScheduleMicrotask(func() {
				emitHooks(hooks.before, self)
				h.Call(reason)
				emitHooks(hooks.after, self)
			})
		}
		if pi != nil {
			pi.mu.Unlock()
		}
		return next
	}).MarkAsMethod())

	Promise.Get("prototype").Set("catch", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newPendingPromise()
		}
		self := args[0]
		var onRejected *jsvalue.JSValue
		if len(args) > 1 {
			onRejected = args[1]
		}
		return self.MethodCall("then", jsvalue.NewUndefined(), onRejected)
	}).MarkAsMethod())

	Promise.Get("prototype").Set("finally", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newPendingPromise()
		}
		self := args[0]
		var callback *jsvalue.JSValue
		if len(args) > 1 {
			callback = args[1]
		}
		next := newPendingPromiseWithParent(self)

		fulfillHandler := jsvalue.NewFunction(func(callArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			value := jsvalue.NewUndefined()
			if len(callArgs) > 0 && callArgs[0] != nil {
				value = callArgs[0]
			}
			if callback != nil && callback.TypeString() == "function" {
				callback.Call()
			}
			fulfill(next, value)
			return jsvalue.NewUndefined()
		})
		rejectHandler := jsvalue.NewFunction(func(callArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			reason := jsvalue.NewUndefined()
			if len(callArgs) > 0 && callArgs[0] != nil {
				reason = callArgs[0]
			}
			if callback != nil && callback.TypeString() == "function" {
				callback.Call()
			}
			reject(next, reason)
			return jsvalue.NewUndefined()
		})
		// Same atomic state check as .then() — prevents TOCTOU race.
		pi := getPromiseInternal(self)
		if pi != nil {
			pi.mu.Lock()
		}
		switch getState(self) {
		case statePending:
			getHandlers(self, promiseFulfillKey).MethodCall("push", fulfillHandler)
			getHandlers(self, promiseRejectKey).MethodCall("push", rejectHandler)
		case stateFulfilled:
			val := self.Get(promiseValueKey)
			h := fulfillHandler
			eventloop.Default.ScheduleMicrotask(func() {
				emitHooks(hooks.before, self)
				h.Call(val)
				emitHooks(hooks.after, self)
			})
		case stateRejected:
			reason := self.Get(promiseValueKey)
			h := rejectHandler
			eventloop.Default.ScheduleMicrotask(func() {
				emitHooks(hooks.before, self)
				h.Call(reason)
				emitHooks(hooks.after, self)
			})
		}
		if pi != nil {
			pi.mu.Unlock()
		}
		return next
	}).MarkAsMethod())

	Promise.Set("resolve", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		p := newPendingPromise()
		if len(args) > 0 {
			return fulfill(p, args[0])
		}
		return fulfill(p, jsvalue.NewUndefined())
	}))
	Promise.Set("reject", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		p := newPendingPromise()
		if len(args) > 0 {
			return reject(p, args[0])
		}
		return reject(p, jsvalue.NewUndefined())
	}))
	Promise.Set("all", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		next := newPendingPromise()
		if len(args) == 0 || args[0] == nil || !args[0].IsArray() {
			return fulfill(next, jsvalue.NewArray())
		}
		items := args[0].Array()
		if len(items) == 0 {
			return fulfill(next, jsvalue.NewArray())
		}
		results := make([]*jsvalue.JSValue, len(items))
		remaining := len(items)
		for i, item := range items {
			index := i
			resolveOne := jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				value := jsvalue.NewUndefined()
				if len(inner) > 0 && inner[0] != nil {
					value = inner[0]
				}
				results[index] = value
				remaining--
				if remaining == 0 {
					fulfill(next, jsvalue.NewArray(results...))
				}
				return jsvalue.NewUndefined()
			})
			rejectOne := jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				value := jsvalue.NewUndefined()
				if len(inner) > 0 && inner[0] != nil {
					value = inner[0]
				}
				reject(next, value)
				return jsvalue.NewUndefined()
			})
			if IsPromise(item) {
				item.MethodCall("then", resolveOne, rejectOne)
			} else {
				resolveOne.Call(item)
			}
		}
		return next
	}))
	Promise.Set("race", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		next := newPendingPromise()
		if len(args) == 0 || args[0] == nil || !args[0].IsArray() {
			return next
		}
		for _, item := range args[0].Array() {
			resolveOne := jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				value := jsvalue.NewUndefined()
				if len(inner) > 0 && inner[0] != nil {
					value = inner[0]
				}
				fulfill(next, value)
				return jsvalue.NewUndefined()
			})
			rejectOne := jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				value := jsvalue.NewUndefined()
				if len(inner) > 0 && inner[0] != nil {
					value = inner[0]
				}
				reject(next, value)
				return jsvalue.NewUndefined()
			})
			if IsPromise(item) {
				item.MethodCall("then", resolveOne, rejectOne)
			} else {
				resolveOne.Call(item)
			}
		}
		return next
	}))
}

// ResolvedPromise creates a promise in the fulfilled state without allocating
// handler arrays or concurrency primitives. Use when the resolution value is
// known at construction time. No event loop liveness tracking.
func ResolvedPromise(value *jsvalue.JSValue) *jsvalue.JSValue {
	p := jsvalue.NewObject()
	p.SetPrototype(Promise.Get("prototype"))
	defineInternal(p, promiseStateKey, jsvalue.NewString(stateFulfilled))
	defineInternal(p, promiseValueKey, value)
	return p
}
