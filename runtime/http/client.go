package nodehttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
)

// clientInternal is the Go-side state for a ClientRequest JSValue.
type clientInternal struct {
	mu          sync.Mutex
	method      string
	scheme      string
	host        string // "host:port" — used for fasthttp.URI host
	addr        string // dial target ("host:port")
	path        string
	headers     map[string]string
	body        []byte
	socketPath  string
	tlsCfg      *tls.Config
	agent       *agentInternal
	noAgent     bool
	autoEnd     bool
	cb          *jsvalue.JSValue
	ended       bool
	timeoutMsec int
}

var (
	clientRegistryMu sync.Mutex
	clientRegistry   = map[*jsvalue.JSValue]*clientInternal{}

	unixClientsMu sync.Mutex
	unixClients   = map[string]*fasthttp.Client{}
)

func clientInternalOf(v *jsvalue.JSValue) *clientInternal {
	clientRegistryMu.Lock()
	defer clientRegistryMu.Unlock()
	return clientRegistry[v]
}

func registerClient(v *jsvalue.JSValue, ci *clientInternal) {
	clientRegistryMu.Lock()
	defer clientRegistryMu.Unlock()
	clientRegistry[v] = ci
}

// ClientRequest implements http.request / http.get / https.request / https.get.
// isTLS=true → https; autoEnd=true → wrap as .get() (auto-finalize).
func ClientRequest(isTLS, autoEnd bool, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	this := jsvalue.NewObjectWithPrototype(clientReqCls.Get("prototype"))
	initEvents(this)

	ci := &clientInternal{
		method:  "GET",
		scheme:  schemeFor(isTLS),
		path:    "/",
		headers: map[string]string{},
		autoEnd: autoEnd,
	}
	if isTLS {
		ci.tlsCfg = &tls.Config{}
	}
	parseClientArgs(ci, this, isTLS, args)

	this.Set("method", jsvalue.NewString(ci.method))
	this.Set("path", jsvalue.NewString(ci.path))
	this.Set("host", jsvalue.NewString(ci.host))

	registerClient(this, ci)

	eventloop.Default.RegisterHandle()

	if autoEnd {
		go func() {
			time.Sleep(time.Millisecond)
			this.MethodCall("end")
		}()
	}
	return this
}

func schemeFor(isTLS bool) string {
	if isTLS {
		return "https"
	}
	return "http"
}

// parseClientArgs handles the (urlString | opts | urlString, opts), [cb] forms.
func parseClientArgs(ci *clientInternal, this *jsvalue.JSValue, isTLS bool, args []*jsvalue.JSValue) {
	for _, a := range args {
		if a == nil {
			continue
		}
		switch a.TypeString() {
		case "function":
			ci.cb = a
			this.MethodCall("once", jsvalue.NewString("response"), a)
		case "string":
			applyURLString(ci, a.String())
		case "object":
			applyOptsObject(ci, a, isTLS)
		}
	}
	if ci.host == "" {
		ci.host = defaultHostPort(isTLS)
		ci.addr = ci.host
	}
}

func applyURLString(ci *clientInternal, raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	if u.Scheme != "" {
		ci.scheme = u.Scheme
	}
	host := u.Host
	if host != "" {
		if !strings.Contains(host, ":") {
			host = host + ":" + defaultPortStr(ci.scheme)
		}
		ci.host = host
		ci.addr = host
	}
	if u.Path != "" {
		ci.path = u.Path
	}
	if u.RawQuery != "" {
		ci.path += "?" + u.RawQuery
	}
}

func applyOptsObject(ci *clientInternal, opts *jsvalue.JSValue, isTLS bool) {
	if v := opts.Get("method"); v.TypeString() == "string" {
		ci.method = strings.ToUpper(v.String())
	}
	if v := opts.Get("protocol"); v.TypeString() == "string" {
		s := strings.TrimSuffix(v.String(), ":")
		if s != "" {
			ci.scheme = s
		}
	}
	host := ""
	if v := opts.Get("hostname"); v.TypeString() == "string" {
		host = v.String()
	} else if v := opts.Get("host"); v.TypeString() == "string" {
		host = v.String()
	}
	port := ""
	if v := opts.Get("port"); v.TypeString() == "number" {
		port = strconv.Itoa(int(v.Number()))
	} else if v := opts.Get("port"); v.TypeString() == "string" {
		port = v.String()
	}
	if host != "" {
		if port == "" {
			port = defaultPortStr(ci.scheme)
		}
		if !strings.Contains(host, ":") {
			ci.host = host + ":" + port
		} else {
			ci.host = host
		}
		ci.addr = ci.host
	}
	if v := opts.Get("path"); v.TypeString() == "string" {
		ci.path = v.String()
	}
	if v := opts.Get("socketPath"); v.TypeString() == "string" {
		ci.socketPath = v.String()
	}
	if v := opts.Get("timeout"); v.TypeString() == "number" {
		ci.timeoutMsec = int(v.Number())
	}
	if v := opts.Get("headers"); v.TypeString() == "object" {
		for _, k := range v.OwnKeys() {
			ci.headers[k] = v.Get(k).String()
		}
	}
	if v := opts.Get("agent"); v != nil {
		switch v.TypeString() {
		case "boolean":
			if !v.Bool() {
				ci.noAgent = true
			}
		case "object":
			ci.agent = agentInternalOf(v)
		}
	}
	if isTLS {
		if v := opts.Get("rejectUnauthorized"); v.TypeString() == "boolean" && !v.Bool() {
			ci.tlsCfg.InsecureSkipVerify = true
		}
		if v := opts.Get("ca"); v.TypeString() == "string" {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM([]byte(v.String()))
			ci.tlsCfg.RootCAs = pool
		}
		if v := opts.Get("servername"); v.TypeString() == "string" {
			ci.tlsCfg.ServerName = v.String()
		} else if ci.host != "" {
			h, _, err := net.SplitHostPort(ci.host)
			if err == nil {
				ci.tlsCfg.ServerName = h
			}
		}
	}
}

func defaultHostPort(isTLS bool) string {
	if isTLS {
		return "localhost:443"
	}
	return "localhost:80"
}

func defaultPortStr(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

// dispatchRequest sends the request via fasthttp and emits response/error events.
func (ci *clientInternal) dispatchRequest(this *jsvalue.JSValue) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethod(ci.method)
	for k, v := range ci.headers {
		req.Header.Set(k, v)
	}

	uri := req.URI()
	uri.SetScheme(ci.scheme)
	if ci.host != "" {
		uri.SetHost(ci.host)
	}
	if ci.path != "" {
		if i := strings.IndexByte(ci.path, '?'); i >= 0 {
			uri.SetPath(ci.path[:i])
			uri.SetQueryString(ci.path[i+1:])
		} else {
			uri.SetPath(ci.path)
		}
	}
	if len(ci.body) > 0 {
		req.SetBody(ci.body)
	}

	timeout := time.Duration(ci.timeoutMsec) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var doErr error
	switch {
	case ci.socketPath != "":
		c := unixClientFor(ci.socketPath)
		// fasthttp requires a host header; default to localhost when caller didn't set one.
		if req.URI().Host() == nil || len(req.URI().Host()) == 0 {
			uri.SetHost("localhost")
		}
		doErr = c.DoTimeout(req, resp, timeout)
	case ci.agent != nil && !ci.noAgent:
		hc := ci.agent.hostClient(ci.addr, ci.scheme == "https")
		if ci.tlsCfg != nil {
			hc.TLSConfig = ci.tlsCfg
		}
		doErr = hc.DoTimeout(req, resp, timeout)
	default:
		c := &fasthttp.Client{}
		if ci.tlsCfg != nil {
			c.TLSConfig = ci.tlsCfg
		}
		doErr = c.DoTimeout(req, resp, timeout)
	}

	if doErr != nil {
		errVal := jserror.Error.Call(jsvalue.NewString(doErr.Error()))
		errVal.Set("code", jsvalue.NewString(classifyClientErr(doErr)))
		this.MethodCall("emit", jsvalue.NewString("error"), errVal)
		clientRegistryMu.Lock()
		delete(clientRegistry, this)
		clientRegistryMu.Unlock()
		eventloop.Default.UnregisterHandle()
		return
	}

	respMsg := newClientResponseMessage(resp)
	body := append([]byte(nil), resp.Body()...)

	this.MethodCall("emit", jsvalue.NewString("response"), respMsg)

	go func() {
		defer eventloop.Default.UnregisterHandle()
		if len(body) > 0 {
			respMsg.MethodCall("emit", jsvalue.NewString("data"), jsvalue.NewString(string(body)))
		}
		respMsg.MethodCall("emit", jsvalue.NewString("end"))
		clientRegistryMu.Lock()
		delete(clientRegistry, this)
		clientRegistryMu.Unlock()
	}()
}

func classifyClientErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "timeout"):
		return "ETIMEDOUT"
	case strings.Contains(msg, "refused"):
		return "ECONNREFUSED"
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "lookup"):
		return "ENOTFOUND"
	case strings.Contains(msg, "certificate"), strings.Contains(msg, "tls:"):
		return "CERT_ERROR"
	}
	return "EREQUEST"
}

func unixClientFor(path string) *fasthttp.Client {
	unixClientsMu.Lock()
	defer unixClientsMu.Unlock()
	if c, ok := unixClients[path]; ok {
		return c
	}
	c := &fasthttp.Client{
		Dial: func(_ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(context.Background(), "unix", path)
		},
	}
	unixClients[path] = c
	return c
}

func setupClientPrototype() {
	proto := clientReqCls.Get("prototype")

	proto.Set("setHeader", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 {
			return jsvalue.NewUndefined()
		}
		ci := clientInternalOf(args[0])
		if ci == nil {
			return jsvalue.NewUndefined()
		}
		ci.mu.Lock()
		ci.headers[args[1].String()] = args[2].String()
		ci.mu.Unlock()
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("getHeader", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewUndefined()
		}
		ci := clientInternalOf(args[0])
		if ci == nil {
			return jsvalue.NewUndefined()
		}
		ci.mu.Lock()
		defer ci.mu.Unlock()
		key := args[1].String()
		for k, v := range ci.headers {
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
		ci := clientInternalOf(args[0])
		if ci == nil {
			return jsvalue.NewUndefined()
		}
		ci.mu.Lock()
		defer ci.mu.Unlock()
		key := args[1].String()
		for k := range ci.headers {
			if strings.EqualFold(k, key) {
				delete(ci.headers, k)
			}
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewBool(true)
		}
		ci := clientInternalOf(args[0])
		if ci == nil {
			return jsvalue.NewBool(false)
		}
		ci.mu.Lock()
		ci.body = append(ci.body, []byte(args[1].String())...)
		ci.mu.Unlock()
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("end", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		ci := clientInternalOf(this)
		if ci == nil {
			return jsvalue.NewUndefined()
		}
		ci.mu.Lock()
		if ci.ended {
			ci.mu.Unlock()
			return jsvalue.NewUndefined()
		}
		ci.ended = true
		if len(args) > 1 && args[1] != nil && args[1].TypeString() != "undefined" {
			ci.body = append(ci.body, []byte(args[1].String())...)
		}
		ci.mu.Unlock()

		go func() {
			ci.dispatchRequest(this)
		}()
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("abort", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		this.MethodCall("emit", jsvalue.NewString("abort"))
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
}
