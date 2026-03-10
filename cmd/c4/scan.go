package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Avalanche-io/c4/progscan"
	flag "github.com/spf13/pflag"
)

func runScan(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)

	var (
		outFile string
		level   int
		compact bool
	)

	fs.StringVarP(&outFile, "output", "o", "", "Write c4m to file (default: stdout)")
	fs.IntVar(&level, "level", 2, "Stop after phase: 0=structure, 1=metadata, 2=identity")
	fs.BoolVar(&compact, "compact", false, "Output canonical c4m without padding")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `c4 scan - Progressive filesystem scan

Usage:
  c4 scan [options] <path>

Scans a directory tree and produces a c4m file in three phases:
  Phase 0: Structure discovery (readdir, no stat)
  Phase 1: Metadata resolution (stat for mode, size, timestamps)
  Phase 2: Content identification (C4 hash computation)

The working file is always a valid c4m — null fields indicate
unresolved data. Use --level to stop early for faster results.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  c4 scan ~/projects                    # Full scan to stdout
  c4 scan ~/projects -o project.c4m     # Write to file
  c4 scan ~/projects --level 0          # Structure only (fastest)
  c4 scan ~/projects --level 1          # Structure + metadata
  c4 scan ~/projects --compact          # Canonical c4m output
`)
	}

	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	root := fs.Arg(0)

	// Resolve the root path.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: %v\n", err)
		os.Exit(1)
	}
	fi, err := os.Stat(absRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: %v\n", err)
		os.Exit(1)
	}
	if !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "c4 scan: %s is not a directory\n", root)
		os.Exit(1)
	}

	// Determine output: file or temp file (for stdout mode).
	workFile := outFile
	toStdout := outFile == ""
	if toStdout {
		tmp, err := os.CreateTemp("", "c4scan-*.c4m")
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4 scan: create temp: %v\n", err)
			os.Exit(1)
		}
		workFile = tmp.Name()
		tmp.Close()
		defer os.Remove(workFile)
	}

	sc, err := progscan.New(absRoot, workFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: %v\n", err)
		os.Exit(1)
	}
	defer sc.Close()

	// Phase 0: Structure.
	start := time.Now()
	if err := sc.Phase0(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 0: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Phase 0: %d files, %d dirs (%s)\n",
		sc.Files, sc.Dirs, time.Since(start).Round(time.Millisecond))

	if level < 1 {
		outputResult(sc, workFile, toStdout, compact)
		return
	}

	// Phase 1: Metadata.
	start = time.Now()
	if err := sc.Phase1(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 1: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Phase 1: metadata (%s)\n",
		time.Since(start).Round(time.Millisecond))

	if level < 2 {
		outputResult(sc, workFile, toStdout, compact)
		return
	}

	// Phase 2: Identity.
	start = time.Now()
	if err := sc.Phase2(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 2: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Phase 2: C4 IDs (%s)\n",
		time.Since(start).Round(time.Millisecond))

	outputResult(sc, workFile, toStdout, compact)
}

func outputResult(sc *progscan.Scanner, workFile string, toStdout, compact bool) {
	if toStdout {
		if compact {
			if err := sc.Compact(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "c4 scan: compact: %v\n", err)
				os.Exit(1)
			}
		} else {
			data, err := os.ReadFile(workFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "c4 scan: read: %v\n", err)
				os.Exit(1)
			}
			os.Stdout.Write(data)
		}
		return
	}

	// Writing to file — compact if requested.
	if compact {
		compactPath := workFile + ".compact"
		f, err := os.Create(compactPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4 scan: %v\n", err)
			os.Exit(1)
		}
		if err := sc.Compact(f); err != nil {
			f.Close()
			os.Remove(compactPath)
			fmt.Fprintf(os.Stderr, "c4 scan: compact: %v\n", err)
			os.Exit(1)
		}
		f.Close()
		sc.Close()
		if err := os.Rename(compactPath, workFile); err != nil {
			fmt.Fprintf(os.Stderr, "c4 scan: rename: %v\n", err)
			os.Exit(1)
		}
	}

	fi, _ := os.Stat(workFile)
	fmt.Fprintf(os.Stderr, "Wrote %s (%s)\n", workFile, formatBytes(fi.Size()))
}
