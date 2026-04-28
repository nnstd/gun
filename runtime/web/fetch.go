package web

import (
	"net/url"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
	nodehttp "github.com/nnstd/gun/runtime/http"
	"github.com/nnstd/gun/runtime/promise"
)

var Fetch = jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var input *jsvalue.JSValue
	if len(args) > 0 {
		input = args[0]
	}
	var init *jsvalue.JSValue
	if len(args) > 1 {
		init = args[1]
	}

	return promise.Promise.Call(jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
		resolve := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() })
		reject := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() })
		if len(inner) > 0 && inner[0] != nil {
			resolve = inner[0]
		}
		if len(inner) > 1 && inner[1] != nil {
			reject = inner[1]
		}

		spec, errVal := buildFetchTransport(input, init)
		if errVal != nil {
			reject.Call(errVal)
			return jsvalue.NewUndefined()
		}

		nodehttp.DoTransportAsync(spec, func(resp *nodehttp.TransportResponse, err error) {
			eventloop.Default.ScheduleCallback(func() {
				if err != nil {
					reject.Call(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
					return
				}
				resolve.Call(responseFromTransport(resp))
			})
		})
		return jsvalue.NewUndefined()
	}))
})

func init() {
	jsvalue.RegisterGlobal("Headers", Headers)
	jsvalue.RegisterGlobal("Request", Request)
	jsvalue.RegisterGlobal("Response", Response)
	jsvalue.RegisterGlobal("URL", URL)
	jsvalue.RegisterGlobal("File", File)
	jsvalue.RegisterGlobal("AbortController", AbortController)
	jsvalue.RegisterGlobal("AbortSignal", AbortSignal)
	jsvalue.RegisterGlobal("fetch", Fetch)
}

func buildFetchTransport(input, init *jsvalue.JSValue) (*nodehttp.TransportRequest, *jsvalue.JSValue) {
	req, errVal := resolveFetchRequest(input, init)
	if errVal != nil {
		return nil, errVal
	}

	method := strings.ToUpper(req.Get("method").String())
	body := req.Get("body").String()
	if (method == "GET" || method == "HEAD") && body != "" {
		return nil, jserror.TypeError.Call(jsvalue.NewString("Request with GET/HEAD method cannot have body"))
	}

	spec := &nodehttp.TransportRequest{
		Method:    method,
		URL:       req.Get("url").String(),
		PublicURL: req.Get("url").String(),
		Headers:   headersToMap(req.Get("headers")),
	}
	if _, err := parseAbsoluteURL(spec.URL); err != nil {
		return nil, jserror.TypeError.Call(jsvalue.NewString(err.Error()))
	}
	if body != "" {
		spec.Body = []byte(body)
	}

	if init != nil && init.TypeString() == "object" {
		if v := init.Get("timeout"); v.TypeString() == "number" {
			spec.TimeoutMsec = int(v.Number())
		}
		if v := init.Get("rejectUnauthorized"); v.TypeString() == "boolean" {
			b := v.Bool()
			spec.RejectUnauthorized = &b
		}
		if v := init.Get("ca"); v.TypeString() == "string" {
			spec.CA = v.String()
		}
		if v := init.Get("servername"); v.TypeString() == "string" {
			spec.ServerName = v.String()
		}
	}
	return spec, nil
}

func resolveFetchRequest(input, init *jsvalue.JSValue) (*jsvalue.JSValue, *jsvalue.JSValue) {
	base := newRequestValueWithPrototype(Request.Get("prototype"), "", "GET", Headers.Call(), "")
	base.Set("url", jsvalue.NewString(""))
	if input != nil {
		switch input.TypeString() {
		case "object":
			if input.Get("url").TypeString() == "string" {
				if input.Get("bodyUsed").Bool() {
					return nil, jserror.TypeError.Call(jsvalue.NewString("Cannot construct a fetch request from a consumed Request body"))
				}
				input.Set("bodyUsed", jsvalue.NewBool(true))
				base = newRequestValueWithPrototype(
					Request.Get("prototype"),
					input.Get("url").String(),
					valueOrDefault(input.Get("method"), "GET"),
					cloneHeaders(input.Get("headers")),
					input.Get("body").String(),
				)
			} else if input.Get("href").TypeString() == "string" {
				base = newRequestValueWithPrototype(Request.Get("prototype"), input.Get("href").String(), "GET", Headers.Call(), "")
			} else {
				return nil, jserror.TypeError.Call(jsvalue.NewString("fetch input must be a string, Request, or URL with href"))
			}
		case "string":
			base = newRequestValueWithPrototype(Request.Get("prototype"), input.String(), "GET", Headers.Call(), "")
		default:
			return nil, jserror.TypeError.Call(jsvalue.NewString("fetch input must be a string, Request, or URL with href"))
		}
	}

	if init != nil && init.TypeString() == "object" {
		if v := init.Get("method"); v.TypeString() == "string" {
			base.Set("method", jsvalue.NewString(strings.ToUpper(v.String())))
		}
		if v := init.Get("headers"); v.TypeString() == "object" {
			base.Set("headers", cloneHeaders(v))
		}
		if v := init.Get("body"); v.TypeString() == "string" {
			base.Set("body", jsvalue.NewString(v.String()))
		}
	}

	if base.Get("url").TypeString() != "string" || strings.TrimSpace(base.Get("url").String()) == "" {
		return nil, jserror.TypeError.Call(jsvalue.NewString("fetch requires a non-empty absolute URL"))
	}
	return base, nil
}

func parseAbsoluteURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, url.InvalidHostError("absolute URL required")
	}
	if u.User != nil {
		return nil, url.InvalidHostError("URLs with embedded credentials are not supported")
	}
	return u, nil
}

func cloneHeaders(v *jsvalue.JSValue) *jsvalue.JSValue {
	if v == nil || v.TypeString() != "object" {
		return Headers.Call()
	}
	headers := Headers.Call()
	for _, key := range v.OwnKeys() {
		headers.Set(normalizeHeaderKey(jsvalue.NewString(key)), jsvalue.NewString(v.Get(key).String()))
	}
	return headers
}

func headersToMap(v *jsvalue.JSValue) map[string]string {
	out := map[string]string{}
	if v == nil || v.TypeString() != "object" {
		return out
	}
	for _, key := range v.OwnKeys() {
		out[key] = v.Get(key).String()
	}
	return out
}

func responseFromTransport(resp *nodehttp.TransportResponse) *jsvalue.JSValue {
	if resp == nil {
		res := newResponseValue(Response.Get("prototype"), jsvalue.NewString(""), jsvalue.NewNumber(0), jsvalue.NewString(""), Headers.Call())
		res.Set("url", jsvalue.NewString(""))
		res.Set("redirected", jsvalue.NewBool(false))
		return res
	}

	headers := Headers.Call()
	for key, value := range resp.Headers {
		headers.Set(normalizeHeaderKey(jsvalue.NewString(key)), jsvalue.NewString(value))
	}

	status := jsvalue.NewNumber(float64(resp.StatusCode))
	statusText := jsvalue.NewString(resp.StatusText)
	body := jsvalue.NewString(string(resp.Body))
	res := newResponseValue(Response.Get("prototype"), body, status, statusText, headers)
	res.Set("url", jsvalue.NewString(resp.URL))
	res.Set("redirected", jsvalue.NewBool(false))
	return res
}

func valueOrDefault(v *jsvalue.JSValue, fallback string) string {
	if v == nil {
		return fallback
	}
	if v.TypeString() == "string" {
		return v.String()
	}
	return fallback
}
