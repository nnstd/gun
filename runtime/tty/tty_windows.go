//go:build windows

package nodetty

import (
	"golang.org/x/sys/windows"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func isatty(fd int) bool {
	ft, _ := windows.GetFileType(windows.Handle(fd))
	return ft == windows.FILE_TYPE_CHAR
}

func getTerminalSize(fd int) (int, int) {
	var info windows.ConsoleScreenBufferInfo
	err := windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info)
	if err != nil {
		return 80, 24
	}
	cols := int(info.Window.Right - info.Window.Left + 1)
	rows := int(info.Window.Bottom - info.Window.Top + 1)
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return cols, rows
}

func setRawMode(fd int, mode bool) error {
	termiosMu.Lock()
	defer termiosMu.Unlock()

	var consoleMode uint32
	err := windows.GetConsoleMode(windows.Handle(fd), &consoleMode)
	if err != nil {
		return err
	}

	if mode {
		originalTermios[fd] = termiosState{data: consoleMode}
		rawMode := consoleMode &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT)
		return windows.SetConsoleMode(windows.Handle(fd), rawMode)
	}

	if saved, ok := originalTermios[fd]; ok {
		old := saved.data.(uint32)
		delete(originalTermios, fd)
		return windows.SetConsoleMode(windows.Handle(fd), old)
	}
	return nil
}

func startResizeMonitorUnix(stream *jsvalue.JSValue, fd int) {
	// No SIGWINCH on Windows
}
