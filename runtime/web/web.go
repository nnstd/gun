package web

import (
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

var Headers = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) > 0 && args[0] != nil {
		initHeaders(this, args[0])
	}
	return nil
}, nil)

var Request = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var input *jsvalue.JSValue
	if len(args) > 0 {
		input = args[0]
	}
	var init *jsvalue.JSValue
	if len(args) > 1 {
		init = args[1]
	}

	url := ""
	if input != nil {
		if input.Get("url").Bool() || input.Get("url").TypeString() == "string" {
			url = input.Get("url").String()
		} else {
			url = input.String()
		}
	}
	if url == "" {
		url = "http://127.0.0.1/"
	}

	method := jsvalue.NewString("GET")
	headers := Headers.Call()
	body := jsvalue.NewString("")

	if init != nil && init.TypeString() == "object" {
		if init.Get("method").Bool() || init.Get("method").TypeString() == "string" {
			method = jsvalue.NewString(strings.ToUpper(init.Get("method").String()))
		}
		if init.Get("headers").Bool() || init.Get("headers").TypeString() == "object" {
			headers = Headers.Call(init.Get("headers"))
		}
		if init.Get("body").Bool() || init.Get("body").TypeString() == "string" {
			body = jsvalue.NewString(init.Get("body").String())
		}
	}

	this.Set("url", jsvalue.NewString(url))
	this.Set("method", method)
	this.Set("headers", headers)
	this.Set("body", body)
	this.Set("raw", this)
	return nil
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
	headers := Headers.Call()

	if init != nil && init.TypeString() == "object" {
		if init.Get("status").Bool() || init.Get("status").TypeString() == "number" {
			status = jsvalue.NewNumber(init.Get("status").Number())
		}
		if init.Get("statusText").Bool() || init.Get("statusText").TypeString() == "string" {
			statusText = jsvalue.NewString(init.Get("statusText").String())
		}
		if init.Get("headers").Bool() || init.Get("headers").TypeString() == "object" {
			headers = Headers.Call(init.Get("headers"))
		}
	}

	if body == nil {
		body = jsvalue.NewString("")
	}

	this.Set("status", status)
	this.Set("statusText", statusText)
	this.Set("headers", headers)
	this.Set("body", body)
	this.Set("_bodyInit", body)
	this.Set("ok", jsvalue.NewBool(status.Number() >= 200 && status.Number() < 300))
	return nil
}, nil)

var URL = jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) > 0 {
		return ParseURL(args[0])
	}
	return ParseURL(jsvalue.NewString(""))
})

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
			return jsvalue.NewString("")
		}
		return jsvalue.NewString(args[0].Get("body").String())
	}).MarkAsMethod())
	Request.Get("prototype").Set("json", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		return jsvalue.NewString(args[0].Get("body").String())
	}).MarkAsMethod())

	Response.Get("prototype").Set("text", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewString("")
		}
		return jsvalue.NewString(args[0].Get("body").String())
	}).MarkAsMethod())
	Response.Get("prototype").Set("json", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		return jsvalue.NewString(args[0].Get("body").String())
	}).MarkAsMethod())
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
	headers := Headers.Call()
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

	req := Request.Call(
		jsvalue.NewString(r.URL.String()),
		jsvalue.ObjectFrom(
			"method", jsvalue.NewString(r.Method),
			"headers", HeadersFromHTTP(r.Header),
			"body", jsvalue.NewString(body),
		),
	)
	req.Set("raw", req)
	return req
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
				w.Header().Set(key, headers.Get(key).String())
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
