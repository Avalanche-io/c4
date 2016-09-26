package main

import (
	"fmt"
	"os"
	"runtime"

	flag "github.com/ogier/pflag"
)

const version_number = "0.7.1"

func versionString() string {
	return `c4 version ` + version_number + ` (` + runtime.GOOS + `)`
}

func main() {
	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(0)
	}
	switch os.Args[1] {
	case "-h", "--help":
		flag.Usage()
		os.Exit(0)
	case "-version":
		fmt.Fprintf(os.Stderr, versionString())
		os.Exit(0)
	case "id":
		if err := id_flags.Parse(os.Args[2:]); err == nil {
			id_main(id_flags)
		} else {
			fmt.Fprintf(os.Stderr, "id_flags %s\n", err)
		}
	case "cp":
		if err := cp_flags.Parse(os.Args[2:]); err == nil {
			cp_main(cp_flags)
		} else {
			fmt.Fprintf(os.Stderr, "cp_flags %s\n", err)
		}
	default:
		flag.Usage()
		os.Exit(0)
	}
}
