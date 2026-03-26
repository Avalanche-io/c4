package main

import (
	"fmt"
	"os"
)

const version = "1.0.9"

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
		case "paths":
			runPaths(os.Args[2:])
			return
		case "intersect":
			runIntersect(os.Args[2:])
			return
		case "explain":
			runExplain(os.Args[2:])
			return
		case "version":
			runVersion(os.Args[2:])
			return
		}
	}

	// Check for -x flag (exclude from store) in bare args.
	excludeStore := false
	var bareArgs []string
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			if arg == "-x" || arg == "--exclude-store" {
				excludeStore = true
			} else {
				bareArgs = append(bareArgs, arg)
			}
		}
	}

	// If a non-flag arg looks like a path, treat as c4 id [-s].
	// The bare form stores content by default; -x skips storage.
	if len(bareArgs) > 0 {
		for _, arg := range bareArgs {
			if _, err := os.Stat(arg); err == nil {
				if excludeStore {
					runID(bareArgs)
				} else {
					runID(append([]string{"-s"}, bareArgs...))
				}
				return
			}
		}
	}

	// Bare c4 with piped stdin → identify + store (unless -x).
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		doStdin(!excludeStore)
		return
	}

	usage()
	os.Exit(1)
}

func usage() {
	fmt.Fprintf(os.Stderr, `c4 - Content-addressable identification using C4 IDs (SMPTE ST 2114)

Usage:
  c4 id [flags] <path>...         Identify files, directories, or c4m files
  c4 cat [-e] [-r] <c4id|path>    Retrieve/display content (c4m-aware)
  c4 diff <old> <new>             Produce c4m diff (patch)
  c4 patch <target> [<dest>]     Apply target state (resolve diffs or reconcile)
  c4 merge <path>...              Combine filesystem trees (c4m or directories)
  c4 paths [<file.c4m> | -]       Convert between c4m and path lists
  c4 intersect <id|path> <a> <b> Find common entries between c4m files
  c4 log <file.c4m>...            List patches in a chain
  c4 explain <command> [args]       Human-readable command narration
  c4 split <file.c4m> <N> <before.c4m> <after.c4m>
                                  Split chain at patch N
  c4 version                      Print version

  c4 <path>                      Identify + store (shortcut for c4 id -s)
  c4 <path> -x                   Identify only, skip store
  echo "data" | c4               Identify + store from stdin
  echo "data" | c4 -x            Identify only from stdin

Version: %s
`, version)
}
