package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
)

// runMkdir implements "c4 mkdir" — create a directory entry in a c4m file.
//
//	c4 mkdir project.c4m:renders/             # create renders/ (parent must exist or be root)
//	c4 mkdir -p project.c4m:renders/shots/    # create renders/ and shots/ if needed
func runMkdir(args []string) {
	// Parse -p flag
	var createParents bool
	var filtered []string
	for _, a := range args {
		if a == "-p" {
			createParents = true
		} else {
			filtered = append(filtered, a)
		}
	}

	if len(filtered) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 mkdir [-p] <name>.c4m:<path>/\n")
		os.Exit(1)
	}

	spec, err := pathspec.Parse(filtered[0], establish.IsLocationEstablished)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if spec.Type != pathspec.C4m {
		fmt.Fprintf(os.Stderr, "Error: mkdir requires a c4m file path (e.g. project.c4m:renders/)\n")
		os.Exit(1)
	}

	if spec.SubPath == "" {
		fmt.Fprintf(os.Stderr, "Error: must specify a directory path within the c4m file\n")
		os.Exit(1)
	}

	// Ensure trailing slash
	dirPath := spec.SubPath
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	// Check establishment
	if !establish.IsC4mEstablished(spec.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s is not established for writing\n", spec.Source+":")
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", spec.Source)
		os.Exit(1)
	}

	// Lock the c4m file for the read-modify-write cycle
	unlock, err := lockC4mFile(spec.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", spec.Source, err)
		os.Exit(1)
	}
	defer unlock()

	// Load or create manifest
	manifest, err := loadOrCreateManifest(spec.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Decompose path into components: "renders/shots/" -> ["renders", "shots"]
	parts := strings.Split(strings.TrimSuffix(dirPath, "/"), "/")

	var created bool
	if createParents {
		// -p mode: create all missing intermediate directories
		created = mkdirParents(manifest, parts)
	} else {
		// Standard mode: parent must already exist
		var err error
		created, err = mkdirStrict(manifest, parts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	if !created {
		fmt.Fprintf(os.Stderr, "%s already exists in %s\n", dirPath, spec.Source)
		os.Exit(0)
	}

	manifest.SortEntries()

	// Write manifest
	if err := writeManifest(spec.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created %s:%s\n", spec.Source, dirPath)
}

// mkdirStrict creates the final directory only if all parents exist.
// Returns (created, error). created is false if the directory already exists.
func mkdirStrict(manifest *c4m.Manifest, parts []string) (bool, error) {
	// Verify all parents exist (all but the last component)
	for i := 0; i < len(parts)-1; i++ {
		dirName := parts[i] + "/"
		if !dirExistsInManifest(manifest, dirName, i) {
			missing := strings.Join(parts[:i+1], "/") + "/"
			return false, fmt.Errorf("cannot create directory: %s does not exist (use -p to create parents)", missing)
		}
	}

	// Create the final directory if it doesn't exist
	finalName := parts[len(parts)-1] + "/"
	finalDepth := len(parts) - 1
	if dirExistsInManifest(manifest, finalName, finalDepth) {
		return false, nil
	}

	manifest.AddEntry(&c4m.Entry{
		Name:      finalName,
		Depth:     finalDepth,
		Mode:      os.ModeDir | 0755,
		Timestamp: c4m.NullTimestamp(),
		Size:      -1,
	})
	return true, nil
}

// mkdirParents creates all missing directories in the path (-p mode).
// Entries are inserted after their parent to maintain tree structure.
// Returns true if any directory was created.
func mkdirParents(manifest *c4m.Manifest, parts []string) bool {
	created := false
	for i, part := range parts {
		dirName := part + "/"
		if dirExistsInManifest(manifest, dirName, i) {
			continue
		}
		entry := &c4m.Entry{
			Name:      dirName,
			Depth:     i,
			Mode:      os.ModeDir | 0755,
			Timestamp: c4m.NullTimestamp(),
			Size:      -1,
		}
		// Insert after parent and its subtree to maintain tree structure
		insertIdx := findInsertionPoint(manifest, parts[:i], i)
		entries := manifest.Entries
		manifest.Entries = make([]*c4m.Entry, 0, len(entries)+1)
		manifest.Entries = append(manifest.Entries, entries[:insertIdx]...)
		manifest.Entries = append(manifest.Entries, entry)
		manifest.Entries = append(manifest.Entries, entries[insertIdx:]...)
		created = true
	}
	return created
}

// findInsertionPoint finds where to insert a new entry at the given depth
// by locating the parent directory and skipping past its subtree.
func findInsertionPoint(manifest *c4m.Manifest, parentParts []string, depth int) int {
	if depth == 0 {
		// Insert at end for root-level entries
		return len(manifest.Entries)
	}
	// Find the parent entry
	parentName := parentParts[len(parentParts)-1] + "/"
	parentDepth := depth - 1
	for i, e := range manifest.Entries {
		if e.Name == parentName && e.Depth == parentDepth && e.IsDir() {
			// Skip past the parent's subtree
			j := i + 1
			for j < len(manifest.Entries) && manifest.Entries[j].Depth > parentDepth {
				j++
			}
			return j
		}
	}
	return len(manifest.Entries)
}

// dirExistsInManifest checks if a directory entry exists at the given depth.
func dirExistsInManifest(manifest *c4m.Manifest, name string, depth int) bool {
	for _, e := range manifest.Entries {
		if e.Name == name && e.Depth == depth && e.IsDir() {
			return true
		}
	}
	return false
}

// lockC4mFile acquires an exclusive advisory lock on a c4m file's sidecar
// lock file. Returns an unlock function that must be deferred. This prevents
// concurrent c4 commands from racing on the same c4m file.
func lockC4mFile(path string) (unlock func(), err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	lockPath := abs + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
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

// writeManifest writes a manifest to a c4m file atomically.
func writeManifest(path string, m *c4m.Manifest) error {
	var buf bytes.Buffer
	if err := c4m.NewEncoder(&buf).Encode(m); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp.*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
