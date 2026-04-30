//go:build darwin

package nodetty

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
)

func isatty(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	return err == nil
}

func getTerminalSize(fd int) (int, int) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}

func setRawMode(fd int, mode bool) error {
	termiosMu.Lock()
	defer termiosMu.Unlock()

	if mode {
		old, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
		if err != nil {
			return err
		}
		originalTermios[fd] = termiosState{data: *old}
		raw := *old
		raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
		raw.Oflag &^= unix.OPOST
		raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
		raw.Cflag &^= unix.CSIZE | unix.PARENB
		raw.Cflag |= unix.CS8
		raw.Cc[unix.VMIN] = 1
		raw.Cc[unix.VTIME] = 0
		return unix.IoctlSetTermios(fd, unix.TIOCSETA, &raw)
	}

	if saved, ok := originalTermios[fd]; ok {
		old := saved.data.(unix.Termios)
		delete(originalTermios, fd)
		return unix.IoctlSetTermios(fd, unix.TIOCSETA, &old)
	}
	return nil
}

func startResizeMonitorUnix(stream *jsvalue.JSValue, fd int) {
	eventloop.Default.RegisterHandle()
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		for range sigCh {
			cols, rows := getTerminalSize(fd)
			eventloop.Default.ScheduleCallback(func() {
				stream.Set("columns", jsvalue.NewNumber(float64(cols)))
				stream.Set("rows", jsvalue.NewNumber(float64(rows)))
				stream.MethodCall("emit", jsvalue.NewString("resize"))
			})
		}
	}()
}
