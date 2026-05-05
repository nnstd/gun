package jscontext

import (
	"sync"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

type Context struct {
	global *jsvalue.JSValue
}

var (
	defaultCtx  *Context
	defaultOnce sync.Once
)

func Default() *Context {
	defaultOnce.Do(func() {
		defaultCtx = New()
		defaultCtx.RegisterBuiltins()
	})
	return defaultCtx
}

func New() *Context {
	return &Context{global: jsvalue.NewObject()}
}

func NewFromGlobal(global *jsvalue.JSValue) *Context {
	return &Context{global: global}
}

func (c *Context) Global() *jsvalue.JSValue     { return c.global }
func (c *Context) Get(name string) *jsvalue.JSValue      { return c.global.Get(name) }
func (c *Context) Set(name string, val *jsvalue.JSValue) { c.global.Set(name, val) }
func (c *Context) AsJSValue() *jsvalue.JSValue   { return c.global }

func (c *Context) RegisterBuiltins() {
	for name, val := range jsvalue.Globals() {
		c.Set(name, val)
	}
	c.Set("globalThis", c.global)
	c.Set("global", c.global)
}
