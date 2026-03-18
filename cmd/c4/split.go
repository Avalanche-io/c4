package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"github.com/Avalanche-io/c4/c4m"
)

func runSplit(args []string) {
	if len(args) != 4 {
		fmt.Fprintf(os.Stderr, "Usage: c4 split <file.c4m> <N> <before.c4m> <after.c4m>\n")
		fmt.Fprintf(os.Stderr, "\nSplit a patch chain at patch N.\n")
		fmt.Fprintf(os.Stderr, "  before.c4m gets base + patches 1..N\n")
		fmt.Fprintf(os.Stderr, "  after.c4m gets patches N+1..end\n")
		os.Exit(1)
	}

	inputPath := args[0]
	n, err := strconv.Atoi(args[1])
	if err != nil || n < 1 {
		fatalf("Error: N must be a positive integer, got %q", args[1])
	}
	beforePath := args[2]
	afterPath := args[3]

	data, err := os.ReadFile(inputPath)
	if err != nil {
		fatalf("Error reading %s: %v", inputPath, err)
	}

	sections, err := c4m.DecodePatchChain(bytes.NewReader(data))
	if err != nil {
		fatalf("Error decoding %s: %v", inputPath, err)
	}

	if n > len(sections) {
		fatalf("Error: only %d sections in chain, cannot split at %d", len(sections), n)
	}

	// Write before: resolve sections 1..N into a single manifest.
	beforeManifest := c4m.ResolvePatchChain(sections[:n], 0)
	writeManifestFile(beforePath, beforeManifest)

	// Write after: the remaining sections as raw patches.
	if n < len(sections) {
		writeRawSections(afterPath, sections[n:])
	} else {
		// Nothing after — write an empty file? Or skip?
		fmt.Fprintf(os.Stderr, "Note: no patches after position %d; %s not created\n", n, afterPath)
	}
}

func writeManifestFile(path string, m *c4m.Manifest) {
	f, err := os.Create(path)
	if err != nil {
		fatalf("Error creating %s: %v", path, err)
	}
	defer f.Close()

	enc := c4m.NewEncoder(f)
	if err := enc.Encode(m); err != nil {
		fatalf("Error writing %s: %v", path, err)
	}
}

func writeRawSections(path string, sections []*c4m.PatchSection) {
	f, err := os.Create(path)
	if err != nil {
		fatalf("Error creating %s: %v", path, err)
	}
	defer f.Close()

	enc := c4m.NewEncoder(f)
	for _, sec := range sections {
		// Write the base ID line if present.
		if !sec.BaseID.IsNil() {
			fmt.Fprintln(f, sec.BaseID)
		}
		// Write entries.
		m := &c4m.Manifest{Version: "1.0", Entries: sec.Entries}
		enc.Encode(m)
	}
}
