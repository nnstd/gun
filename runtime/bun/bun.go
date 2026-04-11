package bun

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/web"
)

var (
	mu        sync.Mutex
	active    int
	waitCh    chan struct{}
	listenFn  = net.Listen
	AsJSValue = func() *jsvalue.JSValue {
		obj := jsvalue.NewObject()
		obj.Set("serve", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
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

	port := int(options.Get("port").Number())
	if port == 0 {
		port = 3000
	}
	fetch := options.Get("fetch")

	serverObj := jsvalue.NewObject()
	serverObj.Set("hostname", jsvalue.NewString("127.0.0.1"))

	listener, err := listenFn("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
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

	registerActive()

	serverObj.Set("stop", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		_ = listener.Close()
		unregisterActive()
		return jsvalue.NewUndefined()
	}))

	go func() {
		defer unregisterActive()
		_ = server.Serve(listener)
	}()

	return serverObj
}

func Wait() {
	mu.Lock()
	ch := waitCh
	if active == 0 || ch == nil {
		mu.Unlock()
		return
	}
	mu.Unlock()
	<-ch
}

func registerActive() {
	mu.Lock()
	defer mu.Unlock()
	if active == 0 {
		waitCh = make(chan struct{})
	}
	active++
}

func unregisterActive() {
	mu.Lock()
	defer mu.Unlock()
	if active == 0 {
		return
	}
	active--
	if active == 0 && waitCh != nil {
		close(waitCh)
		waitCh = nil
	}
}
