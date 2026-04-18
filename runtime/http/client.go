package nodehttp

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// ClientRequest is a stub placeholder; full impl in later stories.
func ClientRequest(isTLS bool, autoEnd bool, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	req := jsvalue.NewObject()
	req.SetPrototype(clientReqCls.Get("prototype"))
	req.Set("_isTLS", jsvalue.NewBool(isTLS))
	req.Set("_autoEnd", jsvalue.NewBool(autoEnd))
	return req
}
