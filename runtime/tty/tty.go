package nodetty

import (
	"fmt"
	"os"
	"sync"

	jserror "github.com/nnstd/gun/runtime/builtin/error"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/events"
)

var (
	ReadStream  *jsvalue.JSValue
	WriteStream *jsvalue.JSValue
	AsJSValue   *jsvalue.JSValue
)

var (
	termiosMu       sync.Mutex
	originalTermios = map[int]termiosState{}
)

type termiosState struct {
	data interface{}
}

func init() {
	ReadStream = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		fd := validateFd(args)
		events.InitEventEmitter(this)
		this.Set("isTTY", jsvalue.NewBool(true))
		this.Set("isRaw", jsvalue.NewBool(false))
		this.Set("_fd", jsvalue.NewNumber(float64(fd)))
		return nil
	}, nil)
	events.MixinEventEmitter(ReadStream)
	setupReadStreamPrototype()

	WriteStream = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		fd := validateFd(args)
		events.InitEventEmitter(this)
		this.Set("isTTY", jsvalue.NewBool(true))
		this.Set("_fd", jsvalue.NewNumber(float64(fd)))
		cols, rows := getTerminalSize(fd)
		this.Set("columns", jsvalue.NewNumber(float64(cols)))
		this.Set("rows", jsvalue.NewNumber(float64(rows)))
		this.Set("_writevCount", jsvalue.NewNumber(0))
		startResizeMonitorUnix(this, fd)
		return nil
	}, nil)
	events.MixinEventEmitter(WriteStream)
	setupWriteStreamPrototype()

	AsJSValue = jsvalue.ObjectFrom(
		"ReadStream", ReadStream,
		"WriteStream", WriteStream,
		"isatty", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 || args[0] == nil {
				return jsvalue.NewBool(false)
			}
			fd := int(args[0].Number())
			return jsvalue.NewBool(isatty(fd))
		}),
	)
}

func validateFd(args []*jsvalue.JSValue) int {
	if len(args) < 1 || args[0] == nil || args[0].TypeString() == "undefined" {
		panic(jserror.RangeError.Call(jsvalue.NewString(`"fd" must be a positive integer: undefined`)))
	}
	fd := args[0].Number()
	if fd != float64(int(fd)) || fd < 0 {
		panic(jserror.RangeError.Call(jsvalue.NewString(fmt.Sprintf(`"fd" must be a positive integer: %v`, args[0].String()))))
	}
	return int(fd)
}

func writeFD(this *jsvalue.JSValue, s string) {
	fd := int(this.Get("_fd").Number())
	f := os.NewFile(uintptr(fd), "")
	if f != nil {
		f.WriteString(s)
	}
}

func setupReadStreamPrototype() {
	proto := ReadStream.Get("prototype")

	proto.Set("setRawMode", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		mode := false
		if args[1] != nil {
			mode = args[1].Bool()
		}
		fd := int(this.Get("_fd").Number())
		if err := setRawMode(fd, mode); err != nil {
			panic(jserror.Error.Call(jsvalue.NewString(err.Error())))
		}
		this.Set("isRaw", jsvalue.NewBool(mode))
		return this
	}).MarkAsMethod())
}

func setupWriteStreamPrototype() {
	proto := WriteStream.Get("prototype")

	proto.Set("clearLine", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		dir := 0
		if len(args) > 1 && args[1] != nil {
			dir = int(args[1].Number())
		}
		switch dir {
		case -1:
			writeFD(args[0], "\x1b[1K")
		case 1:
			writeFD(args[0], "\x1b[0K")
		default:
			writeFD(args[0], "\x1b[2K")
		}
		if len(args) > 2 && args[2] != nil && args[2].TypeString() == "function" {
			args[2].Call()
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("clearScreenDown", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		writeFD(args[0], "\x1b[J")
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "function" {
			args[1].Call()
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("cursorTo", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[1] == nil {
			return jsvalue.NewBool(true)
		}
		x := int(args[1].Number())
		y := -1
		if len(args) > 2 && args[2] != nil {
			y = int(args[2].Number())
		}
		if y >= 0 {
			writeFD(args[0], fmt.Sprintf("\x1b[%d;%dH", y+1, x+1))
		} else {
			writeFD(args[0], fmt.Sprintf("\x1b[%dG", x+1))
		}
		for _, arg := range args[3:] {
			if arg != nil && arg.TypeString() == "function" {
				arg.Call()
				break
			}
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("moveCursor", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 {
			return jsvalue.NewBool(true)
		}
		dx := int(args[1].Number())
		dy := int(args[2].Number())
		if dx > 0 {
			writeFD(args[0], fmt.Sprintf("\x1b[%dC", dx))
		} else if dx < 0 {
			writeFD(args[0], fmt.Sprintf("\x1b[%dD", -dx))
		}
		if dy > 0 {
			writeFD(args[0], fmt.Sprintf("\x1b[%dB", dy))
		} else if dy < 0 {
			writeFD(args[0], fmt.Sprintf("\x1b[%dA", -dy))
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("getWindowSize", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		cols := this.Get("columns").Number()
		rows := this.Get("rows").Number()
		arr := jsvalue.NewArray()
		arr.MethodCall("push", jsvalue.NewNumber(cols))
		arr.MethodCall("push", jsvalue.NewNumber(rows))
		return arr
	}).MarkAsMethod())

	proto.Set("hasColors", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		count := 16
		var env *jsvalue.JSValue
		for _, arg := range args[1:] {
			if arg != nil && arg.TypeString() == "number" {
				count = int(arg.Number())
			} else if arg != nil && arg.TypeString() == "object" {
				env = arg
			}
		}
		depth := getColorDepth(env)
		colorCount := depthToColorCount(depth)
		return jsvalue.NewBool(colorCount >= count)
	}).MarkAsMethod())

	proto.Set("getColorDepth", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var env *jsvalue.JSValue
		for _, arg := range args[1:] {
			if arg != nil && arg.TypeString() == "object" {
				env = arg
				break
			}
		}
		return jsvalue.NewNumber(float64(getColorDepth(env)))
	}).MarkAsMethod())
}

func depthToColorCount(depth int) int {
	switch depth {
	case 1:
		return 2
	case 4:
		return 16
	case 8:
		return 256
	case 24:
		return 16777216
	default:
		return 2
	}
}

func getEnvVar(env *jsvalue.JSValue, key string) string {
	if env != nil {
		v := env.Get(key)
		if v != nil && v.TypeString() != "undefined" {
			return v.String()
		}
		return ""
	}
	return os.Getenv(key)
}

func hasEnvVar(env *jsvalue.JSValue, key string) bool {
	if env != nil {
		v := env.Get(key)
		return v != nil && v.TypeString() != "undefined"
	}
	_, ok := os.LookupEnv(key)
	return ok
}

func getColorDepth(env *jsvalue.JSValue) int {
	if hasEnvVar(env, "NO_COLOR") {
		return 1
	}
	if hasEnvVar(env, "NODE_DISABLE_COLORS") {
		return 1
	}
	if hasEnvVar(env, "FORCE_COLOR") {
		switch getEnvVar(env, "FORCE_COLOR") {
		case "":
			return 4
		case "1":
			return 4
		case "2":
			return 8
		case "3":
			return 24
		case "true":
			return 4
		default:
			return 1
		}
	}
	if !isatty(1) && !isatty(2) {
		return 1
	}
	colorterm := getEnvVar(env, "COLORTERM")
	if colorterm == "truecolor" || colorterm == "24bit" {
		return 24
	}
	termProgram := getEnvVar(env, "TERM_PROGRAM")
	if termProgram == "iTerm.app" || termProgram == "vscode" {
		return 24
	}
	term := getEnvVar(env, "TERM")
	if term == "xterm-256color" || term == "xterm-256color-italic" {
		return 8
	}
	return 4
}
