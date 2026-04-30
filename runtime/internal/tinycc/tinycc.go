// Package tinycc provides an embedded libtcc wrapper adapted from
// github.com/DiMalovanyy/go-tinycc (LGPL-2.1). The original wrapper exposes
// context creation and compilation but leaves relocation and symbol lookup
// incomplete; this package adds the calls needed by bun:ffi cc().
package tinycc

// #cgo LDFLAGS: -L${SRCDIR}/lib/tinycc -ltcc -ldl
// #cgo CFLAGS: -I${SRCDIR}/lib/tinycc -I${SRCDIR}/lib/tinycc/include
// #include <stdlib.h>
// #include <libtcc.h>
// static void gun_tcc_set_lib_path(TCCState *s, const char *path) { tcc_set_lib_path(s, path); }
// static void *gun_tcc_uintptr_to_ptr(uintptr_t addr) { return (void *)addr; }
import "C"

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"unsafe"
)

type OutputMode int

const (
	OutputToMemory OutputMode = iota
	OutputToBinary
	OutputToDynamicLibrary
	OutputToObjectFile
	PreprocessOnly
)

type Context struct {
	state *C.TCCState
}

func NewContext() (*Context, error) {
	state := C.tcc_new()
	if state == nil {
		return nil, errors.New("tcc_new returned null")
	}
	_, file, _, _ := runtime.Caller(0)
	libPath := filepath.Join(filepath.Dir(file), "lib", "tinycc")
	_ = withCString(libPath, func(path *C.char) error {
		C.gun_tcc_set_lib_path(state, path)
		return nil
	})
	return &Context{state: state}, nil
}

func (c *Context) Close() {
	if c == nil || c.state == nil {
		return
	}
	C.tcc_delete(c.state)
	c.state = nil
}

func (c *Context) SetOutputMode(mode OutputMode) error {
	return c.callInt("tcc_set_output_type", func() int { return int(C.tcc_set_output_type(c.state, C.int(mode))) })
}

func (c *Context) CompileString(source string) error {
	return withCString(source, func(ptr *C.char) error {
		return c.callInt("tcc_compile_string", func() int { return int(C.tcc_compile_string(c.state, ptr)) })
	})
}

func (c *Context) AddIncludePath(path string) error {
	return withCString(path, func(ptr *C.char) error {
		return c.callInt("tcc_add_include_path", func() int { return int(C.tcc_add_include_path(c.state, ptr)) })
	})
}

func (c *Context) AddLibraryPath(path string) error {
	return withCString(path, func(ptr *C.char) error {
		return c.callInt("tcc_add_library_path", func() int { return int(C.tcc_add_library_path(c.state, ptr)) })
	})
}

func (c *Context) AddLibrary(name string) error {
	return withCString(name, func(ptr *C.char) error {
		return c.callInt("tcc_add_library", func() int { return int(C.tcc_add_library(c.state, ptr)) })
	})
}

func (c *Context) AddFile(path string) error {
	return withCString(path, func(ptr *C.char) error {
		return c.callInt("tcc_add_file", func() int { return int(C.tcc_add_file(c.state, ptr)) })
	})
}

func (c *Context) DefineSymbol(name string, value string) error {
	return withCString(name, func(namePtr *C.char) error {
		return withCString(value, func(valuePtr *C.char) error {
			C.tcc_define_symbol(c.state, namePtr, valuePtr)
			return nil
		})
	})
}

func (c *Context) AddSymbol(name string, address uintptr) error {
	return withCString(name, func(namePtr *C.char) error {
		return c.callInt("tcc_add_symbol", func() int {
			return int(C.tcc_add_symbol(c.state, namePtr, C.gun_tcc_uintptr_to_ptr(C.uintptr_t(address))))
		})
	})
}

func (c *Context) Relocate() error {
	return c.callInt("tcc_relocate", func() int { return int(C.tcc_relocate(c.state)) })
}

func (c *Context) Symbol(name string) (uintptr, error) {
	var out uintptr
	err := withCString(name, func(namePtr *C.char) error {
		out = uintptr(C.tcc_get_symbol(c.state, namePtr))
		if out == 0 {
			return fmt.Errorf("tcc symbol %q not found", name)
		}
		return nil
	})
	return out, err
}

func (c *Context) callInt(name string, fn func() int) error {
	if c == nil || c.state == nil {
		return errors.New("tinycc context is closed")
	}
	if rc := fn(); rc < 0 {
		return fmt.Errorf("%s failed", name)
	}
	return nil
}

func withCString(s string, fn func(*C.char) error) error {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	return fn(cstr)
}
