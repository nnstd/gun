package perf_hooks

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func EventLoopUtilization(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	idle := float64(eluIdleNs)
	active := float64(eluActiveNs)

	if len(args) > 0 && args[0] != nil {
		u1Idle := args[0].Get("idle").Number()
		u1Active := args[0].Get("active").Number()

		if len(args) > 1 && args[1] != nil {
			u2Idle := args[1].Get("idle").Number()
			u2Active := args[1].Get("active").Number()
			idle = u1Idle - u2Idle
			active = u1Active - u2Active
		} else {
			idle = idle - u1Idle
			active = active - u1Active
		}
	}

	total := idle + active
	util := 0.0
	if total > 0 {
		util = active / total
	}

	obj := jsvalue.NewObject()
	obj.Set("idle", jsvalue.NewNumber(idle))
	obj.Set("active", jsvalue.NewNumber(active))
	obj.Set("utilization", jsvalue.NewNumber(util))
	return obj
}
