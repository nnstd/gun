package nodehttp

import (
	"strconv"
	"sync"

	"github.com/valyala/fasthttp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// agentInternal holds the Go-side state for an Agent instance.
type agentInternal struct {
	mu          sync.Mutex
	hosts       map[string]*fasthttp.HostClient
	keepAlive   bool
	maxConns    int
	timeoutMsec int
	tlsConfig   any // *tls.Config; populated when TLS
}

func (a *agentInternal) hostClient(host string, isTLS bool) *fasthttp.HostClient {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.hosts == nil {
		a.hosts = make(map[string]*fasthttp.HostClient)
	}
	hc, ok := a.hosts[host]
	if ok {
		return hc
	}
	hc = &fasthttp.HostClient{Addr: host, IsTLS: isTLS}
	if a.maxConns > 0 {
		hc.MaxConns = a.maxConns
	}
	a.hosts[host] = hc
	return hc
}

func (a *agentInternal) destroy() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, hc := range a.hosts {
		hc.CloseIdleConnections()
	}
	a.hosts = nil
}

// agentRegistry maps an Agent JSValue to its internal Go state.
var (
	agentRegistryMu sync.Mutex
	agentRegistry   = map[*jsvalue.JSValue]*agentInternal{}
)

func agentInternalOf(v *jsvalue.JSValue) *agentInternal {
	agentRegistryMu.Lock()
	defer agentRegistryMu.Unlock()
	if a, ok := agentRegistry[v]; ok {
		return a
	}
	a := &agentInternal{keepAlive: true}
	agentRegistry[v] = a
	return a
}

func newAgentInstance(opts *jsvalue.JSValue) *jsvalue.JSValue {
	this := jsvalue.NewObject()
	this.SetPrototype(agentClass.Get("prototype"))
	a := agentInternalOf(this)
	if opts != nil && opts.TypeString() == "object" {
		if v := opts.Get("keepAlive"); v.TypeString() == "boolean" {
			a.keepAlive = v.Bool()
		}
		if v := opts.Get("maxSockets"); v.TypeString() == "number" {
			a.maxConns = int(v.Number())
		}
		if v := opts.Get("timeout"); v.TypeString() == "number" {
			a.timeoutMsec = int(v.Number())
		}
	}
	this.Set("keepAlive", jsvalue.NewBool(a.keepAlive))
	this.Set("maxSockets", jsvalue.NewNumber(float64(a.maxConns)))
	this.Set("sockets", jsvalue.NewObject())
	this.Set("requests", jsvalue.NewObject())
	this.Set("freeSockets", jsvalue.NewObject())
	return this
}

func newAgentClass() *jsvalue.JSValue {
	cls := jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var opts *jsvalue.JSValue
		if len(args) > 0 {
			opts = args[0]
		}
		_ = agentInternalOf(this)
		_ = opts
		this.Set("keepAlive", jsvalue.NewBool(true))
		this.Set("sockets", jsvalue.NewObject())
		this.Set("requests", jsvalue.NewObject())
		this.Set("freeSockets", jsvalue.NewObject())
		if opts != nil && opts.TypeString() == "object" {
			a := agentInternalOf(this)
			if v := opts.Get("keepAlive"); v.TypeString() == "boolean" {
				a.keepAlive = v.Bool()
				this.Set("keepAlive", jsvalue.NewBool(a.keepAlive))
			}
			if v := opts.Get("maxSockets"); v.TypeString() == "number" {
				a.maxConns = int(v.Number())
				this.Set("maxSockets", jsvalue.NewNumber(float64(a.maxConns)))
			}
			if v := opts.Get("timeout"); v.TypeString() == "number" {
				a.timeoutMsec = int(v.Number())
			}
		}
		return nil
	}, nil)

	cls.Get("prototype").Set("getName", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[1] == nil {
			return jsvalue.NewString("")
		}
		opts := args[1]
		host := "localhost"
		port := 80
		path := ""
		if v := opts.Get("host"); v.TypeString() == "string" {
			host = v.String()
		}
		if v := opts.Get("port"); v.TypeString() == "number" {
			port = int(v.Number())
		}
		if v := opts.Get("path"); v.TypeString() == "string" {
			path = v.String()
		}
		return jsvalue.NewString(host + ":" + strconv.Itoa(port) + ":" + path)
	}).MarkAsMethod())

	cls.Get("prototype").Set("destroy", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		agentInternalOf(args[0]).destroy()
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	return cls
}
