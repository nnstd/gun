package hono

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Hono is a lightweight HTTP router compatible with the Hono API.
type Hono struct {
	routes []route
	Fetch  http.Handler
}

type route struct {
	method  string
	pattern string
	handler func(*Context) any
}

// New creates a new Hono instance.
func New() *Hono {
	h := &Hono{}
	h.Fetch = h
	return h
}

// Get registers a GET route.
func (h *Hono) Get(pattern string, handler func(*Context) any) {
	h.routes = append(h.routes, route{method: "GET", pattern: pattern, handler: handler})
}

// Post registers a POST route.
func (h *Hono) Post(pattern string, handler func(*Context) any) {
	h.routes = append(h.routes, route{method: "POST", pattern: pattern, handler: handler})
}

// Put registers a PUT route.
func (h *Hono) Put(pattern string, handler func(*Context) any) {
	h.routes = append(h.routes, route{method: "PUT", pattern: pattern, handler: handler})
}

// Delete registers a DELETE route.
func (h *Hono) Delete(pattern string, handler func(*Context) any) {
	h.routes = append(h.routes, route{method: "DELETE", pattern: pattern, handler: handler})
}

// Patch registers a PATCH route.
func (h *Hono) Patch(pattern string, handler func(*Context) any) {
	h.routes = append(h.routes, route{method: "PATCH", pattern: pattern, handler: handler})
}

// ServeHTTP implements http.Handler.
func (h *Hono) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, rt := range h.routes {
		if rt.method != r.Method {
			continue
		}
		params, ok := matchPath(rt.pattern, r.URL.Path)
		if !ok {
			continue
		}
		c := &Context{
			writer:  w,
			request: r,
			params:  params,
			status:  200,
		}
		rt.handler(c)
		return
	}
	http.NotFound(w, r)
}

// Context holds request/response state for a single HTTP request.
type Context struct {
	writer  http.ResponseWriter
	request *http.Request
	params  map[string]string
	status  int
}

// Text sends a plain text response.
func (c *Context) Text(body string) any {
	c.writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.writer.WriteHeader(c.status)
	fmt.Fprint(c.writer, body)
	return nil
}

// Json sends a JSON response.
func (c *Context) Json(data any) any {
	c.writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.writer.WriteHeader(c.status)
	json.NewEncoder(c.writer).Encode(data)
	return nil
}

// Html sends an HTML response.
func (c *Context) Html(body string) any {
	c.writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.writer.WriteHeader(c.status)
	fmt.Fprint(c.writer, body)
	return nil
}

// Status sets the HTTP status code and returns the context for chaining.
func (c *Context) Status(code int) *Context {
	c.status = code
	return c
}

// Param returns a path parameter by name.
func (c *Context) Param(name string) string {
	return c.params[name]
}

// Query returns a query parameter by name.
func (c *Context) Query(name string) string {
	return c.request.URL.Query().Get(name)
}

// matchPath matches a URL path against a pattern with :param support.
// Returns extracted parameters and whether the path matched.
func matchPath(pattern, path string) (map[string]string, bool) {
	params := make(map[string]string)

	// Exact match for simple patterns
	if !strings.Contains(pattern, ":") {
		return params, pattern == path
	}

	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(patternParts) != len(pathParts) {
		return nil, false
	}

	for i, pp := range patternParts {
		if strings.HasPrefix(pp, ":") {
			params[pp[1:]] = pathParts[i]
		} else if pp != pathParts[i] {
			return nil, false
		}
	}

	return params, true
}
