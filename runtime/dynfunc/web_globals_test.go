package dynfunc

import "testing"

func TestInterpreterIncludesFetchGlobal(t *testing.T) {
	interp := newInterpreter()
	if interp.globals["fetch"] == nil {
		t.Fatal("expected fetch in interpreter globals")
	}
}
