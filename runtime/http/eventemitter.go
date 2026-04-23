package nodehttp

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/events"
)

// mixEventEmitter copies EventEmitter prototype methods onto the target class
// prototype and ensures instances have a fresh _events object.
func mixEventEmitter(target *jsvalue.JSValue) {
	events.MixinEventEmitter(target)
}

// initEvents ensures a JSValue instance has the _events bag.
func initEvents(this *jsvalue.JSValue) {
	events.InitEventEmitter(this)
}
