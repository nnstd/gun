package vm

import (
	"sync"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/dynfunc"
	"github.com/nnstd/gun/runtime/jscontext"
)

var AsJSValue *jsvalue.JSValue

type scriptState struct {
	source string
}

var (
	contextRegistry map[*jsvalue.JSValue]*jscontext.Context
	contextMu       sync.RWMutex
	scriptRegistry  map[*jsvalue.JSValue]*scriptState
	scriptMu        sync.RWMutex
)

func init() {
	contextRegistry = make(map[*jsvalue.JSValue]*jscontext.Context)
	scriptRegistry = make(map[*jsvalue.JSValue]*scriptState)

	Script := jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			panic(jsvalue.NewString("TypeError: The \"code\" argument must be of type string. Received undefined"))
		}
		source := args[0].String()

		scriptMu.Lock()
		scriptRegistry[this] = &scriptState{source: source}
		scriptMu.Unlock()

		return nil
	}, nil)

	Script.Get("prototype").Set("runInContext", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[1] == nil {
			panic(jsvalue.NewString("TypeError: The \"contextifiedObject\" argument is required"))
		}
		scriptInstance := args[0]
		contextifiedObj := args[1]

		scriptMu.RLock()
		ss, ok := scriptRegistry[scriptInstance]
		scriptMu.RUnlock()
		if !ok {
			panic(jsvalue.NewString("ReferenceError: script is not defined"))
		}

		contextMu.RLock()
		ctx, ok := contextRegistry[contextifiedObj]
		contextMu.RUnlock()
		if !ok {
			panic(jsvalue.NewString("TypeError: The \"contextifiedObject\" argument must be a vm context"))
		}

		return dynfunc.EvalStatementsHIR(ctx, jsvalue.NewString(ss.source))
	}).MarkAsMethod())

	Script.Get("prototype").Set("runInNewContext", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		scriptInstance := args[0]

		scriptMu.RLock()
		ss, ok := scriptRegistry[scriptInstance]
		scriptMu.RUnlock()
		if !ok {
			panic(jsvalue.NewString("ReferenceError: script is not defined"))
		}

		var sandbox *jsvalue.JSValue
		if len(args) > 1 && args[1] != nil {
			sandbox = args[1]
		}

		ctx, _ := newVMContext(sandbox)
		return dynfunc.EvalStatementsHIR(ctx, jsvalue.NewString(ss.source))
	}).MarkAsMethod())

	Script.Get("prototype").Set("runInThisContext", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		scriptInstance := args[0]

		scriptMu.RLock()
		ss, ok := scriptRegistry[scriptInstance]
		scriptMu.RUnlock()
		if !ok {
			panic(jsvalue.NewString("ReferenceError: script is not defined"))
		}

		return dynfunc.EvalStatementsHIR(jscontext.Default(), jsvalue.NewString(ss.source))
	}).MarkAsMethod())

	Script.Get("prototype").Set("createCachedData", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return buffer.Buffer.Get("from").Call(jsvalue.NewString(""))
	}).MarkAsMethod())

	AsJSValue = jsvalue.ObjectFrom(
		"Script", Script,
		"createContext", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			var sandbox *jsvalue.JSValue
			if len(args) > 0 && args[0] != nil {
				sandbox = args[0]
			}
			_, global := newVMContext(sandbox)
			return global
		}),
		"isContext", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 || args[0] == nil {
				return jsvalue.NewBool(false)
			}
			contextMu.RLock()
			_, ok := contextRegistry[args[0]]
			contextMu.RUnlock()
			return jsvalue.NewBool(ok)
		}),
		"runInContext", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) < 2 {
				panic(jsvalue.NewString("TypeError: code and contextifiedObject are required"))
			}
			contextMu.RLock()
			ctx, ok := contextRegistry[args[1]]
			contextMu.RUnlock()
			if !ok {
				panic(jsvalue.NewString("TypeError: The \"contextifiedObject\" argument must be a vm context"))
			}
			return dynfunc.EvalStatementsHIR(ctx, args[0])
		}),
		"runInNewContext", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 {
				panic(jsvalue.NewString("TypeError: The \"code\" argument is required"))
			}
			var sandbox *jsvalue.JSValue
			if len(args) > 1 && args[1] != nil {
				sandbox = args[1]
			}
			ctx, _ := newVMContext(sandbox)
			return dynfunc.EvalStatementsHIR(ctx, args[0])
		}),
		"runInThisContext", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 {
				panic(jsvalue.NewString("TypeError: The \"code\" argument is required"))
			}
			return dynfunc.EvalStatementsHIR(jscontext.Default(), args[0])
		}),
		"compileFunction", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 {
				panic(jsvalue.NewString("TypeError: The \"code\" argument is required"))
			}
			code := args[0]
			var hirArgs []*jsvalue.JSValue
			if len(args) > 1 && args[1] != nil && args[1].IsArray() {
				for _, p := range args[1].Array() {
					hirArgs = append(hirArgs, p)
				}
			}
			hirArgs = append(hirArgs, code)
			return dynfunc.CompileFunctionHIR(hirArgs...)
		}),
		"constants", jsvalue.ObjectFrom(
			"USE_MAIN_CONTEXT_DEFAULT_LOADER", jsvalue.NewNumber(1),
			"DONT_CONTEXTIFY", jsvalue.NewNumber(2),
		),
	)
}

func newVMContext(sandbox *jsvalue.JSValue) (*jscontext.Context, *jsvalue.JSValue) {
	var ctx *jscontext.Context
	if sandbox != nil {
		ctx = jscontext.NewFromGlobal(sandbox)
	} else {
		ctx = jscontext.New()
		ctx.RegisterBuiltins()
	}

	contextMu.Lock()
	contextRegistry[ctx.Global()] = ctx
	contextMu.Unlock()

	return ctx, ctx.Global()
}
