package nodehttp

import (
	"strings"
	"sync"

	"github.com/valyala/fasthttp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

type responseInternal struct {
	mu          sync.Mutex
	ctx         *fasthttp.RequestCtx
	headers     map[string]string
	headersSent bool
	done        chan struct{}
	closed      bool
}

var (
	responseRegistryMu sync.Mutex
	responseRegistry   = map[*jsvalue.JSValue]*responseInternal{}
)

func responseInternalOf(v *jsvalue.JSValue) *responseInternal {
	responseRegistryMu.Lock()
	defer responseRegistryMu.Unlock()
	return responseRegistry[v]
}

// newServerResponse builds a JSValue ServerResponse and the done channel that
// the fasthttp handler should wait on before returning.
func newServerResponse(ctx *fasthttp.RequestCtx) (*jsvalue.JSValue, chan struct{}) {
	this := jsvalue.NewObjectWithPrototype(responseCls.Get("prototype"))
	initEvents(this)
	this.Set("statusCode", jsvalue.NewNumber(200))
	this.Set("statusMessage", jsvalue.NewString(""))
	this.Set("headersSent", jsvalue.NewBool(false))

	done := make(chan struct{})
	ri := &responseInternal{
		ctx:     ctx,
		headers: map[string]string{},
		done:    done,
	}
	responseRegistryMu.Lock()
	responseRegistry[this] = ri
	responseRegistryMu.Unlock()
	return this, done
}

func (ri *responseInternal) flushHeaders(this *jsvalue.JSValue) {
	if ri.headersSent {
		return
	}
	status := int(this.Get("statusCode").Number())
	if status == 0 {
		status = 200
	}
	ri.ctx.SetStatusCode(status)
	for k, v := range ri.headers {
		ri.ctx.Response.Header.Set(k, v)
	}
	ri.headersSent = true
	this.Set("headersSent", jsvalue.NewBool(true))
}

func (ri *responseInternal) finish() {
	if ri.closed {
		return
	}
	ri.closed = true
	close(ri.done)
}

func init() {
	proto := responseCls.Get("prototype")

	proto.Set("setHeader", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 {
			return jsvalue.NewUndefined()
		}
		ri := responseInternalOf(args[0])
		if ri == nil {
			return jsvalue.NewUndefined()
		}
		ri.mu.Lock()
		defer ri.mu.Unlock()
		ri.headers[args[1].String()] = args[2].String()
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("getHeader", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewUndefined()
		}
		ri := responseInternalOf(args[0])
		if ri == nil {
			return jsvalue.NewUndefined()
		}
		ri.mu.Lock()
		defer ri.mu.Unlock()
		key := args[1].String()
		for k, v := range ri.headers {
			if strings.EqualFold(k, key) {
				return jsvalue.NewString(v)
			}
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("removeHeader", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewUndefined()
		}
		ri := responseInternalOf(args[0])
		if ri == nil {
			return jsvalue.NewUndefined()
		}
		ri.mu.Lock()
		defer ri.mu.Unlock()
		key := args[1].String()
		for k := range ri.headers {
			if strings.EqualFold(k, key) {
				delete(ri.headers, k)
			}
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("hasHeader", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewBool(false)
		}
		ri := responseInternalOf(args[0])
		if ri == nil {
			return jsvalue.NewBool(false)
		}
		ri.mu.Lock()
		defer ri.mu.Unlock()
		key := args[1].String()
		for k := range ri.headers {
			if strings.EqualFold(k, key) {
				return jsvalue.NewBool(true)
			}
		}
		return jsvalue.NewBool(false)
	}).MarkAsMethod())

	proto.Set("writeHead", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return args[0]
		}
		this := args[0]
		ri := responseInternalOf(this)
		if ri == nil {
			return this
		}
		ri.mu.Lock()
		defer ri.mu.Unlock()

		this.Set("statusCode", jsvalue.NewNumber(args[1].Number()))

		var headers *jsvalue.JSValue
		if len(args) > 2 && args[2] != nil {
			if args[2].TypeString() == "string" {
				this.Set("statusMessage", args[2])
				if len(args) > 3 && args[3] != nil && args[3].TypeString() == "object" {
					headers = args[3]
				}
			} else if args[2].TypeString() == "object" {
				headers = args[2]
			}
		}
		if headers != nil {
			for _, k := range headers.OwnKeys() {
				ri.headers[k] = headers.Get(k).String()
			}
		}
		ri.flushHeaders(this)
		return this
	}).MarkAsMethod())

	proto.Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewBool(true)
		}
		this := args[0]
		ri := responseInternalOf(this)
		if ri == nil {
			return jsvalue.NewBool(false)
		}
		ri.mu.Lock()
		defer ri.mu.Unlock()
		ri.flushHeaders(this)
		ri.ctx.Write([]byte(args[1].String()))
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("end", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		ri := responseInternalOf(this)
		if ri == nil {
			return jsvalue.NewUndefined()
		}
		ri.mu.Lock()
		defer ri.mu.Unlock()
		ri.flushHeaders(this)
		if len(args) > 1 && args[1] != nil && args[1].TypeString() != "undefined" {
			ri.ctx.Write([]byte(args[1].String()))
		}
		ri.finish()
		this.MethodCall("emit", jsvalue.NewString("finish"))
		return this
	}).MarkAsMethod())
}
