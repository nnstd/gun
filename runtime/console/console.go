package console

import (
	"fmt"
	"os"

	"github.com/nnstd/gun/runtime/jsvalue"
)

// spreadArgs handles the case where a []*jsvalue.JSValue slice is passed as
// a single argument (from transpiled JS spread: console.log(...args)).
// It unpacks the slice so fmt prints each element separately.
func spreadArgs(args []any) []any {
	if len(args) == 1 {
		if slice, ok := args[0].([]*jsvalue.JSValue); ok {
			out := make([]any, len(slice))
			for i, v := range slice {
				out[i] = v
			}
			return out
		}
	}
	return args
}

func Log(args ...any) {
	fmt.Println(spreadArgs(args)...)
}

func Error(args ...any) {
	fmt.Fprintln(os.Stderr, spreadArgs(args)...)
}

func Warn(args ...any) {
	fmt.Fprintln(os.Stderr, spreadArgs(args)...)
}

func Dir(args ...any) {
	for _, a := range args {
		fmt.Printf("%+v\n", a)
	}
}
