package main

import (
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/c4m"
	flag "github.com/spf13/pflag"
)

func runCompact(args []string) {
	fs := flag.NewFlagSet("compact", flag.ExitOnError)
	outFile := fs.StringP("output", "o", "", "Write compacted c4m to file (default: stdout)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `c4 compact - Remove padding from a c4m file

Usage:
  c4 compact [options] <file.c4m>

Reads a padded c4m file (e.g. from c4 scan) and writes a canonical
c4m without fixed-width padding. The result is smaller and suitable
for storage or transfer.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  c4 compact scan.c4m                   # Compact to stdout
  c4 compact scan.c4m -o clean.c4m      # Write to file
  c4 compact scan.c4m -o scan.c4m       # Compact in place
`)
	}

	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	inPath := fs.Arg(0)

	f, err := os.Open(inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 compact: %v\n", err)
		os.Exit(1)
	}

	m, err := c4m.NewDecoder(f).Decode()
	f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 compact: decode: %v\n", err)
		os.Exit(1)
	}

	if *outFile == "" {
		enc := c4m.NewEncoder(os.Stdout)
		if err := enc.Encode(m); err != nil {
			fmt.Fprintf(os.Stderr, "c4 compact: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Write to temp file then rename (safe for in-place compaction).
	tmpPath := *outFile + ".compact.tmp"
	tmp, err := os.Create(tmpPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 compact: %v\n", err)
		os.Exit(1)
	}

	enc := c4m.NewEncoder(tmp)
	if err := enc.Encode(m); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "c4 compact: %v\n", err)
		os.Exit(1)
	}
	tmp.Close()

	if err := os.Rename(tmpPath, *outFile); err != nil {
		os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "c4 compact: rename: %v\n", err)
		os.Exit(1)
	}

	fi, _ := os.Stat(*outFile)
	fmt.Fprintf(os.Stderr, "Wrote %s (%s)\n", *outFile, formatBytes(fi.Size()))
}
