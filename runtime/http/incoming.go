package nodehttp

import (
	"strings"

	"github.com/valyala/fasthttp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// newIncomingMessage builds a server-side IncomingMessage JSValue from a fasthttp request context.
func newIncomingMessage(ctx *fasthttp.RequestCtx) *jsvalue.JSValue {
	this := jsvalue.NewObjectWithPrototype(incomingCls.Get("prototype"))
	initEvents(this)

	headers := jsvalue.NewObject()
	rawHeaders := jsvalue.NewArray()
	for key, value := range ctx.Request.Header.All() {
		k := strings.ToLower(string(key))
		v := string(value)
		headers.Set(k, jsvalue.NewString(v))
		rawHeaders.MethodCall("push", jsvalue.NewString(string(key)), jsvalue.NewString(v))
	}

	this.Set("method", jsvalue.NewString(string(ctx.Method())))
	this.Set("url", jsvalue.NewString(string(ctx.URI().RequestURI())))
	this.Set("headers", headers)
	this.Set("rawHeaders", rawHeaders)
	this.Set("httpVersion", jsvalue.NewString("1.1"))
	this.Set("httpVersionMajor", jsvalue.NewNumber(1))
	this.Set("httpVersionMinor", jsvalue.NewNumber(1))

	socketStub := jsvalue.NewObject()
	socketStub.Set("remoteAddress", jsvalue.NewString(ctx.RemoteAddr().String()))
	socketStub.Set("remotePort", jsvalue.NewNumber(0))
	this.Set("socket", socketStub)
	this.Set("connection", socketStub)

	return this
}

func newTransportResponseMessage(resp *TransportResponse) *jsvalue.JSValue {
	this := jsvalue.NewObjectWithPrototype(incomingCls.Get("prototype"))
	initEvents(this)

	headers := jsvalue.NewObject()
	rawHeaders := jsvalue.NewArray()
	if resp != nil {
		for key, value := range resp.Headers {
			headers.Set(strings.ToLower(key), jsvalue.NewString(value))
		}
		for _, header := range resp.RawHeaders {
			rawHeaders.MethodCall("push", jsvalue.NewString(header.Key), jsvalue.NewString(header.Value))
		}
		this.Set("statusCode", jsvalue.NewNumber(float64(resp.StatusCode)))
		this.Set("statusMessage", jsvalue.NewString(resp.StatusText))
	} else {
		this.Set("statusCode", jsvalue.NewNumber(0))
		this.Set("statusMessage", jsvalue.NewString(""))
	}
	this.Set("headers", headers)
	this.Set("rawHeaders", rawHeaders)
	this.Set("httpVersion", jsvalue.NewString("1.1"))
	this.Set("httpVersionMajor", jsvalue.NewNumber(1))
	this.Set("httpVersionMinor", jsvalue.NewNumber(1))

	return this
}
