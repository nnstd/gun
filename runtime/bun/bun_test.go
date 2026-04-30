package bun

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/promise"
	urlpkg "github.com/nnstd/gun/runtime/url"
	"github.com/nnstd/gun/runtime/web"
)

type fakeAddr struct{ value string }

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return a.value }

type fakeListener struct {
	addr   net.Addr
	closed chan struct{}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, errors.New("closed")
}

func (l *fakeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

func (l *fakeListener) Addr() net.Addr { return l.addr }

func TestServeReturnsServerObject(t *testing.T) {
	orig := listenFn
	l := &fakeListener{addr: fakeAddr{value: "127.0.0.1:43110"}, closed: make(chan struct{})}
	listenFn = func(network, address string) (net.Listener, error) {
		return l, nil
	}
	defer func() { listenFn = orig }()

	server := Serve(jsvalue.ObjectFrom(
		"port", jsvalue.NewNumber(43110),
		"fetch", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewUndefined()
		}),
	))

	if got := int(server.Get("port").Number()); got != 43110 {
		t.Fatalf("unexpected port: %d", got)
	}
	if server.Get("stop").TypeString() != "function" {
		t.Fatal("expected stop() on Bun server object")
	}
	server.MethodCall("stop")
}

func TestYAMLParse(t *testing.T) {
	parsed := AsJSValue.Get("YAML").Get("parse").Call(jsvalue.NewString("name: Jane\nhobbies:\n  - reading\n  - coding\n"))
	if got := parsed.Get("name").String(); got != "Jane" {
		t.Fatalf("name = %q", got)
	}
	if got := parsed.Get("hobbies").Index(1).String(); got != "coding" {
		t.Fatalf("hobbies[1] = %q", got)
	}
}

func TestYAMLParseInvalidThrowsSyntaxError(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("name").String(); got != "SyntaxError" {
			t.Fatalf("error name = %q, want SyntaxError", got)
		}
	}()
	AsJSValue.Get("YAML").Get("parse").Call(jsvalue.NewString("invalid: yaml: content:"))
}

func TestYAMLStringifyFlowStyleByDefault(t *testing.T) {
	obj := jsvalue.ObjectFrom(
		"abc", jsvalue.NewString("def"),
		"num", jsvalue.NewNumber(123),
	)
	got := AsJSValue.Get("YAML").Get("stringify").Call(obj).String()
	if !strings.Contains(got, "{") || !strings.Contains(got, "abc: def") || !strings.Contains(got, "num: 123") {
		t.Fatalf("unexpected flow YAML: %q", got)
	}
}

func TestYAMLStringifyBlockStyleWithSpace(t *testing.T) {
	obj := jsvalue.ObjectFrom(
		"abc", jsvalue.NewString("def"),
		"nested", jsvalue.ObjectFrom("num", jsvalue.NewNumber(123)),
	)
	got := AsJSValue.Get("YAML").Get("stringify").Call(obj, jsvalue.NewNull(), jsvalue.NewNumber(2)).String()
	if !strings.Contains(got, "abc: def") || !strings.Contains(got, "\nnested:\n  num: 123") {
		t.Fatalf("unexpected block YAML: %q", got)
	}
}

func TestBunFileExportsAndReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"ok":true,"n":7}`), 0644); err != nil {
		t.Fatal(err)
	}

	if AsJSValue.Get("file").TypeString() != "function" {
		t.Fatal("expected Bun.file function")
	}
	if AsJSValue.Get("BunFile").TypeString() != "function" {
		t.Fatal("expected Bun.BunFile class")
	}

	file := AsJSValue.Get("file").Call(jsvalue.NewString(path))
	if !jsvalue.InstanceOf(file, BunFile).Bool() {
		t.Fatal("Bun.file should return a BunFile")
	}
	if got := file.Get("name").String(); got != path {
		t.Fatalf("name = %q, want %q", got, path)
	}
	if got := file.Get("type").String(); got != "application/json" {
		t.Fatalf("type = %q", got)
	}
	if got := file.Get("size").Number(); got != 0 {
		t.Fatalf("size before read = %v, want lazy 0", got)
	}

	text := promise.Await(file.MethodCall("text"))
	if got := text.String(); got != `{"ok":true,"n":7}` {
		t.Fatalf("text() = %q", got)
	}
	if got := int(file.Get("size").Number()); got != len(`{"ok":true,"n":7}`) {
		t.Fatalf("size after read = %d", got)
	}
	parsed := promise.Await(file.MethodCall("json"))
	if !parsed.Get("ok").Bool() || parsed.Get("n").Number() != 7 {
		t.Fatalf("json() parsed unexpected object: ok=%v n=%v", parsed.Get("ok").Bool(), parsed.Get("n").Number())
	}
	bytes := promise.Await(file.MethodCall("bytes"))
	if got := bytes.Index(0).Number(); got != float64('{') {
		t.Fatalf("bytes()[0] = %v", got)
	}
	if got := int(bytes.Get("byteLength").Number()); got != len(`{"ok":true,"n":7}`) {
		t.Fatalf("byteLength = %d", got)
	}
}

func TestBunFileURLStatSliceWriteAndDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	file := AsJSValue.Get("file").Call(urlpkg.PathToFileURL(jsvalue.NewString(path)))
	exists := promise.Await(file.MethodCall("exists"))
	if !exists.Bool() {
		t.Fatal("exists() = false")
	}
	stat := promise.Await(file.MethodCall("stat"))
	if got := int(stat.Get("size").Number()); got != len("hello world") {
		t.Fatalf("stat().size = %d", got)
	}
	if !stat.MethodCall("isFile").Bool() {
		t.Fatal("stat().isFile() = false")
	}

	slice := file.MethodCall("slice", jsvalue.NewNumber(6), jsvalue.NewNumber(11), jsvalue.NewString("text/custom"))
	if got := promise.Await(slice.MethodCall("text")).String(); got != "world" {
		t.Fatalf("slice text = %q", got)
	}
	if got := slice.Get("type").String(); got != "text/custom" {
		t.Fatalf("slice type = %q", got)
	}

	written := promise.Await(file.MethodCall("write", jsvalue.NewString("changed")))
	if got := int(written.Number()); got != len("changed") {
		t.Fatalf("write() = %d", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "changed" {
		t.Fatalf("file contents = %q", string(data))
	}

	promise.Await(file.MethodCall("delete"))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be deleted, stat err = %v", err)
	}
}

func TestBunFileWriterAndStream(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sink.txt")
	file := AsJSValue.Get("file").Call(jsvalue.NewString(path))
	sink := file.MethodCall("writer")
	sink.MethodCall("write", jsvalue.NewString("ab"))
	promise.Await(sink.MethodCall("end", jsvalue.NewString("cd")))

	if got := promise.Await(file.MethodCall("text")).String(); got != "abcd" {
		t.Fatalf("writer contents = %q", got)
	}
	readable := file.MethodCall("stream")
	chunks := readable.Get("_chunks")
	if !chunks.IsArray() || chunks.Len() != 1 {
		t.Fatalf("stream chunks len = %d", chunks.Len())
	}
	if got := chunks.Index(0).MethodCall("toString").String(); got != "abcd" {
		t.Fatalf("stream chunk = %q", got)
	}
}

func TestBunFileInvalidPathMatchesBunShape(t *testing.T) {
	for _, input := range []*jsvalue.JSValue{jsvalue.NewUndefined(), jsvalue.NewNull(), jsvalue.NewObject()} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected invalid path panic")
				}
				err, ok := r.(*jsvalue.JSValue)
				if !ok {
					t.Fatalf("panic = %T", r)
				}
				if got := err.Get("name").String(); got != "TypeError" {
					t.Fatalf("name = %q", got)
				}
				if got := err.Get("code").String(); got != "ERR_INVALID_ARG_TYPE" {
					t.Fatalf("code = %q", got)
				}
				if got := err.Get("message").String(); got != "Expected file path string or file descriptor" {
					t.Fatalf("message = %q", got)
				}
			}()
			AsJSValue.Get("file").Call(input)
		}()
	}
}

func TestBunFileErrorShapesMatchObservedBun(t *testing.T) {
	dir := t.TempDir()
	file := AsJSValue.Get("file").Call(jsvalue.NewString(dir))
	errVal := promise.Await(file.MethodCall("text"))
	if got := errVal.Get("code").String(); got != "EISDIR" {
		t.Fatalf("directory read code = %q", got)
	}
	if got := errVal.Get("syscall").String(); got != "read" {
		t.Fatalf("directory read syscall = %q", got)
	}
	if got := errVal.Get("errno").Number(); got != 0 {
		t.Fatalf("directory read errno = %v", got)
	}
	if got := errVal.Get("message").String(); got != "Directories cannot be read like files" {
		t.Fatalf("directory read message = %q", got)
	}

	jsonPath := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(jsonPath, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	jsonFile := AsJSValue.Get("file").Call(jsvalue.NewString(jsonPath))
	jsonErr := promise.Await(jsonFile.MethodCall("json"))
	if got := jsonErr.Get("name").String(); got != "SyntaxError" {
		t.Fatalf("json error name = %q", got)
	}
	if got := jsonErr.Get("message").String(); got != "Failed to parse JSON" {
		t.Fatalf("json error message = %q", got)
	}
}

func TestWriteResponseFromFetchResult(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	result := jsvalue.ObjectFrom(
		"status", jsvalue.NewNumber(201),
		"_bodyInit", jsvalue.NewString("bun-ok"),
		"headers", jsvalue.ObjectFrom("content-type", jsvalue.NewString("text/plain")),
	)

	web.WriteResponse(rec, result)

	if rec.Code != 201 {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if body := rec.Body.String(); body != "bun-ok" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := rec.Header().Get("content-type"); got != "text/plain" {
		t.Fatalf("unexpected content-type: %q", got)
	}
	_ = req
}

func TestEventLoopRunReturnsWhenNoActiveServers(t *testing.T) {
	done := make(chan struct{})
	go func() {
		eventloop.Default.Run()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("eventloop.Run blocked with no active servers")
	}
}

func TestServeRejectsNonObjectOptions(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("name").String(); got != "TypeError" {
			t.Fatalf("error name = %q, want TypeError", got)
		}
		if got := err.Get("message").String(); got != "Bun.serve expects an object" {
			t.Fatalf("message = %q", got)
		}
		if got := err.Get("code").String(); got != "ERR_INVALID_ARG_TYPE" {
			t.Fatalf("code = %q", got)
		}
	}()
	Serve(jsvalue.NewNumber(123))
}

func TestServeRejectsMissingFetchOrRoutes(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("name").String(); got != "TypeError" {
			t.Fatalf("error name = %q, want TypeError", got)
		}
		if got := err.Get("code").String(); got != "ERR_INVALID_ARG_TYPE" {
			t.Fatalf("code = %q", got)
		}
		if got := err.Get("message").String(); !strings.Contains(got, "Bun.serve() needs either:") {
			t.Fatalf("message = %q", got)
		}
	}()
	Serve(jsvalue.ObjectFrom("port", jsvalue.NewNumber(0)))
}

func TestServeRejectsNonFunctionFetch(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("message").String(); got != "Expected fetch() to be a function" {
			t.Fatalf("message = %q", got)
		}
		if got := err.Get("code").String(); got != "ERR_INVALID_ARG_TYPE" {
			t.Fatalf("code = %q", got)
		}
	}()
	Serve(jsvalue.ObjectFrom("port", jsvalue.NewNumber(0), "fetch", jsvalue.NewNumber(123)))
}

func TestServeRejectsPortAlreadyInUseLikeBun(t *testing.T) {
	orig := listenFn
	listenFn = func(network, address string) (net.Listener, error) {
		return nil, &net.OpError{Op: "listen", Net: network, Err: &os.SyscallError{Syscall: "listen", Err: syscall.EADDRINUSE}}
	}
	defer func() { listenFn = orig }()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("name").String(); got != "Error" {
			t.Fatalf("error name = %q, want Error", got)
		}
		if got := err.Get("message").String(); got != "Failed to start server. Is port 3029 in use?" {
			t.Fatalf("message = %q", got)
		}
		if got := err.Get("syscall").String(); got != "listen" {
			t.Fatalf("syscall = %q", got)
		}
		if got := err.Get("errno").Number(); got != 0 {
			t.Fatalf("errno = %v", got)
		}
		if got := err.Get("code").String(); got != "EADDRINUSE" {
			t.Fatalf("code = %q", got)
		}
	}()

	Serve(jsvalue.ObjectFrom(
		"port", jsvalue.NewNumber(3029),
		"fetch", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() }),
	))
}
