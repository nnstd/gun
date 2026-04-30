package fs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
	promise "github.com/nnstd/gun/runtime/promise"
	"github.com/nnstd/gun/runtime/web"
)

func js(s string) *jsvalue.JSValue { return jsvalue.NewString(s) }

func TestReadWriteFileSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.txt")

	WriteFileSync(js(p), js("hello"))
	data := ReadFileSync(js(p))
	if data.MethodCall("toString").String() != "hello" {
		t.Errorf("got %q, want %q", data.MethodCall("toString").String(), "hello")
	}
	if !buffer.Buffer.Get("isBuffer").Call(data).Bool() {
		t.Fatal("readFileSync without encoding should return Buffer")
	}
	encoded := ReadFileSync(js(p), js("utf8"))
	if encoded.TypeString() != "string" || encoded.String() != "hello" {
		t.Fatalf("encoded read = %s %q, want string hello", encoded.TypeString(), encoded.String())
	}
}

func TestWriteAndAppendAcceptBuffer(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "buffer.txt")
	WriteFileSync(js(p), buffer.Buffer.Get("from").Call(js("a")))
	AppendFileSync(js(p), buffer.Buffer.Get("from").Call(js("b")))
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ab" {
		t.Fatalf("file = %q, want ab", data)
	}
}

func TestExistsSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exists.txt")

	if ExistsSync(js(p)).Bool() {
		t.Error("file should not exist yet")
	}
	os.WriteFile(p, []byte("x"), 0644)
	if !ExistsSync(js(p)).Bool() {
		t.Error("file should exist")
	}
}

func TestMkdirSync(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")

	MkdirSync(js(nested))
	info, err := os.Stat(nested)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestReaddirSync(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	entries := ReaddirSync(js(dir))
	if entries.Len() != 2 {
		t.Errorf("got %d entries, want 2", entries.Len())
	}
}

func TestUnlinkSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "del.txt")
	os.WriteFile(p, []byte("x"), 0644)

	UnlinkSync(js(p))
	if ExistsSync(js(p)).Bool() {
		t.Error("file should be deleted")
	}
}

func TestStatSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stat.txt")
	os.WriteFile(p, []byte("hello"), 0644)

	info := StatSync(js(p))
	if int(info.Get("size").Number()) != 5 {
		t.Errorf("got size %v, want 5", info.Get("size").Number())
	}
}

func TestAppendFileSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "append.txt")
	os.WriteFile(p, []byte("hello"), 0644)

	AppendFileSync(js(p), js(" world"))
	data, _ := os.ReadFile(p)
	if string(data) != "hello world" {
		t.Errorf("got %q, want %q", data, "hello world")
	}
}

func TestCopyFileSync(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("copy me"), 0644)

	CopyFileSync(js(src), js(dst))
	data, _ := os.ReadFile(dst)
	if string(data) != "copy me" {
		t.Errorf("got %q, want %q", data, "copy me")
	}
}

func TestRenameSync(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.txt")
	new_ := filepath.Join(dir, "new.txt")
	os.WriteFile(old, []byte("move me"), 0644)

	RenameSync(js(old), js(new_))
	if ExistsSync(js(old)).Bool() {
		t.Error("old file should not exist")
	}
	data, _ := os.ReadFile(new_)
	if string(data) != "move me" {
		t.Errorf("got %q, want %q", data, "move me")
	}
}

func TestAsJSValueAliases(t *testing.T) {
	if AsJSValue.Get("readFile").TypeString() != "function" {
		t.Fatal("expected readFile function on fs.AsJSValue")
	}
	if AsJSValue.Get("writeFile").TypeString() != "function" {
		t.Fatal("expected writeFile function on fs.AsJSValue")
	}
	if AsJSValue.Get("promises").TypeString() != "object" {
		t.Fatal("expected promises object on fs.AsJSValue")
	}
	if AsJSValue.Get("ReadStream").TypeString() != "function" {
		t.Fatal("expected ReadStream export")
	}
	if AsJSValue.Get("WriteStream").TypeString() != "function" {
		t.Fatal("expected WriteStream export")
	}
}

func TestPromisesAsJSValueReturnsPromises(t *testing.T) {
	p := PromisesAsJSValue.Get("readFile").Call(js("/definitely/missing"))
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected fs.promises.readFile to return a Promise")
	}
}

func TestReadFileCallbackSuccessAndError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cb.txt")
	os.WriteFile(p, []byte("ok"), 0644)

	var gotErr, gotData *jsvalue.JSValue
	AsJSValue.Get("readFile").Call(js(p), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		gotErr = args[0]
		gotData = args[1]
		return jsvalue.NewUndefined()
	}))
	if gotErr == nil || gotErr.TypeString() != "object" || gotErr.Type() != jsvalue.TypeNull {
		t.Fatalf("success err = %v, want null", gotErr)
	}
	if !buffer.Buffer.Get("isBuffer").Call(gotData).Bool() || gotData.MethodCall("toString").String() != "ok" {
		t.Fatalf("callback data = %s", gotData.String())
	}

	var missingErr *jsvalue.JSValue
	AsJSValue.Get("readFile").Call(js(filepath.Join(dir, "missing")), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		missingErr = args[0]
		return jsvalue.NewUndefined()
	}))
	if missingErr == nil || missingErr.Get("code").String() != "ENOENT" {
		t.Fatalf("missing error code = %v", missingErr)
	}
}

func TestReadFileCallbackPreAbortedSignal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "abort.txt")
	os.WriteFile(p, []byte("ok"), 0644)

	controller := web.AbortController.Call()
	controller.MethodCall("abort")
	var gotErr *jsvalue.JSValue
	AsJSValue.Get("readFile").Call(js(p), jsvalue.ObjectFrom("signal", controller.Get("signal")), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		gotErr = args[0]
		return jsvalue.NewUndefined()
	}))
	if gotErr == nil || gotErr.Get("name").String() != "AbortError" || gotErr.Get("code").String() != "ABORT_ERR" {
		t.Fatalf("callback abort error = %v", gotErr)
	}
}

func TestPromisesReadFileResolveRejectAndAbort(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "promise.txt")
	os.WriteFile(p, []byte("ok"), 0644)

	resolved := promise.Await(PromisesAsJSValue.Get("readFile").Call(js(p)))
	if !buffer.Buffer.Get("isBuffer").Call(resolved).Bool() {
		t.Fatal("promise readFile should resolve Buffer by default")
	}
	encoded := promise.Await(PromisesAsJSValue.Get("readFile").Call(js(p), js("utf8")))
	if encoded.TypeString() != "string" || encoded.String() != "ok" {
		t.Fatalf("encoded promise read = %s %q", encoded.TypeString(), encoded.String())
	}

	controller := web.AbortController.Call()
	controller.MethodCall("abort")
	aborted := promise.Await(PromisesAsJSValue.Get("readFile").Call(js(p), jsvalue.ObjectFrom("signal", controller.Get("signal"))))
	if aborted.Get("name").String() != "AbortError" || aborted.Get("code").String() != "ABORT_ERR" {
		t.Fatalf("abort rejection = %s code=%s", aborted.Get("name").String(), aborted.Get("code").String())
	}
}

func TestCreateReadAndWriteStreams(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stream.txt")
	os.WriteFile(p, []byte("hello"), 0644)

	rs := AsJSValue.Get("createReadStream").Call(js(p))
	if !jsvalue.InstanceOf(rs, ReadStream).Bool() {
		t.Fatal("createReadStream should return ReadStream")
	}
	dataCh := make(chan string, 1)
	endCh := make(chan struct{}, 1)
	rs.MethodCall("on", js("data"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		dataCh <- args[0].MethodCall("toString").String()
		return jsvalue.NewUndefined()
	}))
	rs.MethodCall("on", js("end"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		endCh <- struct{}{}
		return jsvalue.NewUndefined()
	}))
	go eventloop.Default.Run()
	select {
	case got := <-dataCh:
		if got != "hello" {
			t.Fatalf("data = %q, want hello", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for data")
	}
	select {
	case <-endCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for end")
	}

	wp := filepath.Join(dir, "write-stream.txt")
	ws := AsJSValue.Get("createWriteStream").Call(js(wp))
	if !jsvalue.InstanceOf(ws, WriteStream).Bool() {
		t.Fatal("createWriteStream should return WriteStream")
	}
	finishCh := make(chan struct{}, 1)
	ws.MethodCall("on", js("finish"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		finishCh <- struct{}{}
		return jsvalue.NewUndefined()
	}))
	ws.MethodCall("write", js("a"))
	ws.MethodCall("end", buffer.Buffer.Get("from").Call(js("b")))
	go eventloop.Default.Run()
	select {
	case <-finishCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for finish")
	}
	data, err := os.ReadFile(wp)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ab" {
		t.Fatalf("written data = %q, want ab", data)
	}
}

func TestCreateReadAndWriteStreamsRespectPreAbortedSignal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stream-abort.txt")
	os.WriteFile(p, []byte("hello"), 0644)

	controller := web.AbortController.Call()
	controller.MethodCall("abort")
	opts := jsvalue.ObjectFrom("signal", controller.Get("signal"))

	rs := AsJSValue.Get("createReadStream").Call(js(p), opts)
	errCh := make(chan *jsvalue.JSValue, 1)
	dataCh := make(chan struct{}, 1)
	rs.MethodCall("on", js("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		errCh <- args[0]
		return jsvalue.NewUndefined()
	}))
	rs.MethodCall("on", js("data"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		dataCh <- struct{}{}
		return jsvalue.NewUndefined()
	}))
	go eventloop.Default.Run()
	select {
	case errVal := <-errCh:
		if errVal.Get("name").String() != "AbortError" || errVal.Get("code").String() != "ABORT_ERR" {
			t.Fatalf("read stream abort = %s code=%s", errVal.Get("name").String(), errVal.Get("code").String())
		}
	case <-dataCh:
		t.Fatal("read stream emitted data despite aborted signal")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for read stream abort")
	}

	wp := filepath.Join(dir, "write-abort.txt")
	ws := AsJSValue.Get("createWriteStream").Call(js(wp), opts)
	var writeErr *jsvalue.JSValue
	ws.MethodCall("on", js("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		writeErr = args[0]
		return jsvalue.NewUndefined()
	}))
	ws.MethodCall("write", js("x"))
	ws.MethodCall("end")
	if writeErr == nil || writeErr.Get("name").String() != "AbortError" || writeErr.Get("code").String() != "ABORT_ERR" {
		t.Fatalf("write stream abort = %v", writeErr)
	}
	if _, err := os.Stat(wp); !os.IsNotExist(err) {
		t.Fatalf("write stream should not create file on pre-abort, stat err=%v", err)
	}
}
