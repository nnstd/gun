package promise

import jsvalue "github.com/nnstd/gun/runtime/builtin"

const (
	statePending   = "pending"
	stateFulfilled = "fulfilled"
	stateRejected  = "rejected"
)

const (
	promiseStateKey    = "__promise_state"
	promiseValueKey    = "__promise_value"
	promiseFulfillKey  = "__promise_fulfill_handlers"
	promiseRejectKey   = "__promise_reject_handlers"
)

var Promise *jsvalue.JSValue

func getHandlers(p *jsvalue.JSValue, key string) *jsvalue.JSValue {
	list := p.Get(key)
	if !list.IsArray() {
		list = jsvalue.NewArray()
		p.Set(key, list)
	}
	return list
}

func newPendingPromise() *jsvalue.JSValue {
	return Promise.Call()
}

func getState(p *jsvalue.JSValue) string {
	if p == nil {
		return ""
	}
	return p.Get(promiseStateKey).String()
}

func isPromise(v *jsvalue.JSValue) bool {
	return v != nil && jsvalue.InstanceOf(v, Promise).Bool()
}

func dispatchHandlers(p *jsvalue.JSValue) {
	if p == nil {
		return
	}
	value := p.Get(promiseValueKey)
	switch getState(p) {
	case stateFulfilled:
		for _, h := range getHandlers(p, promiseFulfillKey).Array() {
			if h != nil {
				h.Call(value)
			}
		}
	case stateRejected:
		for _, h := range getHandlers(p, promiseRejectKey).Array() {
			if h != nil {
				h.Call(value)
			}
		}
	}
	p.Set(promiseFulfillKey, jsvalue.NewArray())
	p.Set(promiseRejectKey, jsvalue.NewArray())
}

func fulfill(p *jsvalue.JSValue, value *jsvalue.JSValue) *jsvalue.JSValue {
	if p == nil || getState(p) != statePending {
		return p
	}
	if isPromise(value) && value != p {
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
	p.Set(promiseStateKey, jsvalue.NewString(stateFulfilled))
	p.Set(promiseValueKey, jsvalue.Nullish(value, jsvalue.NewUndefined()))
	dispatchHandlers(p)
	return p
}

func reject(p *jsvalue.JSValue, reason *jsvalue.JSValue) *jsvalue.JSValue {
	if p == nil || getState(p) != statePending {
		return p
	}
	p.Set(promiseStateKey, jsvalue.NewString(stateRejected))
	p.Set(promiseValueKey, jsvalue.Nullish(reason, jsvalue.NewUndefined()))
	dispatchHandlers(p)
	return p
}

func init() {
	Promise = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this.Set(promiseStateKey, jsvalue.NewString(statePending))
		this.Set(promiseValueKey, jsvalue.NewUndefined())
		this.Set(promiseFulfillKey, jsvalue.NewArray())
		this.Set(promiseRejectKey, jsvalue.NewArray())
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
		next := newPendingPromise()

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

		switch getState(self) {
		case statePending:
			getHandlers(self, promiseFulfillKey).MethodCall("push", fulfillHandler)
			getHandlers(self, promiseRejectKey).MethodCall("push", rejectHandler)
		case stateFulfilled:
			fulfillHandler.Call(self.Get(promiseValueKey))
		case stateRejected:
			rejectHandler.Call(self.Get(promiseValueKey))
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
		next := newPendingPromise()

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
		switch getState(self) {
		case statePending:
			getHandlers(self, promiseFulfillKey).MethodCall("push", fulfillHandler)
			getHandlers(self, promiseRejectKey).MethodCall("push", rejectHandler)
		case stateFulfilled:
			fulfillHandler.Call(self.Get(promiseValueKey))
		case stateRejected:
			rejectHandler.Call(self.Get(promiseValueKey))
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
			if isPromise(item) {
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
			if isPromise(item) {
				item.MethodCall("then", resolveOne, rejectOne)
			} else {
				resolveOne.Call(item)
			}
		}
		return next
	}))
}
