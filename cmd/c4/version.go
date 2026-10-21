package main

import (
	"fmt"
	"os"
	"runtime"
)

func runVersion(_ []string) {
	fmt.Printf("c4 %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
	warnOtherC4s(os.Stderr)
}
