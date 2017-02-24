package main

import (
	"os"
	"runtime"
	// flag "github.com/ogier/pflag"
)

const version_number = "1.0"

func versionString() string {
	return `c4 version ` + version_number + ` (` + runtime.GOOS + `)`
}

func main() {
	if err := id_flags.Parse(os.Args[1:]); err == nil {
		id_main(id_flags)
	}
}
