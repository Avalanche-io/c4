package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
)

// runMkdir implements "c4 mkdir" — create a directory entry in a capsule.
//
//	c4 mkdir project.c4m:renders/
//	c4 mkdir project.c4m:renders/shots/
func runMkdir(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 mkdir <capsule>.c4m:<path>/\n")
		os.Exit(1)
	}

	spec, err := pathspec.Parse(args[0], establish.IsLocationEstablished)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if spec.Type != pathspec.Capsule {
		fmt.Fprintf(os.Stderr, "Error: mkdir requires a capsule path (e.g. project.c4m:renders/)\n")
		os.Exit(1)
	}

	if spec.SubPath == "" {
		fmt.Fprintf(os.Stderr, "Error: must specify a directory path within the capsule\n")
		os.Exit(1)
	}

	// Ensure trailing slash
	dirPath := spec.SubPath
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	// Check establishment
	if !establish.IsCapsuleEstablished(spec.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s is not established for writing\n", spec.Source+":")
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", spec.Source)
		os.Exit(1)
	}

	// Load or create manifest
	manifest, err := loadOrCreateManifest(spec.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check if directory already exists
	for _, e := range manifest.Entries {
		if e.Name == dirPath {
			fmt.Fprintf(os.Stderr, "%s already exists in %s\n", dirPath, spec.Source)
			os.Exit(0)
		}
	}

	// Add directory entry
	manifest.AddEntry(&c4m.Entry{
		Name: dirPath,
		Mode: os.ModeDir | 0755,
		Size: -1,
	})

	manifest.SortEntries()

	// Write manifest
	if err := writeManifest(spec.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created %s:%s\n", spec.Source, dirPath)
}

// loadOrCreateManifest loads a c4m file, or creates a new empty manifest.
func loadOrCreateManifest(path string) (*c4m.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c4m.NewManifest(), nil
		}
		return nil, err
	}
	defer f.Close()
	return c4m.NewDecoder(f).Decode()
}

// writeManifest writes a manifest to a c4m file.
func writeManifest(path string, m *c4m.Manifest) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return c4m.NewEncoder(f).Encode(m)
}
