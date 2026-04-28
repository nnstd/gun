package nodehttp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"maps"
	"net"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

// TransportRequest is a transport-level HTTP request description suitable for
// fetch-style callers and Node client adapters.
type TransportRequest struct {
	Method             string
	URL                string
	PublicURL          string
	Headers            map[string]string
	Body               []byte
	TimeoutMsec        int
	SocketPath         string
	RejectUnauthorized *bool
	CA                 string
	ServerName         string

	scheme  string
	host    string
	addr    string
	path    string
	tlsCfg  *tls.Config
	agent   *agentInternal
	noAgent bool
}

// TransportResponse is the buffered response returned by the shared transport.
type TransportResponse struct {
	StatusCode int
	StatusText string
	Headers    map[string]string
	RawHeaders []TransportHeader
	Body       []byte
	URL        string
}

type TransportHeader struct {
	Key   string
	Value string
}

func newTransportRequest() *TransportRequest {
	return &TransportRequest{
		Method:  "GET",
		Headers: map[string]string{},
		path:    "/",
	}
}

func transportRequestFromClient(ci *clientInternal) *TransportRequest {
	if ci == nil {
		return newTransportRequest()
	}
	body := append([]byte(nil), ci.body...)
	headers := make(map[string]string, len(ci.headers))
	maps.Copy(headers, ci.headers)
	return &TransportRequest{
		Method:      ci.method,
		Headers:     headers,
		Body:        body,
		TimeoutMsec: ci.timeoutMsec,
		SocketPath:  ci.socketPath,
		scheme:      ci.scheme,
		host:        ci.host,
		addr:        ci.addr,
		path:        ci.path,
		tlsCfg:      ci.tlsCfg,
		agent:       ci.agent,
		noAgent:     ci.noAgent,
	}
}

// DoTransportAsync executes req in a goroutine and re-enters JS-visible
// completion on the provided callback.
func DoTransportAsync(req *TransportRequest, onComplete func(*TransportResponse, error)) {
	go func() {
		resp, err := DoTransport(req)
		onComplete(resp, err)
	}()
}

// DoTransport performs the buffered HTTP request synchronously.
func DoTransport(req *TransportRequest) (*TransportResponse, error) {
	cfg, err := normalizeTransportRequest(req)
	if err != nil {
		return nil, err
	}

	freq := fasthttp.AcquireRequest()
	fresp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(freq)
	defer fasthttp.ReleaseResponse(fresp)

	freq.Header.SetMethod(cfg.Method)
	for k, v := range cfg.Headers {
		freq.Header.Set(k, v)
	}

	uri := freq.URI()
	uri.SetScheme(cfg.Scheme)
	if cfg.Host != "" {
		uri.SetHost(cfg.Host)
	}
	if cfg.Path != "" {
		if i := strings.IndexByte(cfg.Path, '?'); i >= 0 {
			uri.SetPath(cfg.Path[:i])
			uri.SetQueryString(cfg.Path[i+1:])
		} else {
			uri.SetPath(cfg.Path)
		}
	}
	if len(cfg.Body) > 0 {
		freq.SetBody(cfg.Body)
	}

	timeout := time.Duration(cfg.TimeoutMsec) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var doErr error
	switch {
	case cfg.SocketPath != "":
		c := unixClientFor(cfg.SocketPath)
		if freq.URI().Host() == nil || len(freq.URI().Host()) == 0 {
			uri.SetHost("localhost")
		}
		doErr = c.DoTimeout(freq, fresp, timeout)
	case cfg.Agent != nil && !cfg.NoAgent:
		hc := cfg.Agent.hostClient(cfg.Addr, cfg.Scheme == "https")
		if cfg.TLSConfig != nil {
			hc.TLSConfig = cfg.TLSConfig
		}
		doErr = hc.DoTimeout(freq, fresp, timeout)
	default:
		c := &fasthttp.Client{}
		if cfg.TLSConfig != nil {
			c.TLSConfig = cfg.TLSConfig
		}
		doErr = c.DoTimeout(freq, fresp, timeout)
	}
	if doErr != nil {
		return nil, doErr
	}

	headers := map[string]string{}
	rawHeaders := []TransportHeader{}
	for key, value := range fresp.Header.All() {
		keyStr := string(key)
		valueStr := string(value)
		headers[strings.ToLower(keyStr)] = valueStr
		rawHeaders = append(rawHeaders, TransportHeader{Key: keyStr, Value: valueStr})
	}
	statusText := string(fresp.Header.StatusMessage())
	if statusText == "" {
		statusText = stdhttp.StatusText(fresp.StatusCode())
	}

	return &TransportResponse{
		StatusCode: fresp.StatusCode(),
		StatusText: statusText,
		Headers:    headers,
		RawHeaders: rawHeaders,
		Body:       append([]byte(nil), fresp.Body()...),
		URL:        cfg.PublicURL,
	}, nil
}

type normalizedTransportRequest struct {
	Method      string
	Scheme      string
	Host        string
	Addr        string
	Path        string
	PublicURL   string
	Headers     map[string]string
	Body        []byte
	SocketPath  string
	TimeoutMsec int
	TLSConfig   *tls.Config
	Agent       *agentInternal
	NoAgent     bool
}

func normalizeTransportRequest(req *TransportRequest) (*normalizedTransportRequest, error) {
	if req == nil {
		req = newTransportRequest()
	}
	cfg := &normalizedTransportRequest{
		Method:      strings.ToUpper(strings.TrimSpace(req.Method)),
		Headers:     map[string]string{},
		Body:        append([]byte(nil), req.Body...),
		SocketPath:  req.SocketPath,
		TimeoutMsec: req.TimeoutMsec,
		Agent:       req.agent,
		NoAgent:     req.noAgent,
	}
	if cfg.Method == "" {
		cfg.Method = "GET"
	}
	maps.Copy(cfg.Headers, req.Headers)

	if req.URL != "" {
		u, err := url.Parse(req.URL)
		if err != nil {
			return nil, err
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("absolute URL required")
		}
		cfg.Scheme = u.Scheme
		cfg.Host = u.Host
		if !strings.Contains(cfg.Host, ":") {
			cfg.Host = cfg.Host + ":" + defaultPortStr(cfg.Scheme)
		}
		cfg.Addr = cfg.Host
		cfg.Path = u.EscapedPath()
		if cfg.Path == "" {
			cfg.Path = "/"
		}
		if u.RawQuery != "" {
			cfg.Path += "?" + u.RawQuery
		}
		cfg.PublicURL = req.URL
	} else {
		cfg.Scheme = req.scheme
		cfg.Host = req.host
		cfg.Addr = req.addr
		cfg.Path = req.path
		cfg.PublicURL = req.PublicURL
	}
	if cfg.Scheme == "" {
		cfg.Scheme = "http"
	}
	if cfg.Host == "" && cfg.SocketPath == "" {
		cfg.Host = defaultHostPort(cfg.Scheme == "https")
		cfg.Addr = cfg.Host
	}
	if cfg.Addr == "" {
		cfg.Addr = cfg.Host
	}
	if cfg.Path == "" {
		cfg.Path = "/"
	}
	if cfg.PublicURL == "" {
		host := req.host
		if host == "" {
			host = cfg.Host
		}
		cfg.PublicURL = cfg.Scheme + "://" + host + cfg.Path
	}

	cfg.TLSConfig = req.tlsCfg
	if cfg.Scheme == "https" && cfg.TLSConfig == nil {
		cfg.TLSConfig = &tls.Config{}
	}
	if cfg.TLSConfig != nil {
		if req.RejectUnauthorized != nil && !*req.RejectUnauthorized {
			cfg.TLSConfig.InsecureSkipVerify = true
		}
		if req.CA != "" {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM([]byte(req.CA))
			cfg.TLSConfig.RootCAs = pool
		}
		if req.ServerName != "" {
			cfg.TLSConfig.ServerName = req.ServerName
		} else if cfg.Host != "" {
			h, _, err := net.SplitHostPort(cfg.Host)
			if err == nil {
				cfg.TLSConfig.ServerName = h
			}
		}
	}
	return cfg, nil
}
