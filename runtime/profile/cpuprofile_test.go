package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultProfileNameMatchesNodeShape(t *testing.T) {
	got := defaultProfileName(time.Date(2026, 4, 22, 9, 46, 13, 0, time.UTC), 30304)
	want := "CPU.20260422.094613.30304.0.001.cpuprofile"
	if got != want {
		t.Fatalf("defaultProfileName() = %q, want %q", got, want)
	}
}

func TestProfilerBuildsJSNativeSamples(t *testing.T) {
	s, err := startCPUProfile("", "test.cpuprofile")
	if err != nil {
		t.Fatalf("startCPUProfile() error = %v", err)
	}
	leaveMain := EnterFrame(Frame{FunctionName: "main.main", File: "/tmp/example/app.ts", Line: 10, Column: 2})
	leaveBusy := EnterFrame(Frame{FunctionName: "main.busy", File: "/tmp/example/app.ts", Line: 12, Column: 7})
	time.Sleep(5 * time.Millisecond)
	leaveBusy()
	leaveMain()
	if err := s.stop(); err != nil {
		t.Fatalf("stop() error = %v", err)
	}

	data, err := os.ReadFile("test.cpuprofile")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	defer os.Remove("test.cpuprofile")

	var profile cpuProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(profile.Samples) == 0 {
		t.Fatal("expected JS-native samples in cpu profile")
	}
	functions := map[string]bool{}
	for _, node := range profile.Nodes {
		functions[node.CallFrame.FunctionName] = true
		if node.CallFrame.FunctionName == "main.busy" {
			if node.CallFrame.LineNumber != 11 {
				t.Fatalf("main.busy lineNumber = %d, want 11", node.CallFrame.LineNumber)
			}
			if node.CallFrame.ColumnNumber != 6 {
				t.Fatalf("main.busy columnNumber = %d, want 6", node.CallFrame.ColumnNumber)
			}
		}
	}
	for _, want := range []string{"main.main", "main.busy"} {
		if !functions[want] {
			t.Fatalf("missing function %q in profile nodes", want)
		}
	}
}

func TestCaptureAndRestoreAsyncContext(t *testing.T) {
	leaveMain := EnterFrame(Frame{FunctionName: "main.main", File: "/tmp/example/app.ts", Line: 5, Column: 1})
	token := CaptureContext()
	leaveMain()
	if token == nil || len(token.frames) != 1 {
		t.Fatalf("expected one-frame context token, got %#v", token)
	}

	called := false
	WithContext(token, func() {
		called = true
		leaveChild := EnterFrame(Frame{FunctionName: "timerCb", File: "/tmp/example/app.ts", Line: 20, Column: 3})
		defer leaveChild()
		ctx := CaptureContext()
		if ctx == nil || len(ctx.frames) != 2 {
			t.Fatalf("expected restored parent + child, got %#v", ctx)
		}
		if ctx.frames[0].FunctionName != "main.main" || ctx.frames[1].FunctionName != "timerCb" {
			t.Fatalf("unexpected restored frames: %#v", ctx.frames)
		}
	})
	if !called {
		t.Fatal("expected WithContext callback to run")
	}
}

func TestResolveTargetPathUsesWorkingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	got, err := resolveTargetPath("", "custom.cpuprofile", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tempDir, "custom.cpuprofile")
	if got != want {
		t.Fatalf("resolveTargetPath() = %q, want %q", got, want)
	}
}
