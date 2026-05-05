package nodenet

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/events"
)

var (
	Socket    *jsvalue.JSValue
	Server    *jsvalue.JSValue
	AsJSValue *jsvalue.JSValue

	socketRegistryMu sync.Mutex
	socketRegistry   = map[*jsvalue.JSValue]*socketState{}

	serverRegistryMu sync.Mutex
	serverRegistry   = map[*jsvalue.JSValue]*serverState{}
)

type socketState struct {
	mu               sync.Mutex
	conn             *net.TCPConn
	allowHalfOpen    bool
	connecting       bool
	destroyed        bool
	ended            bool
	paused           bool
	handleRegistered bool
	readLoopStarted  bool
	bytesRead        int64
	bytesWritten     int64
	encoding         string
	host             string
	port             int
}

type serverState struct {
	mu               sync.Mutex
	listener         *net.TCPListener
	listening        bool
	closed           bool
	handleRegistered bool
}

func socketStateOf(v *jsvalue.JSValue) *socketState {
	socketRegistryMu.Lock()
	defer socketRegistryMu.Unlock()
	if st, ok := socketRegistry[v]; ok {
		return st
	}
	st := &socketState{}
	socketRegistry[v] = st
	return st
}

func getSocketState(v *jsvalue.JSValue) *socketState {
	socketRegistryMu.Lock()
	defer socketRegistryMu.Unlock()
	return socketRegistry[v]
}

func serverStateOf(v *jsvalue.JSValue) *serverState {
	serverRegistryMu.Lock()
	defer serverRegistryMu.Unlock()
	if st, ok := serverRegistry[v]; ok {
		return st
	}
	st := &serverState{}
	serverRegistry[v] = st
	return st
}

func getServerState(v *jsvalue.JSValue) *serverState {
	serverRegistryMu.Lock()
	defer serverRegistryMu.Unlock()
	return serverRegistry[v]
}

func init() {
	Socket = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		events.InitEventEmitter(this)
		state := socketStateOf(this)
		for _, arg := range args {
			if arg != nil && arg.TypeString() == "object" {
				if v := arg.Get("allowHalfOpen"); v.TypeString() == "boolean" {
					state.allowHalfOpen = v.Bool()
				}
			}
		}
		this.Set("connecting", jsvalue.NewBool(false))
		this.Set("destroyed", jsvalue.NewBool(false))
		this.Set("pending", jsvalue.NewBool(true))
		this.Set("readyState", jsvalue.NewString("closed"))
		this.Set("bytesRead", jsvalue.NewNumber(0))
		this.Set("bytesWritten", jsvalue.NewNumber(0))
		return nil
	}, nil)
	events.MixinEventEmitter(Socket)
	setupSocketPrototype()

	Server = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		events.InitEventEmitter(this)
		_ = serverStateOf(this)
		allowHalfOpen := false
		for _, arg := range args {
			if arg == nil {
				continue
			}
			if arg.TypeString() == "object" {
				if v := arg.Get("allowHalfOpen"); v.TypeString() == "boolean" {
					allowHalfOpen = v.Bool()
				}
			}
		}
		this.Set("listening", jsvalue.NewBool(false))
		this.Set("maxConnections", jsvalue.NewNumber(0))
		this.Set("_allowHalfOpen", jsvalue.NewBool(allowHalfOpen))
		for _, arg := range args {
			if arg != nil && arg.TypeString() == "function" {
				this.MethodCall("on", jsvalue.NewString("connection"), arg)
			}
		}
		return nil
	}, nil)
	events.MixinEventEmitter(Server)
	setupServerPrototype()

	AsJSValue = jsvalue.ObjectFrom(
		"createServer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return Server.Call(args...)
		}),
		"createConnection", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return doCreateConnection(args)
		}),
		"connect", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return doCreateConnection(args)
		}),
		"Socket", Socket,
		"Server", Server,
		"isIP", jsvalue.NewFunction(isIPFn),
		"isIPv4", jsvalue.NewFunction(isIPv4Fn),
		"isIPv6", jsvalue.NewFunction(isIPv6Fn),
	)
}

func isIPFn(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil || args[0].TypeString() != "string" {
		return jsvalue.NewNumber(0)
	}
	ip := net.ParseIP(args[0].String())
	if ip == nil {
		return jsvalue.NewNumber(0)
	}
	if ip.To4() != nil {
		return jsvalue.NewNumber(4)
	}
	return jsvalue.NewNumber(6)
}

func isIPv4Fn(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil || args[0].TypeString() != "string" {
		return jsvalue.NewBool(false)
	}
	ip := net.ParseIP(args[0].String())
	return jsvalue.NewBool(ip != nil && ip.To4() != nil)
}

func isIPv6Fn(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil || args[0].TypeString() != "string" {
		return jsvalue.NewBool(false)
	}
	ip := net.ParseIP(args[0].String())
	return jsvalue.NewBool(ip != nil && ip.To4() == nil)
}

func doCreateConnection(args []*jsvalue.JSValue) *jsvalue.JSValue {
	socket := Socket.Call()
	var connectListener *jsvalue.JSValue
	var connectArgs []*jsvalue.JSValue
	for _, arg := range args {
		if arg == nil {
			continue
		}
		if arg.TypeString() == "function" && connectListener == nil {
			connectListener = arg
		} else {
			connectArgs = append(connectArgs, arg)
		}
	}
	if connectListener != nil {
		socket.MethodCall("on", jsvalue.NewString("connect"), connectListener)
	}
	if len(connectArgs) > 0 {
		socket.MethodCall("connect", connectArgs...)
	}
	return socket
}

func setupSocketPrototype() {
	proto := Socket.Get("prototype")

	proto.Set("connect", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := socketStateOf(this)
		var port int
		host := "localhost"
		for _, arg := range args[1:] {
			if arg == nil {
				continue
			}
			switch arg.TypeString() {
			case "number":
				port = int(arg.Number())
			case "string":
				host = arg.String()
			case "function":
				this.MethodCall("on", jsvalue.NewString("connect"), arg)
			case "object":
				if v := arg.Get("port"); v.TypeString() == "number" {
					port = int(v.Number())
				}
				if v := arg.Get("host"); v.TypeString() == "string" {
					host = v.String()
				}
				if v := arg.Get("allowHalfOpen"); v.TypeString() == "boolean" {
					state.allowHalfOpen = v.Bool()
				}
			}
		}
		if port == 0 {
			err := jserror.Error.Call(jserror.Error, jsvalue.NewString(`The "options" or "port" or "path" argument must be specified`))
			err.Set("code", jsvalue.NewString("ERR_MISSING_ARGS"))
			panic(err)
		}
		state.mu.Lock()
		state.connecting = true
		state.host = host
		state.port = port
		if !state.handleRegistered {
			state.handleRegistered = true
			eventloop.Default.RegisterHandle()
		}
		state.mu.Unlock()
		this.Set("connecting", jsvalue.NewBool(true))
		this.Set("pending", jsvalue.NewBool(true))
		this.Set("readyState", jsvalue.NewString("opening"))
		go func(host string, port int) {
			addr := net.JoinHostPort(host, strconv.Itoa(port))
			conn, dialErr := net.Dial("tcp", addr)
				eventloop.Default.ScheduleCallback(func() {
				state.mu.Lock()
				state.connecting = false
				if state.destroyed {
					state.mu.Unlock()
					if conn != nil {
						_ = conn.Close()
					}
					return
				}
				if dialErr != nil {
					state.mu.Unlock()
					this.Set("connecting", jsvalue.NewBool(false))
					this.MethodCall("destroy", connectError(dialErr, host, port))
					return
				}
				tcpConn, ok := conn.(*net.TCPConn)
				if !ok {
					_ = conn.Close()
					state.mu.Unlock()
					this.MethodCall("destroy", connectError(fmt.Errorf("invalid connection type"), host, port))
					return
				}
				state.conn = tcpConn
				startRead := !state.readLoopStarted
				state.readLoopStarted = true
				state.mu.Unlock()
				this.Set("connecting", jsvalue.NewBool(false))
				this.Set("pending", jsvalue.NewBool(false))
				this.Set("readyState", jsvalue.NewString("open"))
				updateSocketAddresses(this, tcpConn)
				if startRead {
					go socketReadLoop(this, state)
				}
				this.MethodCall("emit", jsvalue.NewString("connect"))
				this.MethodCall("emit", jsvalue.NewString("ready"))
			})
		}(host, port)
		return this
	}).MarkAsMethod())

	proto.Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewBool(false)
		}
		this := args[0]
		state := socketStateOf(this)
		state.mu.Lock()
		if state.destroyed {
			state.mu.Unlock()
			errVal := jserror.Error.Call(jsvalue.NewString("Cannot call write after a stream was destroyed"))
			errVal.Set("code", jsvalue.NewString("ERR_STREAM_DESTROYED"))
			this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			return jsvalue.NewBool(false)
		}
		if state.ended {
			state.mu.Unlock()
			errVal := jserror.Error.Call(jsvalue.NewString("write after end"))
			errVal.Set("code", jsvalue.NewString("ERR_STREAM_WRITE_AFTER_END"))
			this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			return jsvalue.NewBool(false)
		}
		conn := state.conn
		state.mu.Unlock()
		if conn == nil {
			return jsvalue.NewBool(false)
		}
		if len(args) < 2 || args[1] == nil {
			return jsvalue.NewBool(false)
		}
		data, coerceErr := coerceWriteData(args[1])
		if coerceErr != nil {
			errVal := jserror.Error.Call(jsvalue.NewString(coerceErr.Error()))
			this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			return jsvalue.NewBool(false)
		}
		var cb *jsvalue.JSValue
		for _, arg := range args[2:] {
			if arg != nil && arg.TypeString() == "function" && cb == nil {
				cb = arg
			}
		}
		go func() {
			n, writeErr := conn.Write(data)
			eventloop.Default.ScheduleCallback(func() {
				state.mu.Lock()
				if state.destroyed {
					state.mu.Unlock()
					return
				}
				if writeErr != nil {
					state.mu.Unlock()
					this.MethodCall("emit", jsvalue.NewString("error"), readError(writeErr))
					return
				}
				state.bytesWritten += int64(n)
				bw := state.bytesWritten
				state.mu.Unlock()
				this.Set("bytesWritten", jsvalue.NewNumber(float64(bw)))
				if cb != nil {
					cb.Call()
				}
			})
		}()
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	proto.Set("end", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := socketStateOf(this)
		state.mu.Lock()
		if state.destroyed {
			state.mu.Unlock()
			return this
		}
		conn := state.conn
		state.ended = true
		state.mu.Unlock()
		var cb *jsvalue.JSValue
		var writeData []byte
		for _, arg := range args[1:] {
			if arg == nil {
				continue
			}
			if arg.TypeString() == "function" && cb == nil {
				cb = arg
			} else if arg.TypeString() == "string" || (arg.TypeString() == "object" && jsvalue.InstanceOf(arg, buffer.Buffer).Bool()) {
				data, _ := coerceWriteData(arg)
				if data != nil {
					writeData = append(writeData, data...)
				}
			}
		}
		go func() {
			if conn != nil && len(writeData) > 0 {
				_, _ = conn.Write(writeData)
			}
			if conn != nil {
				_ = conn.CloseWrite()
			}
		}()
		this.Set("readyState", jsvalue.NewString("readOnly"))
		this.MethodCall("emit", jsvalue.NewString("finish"))
		if cb != nil {
			cb.Call()
		}
		return this
	}).MarkAsMethod())

	proto.Set("destroy", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := getSocketState(this)
		if state == nil {
			return this
		}
		state.mu.Lock()
		if state.destroyed {
			state.mu.Unlock()
			return this
		}
		state.destroyed = true
		conn := state.conn
		hadHandle := state.handleRegistered
		state.handleRegistered = false
		state.connecting = false
		state.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
		socketRegistryMu.Lock()
		delete(socketRegistry, this)
		socketRegistryMu.Unlock()
		if hadHandle {
			eventloop.Default.UnregisterHandle()
		}
		var errVal *jsvalue.JSValue
		if len(args) > 1 && args[1] != nil {
			errVal = args[1]
		}
		this.Set("destroyed", jsvalue.NewBool(true))
		this.Set("connecting", jsvalue.NewBool(false))
		this.Set("pending", jsvalue.NewBool(false))
		this.Set("readyState", jsvalue.NewString("closed"))
		hadError := errVal != nil
		eventloop.Default.ScheduleCallback(func() {
			if hadError {
				this.MethodCall("emit", jsvalue.NewString("error"), errVal)
			}
			this.MethodCall("emit", jsvalue.NewString("close"), jsvalue.NewBool(hadError))
		})
		return this
	}).MarkAsMethod())

	proto.Set("pause", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		state := socketStateOf(args[0])
		state.paused = true
		return args[0]
	}).MarkAsMethod())

	proto.Set("resume", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		state := socketStateOf(args[0])
		state.paused = false
		return args[0]
	}).MarkAsMethod())

	proto.Set("setEncoding", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		state := socketStateOf(args[0])
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "string" {
			state.encoding = args[1].String()
		}
		return args[0]
	}).MarkAsMethod())

	proto.Set("setNoDelay", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := socketStateOf(this)
		state.mu.Lock()
		conn := state.conn
		state.mu.Unlock()
		noDelay := true
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "boolean" {
			noDelay = args[1].Bool()
		}
		if conn != nil {
			_ = conn.SetNoDelay(noDelay)
		}
		return this
	}).MarkAsMethod())

	proto.Set("setKeepAlive", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := socketStateOf(this)
		state.mu.Lock()
		conn := state.conn
		state.mu.Unlock()
		enable := false
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "boolean" {
			enable = args[1].Bool()
		}
		if conn != nil {
			_ = conn.SetKeepAlive(enable)
			if len(args) > 2 && args[2] != nil && args[2].TypeString() == "number" {
				initialDelay := time.Duration(args[2].Number()) * time.Millisecond
				if initialDelay > 0 {
					_ = conn.SetKeepAlivePeriod(initialDelay)
				}
			}
		}
		return this
	}).MarkAsMethod())

	proto.Set("setTimeout", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		if len(args) < 2 || args[1] == nil {
			return this
		}
		timeout := args[1].Number()
		if timeout <= 0 {
			return this
		}
		var cb *jsvalue.JSValue
		if len(args) > 2 && args[2] != nil && args[2].TypeString() == "function" {
			cb = args[2]
		}
		time.AfterFunc(time.Duration(timeout)*time.Millisecond, func() {
			eventloop.Default.ScheduleCallback(func() {
				this.MethodCall("emit", jsvalue.NewString("timeout"))
				if cb != nil {
					cb.Call()
				}
			})
		})
		return this
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
		addr, ok := conn.LocalAddr().(*net.TCPAddr)
		if !ok || addr == nil {
			return jsvalue.NewNull()
		}
		return tcpAddrToJS(addr)
	}).MarkAsMethod())
}

func setupServerPrototype() {
	proto := Server.Get("prototype")

	proto.Set("listen", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := serverStateOf(this)
		var port int
		host := "0.0.0.0"
		var cb *jsvalue.JSValue
		for _, arg := range args[1:] {
			if arg == nil {
				continue
			}
			switch arg.TypeString() {
			case "number":
				port = int(arg.Number())
			case "string":
				host = arg.String()
			case "function":
				if cb == nil {
					cb = arg
				}
			case "object":
				if v := arg.Get("port"); v.TypeString() == "number" {
					port = int(v.Number())
				}
				if v := arg.Get("host"); v.TypeString() == "string" {
					host = v.String()
				}
			}
		}
		validatePort(float64(port))
		state.mu.Lock()
		if state.listening {
			state.mu.Unlock()
			err := jserror.Error.Call(jserror.Error, jsvalue.NewString("Listen method has been called more than once without closing."))
			err.Set("code", jsvalue.NewString("ERR_SERVER_ALREADY_LISTEN"))
			panic(err)
		}
		if !state.handleRegistered {
			state.handleRegistered = true
			eventloop.Default.RegisterHandle()
		}
		state.mu.Unlock()
		go func() {
			addr := net.JoinHostPort(host, strconv.Itoa(port))
			listener, listenErr := net.Listen("tcp", addr)
			eventloop.Default.ScheduleCallback(func() {
				state.mu.Lock()
				if state.closed {
					state.mu.Unlock()
					if listener != nil {
						_ = listener.Close()
					}
					return
				}
				if listenErr != nil {
					state.mu.Unlock()
					this.MethodCall("emit", jsvalue.NewString("error"), listenError(listenErr, host, port))
					return
				}
				tcpListener, ok := listener.(*net.TCPListener)
				if !ok {
					_ = listener.Close()
					state.mu.Unlock()
					this.MethodCall("emit", jsvalue.NewString("error"), listenError(fmt.Errorf("invalid listener type"), host, port))
					return
				}
				state.listener = tcpListener
				state.listening = true
				state.mu.Unlock()
				this.Set("listening", jsvalue.NewBool(true))
				go acceptLoop(this, state, tcpListener)
				this.MethodCall("emit", jsvalue.NewString("listening"))
				if cb != nil {
					cb.Call()
				}
			})
		}()
		return this
	}).MarkAsMethod())

	proto.Set("close", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		state := getServerState(this)
		if state == nil {
			return this
		}
		var cb *jsvalue.JSValue
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "function" {
			cb = args[1]
		}
		state.mu.Lock()
		if state.closed {
			state.mu.Unlock()
			return this
		}
		wasListening := state.listening
		listener := state.listener
		state.listener = nil
		state.listening = false
		state.closed = true
		hadHandle := state.handleRegistered
		state.handleRegistered = false
		state.mu.Unlock()
		if listener != nil {
			_ = listener.Close()
		}
		this.Set("listening", jsvalue.NewBool(false))
		serverRegistryMu.Lock()
		delete(serverRegistry, this)
		serverRegistryMu.Unlock()
		if hadHandle {
			eventloop.Default.UnregisterHandle()
		}
		if !wasListening {
			if cb != nil {
				errVal := jserror.Error.Call(jsvalue.NewString("Server is not running."))
				errVal.Set("code", jsvalue.NewString("ERR_SERVER_NOT_RUNNING"))
				cb.Call(errVal)
			}
			return this
		}
		eventloop.Default.ScheduleCallback(func() {
			this.MethodCall("emit", jsvalue.NewString("close"))
			if cb != nil {
				cb.Call()
			}
		})
		return this
	}).MarkAsMethod())

	proto.Set("address", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewNull()
		}
		state := serverStateOf(args[0])
		state.mu.Lock()
		listener := state.listener
		listening := state.listening
		state.mu.Unlock()
		if !listening || listener == nil {
			return jsvalue.NewNull()
		}
		addr, ok := listener.Addr().(*net.TCPAddr)
		if !ok || addr == nil {
			return jsvalue.NewNull()
		}
		return tcpAddrToJS(addr)
	}).MarkAsMethod())
}

func socketReadLoop(socket *jsvalue.JSValue, state *socketState) {
	buf := make([]byte, 64*1024)
	for {
		state.mu.Lock()
		conn := state.conn
		destroyed := state.destroyed
		state.mu.Unlock()
		if conn == nil || destroyed {
			return
		}
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				eventloop.Default.ScheduleCallback(func() {
					state.mu.Lock()
					if state.destroyed {
						state.mu.Unlock()
						return
					}
					allowHalfOpen := state.allowHalfOpen
					state.mu.Unlock()
					socket.MethodCall("emit", jsvalue.NewString("end"))
					if !allowHalfOpen {
						socket.MethodCall("destroy")
					}
				})
				return
			}
			state.mu.Lock()
			destroyed = state.destroyed
			state.mu.Unlock()
			if destroyed {
				return
			}
			errVal := readError(err)
			eventloop.Default.ScheduleCallback(func() {
				socket.MethodCall("destroy", errVal)
			})
			return
		}
		if n == 0 {
			continue
		}
		state.mu.Lock()
		if state.paused {
			state.bytesRead += int64(n)
			state.mu.Unlock()
			continue
		}
		state.bytesRead += int64(n)
		br := state.bytesRead
		encoding := state.encoding
		state.mu.Unlock()
		payload := append([]byte(nil), buf[:n]...)
		eventloop.Default.ScheduleCallback(func() {
			socket.Set("bytesRead", jsvalue.NewNumber(float64(br)))
			if encoding != "" {
				socket.MethodCall("emit", jsvalue.NewString("data"), jsvalue.NewString(string(payload)))
			} else {
				data := buffer.Buffer.Get("from").Call(jsvalue.NewString(string(payload)))
				socket.MethodCall("emit", jsvalue.NewString("data"), data)
			}
		})
	}
}

func acceptLoop(server *jsvalue.JSValue, state *serverState, listener *net.TCPListener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			errVal := listenError(err, "", 0)
			eventloop.Default.ScheduleCallback(func() {
				server.MethodCall("emit", jsvalue.NewString("error"), errVal)
			})
			continue
		}
		tcpConn, ok := conn.(*net.TCPConn)
		if !ok {
			_ = conn.Close()
			continue
		}
		eventloop.Default.RegisterHandle()
		eventloop.Default.ScheduleCallback(func() {
			socket := Socket.Call()
			ss := socketStateOf(socket)
			ahf := server.Get("_allowHalfOpen")
			if ahf != nil && ahf.TypeString() == "boolean" {
				ss.allowHalfOpen = ahf.Bool()
			}
			ss.mu.Lock()
			ss.conn = tcpConn
			ss.handleRegistered = true
			ss.readLoopStarted = true
			ss.mu.Unlock()
			socket.Set("connecting", jsvalue.NewBool(false))
			socket.Set("pending", jsvalue.NewBool(false))
			socket.Set("destroyed", jsvalue.NewBool(false))
			socket.Set("readyState", jsvalue.NewString("open"))
			updateSocketAddresses(socket, tcpConn)
			go socketReadLoop(socket, ss)
			server.MethodCall("emit", jsvalue.NewString("connection"), socket)
		})
	}
}

func updateSocketAddresses(socket *jsvalue.JSValue, conn *net.TCPConn) {
	if localAddr, ok := conn.LocalAddr().(*net.TCPAddr); ok && localAddr != nil {
		socket.Set("localAddress", jsvalue.NewString(localAddr.IP.String()))
		socket.Set("localPort", jsvalue.NewNumber(float64(localAddr.Port)))
		family := "IPv4"
		if localAddr.IP.To4() == nil {
			family = "IPv6"
		}
		socket.Set("localFamily", jsvalue.NewString(family))
	}
	if remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok && remoteAddr != nil {
		socket.Set("remoteAddress", jsvalue.NewString(remoteAddr.IP.String()))
		socket.Set("remotePort", jsvalue.NewNumber(float64(remoteAddr.Port)))
		family := "IPv4"
		if remoteAddr.IP.To4() == nil {
			family = "IPv6"
		}
		socket.Set("remoteFamily", jsvalue.NewString(family))
	}
}

func tcpAddrToJS(addr *net.TCPAddr) *jsvalue.JSValue {
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

func validatePort(port float64) {
	if port < 0 || port > 65535 || math.IsNaN(port) || math.IsInf(port, 0) {
		err := jserror.Error.Call(jserror.Error, jsvalue.NewString(fmt.Sprintf(
			"Port should be >= 0 and < 65536. Received %v.", port)))
		err.Set("code", jsvalue.NewString("ERR_SOCKET_BAD_PORT"))
		panic(err)
	}
}

func coerceWriteData(v *jsvalue.JSValue) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("data must be a string or Buffer")
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
	return nil, fmt.Errorf("data must be a string or Buffer")
}

func connectError(err error, host string, port int) *jsvalue.JSValue {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	code := "ECONNREFUSED"
	msg := fmt.Sprintf("connect ECONNREFUSED %s", addr)
	syscallName := "connect"
	switch {
	case isTimeout(err):
		code = "ETIMEDOUT"
		msg = fmt.Sprintf("connect ETIMEDOUT %s", addr)
	case isDNSNotFound(err):
		code = "ENOTFOUND"
		msg = fmt.Sprintf("getaddrinfo ENOTFOUND %s", host)
		syscallName = "getaddrinfo"
	case errors.Is(err, syscall.ECONNREFUSED):
		code = "ECONNREFUSED"
	}
	errVal := jserror.Error.Call(jsvalue.NewString(msg))
	errVal.Set("code", jsvalue.NewString(code))
	errVal.Set("syscall", jsvalue.NewString(syscallName))
	if code == "ENOTFOUND" {
		errVal.Set("hostname", jsvalue.NewString(host))
	} else {
		errVal.Set("address", jsvalue.NewString(host))
		errVal.Set("port", jsvalue.NewNumber(float64(port)))
	}
	return errVal
}

func listenError(err error, host string, port int) *jsvalue.JSValue {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	code := "EADDRINUSE"
	msg := fmt.Sprintf("listen EADDRINUSE: address already in use %s", addr)
	switch {
	case errors.Is(err, syscall.EADDRINUSE):
		// keep defaults
	case errors.Is(err, syscall.EACCES):
		code = "EACCES"
		msg = fmt.Sprintf("listen EACCES %s", addr)
	default:
		msg = fmt.Sprintf("listen %s: %v", addr, err)
	}
	errVal := jserror.Error.Call(jsvalue.NewString(msg))
	errVal.Set("code", jsvalue.NewString(code))
	errVal.Set("syscall", jsvalue.NewString("listen"))
	if host != "" {
		errVal.Set("address", jsvalue.NewString(host))
	}
	if port > 0 {
		errVal.Set("port", jsvalue.NewNumber(float64(port)))
	}
	return errVal
}

func readError(err error) *jsvalue.JSValue {
	code := "ECONNRESET"
	syscallName := "read"
	switch {
	case errors.Is(err, syscall.ECONNRESET):
		code = "ECONNRESET"
	case errors.Is(err, syscall.EPIPE):
		code = "EPIPE"
		syscallName = "write"
	}
	errVal := jserror.Error.Call(jsvalue.NewString(err.Error()))
	errVal.Set("code", jsvalue.NewString(code))
	errVal.Set("syscall", jsvalue.NewString(syscallName))
	return errVal
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	type timeout interface{ Timeout() bool }
	if te, ok := err.(timeout); ok {
		return te.Timeout()
	}
	return false
}

func isDNSNotFound(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsNotFound || dnsErr.IsTimeout
	}
	return false
}
