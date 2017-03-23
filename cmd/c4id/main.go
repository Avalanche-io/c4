package main

import (
	"fmt"
	"os"
	"runtime"
	// flag "github.com/ogier/pflag"
)

const version_number = "1.0"

func versionString() string {
	return `c4 version ` + version_number + ` (` + runtime.GOOS + `)`
}

func errout(err error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
}

func main() {
	err := id_flags.Parse(os.Args[1:])
	if err != nil {
		errout(err)
		os.Exit(-1)
	}
	err = id_main(id_flags)
	if err != nil {
		errout(err)
		os.Exit(-1)
	}

}
