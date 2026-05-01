package web

import (
	stdjson "encoding/json"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	jsonpkg "github.com/nnstd/gun/runtime/builtin/json"
	"github.com/nnstd/gun/runtime/promise"
)

var Headers = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) > 0 && args[0] != nil {
		initHeaders(this, args[0])
	}
	return nil
}, nil)

func newHeadersValue(init *jsvalue.JSValue) *jsvalue.JSValue {
	headers := jsvalue.NewObjectWithPrototype(Headers.Get("prototype"))
	if init != nil && init.TypeString() == "object" {
		initHeaders(headers, init)
	}
	return headers
}

func newResponseValue(proto, body, status, statusText, headers *jsvalue.JSValue) *jsvalue.JSValue {
	if status == nil {
		status = jsvalue.NewNumber(200)
	}
	if statusText == nil {
		statusText = jsvalue.NewString("OK")
	}
	if headers == nil {
		headers = Headers.Call()
	}
	if body == nil {
		body = jsvalue.NewString("")
	}

	res := jsvalue.NewObjectWithPrototypeAndProps(proto,
		"status", status,
		"statusText", statusText,
		"headers", headers,
		"body", body,
		"_bodyInit", body,
		"ok", jsvalue.NewBool(status.Number() >= 200 && status.Number() < 300),
		"bodyUsed", jsvalue.NewBool(false),
	)
	return res
}

var Request = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var input *jsvalue.JSValue
	if len(args) > 0 {
		input = args[0]
	}
	var init *jsvalue.JSValue
	if len(args) > 1 {
		init = args[1]
	}

	method := jsvalue.NewString("GET")
	headers := newHeadersValue(nil)
	body := jsvalue.NewString("")
	url := ""

	if input != nil {
		switch {
		case input.Get("url").TypeString() == "string":
			if input.Get("bodyUsed").Bool() {
				panic(jserror.TypeError.Call(jsvalue.NewString("Cannot construct a Request from a consumed body")))
			}
			url = input.Get("url").String()
			if v := input.Get("method"); v.TypeString() == "string" {
				method = jsvalue.NewString(strings.ToUpper(v.String()))
			}
			if v := input.Get("headers"); v.TypeString() == "object" {
				headers = cloneHeaders(v)
			}
			if v := input.Get("body"); v.TypeString() == "string" {
				body = jsvalue.NewString(v.String())
			}
		case input.Get("href").TypeString() == "string":
			url = input.Get("href").String()
		case input.TypeString() == "string":
			url = input.String()
		default:
			panic(jserror.TypeError.Call(jsvalue.NewString("Request input must be a string, Request, or URL with href")))
		}
	}
	if url == "" {
		panic(jserror.TypeError.Call(jsvalue.NewString("Request requires an absolute URL")))
	}
	if _, err := parseAbsoluteURL(url); err != nil {
		panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
	}

	if init != nil && init.TypeString() == "object" {
		if init.Get("method").Bool() || init.Get("method").TypeString() == "string" {
			method = jsvalue.NewString(strings.ToUpper(init.Get("method").String()))
		}
		if init.Get("headers").Bool() || init.Get("headers").TypeString() == "object" {
			headers = newHeadersValue(init.Get("headers"))
		}
		if init.Get("body").Bool() || init.Get("body").TypeString() == "string" {
			body = jsvalue.NewString(init.Get("body").String())
		}
	}

	return newRequestValueWithPrototype(this.GetPrototype(), url, method.String(), headers, body.String())
}, nil)

var Response = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var body *jsvalue.JSValue
	if len(args) > 0 {
		body = args[0]
	}
	var init *jsvalue.JSValue
	if len(args) > 1 {
		init = args[1]
	}

	status := jsvalue.NewNumber(200)
	statusText := jsvalue.NewString("OK")
	headers := newHeadersValue(nil)

	if init != nil && init.TypeString() == "object" {
		if init.Get("status").Bool() || init.Get("status").TypeString() == "number" {
			status = jsvalue.NewNumber(init.Get("status").Number())
		}
		if init.Get("statusText").Bool() || init.Get("statusText").TypeString() == "string" {
			statusText = jsvalue.NewString(init.Get("statusText").String())
		}
		if init.Get("headers").Bool() || init.Get("headers").TypeString() == "object" {
			headers = newHeadersValue(init.Get("headers"))
		}
	}

	return newResponseValue(this.GetPrototype(), body, status, statusText, headers)
}, nil)

var File = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var parts *jsvalue.JSValue
	if len(args) > 0 {
		parts = args[0]
	}
	var name *jsvalue.JSValue
	if len(args) > 1 {
		name = args[1]
	}
	var options *jsvalue.JSValue
	if len(args) > 2 {
		options = args[2]
	}

	this.Set("name", jsvalue.NewString(""))
	if name != nil {
		this.Set("name", jsvalue.NewString(name.String()))
	}
	this.Set("lastModified", jsvalue.NewNumber(0))
	this.Set("type", jsvalue.NewString(""))
	this.Set("size", jsvalue.NewNumber(0))
	this.Set("parts", jsvalue.Or(parts, jsvalue.NewArray()))

	if options != nil && options.TypeString() == "object" {
		if options.Get("lastModified").Bool() || options.Get("lastModified").TypeString() == "number" {
			this.Set("lastModified", jsvalue.NewNumber(options.Get("lastModified").Number()))
		}
		if options.Get("type").Bool() || options.Get("type").TypeString() == "string" {
			this.Set("type", jsvalue.NewString(options.Get("type").String()))
		}
	}
	return nil
}, nil)

var URL = jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) > 0 {
		return ParseURL(args[0])
	}
	return ParseURL(jsvalue.NewString(""))
})

func newRequestValueWithPrototype(proto *jsvalue.JSValue, url, method string, headers *jsvalue.JSValue, body string) *jsvalue.JSValue {
	if method == "" {
		method = "GET"
	}
	if headers == nil {
		headers = Headers.Call()
	}

	req := jsvalue.NewObjectWithPrototypeAndProps(proto,
		"url", jsvalue.NewString(url),
		"method", jsvalue.NewString(method),
		"headers", headers,
		"body", jsvalue.NewString(body),
		"bodyUsed", jsvalue.NewBool(false),
	)
	req.Set("raw", req)
	return req
}

func init() {
	Headers.Get("prototype").Set("get", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		return args[0].Get(normalizeHeaderKey(args[1]))
	}).MarkAsMethod())
	Headers.Get("prototype").Set("set", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		args[0].Set(normalizeHeaderKey(args[1]), jsvalue.NewString(args[2].String()))
		return args[0]
	}).MarkAsMethod())
	Headers.Get("prototype").Set("append", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		key := normalizeHeaderKey(args[1])
		existing := args[0].Get(key)
		if existing.TypeString() == "undefined" {
			args[0].Set(key, jsvalue.NewString(args[2].String()))
		} else {
			args[0].Set(key, jsvalue.NewString(existing.String()+", "+args[2].String()))
		}
		return args[0]
	}).MarkAsMethod())
	Headers.Get("prototype").Set("has", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewBool(false)
		}
		return jsvalue.NewBool(args[0].HasOwnProperty(normalizeHeaderKey(args[1])))
	}).MarkAsMethod())

	Request.Get("prototype").Set("text", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return promise.Promise.Get("resolve").Call(jsvalue.NewString(""))
		}
		text, errVal := consumeBody(args[0])
		if errVal != nil {
			return promise.Promise.Get("reject").Call(errVal)
		}
		return promise.Promise.Get("resolve").Call(jsvalue.NewString(text))
	}).MarkAsMethod())
	Request.Get("prototype").Set("json", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return promise.Promise.Get("resolve").Call(jsvalue.NewUndefined())
		}
		text, errVal := consumeBody(args[0])
		if errVal != nil {
			return promise.Promise.Get("reject").Call(errVal)
		}
		parsed, err := parseJSONBody(text)
		if err != nil {
			return promise.Promise.Get("reject").Call(jserror.SyntaxError.Call(jsvalue.NewString(err.Error())))
		}
		return promise.Promise.Get("resolve").Call(parsed)
	}).MarkAsMethod())

	Response.Get("prototype").Set("text", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return promise.Promise.Get("resolve").Call(jsvalue.NewString(""))
		}
		text, errVal := consumeBody(args[0])
		if errVal != nil {
			return promise.Promise.Get("reject").Call(errVal)
		}
		return promise.Promise.Get("resolve").Call(jsvalue.NewString(text))
	}).MarkAsMethod())
	Response.Get("prototype").Set("json", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return promise.Promise.Get("resolve").Call(jsvalue.NewUndefined())
		}
		text, errVal := consumeBody(args[0])
		if errVal != nil {
			return promise.Promise.Get("reject").Call(errVal)
		}
		parsed, err := parseJSONBody(text)
		if err != nil {
			return promise.Promise.Get("reject").Call(jserror.SyntaxError.Call(jsvalue.NewString(err.Error())))
		}
		return promise.Promise.Get("resolve").Call(parsed)
	}).MarkAsMethod())
}

func consumeBody(v *jsvalue.JSValue) (string, *jsvalue.JSValue) {
	if v == nil {
		return "", nil
	}
	if v.Get("bodyUsed").Bool() {
		return "", jserror.TypeError.Call(jsvalue.NewString("Body has already been read"))
	}
	v.Set("bodyUsed", jsvalue.NewBool(true))
	body := v.Get("body")
	if body == nil || body.TypeString() == "undefined" || body.TypeString() == "null" {
		return "", nil
	}
	return body.String(), nil
}

func parseJSONBody(text string) (*jsvalue.JSValue, error) {
	var decoded any
	if err := stdjson.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, err
	}
	return jsonpkg.Parse(jsvalue.NewString(text)), nil
}

func normalizeHeaderKey(v *jsvalue.JSValue) string {
	if v == nil {
		return ""
	}
	return strings.ToLower(v.String())
}

func initHeaders(target *jsvalue.JSValue, init *jsvalue.JSValue) {
	if target == nil || init == nil {
		return
	}
	for _, key := range init.OwnKeys() {
		target.Set(strings.ToLower(key), jsvalue.NewString(init.Get(key).String()))
	}
}

func HeadersFromHTTP(h http.Header) *jsvalue.JSValue {
	headers := newHeadersValue(nil)
	for key, values := range h {
		headers.Set(strings.ToLower(key), jsvalue.NewString(strings.Join(values, ", ")))
	}
	return headers
}

func RequestFromHTTP(r *http.Request) *jsvalue.JSValue {
	body := ""
	if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		r.Body = io.NopCloser(strings.NewReader(body))
	}

	url := r.URL.String()
	if r.URL != nil && !r.URL.IsAbs() {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		host := r.Host
		if host == "" {
			host = "127.0.0.1"
		}
		target := r.RequestURI
		if target == "" {
			target = r.URL.RequestURI()
		}
		if target == "" {
			target = r.URL.String()
		}
		url = scheme + "://" + host + target
	}

	return newRequestValueWithPrototype(Request.Get("prototype"), url, r.Method, HeadersFromHTTP(r.Header), body)
}

func WriteResponse(w http.ResponseWriter, value *jsvalue.JSValue) {
	if value == nil || value.TypeString() == "undefined" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if value.Get("status").TypeString() == "number" || value.Get("headers").TypeString() == "object" || value.Get("_bodyInit").TypeString() != "undefined" {
		headers := value.Get("headers")
		if headers != nil && headers.TypeString() == "object" {
			for _, key := range headers.OwnKeys() {
				w.Header().Set(strings.Trim(key, "\""), headers.Get(key).String())
			}
		}
		status := int(value.Get("status").Number())
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		body := value.Get("_bodyInit")
		if body.TypeString() == "undefined" {
			body = value.Get("body")
		}
		_, _ = io.WriteString(w, body.String())
		return
	}

	_, _ = io.WriteString(w, value.String())
}

// requestPool recycles request JSValue shells between HTTP requests.
// Set() on an existing property updates desc.Value in-place (zero-alloc),
// so pooled requests avoid ~10 allocations per request.
var requestPool = sync.Pool{
	New: func() any {
		req := jsvalue.NewObjectWithPrototype(Request.Get("prototype"))
		req.Set("url", jsvalue.NewString(""))
		req.Set("method", jsvalue.NewString(""))
		req.Set("headers", newHeadersValue(nil))
		req.Set("body", jsvalue.NewString(""))
		req.Set("bodyUsed", jsvalue.NewBool(false))
		req.Set("raw", req)
		return req
	},
}

func RequestFromFastHTTP(ctx *fasthttp.RequestCtx) *jsvalue.JSValue {
	body := string(ctx.PostBody())

	scheme := "http"
	if ctx.IsTLS() {
		scheme = "https"
	}
	host := string(ctx.Host())
	if host == "" {
		host = "127.0.0.1"
	}
	url := scheme + "://" + host + string(ctx.URI().RequestURI())

	method := string(ctx.Method())

	req := requestPool.Get().(*jsvalue.JSValue)
	req.Set("url", jsvalue.NewString(url))
	req.Set("method", jsvalue.NewString(method))
	req.Set("headers", newHeadersValue(nil))
	req.Set("body", jsvalue.NewString(body))
	req.Set("bodyUsed", jsvalue.NewBool(false))
	return req
}

// ReleaseFastHTTPRequest returns a request to the pool.
// Must be called after the response is fully written.
func ReleaseFastHTTPRequest(req *jsvalue.JSValue) {
	if req == nil {
		return
	}
	requestPool.Put(req)
}

func WriteResponseFastHTTP(ctx *fasthttp.RequestCtx, value *jsvalue.JSValue) {
	if value == nil || value.TypeString() == "undefined" {
		ctx.SetStatusCode(fasthttp.StatusNoContent)
		return
	}

	// Fast path: use GetOwn to skip prototype chain and inline cache
	// overhead. Response objects always have these as own properties.
	statusVal, hasStatus := value.GetOwn("status")
	headersVal, hasHeaders := value.GetOwn("headers")
	bodyInitVal, hasBodyInit := value.GetOwn("_bodyInit")

	if hasStatus || hasHeaders || hasBodyInit {
		if hasHeaders && headersVal != nil && headersVal.TypeString() == "object" {
			for _, key := range headersVal.OwnKeys() {
				ctx.Response.Header.Set(key, headersVal.Get(key).String())
			}
		}
		status := fasthttp.StatusOK
		if hasStatus && statusVal != nil && statusVal.TypeString() == "number" {
			status = int(statusVal.Number())
		}
		ctx.SetStatusCode(status)
		body := bodyInitVal
		if !hasBodyInit || body == nil || body.TypeString() == "undefined" {
			body, _ = value.GetOwn("body")
		}
		if body != nil {
			ctx.WriteString(body.String())
		}
		return
	}

	ctx.WriteString(value.String())
}

func URLString(v *jsvalue.JSValue) *jsvalue.JSValue {
	if v == nil {
		return jsvalue.NewString("")
	}
	return jsvalue.NewString(v.String())
}

func ParseURL(raw *jsvalue.JSValue) *jsvalue.JSValue {
	if raw == nil {
		return jsvalue.NewObject()
	}
	u, err := neturl.Parse(raw.String())
	if err != nil {
		return jsvalue.NewObject()
	}
	obj := jsvalue.NewObject()
	obj.Set("href", jsvalue.NewString(u.String()))
	obj.Set("pathname", jsvalue.NewString(u.Path))
	obj.Set("search", jsvalue.NewString(u.RawQuery))
	obj.Set("hostname", jsvalue.NewString(u.Hostname()))
	obj.Set("port", jsvalue.NewString(u.Port()))
	return obj
}

func DecodeURI(raw *jsvalue.JSValue) *jsvalue.JSValue {
	if raw == nil {
		return jsvalue.NewString("")
	}
	if s, err := neturl.PathUnescape(raw.String()); err == nil {
		return jsvalue.NewString(s)
	}
	return jsvalue.NewString(raw.String())
}

func Unescape(raw *jsvalue.JSValue) *jsvalue.JSValue {
	return DecodeURI(raw)
}

func DecodeURIComponent(raw *jsvalue.JSValue) *jsvalue.JSValue {
	if raw == nil {
		return jsvalue.NewString("")
	}
	if s, err := neturl.QueryUnescape(raw.String()); err == nil {
		return jsvalue.NewString(s)
	}
	if s, err := neturl.PathUnescape(raw.String()); err == nil {
		return jsvalue.NewString(s)
	}
	return jsvalue.NewString(raw.String())
}

func EncodeURI(raw *jsvalue.JSValue) *jsvalue.JSValue {
	if raw == nil {
		return jsvalue.NewString("")
	}
	s := raw.String()
	escaped := neturl.PathEscape(s)
	escaped = strings.ReplaceAll(escaped, "%2F", "/")
	escaped = strings.ReplaceAll(escaped, "%3A", ":")
	escaped = strings.ReplaceAll(escaped, "%3F", "?")
	escaped = strings.ReplaceAll(escaped, "%23", "#")
	escaped = strings.ReplaceAll(escaped, "%5B", "[")
	escaped = strings.ReplaceAll(escaped, "%5D", "]")
	escaped = strings.ReplaceAll(escaped, "%40", "@")
	escaped = strings.ReplaceAll(escaped, "%21", "!")
	escaped = strings.ReplaceAll(escaped, "%24", "$")
	escaped = strings.ReplaceAll(escaped, "%26", "&")
	escaped = strings.ReplaceAll(escaped, "%27", "'")
	escaped = strings.ReplaceAll(escaped, "%28", "(")
	escaped = strings.ReplaceAll(escaped, "%29", ")")
	escaped = strings.ReplaceAll(escaped, "%2A", "*")
	escaped = strings.ReplaceAll(escaped, "%2B", "+")
	escaped = strings.ReplaceAll(escaped, "%2C", ",")
	escaped = strings.ReplaceAll(escaped, "%3B", ";")
	escaped = strings.ReplaceAll(escaped, "%3D", "=")
	return jsvalue.NewString(escaped)
}

func EncodeURIComponent(raw *jsvalue.JSValue) *jsvalue.JSValue {
	if raw == nil {
		return jsvalue.NewString("")
	}
	return jsvalue.NewString(neturl.QueryEscape(raw.String()))
}
