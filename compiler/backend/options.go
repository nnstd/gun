package backend

// CPUProfileConfig controls optional generated-main CPU profiling support.
// Empty Dir/Name values mean "use runtime defaults".
type CPUProfileConfig struct {
	Dir  string
	Name string
}
