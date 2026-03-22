package main

import (
	"fmt"
	"os"

	"github.com/Avalanche-io/c4"
)

const version = "1.0.6"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "id":
			runID(os.Args[2:])
			return
		case "cat":
			runCat(os.Args[2:])
			return
		case "diff":
			runDiff(os.Args[2:])
			return
		case "patch":
			runPatch(os.Args[2:])
			return
		case "merge":
			runMerge(os.Args[2:])
			return
		case "log":
			runLog(os.Args[2:])
			return
		case "split":
			runSplit(os.Args[2:])
			return
		case "version":
			runVersion(os.Args[2:])
			return
		}
	}

	// Bare c4 with piped stdin → output C4 ID
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		id := c4.Identify(os.Stdin)
		fmt.Println(id)
		return
	}

	usage()
	os.Exit(1)
}

func usage() {
	fmt.Fprintf(os.Stderr, `c4 - Content-addressable identification using C4 IDs (SMPTE ST 2114)

Usage:
  c4 id [flags] <path>...         Identify files, directories, or c4m files
  c4 cat <c4id>                   Retrieve content by C4 ID from store
  c4 diff <old> <new>             Produce c4m diff (patch)
  c4 patch <target> [<dest>]     Apply target state (resolve diffs or reconcile)
  c4 merge <path>...              Combine filesystem trees (c4m or directories)
  c4 log <file.c4m>...            List patches in a chain
  c4 split <file.c4m> <N> <before.c4m> <after.c4m>
                                  Split chain at patch N
  c4 version                      Print version

  echo "data" | c4               C4 ID from stdin (shortcut)

Version: %s
`, version)
}
