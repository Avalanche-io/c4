package main

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
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
	"github.com/Avalanche-io/c4/cmd/c4/internal/container"
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
//	c4 cp source/ project.c4m:          # capture into c4m file
//	c4 cp source/ project.c4m:renders/  # capture into subtree
//	c4 cp project.c4m: ./output/        # materialize from c4m file
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
		fmt.Fprintf(os.Stderr, "  c4 cp files/ project.c4m:           # capture into c4m file\n")
		fmt.Fprintf(os.Stderr, "  c4 cp files/ project.c4m:renders/   # capture into subtree\n")
		fmt.Fprintf(os.Stderr, "  c4 cp project.c4m: ./output/        # materialize from c4m file\n")
		fmt.Fprintf(os.Stderr, "  c4 cp project.c4m:renders/ ./out/   # materialize subtree\n")
		os.Exit(1)
	}

	// Handle stdin pipe: c4 cp - <dest>
	if args[0] == "-" {
		isLoc := establish.IsLocationEstablished
		dst, err := pathspec.Parse(args[1], isLoc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing dest: %v\n", err)
			os.Exit(1)
		}
		cpStdinToTarget(dst)
		return
	}

	// Handle [] collapse marker: c4 cp frames.*.exr :frames.[].exr
	if strings.Contains(args[1], "[]") {
		cpGlobToSequence(args[0], args[1])
		return
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

	// Resolve managed source to a manifest-backed c4m-like operation
	if src.Type == pathspec.Managed {
		manifest := getManagedManifest(src.SubPath)
		cpManifestToTarget(manifest, dst)
		return
	}

	switch {
	case src.Type == pathspec.Local && dst.Type == pathspec.C4m:
		cpLocalToC4m(src, dst)
	case src.Type == pathspec.C4m && dst.Type == pathspec.Local:
		cpC4mToLocal(src, dst)
	case src.Type == pathspec.Local && dst.Type == pathspec.Container:
		cpLocalToContainer(src, dst)
	case src.Type == pathspec.Container && dst.Type == pathspec.Local:
		cpContainerToLocal(src, dst)
	case src.Type == pathspec.C4m && dst.Type == pathspec.Container:
		cpC4mToContainer(src, dst)
	case src.Type == pathspec.Container && dst.Type == pathspec.C4m:
		cpContainerToC4m(src, dst)
	case src.Type == pathspec.Local && dst.Type == pathspec.Local:
		fmt.Fprintf(os.Stderr, "Error: use OS cp for local-to-local copies\n")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "Error: %s → %s not yet supported\n", src.Type, dst.Type)
		os.Exit(1)
	}
}

// cpManifestToTarget copies a resolved manifest to the destination.
// Used when the source is a managed directory (: notation).
func cpManifestToTarget(manifest *c4m.Manifest, dst pathspec.PathSpec) {
	switch dst.Type {
	case pathspec.C4m:
		if !establish.IsC4mEstablished(dst.Source) {
			fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", dst.Source)
			fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", dst.Source)
			os.Exit(1)
		}
		unlock, err := lockC4mFile(dst.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", dst.Source, err)
			os.Exit(1)
		}
		defer unlock()
		existing, err := loadOrCreateManifest(dst.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		prefix := dst.SubPath
		prefixDepth := 0
		if prefix != "" {
			prefixDepth = strings.Count(strings.TrimSuffix(prefix, "/"), "/") + 1
			ensureParentDirs(existing, prefix)
		}
		for _, entry := range manifest.Entries {
			e := *entry
			e.Depth += prefixDepth
			existing.AddEntry(&e)
		}
		existing.SortEntries()
		scan.PropagateMetadata(existing.Entries)
		if err := writeManifest(dst.Source, existing); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("exported %d entries to %s\n", len(manifest.Entries), dst)

	case pathspec.Container:
		format := pathspec.ContainerFormat(dst.Source)
		err := container.WriteTar(dst.Source, format, manifest, func(fullPath string, entry *c4m.Entry) (io.ReadCloser, error) {
			// Try local file first (managed dir tracks local files)
			if f, err := os.Open(fullPath); err == nil {
				return f, nil
			}
			if entry.C4ID.IsNil() {
				return io.NopCloser(strings.NewReader("")), nil
			}
			// Fall back to c4d
			resp, err := c4dClient.Get(c4dAddr() + "/" + entry.C4ID.String())
			if err != nil {
				return nil, fmt.Errorf("c4d fetch: %w", err)
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, fmt.Errorf("c4d: %s for %s", resp.Status, entry.C4ID)
			}
			return resp.Body, nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", dst.Source, err)
			os.Exit(1)
		}
		fmt.Printf("exported managed state → %s (%d entries)\n", dst.Source, len(manifest.Entries))

	default:
		fmt.Fprintf(os.Stderr, "Error: managed → %s not yet supported\n", dst.Type)
		os.Exit(1)
	}
}

// cpLocalToC4m captures local files into a c4m file.
func cpLocalToC4m(src, dst pathspec.PathSpec) {
	// Check establishment
	if !establish.IsC4mEstablished(dst.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", dst.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", dst.Source)
		os.Exit(1)
	}

	unlock, err := lockC4mFile(dst.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", dst.Source, err)
		os.Exit(1)
	}
	defer unlock()

	// Walk source and build manifest entries
	info, err := os.Stat(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !info.IsDir() {
		// Single file capture
		cpFileIntoC4m(src.Source, dst)
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

// cpFileIntoC4m captures a single file into a c4m file.
func cpFileIntoC4m(filePath string, dst pathspec.PathSpec) {
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

// cpC4mToLocal materializes c4m file contents to local filesystem.
func cpC4mToLocal(src, dst pathspec.PathSpec) {
	// Load c4m file
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

	// Create directory structure and materialize files with exact metadata
	created := 0
	// Collect directories to restore timestamps after all children are written
	// (writing children changes parent dir mtime)
	type dirMeta struct {
		path  string
		entry *c4m.Entry
	}
	var dirs []dirMeta

	for _, pe := range resolved {
		fullPath := filepath.Join(destDir, pe.fullPath)

		if pe.entry.IsDir() {
			if err := os.MkdirAll(fullPath, pe.entry.Mode.Perm()); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			dirs = append(dirs, dirMeta{fullPath, pe.entry})
			created++
		} else if pe.entry.IsSymlink() {
			os.Remove(fullPath)
			if err := os.Symlink(pe.entry.Target, fullPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating symlink %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			// Symlinks: Lchtimes not portable; timestamp is best-effort
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

	// Restore directory metadata bottom-up (deepest first so parent timestamps aren't
	// overwritten by child creation)
	for i := len(dirs) - 1; i >= 0; i-- {
		restoreMetadata(dirs[i].path, dirs[i].entry)
	}

	fmt.Printf("materialized %d entries from %s to %s\n", created, src, destDir)
}

// writeFileContent fetches content from c4d and streams it to path.
// Sets exact permissions and timestamp for round-trip identity.
func writeFileContent(path string, entry *c4m.Entry) error {
	if entry.C4ID.IsNil() {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode.Perm())
		if err != nil {
			return err
		}
		f.Close()
		restoreMetadata(path, entry)
		return nil
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

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode.Perm())
	if err != nil {
		return err
	}

	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return err
	}
	restoreMetadata(path, entry)
	return nil
}

// restoreMetadata sets exact permissions and timestamp on a materialized path.
// This ensures round-trip identity: c4m → filesystem → c4m produces the same result.
func restoreMetadata(path string, entry *c4m.Entry) {
	// Set exact permissions (no |0644 fallback — preserve what c4m says)
	os.Chmod(path, entry.Mode.Perm())

	// Restore timestamp if not the null sentinel
	if !entry.Timestamp.Equal(c4m.NullTimestamp()) {
		os.Chtimes(path, entry.Timestamp, entry.Timestamp)
	}
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

// cpStdinToTarget reads c4m from stdin and copies local files to the target.
// This enables: c4 . | grep '\.go$' | c4 cp - project.tar:
func cpStdinToTarget(dst pathspec.PathSpec) {
	manifest, err := c4m.NewDecoder(os.Stdin).Decode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}

	switch dst.Type {
	case pathspec.Container:
		// Build C4 ID → local path index for resolving content.
		// Piped c4m may lose directory context (e.g., grep strips parent entries),
		// so we resolve files by content identity rather than path.
		idIndex := buildLocalIDIndex(".")

		format := pathspec.ContainerFormat(dst.Source)
		err = container.WriteTar(dst.Source, format, manifest, func(fullPath string, entry *c4m.Entry) (io.ReadCloser, error) {
			// First try the manifest path directly
			if f, err := os.Open(fullPath); err == nil {
				return f, nil
			}
			// Fall back to C4 ID lookup (handles filtered/reordered stdin)
			if !entry.C4ID.IsNil() {
				if localPath, ok := idIndex[entry.C4ID.String()]; ok {
					return os.Open(localPath)
				}
			}
			return nil, fmt.Errorf("cannot find local file for %s (C4 ID %s)", fullPath, entry.C4ID)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", dst.Source, err)
			os.Exit(1)
		}
		fmt.Printf("created %s (%d entries from stdin)\n", dst.Source, len(manifest.Entries))

	case pathspec.C4m:
		if !establish.IsC4mEstablished(dst.Source) {
			fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", dst.Source)
			fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", dst.Source)
			os.Exit(1)
		}
		unlock, err := lockC4mFile(dst.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", dst.Source, err)
			os.Exit(1)
		}
		defer unlock()
		existing, err := loadOrCreateManifest(dst.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		for _, entry := range manifest.Entries {
			existing.AddEntry(entry)
		}
		existing.SortEntries()
		scan.PropagateMetadata(existing.Entries)
		if err := writeManifest(dst.Source, existing); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("captured %d entries from stdin into %s:\n", len(manifest.Entries), dst.Source)

	default:
		fmt.Fprintf(os.Stderr, "Error: stdin (-) → %s not supported\n", dst.Type)
		os.Exit(1)
	}
}

// cpC4mToContainer exports c4m contents as a tar archive.
// Content bytes are fetched from c4d.
func cpC4mToContainer(src, dst pathspec.PathSpec) {
	manifest, err := loadManifest(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", src.Source, err)
		os.Exit(1)
	}
	if src.SubPath != "" {
		manifest = filterBySubpath(manifest, src.SubPath)
	}

	format := pathspec.ContainerFormat(dst.Source)
	err = container.WriteTar(dst.Source, format, manifest, func(fullPath string, entry *c4m.Entry) (io.ReadCloser, error) {
		if entry.C4ID.IsNil() {
			return io.NopCloser(strings.NewReader("")), nil
		}
		resp, err := c4dClient.Get(c4dAddr() + "/" + entry.C4ID.String())
		if err != nil {
			return nil, fmt.Errorf("c4d fetch: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("c4d: %s for %s", resp.Status, entry.C4ID)
		}
		return resp.Body, nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", dst.Source, err)
		os.Exit(1)
	}
	fmt.Printf("exported %s → %s (%d entries)\n", src.Source, dst.Source, len(manifest.Entries))
}

// cpContainerToC4m imports tar contents into a c4m file.
func cpContainerToC4m(src, dst pathspec.PathSpec) {
	if !establish.IsC4mEstablished(dst.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", dst.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", dst.Source)
		os.Exit(1)
	}

	unlock, err := lockC4mFile(dst.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", dst.Source, err)
		os.Exit(1)
	}
	defer unlock()

	format := pathspec.ContainerFormat(src.Source)
	tarManifest, err := container.ReadManifest(src.Source, format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", src.Source, err)
		os.Exit(1)
	}

	manifest, err := loadOrCreateManifest(dst.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range tarManifest.Entries {
		manifest.AddEntry(entry)
	}
	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(dst.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("imported %s → %s: (%d entries)\n", src.Source, dst.Source, len(tarManifest.Entries))
}

// cpLocalToContainer creates a tar archive from local files.
func cpLocalToContainer(src, dst pathspec.PathSpec) {
	info, err := os.Stat(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: source must be a directory for tar creation\n")
		os.Exit(1)
	}

	gen := scan.NewGeneratorWithOptions(scan.WithC4IDs(true))
	manifest, err := gen.GenerateFromPath(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", src.Source, err)
		os.Exit(1)
	}

	format := pathspec.ContainerFormat(dst.Source)
	srcDir := src.Source
	err = container.WriteTar(dst.Source, format, manifest, func(fullPath string, entry *c4m.Entry) (io.ReadCloser, error) {
		return os.Open(filepath.Join(srcDir, fullPath))
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", dst.Source, err)
		os.Exit(1)
	}

	fmt.Printf("created %s (%d entries)\n", dst.Source, len(manifest.Entries))
}

// cpContainerToLocal extracts a tar archive to local filesystem.
func cpContainerToLocal(src, dst pathspec.PathSpec) {
	format := pathspec.ContainerFormat(src.Source)
	manifest, err := container.ReadManifest(src.Source, format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", src.Source, err)
		os.Exit(1)
	}

	destDir := dst.Source
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", destDir, err)
		os.Exit(1)
	}

	// Re-open and walk the tar to extract file contents
	// (We need a second pass because ReadManifest consumed the content for C4 IDs)
	f, err := os.Open(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	tr, err := openTarReader(f, format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	type dirRestore struct {
		path string
		time time.Time
		mode os.FileMode
	}
	var dirRestores []dirRestore

	created := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading tar: %v\n", err)
			os.Exit(1)
		}

		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		fullPath := filepath.Join(destDir, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fullPath, hdr.FileInfo().Mode().Perm()); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			dirRestores = append(dirRestores, dirRestore{fullPath, hdr.ModTime, hdr.FileInfo().Mode().Perm()})
			created++

		case tar.TypeSymlink:
			os.Remove(fullPath)
			if err := os.Symlink(hdr.Linkname, fullPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating symlink %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			created++

		case tar.TypeReg, tar.TypeRegA:
			dir := filepath.Dir(fullPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			outF, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode().Perm())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			if _, err := io.Copy(outF, tr); err != nil {
				outF.Close()
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", fullPath, err)
				os.Exit(1)
			}
			outF.Close()
			os.Chtimes(fullPath, hdr.ModTime, hdr.ModTime)
			created++
		}
	}

	// Restore directory timestamps bottom-up
	for i := len(dirRestores) - 1; i >= 0; i-- {
		d := dirRestores[i]
		os.Chmod(d.path, d.mode)
		os.Chtimes(d.path, d.time, d.time)
	}

	_ = manifest // used for count only
	fmt.Printf("extracted %d entries from %s to %s\n", created, src.Source, destDir)
}

// openTarReader wraps a reader with decompression. Used by cpContainerToLocal.
func openTarReader(r io.Reader, format string) (*tar.Reader, error) {
	switch format {
	case "gzip":
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		return tar.NewReader(gr), nil
	case "bzip2":
		return tar.NewReader(bzip2.NewReader(r)), nil
	case "tar":
		return tar.NewReader(r), nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// buildLocalIDIndex scans the local directory and builds a map from C4 ID string to local file path.
// Used for resolving piped c4m entries that may have lost directory context.
func buildLocalIDIndex(root string) map[string]string {
	idx := make(map[string]string)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		id := c4.Identify(f)
		f.Close()
		idx[id.String()] = path
		return nil
	})
	return idx
}

// cpGlobToSequence implements the [] collapse marker convention.
// Source is a glob pattern, dest contains [] which absorbs the varying part.
//
//	c4 cp frames.*.exr project.c4m:frames.[].exr
//	c4 cp render.*.exr :render.[].exr
func cpGlobToSequence(srcGlob, dstPattern string) {
	// Expand source glob
	matches, err := filepath.Glob(srcGlob)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid glob pattern: %v\n", err)
		os.Exit(1)
	}
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no files match %s\n", srcGlob)
		os.Exit(1)
	}

	// Build a manifest from the matched files, then detect sequences
	tempManifest := c4m.NewManifest()
	for _, path := range matches {
		info, err := os.Lstat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if info.IsDir() {
			continue
		}

		entry := &c4m.Entry{
			Name:      filepath.Base(path),
			Mode:      info.Mode(),
			Size:      info.Size(),
			Timestamp: info.ModTime().UTC(),
		}

		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			entry.C4ID = c4.Identify(f)
			f.Close()
		}

		tempManifest.AddEntry(entry)
	}

	// Detect sequences (minimum 1 — the user explicitly asked for folding)
	detector := c4m.NewSequenceDetector(1)
	folded := detector.DetectSequences(tempManifest)

	if len(folded.Entries) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no sequences detected in matched files\n")
		os.Exit(1)
	}

	// Parse destination
	isLoc := establish.IsLocationEstablished
	dst, err := pathspec.Parse(dstPattern, isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing dest: %v\n", err)
		os.Exit(1)
	}

	// Override names: the dest pattern's [] becomes the detected range
	// e.g., dest "frames.[].exr" + detected "frames.[001-100].exr" → use detected name
	// The user-provided dest name is a template; the actual range comes from detection.

	switch dst.Type {
	case pathspec.C4m:
		if !establish.IsC4mEstablished(dst.Source) {
			fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", dst.Source)
			fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", dst.Source)
			os.Exit(1)
		}
		unlock, err := lockC4mFile(dst.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", dst.Source, err)
			os.Exit(1)
		}
		defer unlock()
		manifest, err := loadOrCreateManifest(dst.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		prefix := dst.SubPath
		// Strip the filename from subpath to get the directory prefix
		dir := ""
		if idx := strings.LastIndex(prefix, "/"); idx >= 0 {
			dir = prefix[:idx+1]
		}
		prefixDepth := 0
		if dir != "" {
			prefixDepth = strings.Count(strings.TrimSuffix(dir, "/"), "/") + 1
			ensureParentDirs(manifest, dir)
		}

		for _, entry := range folded.Entries {
			e := *entry
			e.Depth += prefixDepth
			manifest.AddEntry(&e)
		}
		for _, db := range folded.DataBlocks {
			manifest.AddDataBlock(db)
		}
		manifest.SortEntries()
		scan.PropagateMetadata(manifest.Entries)

		if err := writeManifest(dst.Source, manifest); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Push file content to c4d
		pushed := 0
		for _, path := range matches {
			if err := putToC4d(path); err != nil {
				fmt.Fprintf(os.Stderr, "error: c4d push failed for %s: %v\n", filepath.Base(path), err)
			} else {
				pushed++
			}
		}

		fmt.Printf("captured %d files as %d sequence(s) into %s\n", len(matches), len(folded.Entries), dst)

	default:
		fmt.Fprintf(os.Stderr, "Error: [] collapse marker requires a c4m or managed destination\n")
		os.Exit(1)
	}
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
				Name:      dirName,
				Depth:     i,
				Mode:      os.ModeDir | 0755,
				Timestamp: c4m.NullTimestamp(),
				Size:      -1,
			})
		}
	}
}

