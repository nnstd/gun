package dgram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"syscall"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/events"
)

var (
	Socket    *jsvalue.JSValue
	AsJSValue *jsvalue.JSValue

	socketRegistryMu sync.Mutex
	socketRegistry   = map[*jsvalue.JSValue]*socketState{}
)

type socketState struct {
	mu               sync.Mutex
	conn             *net.UDPConn
	network          string
	reuseAddr        bool
	reusePort        bool
	bound            bool
	closed           bool
	handleRegistered bool
	readLoopStarted  bool
}

type createSocketOptions struct {
	network   string
	reuseAddr bool
	reusePort bool
	callback  *jsvalue.JSValue
}

type bindOptions struct {
	port     int
	address  string
	callback *jsvalue.JSValue
}

type sendOptions struct {
	payload  []byte
	port     int
	address  string
	callback *jsvalue.JSValue
}

func init() {
	Socket = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		events.InitEventEmitter(this)
		this.Set("listening", jsvalue.NewBool(false))

		opts, err := parseCreateSocketArgs(args)
		if err != nil {
			panic(jserror.InvalidArgType(err.Error()))
		}
		state := socketStateOf(this)
		state.network = opts.network
		state.reuseAddr = opts.reuseAddr
		state.reusePort = opts.reusePort

		this.Set("type", jsvalue.NewString(opts.network))
		if opts.callback != nil {
			this.MethodCall("on", jsvalue.NewString("message"), opts.callback)
		}
		return nil
	}, nil)
	events.MixinEventEmitter(Socket)
	setupSocketPrototype()

	AsJSValue = jsvalue.ObjectFrom(
		"createSocket", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return Socket.Call(args...)
		}),
		"Socket", Socket,
	)
}

func socketStateOf(v *jsvalue.JSValue) *socketState {
	socketRegistryMu.Lock()
	defer socketRegistryMu.Unlock()
	if st, ok := socketRegistry[v]; ok {
		return st
	}
	st := &socketState{network: "udp4"}
	socketRegistry[v] = st
	return st
}

func parseCreateSocketArgs(args []*jsvalue.JSValue) (createSocketOptions, error) {
	opts := createSocketOptions{network: "udp4"}
	for _, arg := range args {
		if arg == nil {
			continue
		}
		switch arg.TypeString() {
		case "string":
			opts.network = arg.String()
		case "function":
			opts.callback = arg
		case "object":
			if v := arg.Get("type"); v.TypeString() == "string" {
				opts.network = v.String()
			}
			if v := arg.Get("reuseAddr"); v.TypeString() == "boolean" {
				opts.reuseAddr = v.Bool()
			}
			if v := arg.Get("reusePort"); v.TypeString() == "boolean" {
				opts.reusePort = v.Bool()
			}
			for _, key := range []string{"lookup", "signal", "recvBufferSize", "sendBufferSize", "receiveBlockList", "sendBlockList"} {
				if v := arg.Get(key); v != nil && v.TypeString() != "undefined" {
					return opts, fmt.Errorf("dgram.createSocket() option %q is not supported", key)
				}
			}
		default:
			return opts, fmt.Errorf("dgram.createSocket() requires a socket type or options object")
		}
	}
	if opts.network != "udp4" && opts.network != "udp6" {
		return opts, fmt.Errorf("dgram.createSocket() type must be \"udp4\" or \"udp6\"")
	}
	return opts, nil
}

func defaultBindAddress(network string) string {
	if network == "udp6" {
		return "::"
	}
	return "0.0.0.0"
}

func parseBindArgs(args []*jsvalue.JSValue, network string) (bindOptions, error) {
	opts := bindOptions{address: defaultBindAddress(network)}
	for _, arg := range args {
		if arg == nil {
			continue
		}
		switch arg.TypeString() {
		case "function":
			opts.callback = arg
		case "number":
			opts.port = int(arg.Number())
		case "string":
			opts.address = arg.String()
		case "object":
			if v := arg.Get("port"); v.TypeString() == "number" {
				opts.port = int(v.Number())
			}
			if v := arg.Get("address"); v.TypeString() == "string" {
				opts.address = v.String()
			}
		default:
			return opts, fmt.Errorf("dgram.bind() received an unsupported argument")
		}
	}
	return opts, nil
}

func parseSendArgs(args []*jsvalue.JSValue) (sendOptions, error) {
	var opts sendOptions
	if len(args) == 0 || args[0] == nil {
		return opts, fmt.Errorf("dgram.send() requires a payload")
	}
	payload, err := coercePayload(args[0])
	if err != nil {
		return opts, err
	}
	opts.payload = payload
	for _, arg := range args[1:] {
		if arg == nil {
			continue
		}
		switch arg.TypeString() {
		case "number":
			opts.port = int(arg.Number())
		case "string":
			opts.address = arg.String()
		case "function":
			opts.callback = arg
		default:
			return opts, fmt.Errorf("dgram.send() received an unsupported argument")
		}
	}
	if opts.port <= 0 {
		return opts, fmt.Errorf("dgram.send() requires a destination port")
	}
	if opts.address == "" {
		return opts, fmt.Errorf("dgram.send() requires a destination address")
	}
	return opts, nil
}

func coercePayload(v *jsvalue.JSValue) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("dgram.send() payload must be a string or Buffer")
	}
	if v.TypeString() == "string" {
		return []byte(v.String()), nil
	}
	if jsvalue.InstanceOf(v, buffer.Buffer).Bool() {
		if bs := v.Bytes(); bs != nil {
			return append([]byte(nil), bs...), nil
		}
		return nil, nil
	}
	return nil, fmt.Errorf("dgram.send() payload must be a string or Buffer")
}

func setupSocketPrototype() {
	proto := Socket.Get("prototype")

	proto.Set("bind", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := socketStateOf(this)

		opts, err := parseBindArgs(args[1:], state.network)
		if err != nil {
			panic(jserror.InvalidArgType(err.Error()))
		}

		state.mu.Lock()
		if state.closed {
			state.mu.Unlock()
			panic(jserror.InvalidArgType("Not running"))
		}
		if state.conn != nil {
			state.mu.Unlock()
			panic(jserror.InvalidArgType("Socket is already bound"))
		}
		state.mu.Unlock()

		conn, err := listenUDP(state.network, opts.address, opts.port, state.reuseAddr, state.reusePort)
		if err != nil {
			errVal := newSocketError(err)
			this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			if opts.callback != nil {
				opts.callback.Call(errVal)
			}
			return this
		}

		state.mu.Lock()
		state.conn = conn
		state.bound = true
		if !state.handleRegistered {
			state.handleRegistered = true
			eventloop.Default.RegisterHandle()
		}
		startReadLoop := !state.readLoopStarted
		state.readLoopStarted = true
		state.mu.Unlock()

		this.Set("listening", jsvalue.NewBool(true))
		if startReadLoop {
			go readLoop(this, state)
		}
		eventloop.Default.ScheduleCallback(func() {
			this.MethodCall("emit", jsvalue.NewString("listening"))
			if opts.callback != nil {
				opts.callback.Call(jsvalue.NewNull())
			}
		})
		return this
	}).MarkAsMethod())

	proto.Set("send", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := socketStateOf(this)

		opts, err := parseSendArgs(args[1:])
		if err != nil {
			errVal := newSocketError(err)
			this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			if cb := extractCallback(args[1:]); cb != nil {
				cb.Call(errVal)
			}
			return jsvalue.NewUndefined()
		}

		state.mu.Lock()
		conn := state.conn
		closed := state.closed
		state.mu.Unlock()

		if closed {
			errVal := newSocketNotRunningError()
			this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			if opts.callback != nil {
				opts.callback.Call(errVal)
			}
			return jsvalue.NewUndefined()
		}
		if conn == nil {
			errVal := newSocketError(fmt.Errorf("socket must be bound before send() in Gun dgram v1"))
			this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			if opts.callback != nil {
				opts.callback.Call(errVal)
			}
			return jsvalue.NewUndefined()
		}

		addr, err := net.ResolveUDPAddr(state.network, net.JoinHostPort(opts.address, strconv.Itoa(opts.port)))
		if err != nil {
			errVal := newSocketError(err)
			this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			if opts.callback != nil {
				opts.callback.Call(errVal)
			}
			return jsvalue.NewUndefined()
		}

		go func(payload []byte, addr *net.UDPAddr, cb *jsvalue.JSValue) {
			n, err := conn.WriteToUDP(payload, addr)
			eventloop.Default.ScheduleCallback(func() {
				if err != nil {
					errVal := newSocketError(err)
					this.MethodCall("emit", jsvalue.NewString("error"), errVal)
					if cb != nil {
						cb.Call(errVal)
					}
					return
				}
				if cb != nil {
					cb.Call(jsvalue.NewNull(), jsvalue.NewNumber(float64(n)))
				}
			})
		}(opts.payload, addr, opts.callback)
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	proto.Set("address", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewNull()
		}
		state := socketStateOf(args[0])
		state.mu.Lock()
		conn := state.conn
		state.mu.Unlock()
		if conn == nil {
			return jsvalue.NewNull()
		}
		addr, ok := conn.LocalAddr().(*net.UDPAddr)
		if !ok || addr == nil {
			return jsvalue.NewNull()
		}
		return udpAddrValue(addr)
	}).MarkAsMethod())

	proto.Set("close", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		var cb *jsvalue.JSValue
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "function" {
			cb = args[1]
		}
		state := socketStateOf(this)

		state.mu.Lock()
		conn := state.conn
		state.conn = nil
		wasClosed := state.closed
		state.closed = true
		state.bound = false
		hadHandle := state.handleRegistered
		state.handleRegistered = false
		state.mu.Unlock()

		this.Set("listening", jsvalue.NewBool(false))
		if conn != nil {
			_ = conn.Close()
		}
		if wasClosed {
			if cb != nil {
				cb.Call()
			}
			return this
		}
		if hadHandle {
			eventloop.Default.ScheduleCallback(func() {
				this.MethodCall("emit", jsvalue.NewString("close"))
				if cb != nil {
					cb.Call()
				}
			})
			eventloop.Default.UnregisterHandle()
			return this
		}
		this.MethodCall("emit", jsvalue.NewString("close"))
		if cb != nil {
			cb.Call()
		}
		return this
	}).MarkAsMethod())
}

func listenUDP(network, address string, port int, reuseAddr, reusePort bool) (*net.UDPConn, error) {
	lc := net.ListenConfig{}
	if reuseAddr || reusePort {
		lc.Control = func(_, _ string, raw syscall.RawConn) error {
			var sockErr error
			if err := raw.Control(func(fd uintptr) {
				if reuseAddr {
					sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
					if sockErr != nil {
						return
					}
				}
				if reusePort {
					sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1)
				}
			}); err != nil {
				return err
			}
			return sockErr
		}
	}

	pc, err := lc.ListenPacket(context.Background(), network, net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, fmt.Errorf("listen packet returned non-UDP connection")
	}
	return conn, nil
}

func readLoop(socket *jsvalue.JSValue, state *socketState) {
	buf := make([]byte, 64*1024)
	for {
		state.mu.Lock()
		conn := state.conn
		closed := state.closed
		state.mu.Unlock()
		if conn == nil || closed {
			return
		}

		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			state.mu.Lock()
			closed = state.closed
			state.mu.Unlock()
			if closed {
				return
			}
			errVal := newSocketError(err)
			eventloop.Default.ScheduleCallback(func() {
				socket.MethodCall("emit", jsvalue.NewString("error"), errVal)
			})
			return
		}

		payload := append([]byte(nil), buf[:n]...)
		rinfo := udpAddrValue(addr)
		rinfo.Set("size", jsvalue.NewNumber(float64(n)))
		eventloop.Default.ScheduleCallback(func() {
			msg := buffer.Buffer.Get("from").Call(jsvalue.NewString(string(payload)))
			socket.MethodCall("emit", jsvalue.NewString("message"), msg, rinfo)
		})
	}
}

func udpAddrValue(addr *net.UDPAddr) *jsvalue.JSValue {
	if addr == nil {
		return jsvalue.NewNull()
	}
	family := "IPv4"
	if addr.IP.To4() == nil {
		family = "IPv6"
	}
	return jsvalue.ObjectFrom(
		"address", jsvalue.NewString(addr.IP.String()),
		"family", jsvalue.NewString(family),
		"port", jsvalue.NewNumber(float64(addr.Port)),
	)
}

func newSocketNotRunningError() *jsvalue.JSValue {
	errVal := jserror.Error.Call(jsvalue.NewString("Not running"))
	errVal.Set("code", jsvalue.NewString("ERR_SOCKET_DGRAM_NOT_RUNNING"))
	return errVal
}

func newSocketError(err error) *jsvalue.JSValue {
	if err == nil {
		return jserror.Error.Call(jsvalue.NewString("dgram error"))
	}
	errVal := jserror.Error.Call(jsvalue.NewString(err.Error()))
	switch {
	case netErrTimeout(err):
		errVal.Set("code", jsvalue.NewString("ETIMEDOUT"))
	case isAddrInUse(err):
		errVal.Set("code", jsvalue.NewString("EADDRINUSE"))
	default:
		errVal.Set("code", jsvalue.NewString("EDGRAM"))
	}
	return errVal
}

func netErrTimeout(err error) bool {
	if err == nil {
		return false
	}
	type timeout interface{ Timeout() bool }
	if te, ok := err.(timeout); ok {
		return te.Timeout()
	}
	return false
}

func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

func extractCallback(args []*jsvalue.JSValue) *jsvalue.JSValue {
	for _, arg := range args {
		if arg != nil && arg.TypeString() == "function" {
			return arg
		}
	}
	return nil
}
