package nodehttp

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/events"
)

// mixEventEmitter copies EventEmitter prototype methods onto the target class
// prototype and ensures instances have a fresh _events object.
func mixEventEmitter(target *jsvalue.JSValue) {
	if target == nil {
		return
	}
	proto := target.Get("prototype")
	if proto == nil || proto.TypeString() == "undefined" {
		proto = jsvalue.NewObject()
		target.Set("prototype", proto)
	}
	src := events.EventEmitter.Get("prototype")
	for _, name := range []string{"on", "addListener", "off", "once", "removeListener", "removeAllListeners", "emit", "listeners"} {
		proto.Set(name, src.Get(name))
	}
}

// initEvents ensures a JSValue instance has the _events bag.
func initEvents(this *jsvalue.JSValue) {
	if this == nil {
		return
	}
	if this.Get("_events").TypeString() != "object" {
		this.Set("_events", jsvalue.NewObject())
	}
}
