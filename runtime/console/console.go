package console

import (
	"fmt"
	"os"
)

func Log(args ...any) {
	fmt.Println(args...)
}

func Error(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
}

func Warn(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
}

func Dir(args ...any) {
	for _, a := range args {
		fmt.Printf("%+v\n", a)
	}
}
