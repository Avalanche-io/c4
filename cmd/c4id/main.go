package main

import (
	"fmt"
	"os"
	"runtime"
	// flag "github.com/ogier/pflag"
)

const VERSION = "1.0"

func versionString() string {
	return fmt.Sprintf("c4id - %s Version %s", runtime.GOOS, VERSION)
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
