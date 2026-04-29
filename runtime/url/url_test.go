package url

import (
	"path/filepath"
	"strings"
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func js(s string) *jsvalue.JSValue { return jsvalue.NewString(s) }

func TestURLConstructorAndAccessors(t *testing.T) {
	u := URLConstructor.Call(js("/api?q=1#top"), js("https://user:pass@example.com:8443/base/index.html"))
	if got := u.Get("href").String(); got != "https://user:pass@example.com:8443/api?q=1#top" {
		t.Fatalf("href = %q", got)
	}
	if got := u.Get("origin").String(); got != "https://example.com:8443" {
		t.Fatalf("origin = %q", got)
	}
	if got := u.Get("protocol").String(); got != "https:" {
		t.Fatalf("protocol = %q", got)
	}
	if got := u.Get("username").String(); got != "user" {
		t.Fatalf("username = %q", got)
	}
	if got := u.Get("password").String(); got != "pass" {
		t.Fatalf("password = %q", got)
	}
	u.Set("pathname", js("/v1/items"))
	u.Set("search", js("?a=1&b=2"))
	u.Set("hash", js("done"))
	if got := u.MethodCall("toString").String(); got != "https://user:pass@example.com:8443/v1/items?a=1&b=2#done" {
		t.Fatalf("toString = %q", got)
	}
}

func TestURLCanParseAndParse(t *testing.T) {
	if !URLConstructor.Get("canParse").Call(js("/x"), js("https://example.com")).Bool() {
		t.Fatal("expected relative URL with base to parse")
	}
	if URLConstructor.Get("canParse").Call(js("/x")).Bool() {
		t.Fatal("expected relative URL without base to fail")
	}
	if got := URLConstructor.Get("parse").Call(js("/x")).TypeString(); got != "object" {
		t.Fatalf("URL.parse invalid returns %s, want object/null", got)
	}
	if URLConstructor.Get("parse").Call(js("/x")) != jsvalue.NewNull() {
		t.Fatal("URL.parse invalid should return null singleton")
	}
}

func TestURLSearchParams(t *testing.T) {
	params := URLSearchParamsConstructor.Call(js("b=2&a=1&a=3"))
	if got := params.MethodCall("get", js("a")).String(); got != "1" {
		t.Fatalf("get(a) = %q", got)
	}
	if got := params.MethodCall("getAll", js("a")).Len(); got != 2 {
		t.Fatalf("getAll(a).length = %d", got)
	}
	params.MethodCall("append", js("c"), js("hello world"))
	if got := params.MethodCall("toString").String(); !strings.Contains(got, "c=hello+world") {
		t.Fatalf("encoded params = %q", got)
	}
	params.MethodCall("set", js("a"), js("9"))
	if got := params.MethodCall("toString").String(); got != "b=2&a=9&c=hello+world" {
		t.Fatalf("after set = %q", got)
	}
	params.MethodCall("sort")
	if got := params.MethodCall("toString").String(); got != "a=9&b=2&c=hello+world" {
		t.Fatalf("after sort = %q", got)
	}
}

func TestURLSearchParamsStayLinkedToURL(t *testing.T) {
	u := URLConstructor.Call(js("https://example.com/?a=1"))
	params := u.Get("searchParams")
	params.MethodCall("append", js("b"), js("2"))
	if got := u.Get("search").String(); got != "?a=1&b=2" {
		t.Fatalf("url.search = %q", got)
	}
	u.Set("search", js("?c=3"))
	if got := params.MethodCall("toString").String(); got != "c=3" {
		t.Fatalf("linked params = %q", got)
	}
}

func TestFileURLPathConversions(t *testing.T) {
	p := filepath.Join(string(filepath.Separator), "tmp", "gun url test.txt")
	u := PathToFileURL(js(p))
	if got := u.Get("protocol").String(); got != "file:" {
		t.Fatalf("protocol = %q", got)
	}
	if got := FileURLToPath(u).String(); got != p {
		t.Fatalf("roundtrip = %q want %q", got, p)
	}
}

func TestDomainConversion(t *testing.T) {
	ascii := DomainToASCII(js("mañana.com")).String()
	if ascii != "xn--maana-pta.com" {
		t.Fatalf("domainToASCII = %q", ascii)
	}
	if got := DomainToUnicode(js(ascii)).String(); got != "mañana.com" {
		t.Fatalf("domainToUnicode = %q", got)
	}
}

func TestLegacyParseFormatResolveAndHttpOptions(t *testing.T) {
	parsed := Parse(js("https://user:pass@example.com:9443/a/b?x=1&x=2#h"), jsvalue.NewBool(true), nil)
	if got := parsed.Get("protocol").String(); got != "https:" {
		t.Fatalf("protocol = %q", got)
	}
	if got := parsed.Get("query").Get("x").Len(); got != 2 {
		t.Fatalf("query.x length = %d", got)
	}
	formatted := Format(jsvalue.ObjectFrom(
		"protocol", js("https:"),
		"hostname", js("example.com"),
		"pathname", js("/docs"),
		"query", jsvalue.ObjectFrom("q", js("gun")),
	)).String()
	if formatted != "https://example.com/docs?q=gun" {
		t.Fatalf("format = %q", formatted)
	}
	if got := Resolve(js("https://example.com/a/b"), js("../c")).String(); got != "https://example.com/c" {
		t.Fatalf("resolve = %q", got)
	}
	u := URLConstructor.Call(js("https://example.com:9443/a?b=1"))
	opts := URLToHttpOptions(u)
	if got := opts.Get("path").String(); got != "/a?b=1" {
		t.Fatalf("http path = %q", got)
	}
	if got := opts.Get("port").Number(); got != 9443 {
		t.Fatalf("http port = %v", got)
	}
}

func TestURLModuleExports(t *testing.T) {
	for _, name := range []string{"URL", "URLSearchParams", "domainToASCII", "domainToUnicode", "fileURLToPath", "pathToFileURL", "format", "parse", "resolve", "urlToHttpOptions"} {
		if AsJSValue.Get(name).TypeString() == "undefined" {
			t.Fatalf("missing export %s", name)
		}
	}
}
