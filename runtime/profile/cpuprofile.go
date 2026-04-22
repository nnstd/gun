package profile

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const DefaultInterval = time.Millisecond

type Frame struct {
	FunctionName string
	File         string
	Line         int
	Column       int
}

type ContextToken struct {
	frames []Frame
}

type session struct {
	startedAt time.Time
	target    string
	interval  time.Duration
	stopCh    chan struct{}
	doneCh    chan struct{}
	stopOnce  sync.Once
}

type cpuProfile struct {
	Nodes      []*profileNode `json:"nodes"`
	StartTime  int64          `json:"startTime"`
	EndTime    int64          `json:"endTime"`
	Samples    []int          `json:"samples"`
	TimeDeltas []int64        `json:"timeDeltas"`
}

type profileNode struct {
	ID        int       `json:"id"`
	CallFrame callFrame `json:"callFrame"`
	HitCount  int       `json:"hitCount"`
	Children  []int     `json:"children,omitempty"`
}

type callFrame struct {
	FunctionName string `json:"functionName"`
	ScriptID     string `json:"scriptId"`
	URL          string `json:"url"`
	LineNumber   int    `json:"lineNumber"`
	ColumnNumber int    `json:"columnNumber"`
}

type goroutineContext struct {
	prefix []Frame
	stack  []Frame
}

type nodeKey struct {
	parent int
	frame  Frame
}

var state struct {
	mu       sync.Mutex
	session  *session
	contexts map[uint64]*goroutineContext
	profile  *cpuProfile
	nextNode int
	nodeFor  map[nodeKey]int
	nodeIdx  map[int]*profileNode
	scriptID map[string]string
	nextSID  int
}

func init() {
	state.contexts = make(map[uint64]*goroutineContext)
}

// StartCPUProfileOrExit starts JS-native statistical profiling and returns a stop function.
func StartCPUProfileOrExit(dir, name string) func() {
	s, err := startCPUProfile(dir, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: start cpu profile: %v\n", err)
		os.Exit(1)
	}
	return func() {
		if err := s.stop(); err != nil {
			fmt.Fprintf(os.Stderr, "error: finalize cpu profile: %v\n", err)
		}
	}
}

func startCPUProfile(dir, name string) (*session, error) {
	startedAt := time.Now()
	target, err := resolveTargetPath(dir, name, startedAt)
	if err != nil {
		return nil, err
	}
	s := &session{startedAt: startedAt, target: target, interval: DefaultInterval, stopCh: make(chan struct{}), doneCh: make(chan struct{})}

	state.mu.Lock()
	state.session = s
	state.profile = &cpuProfile{Nodes: []*profileNode{{
		ID:        1,
		CallFrame: callFrame{FunctionName: "(root)", ScriptID: "0", URL: "", LineNumber: -1, ColumnNumber: -1},
	}}, StartTime: startedAt.UnixMicro()}
	state.nextNode = 2
	state.nodeFor = map[nodeKey]int{}
	state.nodeIdx = map[int]*profileNode{1: state.profile.Nodes[0]}
	state.scriptID = map[string]string{"": "0"}
	state.nextSID = 1
	state.mu.Unlock()

	go s.runSampler()
	go s.handleSignals()
	return s, nil
}

func (s *session) handleSignals() {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	select {
	case sig := <-sigCh:
		_ = s.stop()
		if ps, ok := sig.(syscall.Signal); ok {
			os.Exit(128 + int(ps))
		}
		os.Exit(1)
	case <-s.doneCh:
		return
	}
}

func (s *session) runSampler() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sampleActiveStacks(s.interval)
		case <-s.stopCh:
			return
		}
	}
}

func (s *session) stop() error {
	var stopErr error
	s.stopOnce.Do(func() {
		close(s.stopCh)
		<-s.doneCh

		state.mu.Lock()
		profile := state.profile
		state.profile.EndTime = time.Now().UnixMicro()
		state.session = nil
		state.profile = nil
		state.nodeFor = nil
		state.nodeIdx = nil
		state.scriptID = nil
		state.mu.Unlock()

		data, err := json.Marshal(profile)
		if err != nil {
			stopErr = err
			return
		}
		stopErr = os.WriteFile(s.target, data, 0o644)
	})
	return stopErr
}

func resolveTargetPath(dir, name string, startedAt time.Time) (string, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = wd
	}
	if name == "" {
		name = defaultProfileName(startedAt, os.Getpid())
	}
	return filepath.Join(dir, name), nil
}

func defaultProfileName(startedAt time.Time, pid int) string {
	return fmt.Sprintf("CPU.%04d%02d%02d.%02d%02d%02d.%d.0.001.cpuprofile",
		startedAt.Year(), startedAt.Month(), startedAt.Day(), startedAt.Hour(), startedAt.Minute(), startedAt.Second(), pid)
}

func EnterFrame(frame Frame) func() {
	gid := currentGoroutineID()
	state.mu.Lock()
	ctx := state.contexts[gid]
	if ctx == nil {
		ctx = &goroutineContext{}
		state.contexts[gid] = ctx
	}
	ctx.stack = append(ctx.stack, normalizeFrame(frame))
	state.mu.Unlock()
	return func() {
		state.mu.Lock()
		ctx := state.contexts[gid]
		if ctx != nil && len(ctx.stack) > 0 {
			ctx.stack = ctx.stack[:len(ctx.stack)-1]
			if len(ctx.stack) == 0 && len(ctx.prefix) == 0 {
				delete(state.contexts, gid)
			}
		}
		state.mu.Unlock()
	}
}

func CaptureContext() *ContextToken {
	gid := currentGoroutineID()
	state.mu.Lock()
	defer state.mu.Unlock()
	ctx := state.contexts[gid]
	if ctx == nil {
		return nil
	}
	frames := make([]Frame, 0, len(ctx.prefix)+len(ctx.stack))
	frames = append(frames, ctx.prefix...)
	frames = append(frames, ctx.stack...)
	return &ContextToken{frames: frames}
}

func WithContext(token *ContextToken, fn func()) {
	if token == nil || len(token.frames) == 0 {
		fn()
		return
	}
	gid := currentGoroutineID()
	state.mu.Lock()
	ctx := state.contexts[gid]
	if ctx == nil {
		ctx = &goroutineContext{}
		state.contexts[gid] = ctx
	}
	prev := append([]Frame(nil), ctx.prefix...)
	ctx.prefix = append([]Frame(nil), token.frames...)
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		ctx := state.contexts[gid]
		if ctx != nil {
			ctx.prefix = prev
			if len(ctx.stack) == 0 && len(ctx.prefix) == 0 {
				delete(state.contexts, gid)
			}
		}
		state.mu.Unlock()
	}()
	fn()
}

func sampleActiveStacks(interval time.Duration) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.session == nil || state.profile == nil {
		return
	}
	var stacks [][]Frame
	for _, ctx := range state.contexts {
		frames := make([]Frame, 0, len(ctx.prefix)+len(ctx.stack))
		frames = append(frames, ctx.prefix...)
		frames = append(frames, ctx.stack...)
		if len(frames) == 0 {
			continue
		}
		stacks = append(stacks, frames)
	}
	if len(stacks) == 0 {
		return
	}
	deltaUS := int64(interval / time.Microsecond)
	if deltaUS <= 0 {
		deltaUS = 1000
	}
	perSample := max64(deltaUS/int64(len(stacks)), 1)
	for _, frames := range stacks {
		leaf := internPathLocked(frames)
		if node := state.nodeIdx[leaf]; node != nil {
			node.HitCount++
		}
		state.profile.Samples = append(state.profile.Samples, leaf)
		state.profile.TimeDeltas = append(state.profile.TimeDeltas, perSample)
	}
}

func internPathLocked(frames []Frame) int {
	parentID := 1
	leafID := 1
	for _, frame := range frames {
		frame = normalizeFrame(frame)
		key := nodeKey{parent: parentID, frame: frame}
		nodeID, ok := state.nodeFor[key]
		if !ok {
			sid := state.scriptID[frame.File]
			if sid == "" {
				sid = strconv.Itoa(state.nextSID)
				state.nextSID++
				state.scriptID[frame.File] = sid
			}
			nodeID = state.nextNode
			state.nextNode++
			node := &profileNode{ID: nodeID, CallFrame: callFrame{
				FunctionName: frame.FunctionName,
				ScriptID:     sid,
				URL:          fileURL(frame.File),
				LineNumber:   max(frame.Line-1, 0),
				ColumnNumber: max(frame.Column-1, 0),
			}}
			state.profile.Nodes = append(state.profile.Nodes, node)
			state.nodeIdx[nodeID] = node
			state.nodeIdx[parentID].Children = append(state.nodeIdx[parentID].Children, nodeID)
			state.nodeFor[key] = nodeID
		}
		parentID = nodeID
		leafID = nodeID
	}
	return leafID
}

func normalizeFrame(frame Frame) Frame {
	frame.FunctionName = simplifyFunctionName(frame.FunctionName)
	if frame.FunctionName == "" {
		frame.FunctionName = "(anonymous)"
	}
	return frame
}

func simplifyFunctionName(fn string) string {
	if fn == "" {
		return ""
	}
	if idx := strings.LastIndex(fn, "/"); idx >= 0 {
		fn = fn[idx+1:]
	}
	if strings.HasPrefix(fn, "main.main.func") || strings.Contains(fn, ".func") {
		return ""
	}
	if idx := strings.LastIndex(fn, "."); idx >= 0 {
		prefix := fn[:idx]
		suffix := fn[idx+1:]
		if strings.Contains(prefix, ".") {
			if dot := strings.LastIndex(prefix, "."); dot >= 0 {
				prefix = prefix[dot+1:]
			}
		}
		if prefix != "" {
			return prefix + "." + suffix
		}
		return suffix
	}
	return fn
}

func fileURL(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "node:") || strings.Contains(path, "://") {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

func currentGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := string(buf[:n])
	line = strings.TrimPrefix(line, "goroutine ")
	end := strings.IndexByte(line, ' ')
	if end < 0 {
		return 0
	}
	id, _ := strconv.ParseUint(line[:end], 10, 64)
	return id
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
