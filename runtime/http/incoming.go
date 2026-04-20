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

// newClientResponseMessage builds a client-side IncomingMessage from a fasthttp.Response.
func newClientResponseMessage(resp *fasthttp.Response) *jsvalue.JSValue {
	this := jsvalue.NewObjectWithPrototype(incomingCls.Get("prototype"))
	initEvents(this)

	headers := jsvalue.NewObject()
	rawHeaders := jsvalue.NewArray()
	for key, value := range resp.Header.All() {
		headers.Set(strings.ToLower(string(key)), jsvalue.NewString(string(value)))
		rawHeaders.MethodCall("push", jsvalue.NewString(string(key)), jsvalue.NewString(string(value)))
	}

	this.Set("statusCode", jsvalue.NewNumber(float64(resp.StatusCode())))
	this.Set("statusMessage", jsvalue.NewString(string(resp.Header.StatusMessage())))
	this.Set("headers", headers)
	this.Set("rawHeaders", rawHeaders)
	this.Set("httpVersion", jsvalue.NewString("1.1"))
	this.Set("httpVersionMajor", jsvalue.NewNumber(1))
	this.Set("httpVersionMinor", jsvalue.NewNumber(1))

	return this
}
