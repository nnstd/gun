package dynfunc

import "testing"

func TestInterpreterIncludesFetchGlobal(t *testing.T) {
	interp := newInterpreter(nil)
	if interp.globals["fetch"] == nil {
		t.Fatal("expected fetch in interpreter globals")
	}
}
