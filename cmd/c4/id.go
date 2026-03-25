package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
	"github.com/Avalanche-io/c4/store"
)

func runID(args []string) {
	fs := newFlags("id")
	storeFlag := fs.boolFlag("store", 's', false, "Store content in the configured store")
	quiet := fs.boolFlag("quiet", 'q', false, "Suppress output (useful with -s)")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Output ergonomic form c4m")
	seqFlag := fs.boolFlag("sequence", 'S', false, "Detect and fold file sequences")
	excludeFlags := fs.stringArrayFlag("exclude", "Glob pattern to exclude (repeatable)")
	excludeFileFlag := fs.stringFlag("exclude-file", 0, "", "File of exclude patterns (one per line)")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode: s/1=structure, m/2=metadata, f/3=full")
	continueFlag := fs.stringFlag("continue", 'c', "", "Continue from existing c4m (use as guide)")
	fs.parse(args)

	paths := fs.args

	if len(paths) == 0 {
		// stdin → bare C4 ID
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			doStdin(*storeFlag)
			return
		}
		fmt.Fprintf(os.Stderr, "Usage: c4 id [flags] <path>...\n")
		os.Exit(1)
	}

	mode, err := scan.ParseScanMode(*modeFlag)
	if err != nil {
		fatalf("Error: %v", err)
	}

	shouldStore := *storeFlag
	// Storing only makes sense in full mode.
	if mode != scan.ModeFull {
		shouldStore = false
	}

	// If -s is requested, ensure the store is configured before scanning.
	// This prompts the user immediately rather than after a long scan.
	if shouldStore {
		if s := getOrSetupStore(); s == nil {
			shouldStore = false
		}
	}

	// Build scan options for exclusion.
	var scanExcludes []string
	scanExcludes = append(scanExcludes, *excludeFlags...)
	excludeFile := *excludeFileFlag

	// Load continue guide if specified.
	var guide *c4m.Manifest
	if *continueFlag != "" {
		guide, err = loadManifest(*continueFlag)
		if err != nil {
			fatalf("Error loading guide %s: %v", *continueFlag, err)
		}
	}

	// Collect results — multiple paths produce one combined manifest.
	combined := c4m.NewManifest()

	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil {
			fatalf("Error: %v", err)
		}

		if info.IsDir() {
			m := scanDirectory(p, mode, *seqFlag, shouldStore, scanExcludes, excludeFile, guide)
			if !*quiet {
				outputManifest(m, *ergonomic)
			}
			return
		}

		if strings.HasSuffix(p, ".c4m") {
			// c4m input → normalize to canonical (or ergonomic) form
			m, err := loadManifest(p)
			if err != nil {
				fatalf("Error loading %s: %v", p, err)
			}
			// Store canonical c4m content if -s is set.
			if shouldStore {
				s := getOrSetupStore()
				if s != nil {
					storeManifestAsContent(m, s)
				}
			}
			if !*quiet {
				outputManifest(m, *ergonomic)
			}
			return
		}

		// Regular file → single-entry c4m
		entry := identifyFile(p, info, mode, shouldStore)
		combined.AddEntry(entry)
	}

	if !*quiet {
		outputManifest(combined, *ergonomic)
	}
}

func doStdin(storeFlag bool) {
	if storeFlag {
		s := getOrSetupStore()
		if s != nil {
			id, err := storeContentC4mAware(s, os.Stdin)
			if err != nil {
				fatalf("Error storing: %v", err)
			}
			fmt.Println(id)
			return
		}
	}

	// Read all stdin to detect c4m.
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatalf("Error reading stdin: %v", err)
	}
	if looksLikeC4m(data) {
		canonical, _ := canonicalizeC4mBytes(data)
		if canonical != nil {
			id := c4.Identify(bytes.NewReader(canonical))
			fmt.Println(id)
			return
		}
	}
	id := c4.Identify(bytes.NewReader(data))
	fmt.Println(id)
}

func scanDirectory(dirPath string, mode scan.ScanMode, seqFlag, shouldStore bool, excludes []string, excludeFile string, guide *c4m.Manifest) *c4m.Manifest {
	opts := []scan.GeneratorOption{scan.WithMode(mode)}
	if seqFlag {
		opts = append(opts, scan.WithSequenceDetection(true))
	}
	if len(excludes) > 0 {
		opts = append(opts, scan.WithExclude(excludes))
	}
	if excludeFile != "" {
		opts = append(opts, scan.WithExcludeFile(excludeFile))
	}
	if guide != nil {
		opts = append(opts, scan.WithGuide(guide))
	}
	gen := scan.NewGeneratorWithOptions(opts...)
	manifest, err := gen.GenerateFromPath(dirPath)
	if err != nil {
		fatalf("Error scanning %s: %v", dirPath, err)
	}

	if shouldStore {
		storeManifestContent(manifest, dirPath)
	}

	return manifest
}

func identifyFile(path string, info os.FileInfo, mode scan.ScanMode, shouldStore bool) *c4m.Entry {
	entry := &c4m.Entry{
		Name: filepath.Base(path),
	}

	if mode >= scan.ModeMetadata {
		entry.Mode = info.Mode()
		entry.Timestamp = info.ModTime().UTC()
		entry.Size = info.Size()
	} else {
		entry.Size = -1
	}

	if mode == scan.ModeFull {
		if shouldStore {
			s := getOrSetupStore()
			if s != nil {
				entry.C4ID = storeC4mAware(s, path)
			} else {
				id, _ := identifyC4mFile(path)
				entry.C4ID = id
			}
		} else {
			id, _ := identifyC4mFile(path)
			entry.C4ID = id
		}
	}

	return entry
}

func storeManifestContent(manifest *c4m.Manifest, baseDir string) {
	s := getOrSetupStore()
	if s == nil {
		return
	}

	// Walk manifest entries and store file content.
	var dirStack []string
	for _, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name

			// Store the directory's c4m as content (enables c4 cat -r).
			if !entry.C4ID.IsNil() && !s.Has(entry.C4ID) {
				storeDirectoryC4m(manifest, entry, s)
			}
			continue
		}
		if entry.C4ID.IsNil() || s.Has(entry.C4ID) {
			continue
		}

		// Reconstruct path relative to baseDir.
		relPath := strings.Join(dirStack, "") + entry.Name
		fullPath := filepath.Join(baseDir, relPath)

		// Use c4m-aware storage: c4m files within directories get
		// canonicalized before storing.
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue // skip files we can't open
		}
		var storeData []byte
		if strings.HasSuffix(entry.Name, ".c4m") || looksLikeC4m(data) {
			canonical, _ := canonicalizeC4mBytes(data)
			if canonical != nil {
				storeData = canonical
			}
		}
		if storeData == nil {
			storeData = data
		}
		newID, err := s.Put(bytes.NewReader(storeData))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store %s: %v\n", relPath, err)
			continue
		}

		// If canonicalization changed the ID (c4m file), update the entry.
		if newID != entry.C4ID {
			entry.C4ID = newID
		}
	}
}

// storeDirectoryC4m extracts a directory's direct children from a manifest
// and stores the resulting one-level c4m as content. This enables c4 cat <dir-id>
// to retrieve the directory listing, and c4 cat -r to recursively expand.
//
// The stored c4m is canonical: only direct children at depth 0, sorted.
// This matches how directory C4 IDs are computed (one-level canonical form).
func storeDirectoryC4m(manifest *c4m.Manifest, dirEntry *c4m.Entry, s store.Store) {
	children := manifest.Children(dirEntry)
	if len(children) == 0 {
		return
	}

	sub := c4m.NewManifest()
	for _, child := range children {
		entryCopy := *child
		entryCopy.Depth = 0 // Direct children at root level
		sub.AddEntry(&entryCopy)
	}

	// Match how ComputeC4ID() works: canonicalize (propagate metadata)
	// then produce canonical text.
	sub.Canonicalize()
	sub.SortEntries()
	canonical := sub.Canonical()
	if canonical == "" {
		return
	}

	if _, err := s.Put(strings.NewReader(canonical)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store directory c4m for %s: %v\n", dirEntry.Name, err)
	}
}

func getOrSetupStore() store.Store {
	s, err := store.OpenStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: store error: %v\n", err)
		return nil
	}
	if s != nil {
		return s
	}

	// No store configured — offer to create default (local only).
	if !isTerminal() {
		return nil
	}

	fmt.Fprintf(os.Stderr, "No content store configured. Create %s? [Y/n] ", store.DefaultStorePath())
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "" || answer == "y" || answer == "yes" {
		s, err := store.SetupDefaultStore()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating store: %v\n", err)
			return nil
		}
		return s
	}

	fmt.Fprintf(os.Stderr, "Set C4_STORE=/path/to/store or s3://bucket/prefix\n")
	return nil
}

func isTerminal() bool {
	stat, _ := os.Stdin.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func outputManifest(manifest *c4m.Manifest, ergonomic bool) {
	enc := c4m.NewEncoder(os.Stdout)
	if ergonomic {
		enc.SetPretty(true)
	}
	enc.Encode(manifest)
}
