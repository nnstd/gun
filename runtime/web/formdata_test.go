package web

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestFormDataAppendGetGetAllHasDelete(t *testing.T) {
	fd := FormData.Call()
	fd.MethodCall("append", jsvalue.NewString("a"), jsvalue.NewNumber(1))
	fd.MethodCall("append", jsvalue.NewString("a"), jsvalue.NewString("two"))
	fd.MethodCall("append", jsvalue.NewString("b"), jsvalue.NewBool(true))

	if got := fd.MethodCall("get", jsvalue.NewString("a")).String(); got != "1" {
		t.Fatalf("get(a) = %q", got)
	}
	all := fd.MethodCall("getAll", jsvalue.NewString("a"))
	if all.Len() != 2 || all.Index(0).String() != "1" || all.Index(1).String() != "two" {
		t.Fatalf("getAll(a) unexpected: len=%d first=%q second=%q", all.Len(), all.Index(0).String(), all.Index(1).String())
	}
	if !fd.MethodCall("has", jsvalue.NewString("b")).Bool() {
		t.Fatal("has(b) = false")
	}
	if got := fd.MethodCall("get", jsvalue.NewString("missing")); got.TypeString() != "object" || got.String() != "null" {
		t.Fatalf("missing get = %s %q", got.TypeString(), got.String())
	}

	fd.MethodCall("delete", jsvalue.NewString("a"))
	if fd.MethodCall("has", jsvalue.NewString("a")).Bool() {
		t.Fatal("delete(a) did not remove all values")
	}
}

func TestFormDataSetReplacesExistingInOrder(t *testing.T) {
	fd := FormData.Call()
	fd.MethodCall("append", jsvalue.NewString("a"), jsvalue.NewString("one"))
	fd.MethodCall("append", jsvalue.NewString("b"), jsvalue.NewString("two"))
	fd.MethodCall("append", jsvalue.NewString("a"), jsvalue.NewString("three"))
	fd.MethodCall("set", jsvalue.NewString("a"), jsvalue.NewString("changed"))

	all := fd.MethodCall("getAll", jsvalue.NewString("a"))
	if all.Len() != 1 || all.Index(0).String() != "changed" {
		t.Fatalf("set(a) values unexpected")
	}
	entries := fd.MethodCall("entries")
	if entries.Len() != 2 || entries.Index(0).Index(0).String() != "a" || entries.Index(1).Index(0).String() != "b" {
		t.Fatalf("entries order after set unexpected")
	}
}

func TestFormDataEntriesKeysValuesForEach(t *testing.T) {
	fd := FormData.Call()
	fd.MethodCall("append", jsvalue.NewString("x"), jsvalue.NewString("1"))
	fd.MethodCall("append", jsvalue.NewString("y"), jsvalue.NewString("2"))

	entries := fd.MethodCall("entries")
	if entries.Len() != 2 || entries.Index(0).Index(0).String() != "x" || entries.Index(0).Index(1).String() != "1" {
		t.Fatalf("entries unexpected")
	}
	keys := fd.MethodCall("keys")
	if keys.Len() != 2 || keys.Index(0).String() != "x" || keys.Index(1).String() != "y" {
		t.Fatalf("keys unexpected")
	}
	values := fd.MethodCall("values")
	if values.Len() != 2 || values.Index(0).String() != "1" || values.Index(1).String() != "2" {
		t.Fatalf("values unexpected")
	}

	seen := jsvalue.NewArray()
	fd.MethodCall("forEach", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		seen.MethodCall("push", jsvalue.NewString(args[1].String()+"="+args[0].String()))
		if args[2] != fd {
			t.Fatalf("third forEach arg should be FormData")
		}
		return jsvalue.NewUndefined()
	}))
	if seen.Len() != 2 || seen.Index(0).String() != "x=1" || seen.Index(1).String() != "y=2" {
		t.Fatalf("forEach seen unexpected")
	}
}

func TestFormDataFileAndBlobValues(t *testing.T) {
	fd := FormData.Call()
	file := File.Call(jsvalue.NewArray(jsvalue.NewString("content")), jsvalue.NewString("a.txt"), jsvalue.ObjectFrom("type", jsvalue.NewString("text/plain")))
	fd.MethodCall("append", jsvalue.NewString("file"), file, jsvalue.NewString("renamed.txt"))

	gotFile := fd.MethodCall("get", jsvalue.NewString("file"))
	if !jsvalue.InstanceOf(gotFile, File).Bool() {
		t.Fatal("file value should remain File")
	}
	if got := gotFile.Get("name").String(); got != "renamed.txt" {
		t.Fatalf("file name = %q", got)
	}
	if got := gotFile.Get("type").String(); got != "text/plain" {
		t.Fatalf("file type = %q", got)
	}

	blob := jsvalue.ObjectFrom("size", jsvalue.NewNumber(3), "type", jsvalue.NewString("TEXT/CUSTOM"))
	fd.MethodCall("append", jsvalue.NewString("blob"), blob)
	gotBlob := fd.MethodCall("get", jsvalue.NewString("blob"))
	if !jsvalue.InstanceOf(gotBlob, File).Bool() {
		t.Fatal("blob value should be converted to File")
	}
	if got := gotBlob.Get("name").String(); got != "blob" {
		t.Fatalf("blob filename = %q", got)
	}
	if got := gotBlob.Get("type").String(); got != "text/custom" {
		t.Fatalf("blob type = %q", got)
	}
}

func TestFormDataConstructorCopiesExistingFormData(t *testing.T) {
	fd := FormData.Call()
	fd.MethodCall("append", jsvalue.NewString("a"), jsvalue.NewString("1"))
	copy := FormData.Call(fd)
	fd.MethodCall("append", jsvalue.NewString("a"), jsvalue.NewString("2"))

	all := copy.MethodCall("getAll", jsvalue.NewString("a"))
	if all.Len() != 1 || all.Index(0).String() != "1" {
		t.Fatalf("copied FormData should not track source mutations")
	}
}
