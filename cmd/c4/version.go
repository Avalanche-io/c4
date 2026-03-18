package main

import (
	"fmt"
	"runtime"
)

func runVersion(_ []string) {
	fmt.Printf("c4 %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
