package v8

import (
	"encoding/binary"
	"encoding/json"
	"hash/fnv"
	"runtime"
	"sort"
	"strconv"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/profile"
	promise "github.com/nnstd/gun/runtime/promise"
)

var AsJSValue *jsvalue.JSValue

type serializerState struct {
	data string
}

type deserializerState struct {
	data string
}

type serializedEnvelope struct {
	Root  encodedValue        `json:"root"`
	Nodes map[int]encodedNode `json:"nodes,omitempty"`
}

type encodedValue struct {
	Type  string `json:"type"`
	Bool  bool   `json:"bool,omitempty"`
	Num   string `json:"num,omitempty"`
	Str   string `json:"str,omitempty"`
	Ref   int    `json:"ref,omitempty"`
	Big   string `json:"big,omitempty"`
	Regex string `json:"regex,omitempty"`
}

type encodedNode struct {
	Kind    string         `json:"kind"`
	Props   []encodedProp  `json:"props,omitempty"`
	Items   []encodedValue `json:"items,omitempty"`
	Entries []encodedEntry `json:"entries,omitempty"`
	Regex   string         `json:"regex,omitempty"`
}

type encodedProp struct {
	Key   string       `json:"key"`
	Value encodedValue `json:"value"`
}

type encodedEntry struct {
	Key   encodedValue `json:"key"`
	Value encodedValue `json:"value"`
}

var (
	serializerRegistry   = map[*jsvalue.JSValue]*serializerState{}
	deserializerRegistry = map[*jsvalue.JSValue]*deserializerState{}
)

func init() {
	Serializer := jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		serializerRegistry[this] = &serializerState{}
		return nil
	}, nil)
	Serializer.Get("prototype").Set("writeHeader", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	Serializer.Get("prototype").Set("writeValue", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		state := serializerRegistry[args[0]]
		if state == nil {
			state = &serializerState{}
			serializerRegistry[args[0]] = state
		}
		if len(args) > 1 {
			state.data = serializeData(args[1])
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	Serializer.Get("prototype").Set("releaseBuffer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return buffer.Buffer.Get("from").Call(jsvalue.NewString(""))
		}
		state := serializerRegistry[args[0]]
		if state == nil {
			return buffer.Buffer.Get("from").Call(jsvalue.NewString(""))
		}
		return buffer.Buffer.Get("from").Call(jsvalue.NewString(state.data))
	}).MarkAsMethod())

	Deserializer := jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		state := &deserializerState{}
		if len(args) > 0 && args[0] != nil {
			state.data = deserializeBytes(args[0])
		}
		deserializerRegistry[this] = state
		return nil
	}, nil)
	Deserializer.Get("prototype").Set("readHeader", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	Deserializer.Get("prototype").Set("readValue", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		state := deserializerRegistry[args[0]]
		if state == nil {
			return jsvalue.NewUndefined()
		}
		out, err := unmarshalSerialized(state.data)
		if err != nil {
			panic(jsvalue.NewString(err.Error()))
		}
		return out
	}).MarkAsMethod())

	promiseHooks := jsvalue.ObjectFrom(
		"onInit", hookRegistrar(promise.RegisterInitHook),
		"onBefore", hookRegistrar(promise.RegisterBeforeHook),
		"onAfter", hookRegistrar(promise.RegisterAfterHook),
		"onSettled", hookRegistrar(promise.RegisterSettledHook),
		"createHook", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 || args[0] == nil {
				return jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() })
			}
			cfg := args[0]
			var stops []func()
			for key, reg := range map[string]func(*jsvalue.JSValue) func(){
				"init":    promise.RegisterInitHook,
				"before":  promise.RegisterBeforeHook,
				"after":   promise.RegisterAfterHook,
				"settled": promise.RegisterSettledHook,
			} {
				if fn := cfg.Get(key); fn != nil && fn.TypeString() == "function" {
					if fn.IsAsyncFunction() {
						panic(jsvalue.NewString("promiseHooks callbacks must be plain functions"))
					}
					stops = append(stops, reg(fn))
				}
			}
			return jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				for _, stop := range stops {
					stop()
				}
				return jsvalue.NewUndefined()
			})
		}),
	)

	AsJSValue = jsvalue.ObjectFrom(
		"cachedDataVersionTag", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewNumber(float64(cachedDataVersionTag()))
		}),
		"getHeapStatistics", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return heapStatistics()
		}),
		"getHeapSpaceStatistics", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return heapSpaceStatistics()
		}),
		"getHeapCodeStatistics", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return heapCodeStatistics()
		}),
		"serialize", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 {
				return buffer.Buffer.Get("from").Call(jsvalue.NewString("null"))
			}
			return buffer.Buffer.Get("from").Call(jsvalue.NewString(serializeData(args[0])))
		}),
		"deserialize", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 {
				return jsvalue.NewUndefined()
			}
			out, err := unmarshalSerialized(deserializeBytes(args[0]))
			if err != nil {
				panic(jsvalue.NewString(err.Error()))
			}
			return out
		}),
		"Serializer", Serializer,
		"Deserializer", Deserializer,
		"DefaultSerializer", Serializer,
		"DefaultDeserializer", Deserializer,
		"promiseHooks", promiseHooks,
		"startCpuProfile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			handle, err := profile.StartCPUProfile()
			if err != nil {
				panic(jsvalue.NewString(err.Error()))
			}
			return jsvalue.ObjectFrom("stop", jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				out, err := handle.Stop()
				if err != nil {
					panic(jsvalue.NewString(err.Error()))
				}
				return jsvalue.NewString(out)
			}))
		}),
	)
}

func hookRegistrar(register func(*jsvalue.JSValue) func()) *jsvalue.JSValue {
	return jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil || args[0].TypeString() != "function" {
			return jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() })
		}
		if args[0].IsAsyncFunction() {
			panic(jsvalue.NewString("promiseHooks callbacks must be plain functions"))
		}
		stop := register(args[0])
		return jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
			stop()
			return jsvalue.NewUndefined()
		})
	})
}

func serializeData(v *jsvalue.JSValue) string {
	data, err := marshalSerialized(v)
	if err != nil {
		panic(jsvalue.NewString(err.Error()))
	}
	return data
}

func deserializeBytes(v *jsvalue.JSValue) string {
	if v == nil {
		return ""
	}
	if bs := v.Bytes(); bs != nil {
		return string(bs)
	}
	return v.String()
}

func marshalSerialized(v *jsvalue.JSValue) (string, error) {
	env := serializedEnvelope{Nodes: map[int]encodedNode{}}
	seen := map[*jsvalue.JSValue]int{}
	nextID := 1
	var encode func(*jsvalue.JSValue) (encodedValue, error)
	encode = func(v *jsvalue.JSValue) (encodedValue, error) {
		if v == nil {
			return encodedValue{Type: "null"}, nil
		}
		switch v.Type() {
		case jsvalue.TypeUndefined:
			return encodedValue{Type: "undefined"}, nil
		case jsvalue.TypeNull:
			return encodedValue{Type: "null"}, nil
		case jsvalue.TypeBoolean:
			return encodedValue{Type: "boolean", Bool: v.Bool()}, nil
		case jsvalue.TypeNumber:
			return encodedValue{Type: "number", Num: strconv.FormatFloat(v.Number(), 'g', -1, 64)}, nil
		case jsvalue.TypeString:
			return encodedValue{Type: "string", Str: v.String()}, nil
		case jsvalue.TypeBigInt:
			return encodedValue{Type: "bigint", Big: v.String()}, nil
		case jsvalue.TypeRegex:
			return encodedValue{Type: "regex", Regex: v.String()}, nil
		case jsvalue.TypeFunction, jsvalue.TypeSymbol:
			return encodedValue{}, errUnsupported("cannot serialize " + v.TypeString())
		case jsvalue.TypeMap, jsvalue.TypeSet, jsvalue.TypeObject:
			if id, ok := seen[v]; ok {
				return encodedValue{Type: "ref", Ref: id}, nil
			}
			id := nextID
			nextID++
			seen[v] = id
			node := encodedNode{}
			switch {
			case v.Type() == jsvalue.TypeMap:
				node.Kind = "map"
				for _, pair := range v.MethodCall("entries").Array() {
					ev, err := encode(pair.Get("0"))
					if err != nil {
						return encodedValue{}, err
					}
					evv, err := encode(pair.Get("1"))
					if err != nil {
						return encodedValue{}, err
					}
					node.Entries = append(node.Entries, encodedEntry{Key: ev, Value: evv})
				}
			case v.Type() == jsvalue.TypeSet:
				node.Kind = "set"
				for _, item := range v.MethodCall("values").Array() {
					ev, err := encode(item)
					if err != nil {
						return encodedValue{}, err
					}
					node.Items = append(node.Items, ev)
				}
			case v.IsArray():
				node.Kind = "array"
				for _, item := range v.Array() {
					ev, err := encode(item)
					if err != nil {
						return encodedValue{}, err
					}
					node.Items = append(node.Items, ev)
				}
			default:
				node.Kind = "object"
				keys := v.OwnKeys()
				sort.Strings(keys)
				for _, key := range keys {
					ev, err := encode(v.Get(key))
					if err != nil {
						return encodedValue{}, err
					}
					node.Props = append(node.Props, encodedProp{Key: key, Value: ev})
				}
			}
			env.Nodes[id] = node
			return encodedValue{Type: "ref", Ref: id}, nil
		default:
			return encodedValue{Type: "string", Str: v.String()}, nil
		}
	}
	root, err := encode(v)
	if err != nil {
		return "", err
	}
	env.Root = root
	data, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalSerialized(data string) (*jsvalue.JSValue, error) {
	var env serializedEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return nil, err
	}
	created := map[int]*jsvalue.JSValue{}
	var decode func(encodedValue) (*jsvalue.JSValue, error)
	decode = func(v encodedValue) (*jsvalue.JSValue, error) {
		switch v.Type {
		case "undefined":
			return jsvalue.NewUndefined(), nil
		case "null":
			return jsvalue.NewNull(), nil
		case "boolean":
			return jsvalue.NewBool(v.Bool), nil
		case "number":
			f, err := strconv.ParseFloat(v.Num, 64)
			if err != nil {
				return nil, err
			}
			return jsvalue.NewNumber(f), nil
		case "string":
			return jsvalue.NewString(v.Str), nil
		case "bigint":
			return jsvalue.BigIntCtor.Call(jsvalue.NewString(v.Big)), nil
		case "regex":
			return jsvalue.NewRegex(jsvalue.CompileRegex(v.Regex)), nil
		case "ref":
			if out, ok := created[v.Ref]; ok {
				return out, nil
			}
			node := env.Nodes[v.Ref]
			var out *jsvalue.JSValue
			switch node.Kind {
			case "array":
				out = jsvalue.NewArray()
				created[v.Ref] = out
				for _, item := range node.Items {
					dec, err := decode(item)
					if err != nil {
						return nil, err
					}
					out.MethodCall("push", dec)
				}
			case "map":
				out = jsvalue.NewMap()
				created[v.Ref] = out
				for _, entry := range node.Entries {
					k, err := decode(entry.Key)
					if err != nil {
						return nil, err
					}
					val, err := decode(entry.Value)
					if err != nil {
						return nil, err
					}
					out.MethodCall("set", k, val)
				}
			case "set":
				out = jsvalue.NewSet()
				created[v.Ref] = out
				for _, item := range node.Items {
					dec, err := decode(item)
					if err != nil {
						return nil, err
					}
					out.MethodCall("add", dec)
				}
			default:
				out = jsvalue.NewObject()
				created[v.Ref] = out
				for _, prop := range node.Props {
					dec, err := decode(prop.Value)
					if err != nil {
						return nil, err
					}
					out.Set(prop.Key, dec)
				}
			}
			return out, nil
		default:
			return jsvalue.NewUndefined(), nil
		}
	}
	return decode(env.Root)
}

func errUnsupported(msg string) error { return unsupportedError(msg) }

type unsupportedError string

func (e unsupportedError) Error() string { return string(e) }

func cachedDataVersionTag() uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(runtime.Version()))
	_, _ = h.Write([]byte(runtime.GOOS))
	_, _ = h.Write([]byte(runtime.GOARCH))
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], 1)
	_, _ = h.Write(buf[:])
	return h.Sum32()
}

func heapStatistics() *jsvalue.JSValue {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return jsvalue.ObjectFrom(
		"total_heap_size", jsvalue.NewNumber(float64(m.HeapSys)),
		"total_heap_size_executable", jsvalue.NewNumber(float64(m.HeapInuse)),
		"total_physical_size", jsvalue.NewNumber(float64(m.Sys)),
		"total_available_size", jsvalue.NewNumber(float64(m.HeapIdle)),
		"used_heap_size", jsvalue.NewNumber(float64(m.HeapAlloc)),
		"heap_size_limit", jsvalue.NewNumber(float64(m.HeapSys+m.StackSys)),
		"malloced_memory", jsvalue.NewNumber(float64(m.Mallocs)),
		"peak_malloced_memory", jsvalue.NewNumber(float64(m.TotalAlloc)),
		"does_zap_garbage", jsvalue.NewNumber(0),
		"number_of_native_contexts", jsvalue.NewNumber(1),
		"number_of_detached_contexts", jsvalue.NewNumber(0),
		"total_global_handles_size", jsvalue.NewNumber(float64(m.StackInuse)),
		"used_global_handles_size", jsvalue.NewNumber(float64(m.StackInuse)),
		"external_memory", jsvalue.NewNumber(float64(m.OtherSys)),
	)
}

func heapSpaceStatistics() *jsvalue.JSValue {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return jsvalue.NewArray(
		jsvalue.ObjectFrom(
			"space_name", jsvalue.NewString("old_space"),
			"space_size", jsvalue.NewNumber(float64(m.HeapSys)),
			"space_used_size", jsvalue.NewNumber(float64(m.HeapAlloc)),
			"space_available_size", jsvalue.NewNumber(float64(m.HeapIdle)),
			"physical_space_size", jsvalue.NewNumber(float64(m.HeapInuse)),
		),
	)
}

func heapCodeStatistics() *jsvalue.JSValue {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return jsvalue.ObjectFrom(
		"code_and_metadata_size", jsvalue.NewNumber(float64(m.GCSys)),
		"bytecode_and_metadata_size", jsvalue.NewNumber(float64(m.BuckHashSys)),
		"external_script_source_size", jsvalue.NewNumber(float64(m.OtherSys)),
		"cpu_profiler_metadata_size", jsvalue.NewNumber(0),
	)
}
