package module

import (
	"github.com/nnstd/gun/runtime/assert"
	"github.com/nnstd/gun/runtime/buffer"
	"github.com/nnstd/gun/runtime/child_process"
	"github.com/nnstd/gun/runtime/crypto"
	"github.com/nnstd/gun/runtime/dgram"
	"github.com/nnstd/gun/runtime/dns"
	"github.com/nnstd/gun/runtime/events"
	"github.com/nnstd/gun/runtime/fs"
	nodehttp "github.com/nnstd/gun/runtime/http"
	nodeos "github.com/nnstd/gun/runtime/os"
	"github.com/nnstd/gun/runtime/path"
	"github.com/nnstd/gun/runtime/process"
	"github.com/nnstd/gun/runtime/stream"
	"github.com/nnstd/gun/runtime/string_decoder"
	"github.com/nnstd/gun/runtime/timers"
	"github.com/nnstd/gun/runtime/url"
	"github.com/nnstd/gun/runtime/util"
	"github.com/nnstd/gun/runtime/v8"
	"github.com/nnstd/gun/runtime/zlib"
)

func init() {
	RegisterBuiltins()
}

// RegisterBuiltins populates ModuleRegistry with all builtin runtime modules.
// Invoked from init() so all imported packages have already completed their
// own init() and any init-assigned AsJSValue vars are populated.
func RegisterBuiltins() {
	registryMu.Lock()
	defer registryMu.Unlock()

	// Package-level IIFE or plain-var AsJSValue — all *jsvalue.JSValue values.
	ModuleRegistry["fs"] = fs.AsJSValue
	ModuleRegistry["path"] = path.AsJSValue
	ModuleRegistry["os"] = nodeos.AsJSValue
	ModuleRegistry["url"] = url.AsJSValue
	ModuleRegistry["node:url"] = url.AsJSValue
	ModuleRegistry["assert"] = assert.AsJSValue
	ModuleRegistry["module"] = AsJSValue
	ModuleRegistry["crypto"] = crypto.AsJSValue
	ModuleRegistry["dgram"] = dgram.AsJSValue
	ModuleRegistry["util"] = util.AsJSValue
	ModuleRegistry["child_process"] = child_process.AsJSValue
	ModuleRegistry["dns"] = dns.AsJSValue
	ModuleRegistry["zlib"] = zlib.AsJSValue
	ModuleRegistry["timers"] = timers.AsJSValue

	// Init-assigned vars (safe: importing packages run init first).
	ModuleRegistry["buffer"] = buffer.AsJSValue
	ModuleRegistry["events"] = events.AsJSValue
	ModuleRegistry["http"] = nodehttp.AsJSValue
	ModuleRegistry["https"] = nodehttp.HTTPSAsJSValue
	ModuleRegistry["stream"] = stream.AsJSValue
	ModuleRegistry["string_decoder"] = string_decoder.AsJSValue

	ModuleRegistry["process"] = process.AsJSValueCached
	ModuleRegistry["v8"] = v8.AsJSValue
}
