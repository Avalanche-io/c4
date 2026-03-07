package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

// runMv implements "c4 mv" — move/rename entries within a c4m file.
//
//	c4 mv project.c4m:old.txt project.c4m:new.txt
//	c4 mv project.c4m:renders/draft/ project.c4m:renders/final/
func runMv(args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 mv <source> <dest>\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  c4 mv project.c4m:old.txt project.c4m:new.txt\n")
		fmt.Fprintf(os.Stderr, "  c4 mv project.c4m:renders/draft/ project.c4m:renders/final/\n")
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

	if src.Type != pathspec.C4m || dst.Type != pathspec.C4m {
		fmt.Fprintf(os.Stderr, "Error: mv currently supports c4m paths only\n")
		os.Exit(1)
	}
	if src.Source != dst.Source {
		fmt.Fprintf(os.Stderr, "Error: mv across different c4m files not yet supported\n")
		os.Exit(1)
	}
	if src.SubPath == "" || dst.SubPath == "" {
		fmt.Fprintf(os.Stderr, "Error: must specify paths within the c4m\n")
		os.Exit(1)
	}

	if !establish.IsC4mEstablished(src.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", src.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", src.Source)
		os.Exit(1)
	}

	unlock, err := lockC4mFile(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", src.Source, err)
		os.Exit(1)
	}
	defer unlock()

	manifest, err := loadManifest(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !mvEntries(manifest, src.SubPath, dst.SubPath) {
		fmt.Fprintf(os.Stderr, "Error: %s not found in %s\n", src.SubPath, src.Source)
		os.Exit(1)
	}

	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(src.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("moved %s:%s → %s:%s\n", src.Source, src.SubPath, dst.Source, dst.SubPath)
}

// mvEntries renames/moves an entry (and children if directory) within a manifest.
func mvEntries(manifest *scan.Manifest, srcPath, dstPath string) bool {
	// Resolve full paths from depth-based hierarchy
	type resolved struct {
		fullPath string
		index    int
	}
	var all []resolved
	var dirStack []string

	for i, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		var fp string
		if len(dirStack) > 0 {
			fp = strings.Join(dirStack, "") + entry.Name
		} else {
			fp = entry.Name
		}
		all = append(all, resolved{fp, i})
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}

	// Find source entry
	srcIdx := -1
	for _, r := range all {
		if r.fullPath == srcPath {
			srcIdx = r.index
			break
		}
	}
	if srcIdx == -1 {
		return false
	}

	entry := manifest.Entries[srcIdx]
	oldDepth := entry.Depth

	// Parse destination path into name and depth
	cleanDst := strings.TrimSuffix(dstPath, "/")
	parts := strings.Split(cleanDst, "/")
	newDepth := len(parts) - 1
	newName := parts[len(parts)-1]
	if entry.IsDir() {
		newName += "/"
	}

	depthDelta := newDepth - oldDepth

	// Update entry
	entry.Name = newName
	entry.Depth = newDepth

	// If directory, update all children's depth
	if entry.IsDir() {
		for j := srcIdx + 1; j < len(manifest.Entries); j++ {
			child := manifest.Entries[j]
			if child.Depth <= oldDepth {
				break
			}
			child.Depth += depthDelta
		}
	}

	// Ensure parent directories exist for the destination
	if newDepth > 0 {
		parentPath := strings.Join(parts[:len(parts)-1], "/") + "/"
		ensureParentDirs(manifest, parentPath)
	}

	manifest.InvalidateIndex()
	return true
}
