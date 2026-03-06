package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

// runLn implements "c4 ln" — create links in a c4m file.
//
//	c4 ln project.c4m:master.exr project.c4m:backup.exr     # hard link
//	c4 ln -s ../shared/config.yaml project.c4m:config.yaml   # symlink
func runLn(args []string) {
	// Check for -s flag (symlink)
	var symlink bool
	var filtered []string
	for _, a := range args {
		if a == "-s" {
			symlink = true
		} else {
			filtered = append(filtered, a)
		}
	}

	if symlink {
		lnSymlink(filtered)
	} else {
		lnHard(filtered)
	}
}

// lnSymlink creates a symbolic link entry in a c4m file.
func lnSymlink(args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 ln -s <target-path> <link-location>\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  c4 ln -s ../shared/config.yaml project.c4m:config.yaml\n")
		os.Exit(1)
	}

	target := args[0] // symlink target path (literal string, not a pathspec)

	isLoc := establish.IsLocationEstablished
	link, err := pathspec.Parse(args[1], isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if link.Type != pathspec.Capsule {
		fmt.Fprintf(os.Stderr, "Error: link location must be a c4m path\n")
		os.Exit(1)
	}
	if link.SubPath == "" {
		fmt.Fprintf(os.Stderr, "Error: must specify a path within the c4m\n")
		os.Exit(1)
	}

	if !establish.IsCapsuleEstablished(link.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", link.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", link.Source)
		os.Exit(1)
	}

	manifest, err := loadOrCreateManifest(link.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Parse link path for depth and name
	parts := strings.Split(link.SubPath, "/")
	// Remove trailing empty string from trailing slash
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	depth := len(parts) - 1
	name := parts[len(parts)-1]

	// Ensure parent directories exist
	if depth > 0 {
		parentPath := strings.Join(parts[:len(parts)-1], "/") + "/"
		ensureParentDirs(manifest, parentPath)
	}

	entry := &c4m.Entry{
		Name:      name,
		Depth:     depth,
		Mode:      os.ModeSymlink | 0777,
		Timestamp: c4m.NullTimestamp(),
		Target:    target,
		Size:      -1,
	}

	manifest.AddEntry(entry)
	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(link.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created symlink %s:%s → %s\n", link.Source, link.SubPath, target)
}

// lnHard creates a hard link — a new entry sharing the same C4 ID as an existing entry.
func lnHard(args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 ln <source> <link-name>\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  c4 ln project.c4m:master.exr project.c4m:backup.exr\n")
		os.Exit(1)
	}

	isLoc := establish.IsLocationEstablished
	src, err := pathspec.Parse(args[0], isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	dst, err := pathspec.Parse(args[1], isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if src.Type != pathspec.Capsule || dst.Type != pathspec.Capsule {
		fmt.Fprintf(os.Stderr, "Error: ln currently supports c4m paths only\n")
		os.Exit(1)
	}
	if src.Source != dst.Source {
		fmt.Fprintf(os.Stderr, "Error: ln across different c4m files not yet supported\n")
		os.Exit(1)
	}
	if src.SubPath == "" || dst.SubPath == "" {
		fmt.Fprintf(os.Stderr, "Error: must specify paths within the c4m\n")
		os.Exit(1)
	}

	if !establish.IsCapsuleEstablished(src.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", src.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", src.Source)
		os.Exit(1)
	}

	manifest, err := loadManifest(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Find source entry
	srcEntry := findEntry(manifest, src.SubPath)
	if srcEntry == nil {
		fmt.Fprintf(os.Stderr, "Error: %s not found in %s\n", src.SubPath, src.Source)
		os.Exit(1)
	}
	if srcEntry.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: cannot hard link directories\n")
		os.Exit(1)
	}

	// Parse destination path
	parts := strings.Split(dst.SubPath, "/")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	depth := len(parts) - 1
	name := parts[len(parts)-1]

	// Ensure parent directories exist
	if depth > 0 {
		parentPath := strings.Join(parts[:len(parts)-1], "/") + "/"
		ensureParentDirs(manifest, parentPath)
	}

	// Create new entry with same content identity
	newEntry := &c4m.Entry{
		Name:      name,
		Depth:     depth,
		Mode:      srcEntry.Mode,
		Timestamp: srcEntry.Timestamp,
		Size:      srcEntry.Size,
		C4ID:      srcEntry.C4ID,
	}

	manifest.AddEntry(newEntry)
	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(src.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("linked %s:%s → %s:%s\n", src.Source, src.SubPath, dst.Source, dst.SubPath)
}

// findEntry locates an entry by its full path within a manifest.
func findEntry(manifest *c4m.Manifest, subPath string) *c4m.Entry {
	var dirStack []string
	for _, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + entry.Name
		} else {
			fullPath = entry.Name
		}
		if fullPath == subPath {
			return entry
		}
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}
	return nil
}
