package bun

import (
	"context"
	stdErrors "errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	error "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/web"
)

var (
	listenFn  = net.Listen
	AsJSValue = func() *jsvalue.JSValue {
		obj := jsvalue.NewObject()
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
)

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
			panic(error.InvalidArgType("Bun.serve() needs either:\n\n  - A routes object:\n     routes: {\n       \"/path\": {\n         GET: (req) => new Response(\"Hello\")\n       }\n     }\n\n  - Or a fetch handler:\n     fetch: (req) => {\n       return new Response(\"Hello\")\n     }\n\nLearn more at https://bun.com/docs/api/http"))
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

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := web.RequestFromHTTP(r)
			res := fetch.Call(req, serverObj)
			web.WriteResponse(w, res)
		}),
	}

	eventloop.Default.RegisterServer()

	serverObj.Set("stop", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		_ = listener.Close()
		return jsvalue.NewUndefined()
	}))

	go func() {
		defer eventloop.Default.UnregisterServer()
		_ = server.Serve(listener)
	}()

	return serverObj
}
