package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const version = "1.0.16"

func main() {
	// A dying stdout (EPIPE) must never kill the process mid-claim:
	// with SIGPIPE ignored, writes return errors the verbs handle —
	// under -s the snapshot completes and journals; readers exit 1.
	signal.Ignore(syscall.SIGPIPE)
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			fmt.Printf("%s\nVersion: %s\n", topHelp, version)
			return
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
		case "restore":
			runRestore(os.Args[2:])
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

	// Check for -s (snapshot) in bare args. Nothing is written anywhere
	// without -s: the bare forms are read-only.
	snapshot := false
	var bareArgs []string
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			if arg == "-s" || arg == "--store" {
				snapshot = true
			} else {
				bareArgs = append(bareArgs, arg)
			}
		}
	}

	// If a non-flag arg looks like a path, treat as c4 id [-s].
	if len(bareArgs) > 0 {
		for _, arg := range bareArgs {
			if _, err := os.Stat(arg); err == nil {
				if snapshot {
					runID(append([]string{"-s"}, bareArgs...))
				} else {
					runID(bareArgs)
				}
				return
			}
		}
	}

	// Piped stdin: ...|c4 prints the ID of stdin (nothing stored);
	// ...|c4 -s stores stdin and prints its ID.
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		doStdin(snapshot)
		return
	}

	usage()
	os.Exit(1)
}

func usage() {
	fmt.Fprintf(os.Stderr, "%s\nVersion: %s\n", topHelp, version)
}
