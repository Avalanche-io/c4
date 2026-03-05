package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

// c4dClient has proper timeouts: 10s to connect, 30s for response headers,
// but unlimited time for body transfer (correct for large files).
var c4dClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

// runCp implements "c4 cp" — the universal copy verb.
//
//	c4 cp source/ project.c4m:          # capture into capsule
//	c4 cp source/ project.c4m:renders/  # capture into subtree
//	c4 cp project.c4m: ./output/        # materialize from capsule
//	c4 cp project.c4m:renders/ ./out/   # materialize subtree
func runCp(args []string) {
	// Strip -r/--recursive flag (scan is always recursive)
	var filtered []string
	for _, a := range args {
		if a != "-r" && a != "--recursive" {
			filtered = append(filtered, a)
		}
	}
	args = filtered

	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 cp [-r] <source> <dest>\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  c4 cp files/ project.c4m:           # capture into capsule\n")
		fmt.Fprintf(os.Stderr, "  c4 cp files/ project.c4m:renders/   # capture into subtree\n")
		fmt.Fprintf(os.Stderr, "  c4 cp project.c4m: ./output/        # materialize from capsule\n")
		fmt.Fprintf(os.Stderr, "  c4 cp project.c4m:renders/ ./out/   # materialize subtree\n")
		os.Exit(1)
	}

	isLoc := establish.IsLocationEstablished
	src, err := pathspec.Parse(args[0], isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing source: %v\n", err)
		os.Exit(1)
	}
	dst, err := pathspec.Parse(args[1], isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing dest: %v\n", err)
		os.Exit(1)
	}

	switch {
	case src.Type == pathspec.Local && dst.Type == pathspec.Capsule:
		cpLocalToCapsule(src, dst)
	case src.Type == pathspec.Capsule && dst.Type == pathspec.Local:
		cpCapsuleToLocal(src, dst)
	case src.Type == pathspec.Local && dst.Type == pathspec.Local:
		fmt.Fprintf(os.Stderr, "Error: use OS cp for local-to-local copies\n")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "Error: %s → %s not yet supported\n", src.Type, dst.Type)
		os.Exit(1)
	}
}

// cpLocalToCapsule captures local files into a capsule.
func cpLocalToCapsule(src, dst pathspec.PathSpec) {
	// Check establishment
	if !establish.IsCapsuleEstablished(dst.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", dst.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", dst.Source)
		os.Exit(1)
	}

	// Walk source and build manifest entries
	info, err := os.Stat(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !info.IsDir() {
		// Single file capture
		cpFileIntoCapsule(src.Source, dst)
		return
	}

	// Directory capture — use the scan generator
	gen := scan.NewGeneratorWithOptions(scan.WithC4IDs(true))
	scanned, err := gen.GenerateFromPath(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", src.Source, err)
		os.Exit(1)
	}

	// Load existing manifest or create new
	manifest, err := loadOrCreateManifest(dst.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", dst.Source, err)
		os.Exit(1)
	}

	// Add/merge entries, adjusting depth for subpath prefix
	prefix := dst.SubPath
	prefixDepth := 0
	if prefix != "" {
		prefixDepth = strings.Count(strings.TrimSuffix(prefix, "/"), "/") + 1
		ensureParentDirs(manifest, prefix)
	}

	added := 0
	for _, entry := range scanned.Entries {
		newEntry := *entry // copy
		newEntry.Depth += prefixDepth
		manifest.AddEntry(&newEntry)
		added++
	}

	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(dst.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", dst.Source, err)
		os.Exit(1)
	}

	// Push file content to c4d
	pushed := 0
	pushFailed := 0
	filepath.Walk(src.Source, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if err := putToC4d(path); err != nil {
			fmt.Fprintf(os.Stderr, "error: c4d push failed for %s: %v\n", filepath.Base(path), err)
			pushFailed++
		} else {
			pushed++
		}
		return nil
	})

	fmt.Printf("captured %d entries into %s\n", added, dst)
	if pushed > 0 {
		fmt.Printf("pushed %d files to c4d\n", pushed)
	}
	if pushFailed > 0 {
		fmt.Fprintf(os.Stderr, "error: %d files not stored in c4d — materialization will fail for unstored content\n", pushFailed)
		os.Exit(1)
	}
}

// cpFileIntoCapsule captures a single file into a capsule.
func cpFileIntoCapsule(filePath string, dst pathspec.PathSpec) {
	info, err := os.Stat(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	id := c4.Identify(f)

	manifest, err := loadOrCreateManifest(dst.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	name := filepath.Base(filePath)
	depth := 0
	if dst.SubPath != "" {
		ensureParentDirs(manifest, dst.SubPath)
		depth = strings.Count(strings.TrimSuffix(dst.SubPath, "/"), "/") + 1
	}

	entry := &c4m.Entry{
		Name:      name,
		Depth:     depth,
		Mode:      info.Mode(),
		Size:      info.Size(),
		Timestamp: info.ModTime().UTC(),
		C4ID:      id,
	}
	insertUnderParent(manifest, entry, dst.SubPath)

	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(dst.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Push file content to c4d
	if err := putToC4d(filePath); err != nil {
		fmt.Fprintf(os.Stderr, "error: c4d push failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "c4m updated, but content not stored — materialization will fail\n")
		os.Exit(1)
	}

	fmt.Printf("captured %s into %s\n", filepath.Base(filePath), dst)
}

// cpCapsuleToLocal materializes capsule contents to local filesystem.
func cpCapsuleToLocal(src, dst pathspec.PathSpec) {
	// Load capsule
	manifest, err := loadManifest(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", src.Source, err)
		os.Exit(1)
	}

	// Reconstruct full paths from depth-based hierarchy
	type pathEntry struct {
		fullPath string
		entry    *c4m.Entry
	}
	var resolved []pathEntry
	var dirStack []string // stack of directory names by depth

	for _, entry := range manifest.Entries {
		// Trim dirStack to current depth
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}

		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + entry.Name
		} else {
			fullPath = entry.Name
		}

		resolved = append(resolved, pathEntry{fullPath: fullPath, entry: entry})

		// If this is a directory, push onto stack for children
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}

	// Filter by subpath if specified
	if src.SubPath != "" {
		var filtered []pathEntry
		for _, pe := range resolved {
			if strings.HasPrefix(pe.fullPath, src.SubPath) {
				// Strip the prefix for materialization
				pe.fullPath = strings.TrimPrefix(pe.fullPath, src.SubPath)
				if pe.fullPath == "" {
					continue // skip the directory entry itself
				}
				filtered = append(filtered, pe)
			}
		}
		resolved = filtered
	}

	if len(resolved) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no entries match %s\n", src)
		os.Exit(1)
	}

	// Ensure destination directory exists
	destDir := dst.Source
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", destDir, err)
		os.Exit(1)
	}

	// Create directory structure and write placeholder files
	created := 0
	for _, pe := range resolved {
		fullPath := filepath.Join(destDir, pe.fullPath)

		if pe.entry.IsDir() {
			if err := os.MkdirAll(fullPath, pe.entry.Mode.Perm()|0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			created++
		} else if pe.entry.IsSymlink() {
			os.Remove(fullPath)
			if err := os.Symlink(pe.entry.Target, fullPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating symlink %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			created++
		} else {
			dir := filepath.Dir(fullPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			if err := writeFileContent(fullPath, pe.entry); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			created++
		}
	}

	fmt.Printf("materialized %d entries from %s to %s\n", created, src, destDir)
}

// writeFileContent fetches content from c4d and streams it to path.
// Returns an error if content cannot be fetched — never writes stubs.
func writeFileContent(path string, entry *c4m.Entry) error {
	if entry.C4ID.IsNil() {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode.Perm()|0644)
		if err != nil {
			return err
		}
		return f.Close()
	}

	// Fetch from c4d and stream directly to file (no memory buffering)
	resp, err := c4dClient.Get(c4dAddr() + "/" + entry.C4ID.String())
	if err != nil {
		return fmt.Errorf("c4d fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("c4d fetch: %s", resp.Status)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode.Perm()|0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// c4d HTTP integration

func c4dAddr() string {
	if addr := os.Getenv("C4D_ADDR"); addr != "" {
		return addr
	}
	return "http://localhost:17433"
}

// putToC4d pushes file content to c4d via PUT.
func putToC4d(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	req, err := http.NewRequest(http.MethodPut, c4dAddr()+"/", f)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c4dClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("c4d PUT: %s", resp.Status)
	}
	return nil
}

// loadManifest loads a c4m file, returns error if not found.
func loadManifest(path string) (*c4m.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return c4m.NewDecoder(f).Decode()
}

// insertUnderParent inserts an entry at the correct position in the manifest,
// adjacent to its parent directory. This prevents sortSiblingsHierarchically
// from misassigning the entry to the wrong parent.
func insertUnderParent(manifest *c4m.Manifest, entry *c4m.Entry, subPath string) {
	if subPath == "" {
		manifest.AddEntry(entry)
		return
	}

	// Find the parent directory entry
	parentName := filepath.Base(strings.TrimSuffix(subPath, "/")) + "/"
	parentDepth := entry.Depth - 1

	for i, e := range manifest.Entries {
		if e.Name == parentName && e.Depth == parentDepth && e.IsDir() {
			// Found parent — find end of its children
			insertIdx := i + 1
			for j := i + 1; j < len(manifest.Entries); j++ {
				if manifest.Entries[j].Depth <= parentDepth {
					break
				}
				insertIdx = j + 1
			}
			// Insert at correct position
			manifest.Entries = append(manifest.Entries, nil)
			copy(manifest.Entries[insertIdx+1:], manifest.Entries[insertIdx:])
			manifest.Entries[insertIdx] = entry
			manifest.InvalidateIndex()
			return
		}
	}

	// Parent not found — fall back to append
	manifest.AddEntry(entry)
}

// ensureParentDirs adds missing parent directory entries with proper depth.
func ensureParentDirs(manifest *c4m.Manifest, path string) {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for i, part := range parts {
		dirName := part + "/"
		// Check if this directory already exists at this depth
		found := false
		for _, e := range manifest.Entries {
			if e.Name == dirName && e.Depth == i {
				found = true
				break
			}
		}
		if !found {
			manifest.AddEntry(&c4m.Entry{
				Name:  dirName,
				Depth: i,
				Mode:  os.ModeDir | 0755,
				Size:  -1,
			})
		}
	}
}

