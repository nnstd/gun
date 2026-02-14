package hono

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	app := New()
	if app == nil {
		t.Fatal("New() returned nil")
	}
	if app.Fetch == nil {
		t.Fatal("Fetch field is nil")
	}
}

func TestGetRoute(t *testing.T) {
	app := New()
	app.Get("/", func(c *Context) any {
		return c.Text("hello")
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "hello" {
		t.Fatalf("expected 'hello', got %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected text/plain content type, got %q", ct)
	}
}

func TestPostRoute(t *testing.T) {
	app := New()
	app.Post("/items", func(c *Context) any {
		return c.Text("created")
	})

	req := httptest.NewRequest("POST", "/items", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "created" {
		t.Fatalf("expected 'created', got %q", w.Body.String())
	}
}

func TestParamRoute(t *testing.T) {
	app := New()
	app.Get("/users/:id", func(c *Context) any {
		return c.Text("user:" + c.Param("id"))
	})

	req := httptest.NewRequest("GET", "/users/42", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "user:42" {
		t.Fatalf("expected 'user:42', got %q", w.Body.String())
	}
}

func TestQueryParam(t *testing.T) {
	app := New()
	app.Get("/search", func(c *Context) any {
		return c.Text("q:" + c.Query("q"))
	})

	req := httptest.NewRequest("GET", "/search?q=hello", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Body.String() != "q:hello" {
		t.Fatalf("expected 'q:hello', got %q", w.Body.String())
	}
}

func TestJsonResponse(t *testing.T) {
	app := New()
	app.Get("/data", func(c *Context) any {
		return c.Json(map[string]string{"key": "value"})
	})

	req := httptest.NewRequest("GET", "/data", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("expected json content type, got %q", ct)
	}
}

func TestHtmlResponse(t *testing.T) {
	app := New()
	app.Get("/page", func(c *Context) any {
		return c.Html("<h1>Hi</h1>")
	})

	req := httptest.NewRequest("GET", "/page", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected html content type, got %q", ct)
	}
	if w.Body.String() != "<h1>Hi</h1>" {
		t.Fatalf("expected '<h1>Hi</h1>', got %q", w.Body.String())
	}
}

func TestStatusCode(t *testing.T) {
	app := New()
	app.Get("/notfound", func(c *Context) any {
		return c.Status(404).Text("not found")
	})

	req := httptest.NewRequest("GET", "/notfound", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestNotFound(t *testing.T) {
	app := New()
	app.Get("/exists", func(c *Context) any {
		return c.Text("ok")
	})

	req := httptest.NewRequest("GET", "/missing", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMethodMismatch(t *testing.T) {
	app := New()
	app.Get("/only-get", func(c *Context) any {
		return c.Text("ok")
	})

	req := httptest.NewRequest("POST", "/only-get", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for method mismatch, got %d", w.Code)
	}
}

func TestFetchAsHandler(t *testing.T) {
	app := New()
	app.Get("/", func(c *Context) any {
		return c.Text("via fetch")
	})

	// Use app.Fetch as http.Handler (same as app itself)
	srv := httptest.NewServer(app.Fetch)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		match   bool
		params  map[string]string
	}{
		{"/", "/", true, map[string]string{}},
		{"/foo", "/foo", true, map[string]string{}},
		{"/foo", "/bar", false, nil},
		{"/users/:id", "/users/123", true, map[string]string{"id": "123"}},
		{"/users/:id/posts/:pid", "/users/1/posts/2", true, map[string]string{"id": "1", "pid": "2"}},
		{"/users/:id", "/users", false, nil},
		{"/users/:id", "/users/1/extra", false, nil},
	}

	for _, tt := range tests {
		params, ok := matchPath(tt.pattern, tt.path)
		if ok != tt.match {
			t.Errorf("matchPath(%q, %q) match = %v, want %v", tt.pattern, tt.path, ok, tt.match)
		}
		if ok && tt.params != nil {
			for k, v := range tt.params {
				if params[k] != v {
					t.Errorf("matchPath(%q, %q) param %q = %q, want %q", tt.pattern, tt.path, k, params[k], v)
				}
			}
		}
	}
}
