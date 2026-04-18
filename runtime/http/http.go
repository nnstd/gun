package nodehttp

import (
	"strconv"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
)

var (
	statusCodes  *jsvalue.JSValue
	methods      *jsvalue.JSValue
	httpAgent    *jsvalue.JSValue
	httpsAgent   *jsvalue.JSValue
	httpGlobal   *jsvalue.JSValue
	httpsGlobal  *jsvalue.JSValue
	serverClass  *jsvalue.JSValue
	responseCls  *jsvalue.JSValue
	incomingCls  *jsvalue.JSValue
	clientReqCls *jsvalue.JSValue
	agentClass   *jsvalue.JSValue

	AsJSValue      *jsvalue.JSValue
	HTTPSAsJSValue *jsvalue.JSValue
)

func buildStatusCodes() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	for code, reason := range statusCodeReasons {
		obj.Set(strconv.Itoa(code), jsvalue.NewString(reason))
	}
	return obj
}

func buildMethods() *jsvalue.JSValue {
	arr := jsvalue.NewArray()
	for _, m := range methodNames {
		arr.MethodCall("push", jsvalue.NewString(m))
	}
	return arr
}

func buildAgent(defaultPort int) *jsvalue.JSValue {
	a := newAgentInstance(jsvalue.NewObject())
	a.Set("defaultPort", jsvalue.NewNumber(float64(defaultPort)))
	a.Set("protocol", jsvalue.NewString(map[bool]string{true: "https:", false: "http:"}[defaultPort == 443]))
	return a
}

// validateHeaderName matches Node's check: must be a non-empty token (RFC 7230).
func validateHeaderName(name *jsvalue.JSValue) {
	if name == nil || name.TypeString() != "string" {
		panic(jserror.InvalidArgType("Header name must be a valid HTTP token"))
	}
	s := name.String()
	if s == "" {
		panic(jserror.InvalidArgType("Header name must be a valid HTTP token [\"\"]"))
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isTokenChar(c) {
			panic(jserror.InvalidArgType("Header name must be a valid HTTP token [\"" + s + "\"]"))
		}
	}
}

// validateHeaderValue rejects values containing CR/LF or NUL.
func validateHeaderValue(name, value *jsvalue.JSValue) {
	if value == nil || value.TypeString() == "undefined" {
		panic(jserror.InvalidArgType("Invalid value \"undefined\" for header \"" + headerNameStr(name) + "\""))
	}
	s := value.String()
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\r' || c == '\n' || c == 0 {
			panic(jserror.InvalidArgType("Invalid character in header content [\"" + headerNameStr(name) + "\"]"))
		}
	}
}

func headerNameStr(v *jsvalue.JSValue) string {
	if v == nil {
		return ""
	}
	return v.String()
}

func isTokenChar(c byte) bool {
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	if c >= '0' && c <= '9' {
		return true
	}
	if c >= 'a' && c <= 'z' {
		return true
	}
	if c >= 'A' && c <= 'Z' {
		return true
	}
	return false
}

func init() {
	statusCodes = buildStatusCodes()
	methods = buildMethods()

	// Class stubs — full implementations land in server.go / client.go.
	serverClass = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this.Set("_isTLS", jsvalue.NewBool(false))
		return nil
	}, nil)
	responseCls = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this.Set("statusCode", jsvalue.NewNumber(200))
		this.Set("headersSent", jsvalue.NewBool(false))
		return nil
	}, nil)
	incomingCls = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this.Set("httpVersion", jsvalue.NewString("1.1"))
		return nil
	}, nil)
	clientReqCls = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return nil
	}, nil)
	agentClass = newAgentClass()

	mixEventEmitter(serverClass)
	mixEventEmitter(responseCls)
	mixEventEmitter(incomingCls)
	mixEventEmitter(clientReqCls)

	httpAgent = buildAgent(80)
	httpsAgent = buildAgent(443)

	httpGlobal = jsvalue.ObjectFrom(
		"createServer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return CreateServer(false, args...)
		}),
		"request", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return ClientRequest(false, false, args...)
		}),
		"get", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return ClientRequest(false, true, args...)
		}),
		"METHODS", methods,
		"STATUS_CODES", statusCodes,
		"globalAgent", httpAgent,
		"maxHeaderSize", jsvalue.NewNumber(16384),
		"validateHeaderName", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 {
				validateHeaderName(nil)
			} else {
				validateHeaderName(args[0])
			}
			return jsvalue.NewUndefined()
		}),
		"validateHeaderValue", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			var name, value *jsvalue.JSValue
			if len(args) > 0 {
				name = args[0]
			}
			if len(args) > 1 {
				value = args[1]
			}
			validateHeaderValue(name, value)
			return jsvalue.NewUndefined()
		}),
		"Server", serverClass,
		"ServerResponse", responseCls,
		"IncomingMessage", incomingCls,
		"ClientRequest", clientReqCls,
		"Agent", agentClass,
	)
	AsJSValue = httpGlobal

	httpsGlobal = jsvalue.ObjectFrom(
		"createServer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return CreateServer(true, args...)
		}),
		"request", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return ClientRequest(true, false, args...)
		}),
		"get", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return ClientRequest(true, true, args...)
		}),
		"globalAgent", httpsAgent,
		"Server", serverClass,
		"Agent", agentClass,
	)
	HTTPSAsJSValue = httpsGlobal
}

// normalizeHeaderKey returns the lowercased form of a header name.
func normalizeHeaderKey(s string) string {
	return strings.ToLower(s)
}
