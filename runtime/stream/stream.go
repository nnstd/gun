package stream

import jsvalue "github.com/nnstd/gun/runtime/builtin"

var (
	Readable    *jsvalue.JSValue
	Writable    *jsvalue.JSValue
	Duplex      *jsvalue.JSValue
	Transform   *jsvalue.JSValue
	PassThrough *jsvalue.JSValue
	AsJSValue   *jsvalue.JSValue
)

func initReadable(this *jsvalue.JSValue) {
	this.Set("_events", jsvalue.NewObject())
	this.Set("_chunks", jsvalue.NewArray())
}

func initWritable(this *jsvalue.JSValue) {
	this.Set("_events", jsvalue.NewObject())
	this.Set("_written", jsvalue.NewArray())
}

func initStreamProto(proto *jsvalue.JSValue) {
	proto.Set("on", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		event := args[1].String()
		handlers := args[0].Get("_events")
		list := handlers.Get(event)
		if !list.IsArray() {
			list = jsvalue.NewArray()
		}
		list.MethodCall("push", args[2])
		handlers.Set(event, list)
		return args[0]
	}).MarkAsMethod())
	proto.Set("emit", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewBool(false)
		}
		event := args[1].String()
		handlers := args[0].Get("_events").Get(event)
		if !handlers.IsArray() {
			return jsvalue.NewBool(false)
		}
		for _, handler := range handlers.Array() {
			if handler != nil {
				handler.Call(args[2:]...)
			}
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())
}

func init() {
	Readable = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initReadable(this)
		return nil
	}, nil)
	initStreamProto(Readable.Get("prototype"))
	Readable.Get("prototype").Set("push", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && args[0] != nil {
			args[0].Get("_chunks").MethodCall("push", args[1])
			args[0].MethodCall("emit", jsvalue.NewString("data"), args[1])
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())
	Readable.Get("prototype").Set("pipe", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		dest := args[1]
		for _, chunk := range args[0].Get("_chunks").Array() {
			dest.MethodCall("write", chunk)
		}
		return dest
	}).MarkAsMethod())

	Writable = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initWritable(this)
		return nil
	}, nil)
	initStreamProto(Writable.Get("prototype"))
	Writable.Get("prototype").Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && args[0] != nil {
			args[0].Get("_written").MethodCall("push", args[1])
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())
	Writable.Get("prototype").Set("end", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && args[1] != nil {
			args[0].MethodCall("write", args[1])
		}
		args[0].MethodCall("emit", jsvalue.NewString("finish"))
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	Duplex = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initReadable(this)
		initWritable(this)
		return nil
	}, nil)
	initStreamProto(Duplex.Get("prototype"))
	Duplex.Get("prototype").Set("push", Readable.Get("prototype").Get("push"))
	Duplex.Get("prototype").Set("pipe", Readable.Get("prototype").Get("pipe"))
	Duplex.Get("prototype").Set("write", Writable.Get("prototype").Get("write"))
	Duplex.Get("prototype").Set("end", Writable.Get("prototype").Get("end"))

	Transform = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initReadable(this)
		initWritable(this)
		return nil
	}, Duplex)
	PassThrough = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initReadable(this)
		initWritable(this)
		return nil
	}, Transform)

	AsJSValue = jsvalue.ObjectFrom(
		"Readable", Readable,
		"Writable", Writable,
		"Duplex", Duplex,
		"Transform", Transform,
		"PassThrough", PassThrough,
		"pipeline", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 {
				return jsvalue.NewUndefined()
			}
			var cb *jsvalue.JSValue
			end := len(args)
			if last := args[len(args)-1]; last != nil && last.TypeString() == "function" {
				cb = last
				end--
			}
			for i := 0; i+1 < end; i++ {
				args[i].MethodCall("pipe", args[i+1])
			}
			if cb != nil {
				cb.Call(jsvalue.NewUndefined())
			}
			if end > 0 {
				return args[end-1]
			}
			return jsvalue.NewUndefined()
		}),
		"finished", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) >= 2 && args[1] != nil && args[1].TypeString() == "function" {
				args[1].Call(jsvalue.NewUndefined())
			}
			return jsvalue.NewUndefined()
		}),
	)
}
