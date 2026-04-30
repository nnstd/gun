package bun

import (
	stdErrors "errors"
	"fmt"
	"log"
	"math"
	"net"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall"

	"github.com/valyala/fasthttp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	error "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/promise"
	"github.com/nnstd/gun/runtime/web"
)

func init() {
	// Reduce GC frequency for server workloads (default 100 → 200).
	// Trade ~50% more peak heap for ~5-10% less CPU from GC.
	if debug.SetGCPercent(200) < 0 {
		// GC percent was not previously set; 200 is our default.
	}
	// Cap OS threads to reduce scheduling overhead. Most Gun workloads
	// are bottlenecked on single-threaded JS execution, not parallelism.
	// GOMAXPROCS=2 reduces Go runtime overhead (goroutine scheduling,
	// cache coherency) while maintaining sufficient parallelism for
	// fasthttp's accept loop + request handling.
	if v := runtime.GOMAXPROCS(0); v > 2 {
		runtime.GOMAXPROCS(2)
	}
}

var (
	listenFn = net.Listen

	AsJSValue *jsvalue.JSValue
)

func init() {
	AsJSValue = func() *jsvalue.JSValue {
		obj := jsvalue.NewObject()
		obj.Set("YAML", YAMLAsJSValue())
		obj.Set("BunFile", BunFile)
		obj.Set("file", jsvalue.NewFunction(file))
		obj.Set("serve", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			defer func() {
				if r := recover(); r != nil {
					if err, ok := error.AsJSError(r); ok {
						panic(error.RefreshStack(err, 3))
					}
					panic(r)
				}
			}()
			if len(args) > 0 {
				return Serve(args[0])
			}
			return Serve(jsvalue.NewObject())
		}))
		return obj
	}()
}

func yamlSpace(space *jsvalue.JSValue) int {
	switch space.TypeString() {
	case "number":
		n := int(math.Trunc(space.Number()))
		if n < 0 {
			return 0
		}
		if n > 10 {
			return 10
		}
		return n
	case "string":
		n := len([]rune(space.String()))
		if n > 10 {
			return 10
		}
		return n
	default:
		return 0
	}
}

func jsValueToNative(v *jsvalue.JSValue) any {
	if v == nil {
		return nil
	}
	if v.Type() == jsvalue.TypeObject {
		if raw := v.MethodCall("valueOf"); raw != nil && raw != v && raw.Type() != jsvalue.TypeObject && raw.Type() != jsvalue.TypeFunction {
			return jsValueToNative(raw)
		}
	}
	switch v.TypeString() {
	case "string":
		return v.String()
	case "number":
		return v.Number()
	case "boolean":
		return v.Bool()
	case "null", "undefined":
		return nil
	case "object":
		if v.IsArray() {
			arr := make([]any, v.Len())
			for i := 0; i < v.Len(); i++ {
				arr[i] = jsValueToNative(v.Index(i))
			}
			return arr
		}
		m := make(map[string]any)
		for _, key := range v.OwnKeys() {
			m[key] = jsValueToNative(v.Get(key))
		}
		return m
	default:
		return v.String()
	}
}

func Serve(options *jsvalue.JSValue) *jsvalue.JSValue {
	if options == nil {
		options = jsvalue.NewObject()
	}
	if options.TypeString() != "object" {
		panic(error.InvalidArgType("Bun.serve expects an object"))
	}

	fetch := options.Get("fetch")
	routes := options.Get("routes")
	if !routes.Bool() && routes.TypeString() != "object" {
		if fetch.TypeString() == "undefined" {
			panic(error.InvalidArgType("Bun.serve() needs either:\n\n - A routes object:\n routes: {\n \"/path\": {\n GET: (req) => new Response(\"Hello\")\n }\n }\n\n - Or a fetch handler:\n fetch: (req) => {\n return new Response(\"Hello\")\n }\n\nLearn more at https://bun.com/docs/api/http"))
		}
		if fetch.TypeString() != "function" {
			panic(error.InvalidArgType("Expected fetch() to be a function"))
		}
	}

	port := int(options.Get("port").Number())
	if port == 0 {
		port = 3000
	}

	serverObj := jsvalue.NewObject()
	serverObj.Set("hostname", jsvalue.NewString("127.0.0.1"))

	listener, err := listenFn("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if stdErrors.Is(err, syscall.EADDRINUSE) {
			bunErr := error.Error.Call(jsvalue.NewString(fmt.Sprintf("Failed to start server. Is port %d in use?", port)))
			bunErr.Set("syscall", jsvalue.NewString("listen"))
			bunErr.Set("errno", jsvalue.NewNumber(0))
			bunErr.Set("code", jsvalue.NewString("EADDRINUSE"))
			panic(bunErr)
		}
		panic(err)
	}

	actualPort := port
	if _, rawPort, err := net.SplitHostPort(listener.Addr().String()); err == nil {
		if parsedPort, err := strconv.Atoi(rawPort); err == nil {
			actualPort = parsedPort
		}
	}
	serverObj.Set("port", jsvalue.NewNumber(float64(actualPort)))
	serverObj.Set("url", jsvalue.NewString(fmt.Sprintf("http://127.0.0.1:%d", actualPort)))

	server := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[gun] panic serving %s %s: %v\n%s", ctx.Method(), ctx.Path(), r, debug.Stack())
					ctx.SetStatusCode(500)
					ctx.SetBodyString(fmt.Sprintf("Internal Server Error: %v", r))
				}
			}()

			req := web.RequestFromFastHTTP(ctx)
			res := fetch.Call(req, serverObj)

			if !promise.IsPromise(res) {
				web.WriteResponseFastHTTP(ctx, res)
				web.ReleaseFastHTTPRequest(req)
				return
			}

			// Async: wait for promise to settle
			done := make(chan struct{})
			var result *jsvalue.JSValue
			var errResult *jsvalue.JSValue

			res.MethodCall("then",
				jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
					if len(args) > 0 && args[0] != nil {
						result = args[0]
					}
					close(done)
					return jsvalue.NewUndefined()
				}),
				jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
					if len(args) > 0 && args[0] != nil {
						errResult = jsvalue.NewString(args[0].String())
					} else {
						errResult = jsvalue.NewString("Internal Server Error")
					}
					close(done)
					return jsvalue.NewUndefined()
				}),
			)
			<-done

			if errResult != nil {
				ctx.SetStatusCode(500)
				ctx.SetBodyString(errResult.String())
				web.ReleaseFastHTTPRequest(req)
				return
			}
			web.WriteResponseFastHTTP(ctx, result)
			web.ReleaseFastHTTPRequest(req)
		},
	}

	eventloop.Default.RegisterHandle()

	serverObj.Set("stop", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		_ = server.Shutdown()
		_ = listener.Close()
		return jsvalue.NewUndefined()
	}))

	go func() {
		defer eventloop.Default.UnregisterHandle()
		_ = server.Serve(listener)
	}()

	return serverObj
}
