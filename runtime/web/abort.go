package web

import (
	"fmt"
	"math"
	"slices"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
)

var AbortSignal *jsvalue.JSValue
var AbortController *jsvalue.JSValue

func init() {
	AbortSignal = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initAbortSignal(this)
		return nil
	}, nil)

	AbortController = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this.Set("signal", NewAbortSignal())
		return nil
	}, nil)

	initAbortSignalPrototype()
	initAbortControllerPrototype()
	initAbortSignalStatics()
}

func NewAbortSignal() *jsvalue.JSValue {
	signal := jsvalue.NewObjectWithPrototype(AbortSignal.Get("prototype"))
	initAbortSignal(signal)
	return signal
}

func NewAbortError(message string) *jsvalue.JSValue {
	if message == "" {
		message = "This operation was aborted"
	}
	errVal := jserror.Error.Call(jsvalue.NewString(message))
	errVal.Set("name", jsvalue.NewString("AbortError"))
	errVal.Set("code", jsvalue.NewNumber(20))
	return errVal
}

func NewTimeoutError() *jsvalue.JSValue {
	errVal := jserror.Error.Call(jsvalue.NewString("The operation was aborted due to timeout"))
	errVal.Set("name", jsvalue.NewString("TimeoutError"))
	errVal.Set("code", jsvalue.NewNumber(23))
	return errVal
}

func IsAbortSignal(v *jsvalue.JSValue) bool {
	return v != nil && jsvalue.InstanceOf(v, AbortSignal).Bool()
}

func IsAborted(signal *jsvalue.JSValue) bool {
	return IsAbortSignal(signal) && signal.Get("aborted").Bool()
}

func AbortReason(signal *jsvalue.JSValue) *jsvalue.JSValue {
	if !IsAbortSignal(signal) {
		return jsvalue.NewUndefined()
	}
	return signal.Get("reason")
}

func AbortSignalFromOptions(options *jsvalue.JSValue) *jsvalue.JSValue {
	if options == nil || options.TypeString() != "object" {
		return nil
	}
	signal := options.Get("signal")
	if IsAbortSignal(signal) {
		return signal
	}
	return nil
}

func initAbortSignal(signal *jsvalue.JSValue) {
	signal.Set("aborted", jsvalue.NewBool(false))
	signal.Set("reason", jsvalue.NewUndefined())
	signal.Set("onabort", jsvalue.NewNull())
	signal.Set("_listeners", jsvalue.NewArray())
}

func initAbortSignalPrototype() {
	proto := AbortSignal.Get("prototype")
	proto.Set("addEventListener", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		if args[1].String() != "abort" || args[2] == nil || args[2].TypeString() != "function" {
			return jsvalue.NewUndefined()
		}
		listeners := abortListeners(args[0])
		if !listenerExists(listeners, args[2]) {
			listeners.MethodCall("push", args[2])
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("removeEventListener", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil || args[1].String() != "abort" {
			return jsvalue.NewUndefined()
		}
		listeners := abortListeners(args[0])
		kept := make([]*jsvalue.JSValue, 0, listeners.Len())
		for _, listener := range listeners.Array() {
			if listener != args[2] {
				kept = append(kept, listener)
			}
		}
		args[0].Set("_listeners", jsvalue.NewArray(kept...))
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("dispatchEvent", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil || args[1] == nil {
			return jsvalue.NewBool(false)
		}
		if args[1].Get("type").String() == "abort" {
			dispatchAbortEvent(args[0])
			return jsvalue.NewBool(true)
		}
		return jsvalue.NewBool(false)
	}).MarkAsMethod())

	proto.Set("throwIfAborted", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 && args[0] != nil && args[0].Get("aborted").Bool() {
			panic(args[0].Get("reason"))
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
}

func initAbortControllerPrototype() {
	AbortController.Get("prototype").Set("abort", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		reason := jsvalue.NewUndefined()
		if len(args) > 1 {
			reason = args[1]
		}
		AbortSignalValue(args[0].Get("signal"), reason)
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
}

func initAbortSignalStatics() {
	AbortSignal.Set("abort", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		reason := jsvalue.NewUndefined()
		if len(args) > 0 {
			reason = args[0]
		}
		signal := NewAbortSignal()
		AbortSignalValue(signal, reason)
		return signal
	}))

	AbortSignal.Set("timeout", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		delay := 0
		if len(args) > 0 {
			delay = int(math.Max(0, args[0].Number()))
		}
		signal := NewAbortSignal()
		eventloop.SetTimeout(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			AbortSignalValue(signal, NewTimeoutError())
			return jsvalue.NewUndefined()
		}), jsvalue.NewNumber(float64(delay)))
		return signal
	}))

	AbortSignal.Set("any", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		combined := NewAbortSignal()
		if len(args) == 0 || args[0] == nil || !args[0].IsArray() {
			return combined
		}
		for _, signal := range args[0].Array() {
			if !IsAbortSignal(signal) {
				continue
			}
			if signal.Get("aborted").Bool() {
				AbortSignalValue(combined, signal.Get("reason"))
				return combined
			}
			s := signal
			s.MethodCall("addEventListener", jsvalue.NewString("abort"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
				AbortSignalValue(combined, s.Get("reason"))
				return jsvalue.NewUndefined()
			}))
		}
		return combined
	}))
}

func AbortSignalValue(signal, reason *jsvalue.JSValue) {
	if !IsAbortSignal(signal) || signal.Get("aborted").Bool() {
		return
	}
	if reason == nil || reason.TypeString() == "undefined" {
		reason = NewAbortError("")
	}
	signal.Set("aborted", jsvalue.NewBool(true))
	signal.Set("reason", reason)
	dispatchAbortEvent(signal)
}

func abortListeners(signal *jsvalue.JSValue) *jsvalue.JSValue {
	listeners := signal.Get("_listeners")
	if listeners == nil || !listeners.IsArray() {
		listeners = jsvalue.NewArray()
		signal.Set("_listeners", listeners)
	}
	return listeners
}

func listenerExists(listeners, candidate *jsvalue.JSValue) bool {
	return slices.Contains(listeners.Array(), candidate)
}

func dispatchAbortEvent(signal *jsvalue.JSValue) {
	event := jsvalue.ObjectFrom(
		"type", jsvalue.NewString("abort"),
		"target", signal,
	)
	if onabort := signal.Get("onabort"); onabort != nil && onabort.TypeString() == "function" {
		onabort.Call(event)
	}
	listeners := append([]*jsvalue.JSValue(nil), abortListeners(signal).Array()...)
	for _, listener := range listeners {
		if listener != nil && listener.TypeString() == "function" {
			listener.Call(event)
		}
	}
}

func DebugAbortSignal(signal *jsvalue.JSValue) string {
	if !IsAbortSignal(signal) {
		return "AbortSignal<invalid>"
	}
	return fmt.Sprintf("AbortSignal<aborted=%v reason=%s>", signal.Get("aborted").Bool(), signal.Get("reason").String())
}
