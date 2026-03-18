package main

import (
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

func runMerge(args []string) {
	fs := newFlags("merge")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Output ergonomic form")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode for directories: s/m/f")
	fs.parse(args)

	if len(fs.args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 merge [-e] [-m mode] <path>...\n")
		fmt.Fprintf(os.Stderr, "\nCombine two or more filesystem trees into one c4m.\n")
		fmt.Fprintf(os.Stderr, "Each path can be a c4m file or a real directory.\n")
		os.Exit(1)
	}

	mode, err := scan.ParseScanMode(*modeFlag)
	if err != nil {
		fatalf("Error: %v", err)
	}

	// Load all inputs as manifests.
	var manifests []*c4m.Manifest
	for _, path := range fs.args {
		m := resolveManifestOrDir(path, mode)
		manifests = append(manifests, m)
	}

	// Merge all manifests. Use three-way merge with nil base (union).
	result := manifests[0]
	for i := 1; i < len(manifests); i++ {
		merged, conflicts, err := c4m.Merge(nil, result, manifests[i])
		if err != nil {
			fatalf("Error merging: %v", err)
		}
		if len(conflicts) > 0 {
			for _, c := range conflicts {
				fmt.Fprintf(os.Stderr, "conflict: %s\n", c.Path)
			}
			fatalf("Error: %d conflicts", len(conflicts))
		}
		result = merged
	}

	outputManifest(result, *ergonomic)
}

// resolveManifestOrDir loads a c4m file, reads stdin, or scans a directory.
func resolveManifestOrDir(path string, mode scan.ScanMode) *c4m.Manifest {
	if path == "-" {
		m, err := c4m.NewDecoder(os.Stdin).Decode()
		if err != nil {
			fatalf("Error reading stdin: %v", err)
		}
		return m
	}

	info, err := os.Stat(path)
	if err != nil {
		fatalf("Error: %v", err)
	}

	if info.IsDir() {
		gen := scan.NewGeneratorWithOptions(scan.WithMode(mode))
		m, err := gen.GenerateFromPath(path)
		if err != nil {
			fatalf("Error scanning %s: %v", path, err)
		}
		return m
	}

	m, err := loadManifest(path)
	if err != nil {
		fatalf("Error loading %s: %v", path, err)
	}
	return m
}
