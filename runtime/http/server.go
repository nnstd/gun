package nodehttp

import (
	stdErrors "errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/valyala/fasthttp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
)

// serverInternal is the Go-side state attached to each Server JSValue.
type serverInternal struct {
	mu          sync.Mutex
	server      *fasthttp.Server
	listener    net.Listener
	socketPath  string
	isTLS       bool
	certPEM     []byte
	keyPEM      []byte
	listening   atomic.Bool
	closed      atomic.Bool
	requestHand *jsvalue.JSValue // primary listener from createServer(listener)
}

var (
	serverRegistryMu sync.Mutex
	serverRegistry   = map[*jsvalue.JSValue]*serverInternal{}
)

func serverInternalOf(v *jsvalue.JSValue) *serverInternal {
	serverRegistryMu.Lock()
	defer serverRegistryMu.Unlock()
	if s, ok := serverRegistry[v]; ok {
		return s
	}
	s := &serverInternal{}
	serverRegistry[v] = s
	return s
}

// CreateServer builds a JSValue Server. For https.createServer the first arg is
// an options object containing key/cert PEM; for http it's the listener directly.
func CreateServer(isTLS bool, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var opts *jsvalue.JSValue
	var listener *jsvalue.JSValue

	if isTLS {
		if len(args) > 0 && args[0] != nil && args[0].TypeString() == "object" {
			opts = args[0]
		}
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "function" {
			listener = args[1]
		} else if len(args) > 0 && args[0] != nil && args[0].TypeString() == "function" {
			listener = args[0]
		}
	} else {
		// http.createServer([options], [listener])
		if len(args) > 0 && args[0] != nil && args[0].TypeString() == "object" {
			opts = args[0]
		}
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "function" {
			listener = args[1]
		} else if len(args) > 0 && args[0] != nil && args[0].TypeString() == "function" {
			listener = args[0]
		}
	}

	this := jsvalue.NewObject()
	this.SetPrototype(serverClass.Get("prototype"))
	initEvents(this)
	this.Set("_isTLS", jsvalue.NewBool(isTLS))
	this.Set("listening", jsvalue.NewBool(false))

	si := serverInternalOf(this)
	si.isTLS = isTLS

	if isTLS {
		if opts == nil {
			panic(jserror.InvalidArgType("https.createServer requires an options object with key and cert"))
		}
		key := opts.Get("key")
		cert := opts.Get("cert")
		if key.TypeString() != "string" || cert.TypeString() != "string" {
			panic(jserror.InvalidArgType("https.createServer options must include string \"key\" and \"cert\" PEM data"))
		}
		si.keyPEM = []byte(key.String())
		si.certPEM = []byte(cert.String())
	}

	if listener != nil {
		si.requestHand = listener
		this.MethodCall("on", jsvalue.NewString("request"), listener)
	}

	return this
}

// resolveListenArgs picks (port, host, path, callback) from listen() args.
// path != "" means unix socket.
//
// Node positional forms:
//
//	listen(port[, host[, backlog]][, cb])
//	listen(path[, backlog][, cb])           // unix socket
//	listen(opts[, cb])                      // {port, host, path}
func resolveListenArgs(args []*jsvalue.JSValue) (port int, host string, path string, cb *jsvalue.JSValue) {
	host = "0.0.0.0"

	rest := make([]*jsvalue.JSValue, 0, len(args))
	for _, a := range args {
		if a == nil {
			continue
		}
		if a.TypeString() == "function" {
			cb = a
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) == 0 {
		return
	}

	if rest[0].TypeString() == "object" {
		opt := rest[0]
		if v := opt.Get("path"); v.TypeString() == "string" {
			path = checkPipePath(v.String())
			return
		}
		if v := opt.Get("port"); v.TypeString() == "number" {
			port = int(v.Number())
		}
		if v := opt.Get("host"); v.TypeString() == "string" {
			host = v.String()
		}
		return
	}

	switch rest[0].TypeString() {
	case "number":
		port = int(rest[0].Number())
		if len(rest) > 1 && rest[1].TypeString() == "string" {
			host = rest[1].String()
		}
	case "string":
		path = checkPipePath(rest[0].String())
	}
	return
}

func checkPipePath(s string) string {
	if strings.HasPrefix(s, "\\\\.\\pipe\\") || strings.HasPrefix(s, "\\\\?\\pipe\\") {
		panic(jserror.InvalidArgType("Windows named pipes are not supported by gun runtime/http"))
	}
	return s
}

func init() {
	proto := serverClass.Get("prototype")

	proto.Set("listen", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		port, host, path, cb := resolveListenArgs(args[1:])
		si := serverInternalOf(this)
		si.mu.Lock()
		defer si.mu.Unlock()
		if si.listening.Load() {
			panic(jserror.InvalidArgType("Server is already listening"))
		}

		var ln net.Listener
		var err error
		var addrLabel string
		if path != "" {
			_ = os.Remove(path)
			addrLabel = path
			ln, err = net.Listen("unix", path)
			if err == nil {
				si.socketPath = path
			}
		} else {
			addrLabel = fmt.Sprintf("%s:%d", host, port)
			ln, err = net.Listen("tcp", addrLabel)
		}
		if err != nil {
			errVal := buildListenError(err, addrLabel, "listen")
			go func() {
				time.Sleep(time.Millisecond)
				this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			}()
			return this
		}
		si.listener = ln
		si.listening.Store(true)
		this.Set("listening", jsvalue.NewBool(true))

		si.server = &fasthttp.Server{
			Handler: makeRequestHandler(this),
		}

		eventloop.Default.RegisterServer()

		go func() {
			defer eventloop.Default.UnregisterServer()
			var serveErr error
			if si.isTLS {
				serveErr = si.server.ServeTLSEmbed(ln, si.certPEM, si.keyPEM)
			} else {
				serveErr = si.server.Serve(ln)
			}
			if serveErr != nil && !si.closed.Load() {
				errVal := jserror.Error.Call(jsvalue.NewString(serveErr.Error()))
				this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			}
		}()

		// Fire 'listening' on next event loop tick (defer to JS handler register).
		go func() {
			time.Sleep(time.Millisecond)
			this.MethodCall("emit", jsvalue.NewString("listening"))
			if cb != nil {
				cb.Call()
			}
		}()
		return this
	}).MarkAsMethod())

	proto.Set("address", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewNull()
		}
		this := args[0]
		si := serverInternalOf(this)
		si.mu.Lock()
		defer si.mu.Unlock()
		if si.listener == nil {
			return jsvalue.NewNull()
		}
		if si.socketPath != "" {
			return jsvalue.NewString(si.socketPath)
		}
		addr := si.listener.Addr().String()
		hostStr, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			return jsvalue.NewString(addr)
		}
		p, _ := strconv.Atoi(portStr)
		family := "IPv4"
		if strings.Contains(hostStr, ":") {
			family = "IPv6"
		}
		return jsvalue.ObjectFrom(
			"port", jsvalue.NewNumber(float64(p)),
			"family", jsvalue.NewString(family),
			"address", jsvalue.NewString(hostStr),
		)
	}).MarkAsMethod())

	proto.Set("close", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		var cb *jsvalue.JSValue
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "function" {
			cb = args[1]
		}
		si := serverInternalOf(this)
		si.mu.Lock()
		si.closed.Store(true)
		if si.server != nil {
			_ = si.server.Shutdown()
		}
		if si.listener != nil {
			_ = si.listener.Close()
		}
		path := si.socketPath
		si.mu.Unlock()
		if path != "" {
			_ = os.Remove(path)
		}
		this.Set("listening", jsvalue.NewBool(false))
		this.MethodCall("emit", jsvalue.NewString("close"))
		serverRegistryMu.Lock()
		delete(serverRegistry, this)
		serverRegistryMu.Unlock()
		if cb != nil {
			cb.Call()
		}
		return this
	}).MarkAsMethod())
}

func buildListenError(err error, addr, syscallName string) *jsvalue.JSValue {
	msg := fmt.Sprintf("listen %s: %v", addr, err)
	code := "ELISTEN"
	if stdErrors.Is(err, syscall.EADDRINUSE) {
		msg = fmt.Sprintf("listen EADDRINUSE: address already in use %s", addr)
		code = "EADDRINUSE"
	} else if stdErrors.Is(err, syscall.EACCES) {
		code = "EACCES"
	} else if stdErrors.Is(err, syscall.EADDRNOTAVAIL) {
		code = "EADDRNOTAVAIL"
	}
	bunErr := jserror.Error.Call(jsvalue.NewString(msg))
	bunErr.Set("syscall", jsvalue.NewString(syscallName))
	bunErr.Set("errno", jsvalue.NewNumber(0))
	bunErr.Set("code", jsvalue.NewString(code))
	bunErr.Set("address", jsvalue.NewString(addr))
	return bunErr
}

// makeRequestHandler returns a fasthttp Handler bound to the given Server JSValue.
func makeRequestHandler(server *jsvalue.JSValue) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[gun http] panic serving %s %s: %v\n%s",
					ctx.Method(), ctx.Path(), r, debug.Stack())
				ctx.SetStatusCode(500)
				ctx.SetBodyString(fmt.Sprintf("Internal Server Error: %v", r))
			}
		}()

		req := newIncomingMessage(ctx)
		res, done := newServerResponse(ctx)

		// Fire 'request' for any extra listeners.
		server.MethodCall("emit", jsvalue.NewString("request"), req, res)

		// Dispatch synthetic body events after handler had a chance to register.
		go func() {
			body := ctx.PostBody()
			if len(body) > 0 {
				req.MethodCall("emit", jsvalue.NewString("data"), jsvalue.NewString(string(body)))
			}
			req.MethodCall("emit", jsvalue.NewString("end"))
		}()

		<-done
	}
}
