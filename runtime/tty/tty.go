package nodetty

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/events"
)

var (
	ReadStream  *jsvalue.JSValue
	WriteStream *jsvalue.JSValue
	AsJSValue   *jsvalue.JSValue
)

func init() {
	ReadStream = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		events.InitEventEmitter(this)
		this.Set("isTTY", jsvalue.NewBool(true))
		this.Set("isRaw", jsvalue.NewBool(false))
		return nil
	}, nil)
	events.MixinEventEmitter(ReadStream)
	setupReadStreamPrototype()

	WriteStream = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		events.InitEventEmitter(this)
		this.Set("isTTY", jsvalue.NewBool(true))
		cols, rows := getTerminalSize()
		this.Set("columns", jsvalue.NewNumber(float64(cols)))
		this.Set("rows", jsvalue.NewNumber(float64(rows)))
		this.Set("_writevCount", jsvalue.NewNumber(0))
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
			os.Stderr.WriteString("\x1b[1K")
		case 1:
			os.Stderr.WriteString("\x1b[0K")
		default:
			os.Stderr.WriteString("\x1b[2K")
		}
		if len(args) > 2 && args[2] != nil && args[2].TypeString() == "function" {
			args[2].Call()
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("clearScreenDown", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		os.Stderr.WriteString("\x1b[J")
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
			os.Stderr.WriteString(fmt.Sprintf("\x1b[%d;%dH", y+1, x+1))
		} else {
			os.Stderr.WriteString(fmt.Sprintf("\x1b[%dG", x+1))
		}
		// Find callback in remaining args
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
			os.Stderr.WriteString(fmt.Sprintf("\x1b[%dC", dx))
		} else if dx < 0 {
			os.Stderr.WriteString(fmt.Sprintf("\x1b[%dD", -dx))
		}
		if dy > 0 {
			os.Stderr.WriteString(fmt.Sprintf("\x1b[%dB", dy))
		} else if dy < 0 {
			os.Stderr.WriteString(fmt.Sprintf("\x1b[%dA", -dy))
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
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("getColorDepth", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewNumber(8)
	}).MarkAsMethod())
}

func isatty(fd int) bool {
	var st syscall.Stat_t
	err := syscall.Fstat(fd, &st)
	if err != nil {
		return false
	}
	return (st.Mode&syscall.S_IFMT) == syscall.S_IFCHR
}

func getTerminalSize() (int, int) {
	type winsize struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stderr.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}
