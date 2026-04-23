package events

import jsvalue "github.com/nnstd/gun/runtime/builtin"

// MixinEventEmitter copies EventEmitter prototype methods onto the target
// class prototype.
func MixinEventEmitter(target *jsvalue.JSValue) {
	if target == nil {
		return
	}
	proto := target.Get("prototype")
	if proto == nil || proto.TypeString() == "undefined" {
		proto = jsvalue.NewObject()
		target.Set("prototype", proto)
	}
	src := EventEmitter.Get("prototype")
	for _, name := range []string{"on", "addListener", "off", "once", "removeListener", "removeAllListeners", "emit", "listeners"} {
		proto.Set(name, src.Get(name))
	}
}

// InitEventEmitter ensures a JSValue instance has an _events bag.
func InitEventEmitter(this *jsvalue.JSValue) {
	if this == nil {
		return
	}
	if this.Get("_events").TypeString() != "object" {
		this.Set("_events", jsvalue.NewObject())
	}
}
