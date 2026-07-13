package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
	"github.com/Avalanche-io/c4/store"
)

func runID(args []string) {
	fs := newFlags("id")
	storeFlag := fs.boolFlag("store", 's', false, "Store content in the configured store")
	quiet := fs.boolFlag("quiet", 'q', false, "Print one bare ID line per path (THE identity; byte-pure)")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Output ergonomic form c4m")
	seqFlag := fs.boolFlag("sequence", 'S', false, "Detect and fold file sequences")
	excludeFlags := fs.stringArrayFlag("exclude", "Glob pattern to exclude (repeatable)")
	excludeFileFlag := fs.stringFlag("exclude-file", 0, "", "File of exclude patterns (one per line)")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode: s=structure, m=metadata, f=full, c=content (machine-independent)")
	continueFlag := fs.stringFlag("continue", 'c', "", "Reuse guide: trust unchanged size+mtime from this c4m (racy-safe)")
	verifyFlag := fs.boolFlag("verify", 0, false, "Re-hash everything; with -c, report changes hidden under unchanged metadata")
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
	// Storing only makes sense in modes that hash content.
	if mode != scan.ModeFull && mode != scan.ModeContent {
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

	// Load the reuse guide if specified. Its scan start anchors the
	// racy rule: the journal's claim for the guide's root ID carries
	// the true scan start; the guide file's own mtime is the
	// conservative stand-in (never earlier than the scan it recorded).
	var guide *c4m.Manifest
	var guideStart time.Time
	if *continueFlag != "" {
		guide, err = loadManifest(*continueFlag)
		if err != nil {
			fatalf("Error loading guide %s: %v", *continueFlag, err)
		}
		guideStart = resolveGuideScanStart(*continueFlag, guide)
	}

	// Collect results — multiple paths produce one combined manifest.
	// Under -q the contract is: exactly one bare ID line per path that
	// SUCCEEDS (a failed path prints nothing and the exit code is 1),
	// so scripts capture THE identity without parsing prose.
	combined := c4m.NewManifest()
	failed := false

	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil {
			if *quiet {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				failed = true
				continue
			}
			fatalf("Error: %v", err)
		}

		if info.IsDir() {
			m := scanDirectory(p, mode, *seqFlag, shouldStore, scanExcludes, excludeFile, guide, guideStart, *verifyFlag)
			if *quiet {
				// THE ID of a directory scan is the identity of its
				// description: the manifest's canonical-text ID — the
				// same ID `stored:` reports and self-capture stores.
				fmt.Println(m.ComputeC4ID())
				continue
			}
			outputManifest(m, *ergonomic)
			return
		}

		if strings.HasSuffix(p, ".c4m") {
			// c4m input → normalize to canonical (or ergonomic) form
			m, err := loadManifest(p)
			if err != nil {
				if *quiet {
					fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", p, err)
					failed = true
					continue
				}
				fatalf("Error loading %s: %v", p, err)
			}
			// Store canonical c4m content if -s is set.
			if shouldStore {
				s := getOrSetupStore()
				if s != nil {
					start := time.Now().UTC()
					id, size := storeManifestSelf(s, m)
					syncStore(s)
					journalClaim(s, id, size, claimName(p, true), claimOrigin(p), start)
					reportStored(id)
				}
			}
			if *quiet {
				// THE ID of a description is the ID of its canonical
				// text at its own knowledge level.
				fmt.Println(m.ComputeC4ID())
				continue
			}
			outputManifest(m, *ergonomic)
			return
		}

		// Regular file → single-entry c4m
		entry := identifyFile(p, info, mode, shouldStore)
		combined.AddEntry(entry)
		if *quiet {
			if entry.C4ID.IsNil() {
				fmt.Fprintf(os.Stderr, "Error: no ID for %s (mode %q does not hash)\n", p, *modeFlag)
				failed = true
				continue
			}
			fmt.Println(entry.C4ID)
		}
	}

	if shouldStore && len(combined.Entries) > 0 {
		if s := getOrSetupStore(); s != nil {
			start := time.Now().UTC()
			id, size := storeManifestSelf(s, combined)
			syncStore(s)
			journalClaim(s, id, size, claimName(paths[0], true), claimOrigin(paths[0]), start)
			reportStored(id)
		}
	}

	if !*quiet {
		outputManifest(combined, *ergonomic)
	}
	if failed {
		os.Exit(1)
	}
}

func doStdin(storeFlag bool) {
	if storeFlag {
		s := getOrSetupStore()
		if s != nil {
			start := time.Now().UTC()
			id, size, isDesc, err := storeContentC4mAware(s, os.Stdin)
			if err != nil {
				fatalf("Error storing: %v", err)
			}
			syncStore(s)
			name := "stdin"
			if isDesc {
				name = "stdin.c4m"
			}
			journalClaim(s, id, size, name, "", start)
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

func scanDirectory(dirPath string, mode scan.ScanMode, seqFlag, shouldStore bool, excludes []string, excludeFile string, guide *c4m.Manifest, guideStart time.Time, verify bool) *c4m.Manifest {
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
	if guide != nil && !verify {
		opts = append(opts, scan.WithReuseGuide(guide, guideStart))
	}
	gen := scan.NewGeneratorWithOptions(opts...)
	scanStart := time.Now().UTC()
	manifest, err := gen.GenerateFromPath(dirPath)
	if err != nil {
		fatalf("Error scanning %s: %v", dirPath, err)
	}

	if guide != nil && !verify {
		r, h := gen.ReuseStats()
		fmt.Fprintf(os.Stderr, "reuse: %d reused, %d rehashed\n", r, h)
	}
	if guide != nil && verify {
		reportObfuscated(manifest, guide)
	}

	if shouldStore {
		storeManifestContent(manifest, dirPath, scanStart)
	}

	return manifest
}

// resolveGuideScanStart finds the guide's true scan start: the
// journal claim for the guide's root ID if the configured store has
// one, else the guide file's own mtime (conservative — never earlier
// than the scan it recorded).
func resolveGuideScanStart(guidePath string, guide *c4m.Manifest) time.Time {
	rootID := guide.ComputeC4ID()
	if s, _ := store.OpenStore(); s != nil {
		if r, ok := s.(interface{ Root() string }); ok {
			if claims, err := c4m.OpenJournal(r.Root()).Claims(); err == nil {
				var found time.Time
				for _, c := range claims {
					if c.ID == rootID {
						found = c.ScanStart // latest match wins
					}
				}
				if !found.IsZero() {
					return found
				}
			}
		}
	}
	if info, err := os.Stat(guidePath); err == nil {
		return info.ModTime().UTC()
	}
	return time.Time{} // zero: reuse disabled by the racy rule
}

// reportObfuscated compares a fully re-hashed scan against the guide
// and reports every file whose bytes changed while size and mtime
// stayed identical — the change class the default re-scan trust is
// documented not to see.
func reportObfuscated(m, guide *c4m.Manifest) {
	type meta struct {
		size int64
		ts   time.Time
		id   string
	}
	build := func(mm *c4m.Manifest) map[string]meta {
		out := make(map[string]meta)
		var stack []string
		for _, e := range mm.Entries {
			if e.Depth < len(stack) {
				stack = stack[:e.Depth]
			}
			if e.IsDir() {
				for len(stack) <= e.Depth {
					stack = append(stack, "")
				}
				stack[e.Depth] = strings.TrimSuffix(e.Name, "/") + "/"
				continue
			}
			if e.C4ID.IsNil() || e.Depth > len(stack) {
				continue
			}
			p := strings.Join(stack[:e.Depth], "") + e.Name
			out[p] = meta{e.Size, e.Timestamp.UTC().Truncate(time.Second), e.C4ID.String()}
		}
		return out
	}
	got := build(m)
	want := build(guide)
	for p, g := range got {
		w, ok := want[p]
		if !ok {
			continue
		}
		if g.size == w.size && g.ts.Equal(w.ts) && g.id != w.id {
			fmt.Fprintf(os.Stderr, "verify: content changed under unchanged metadata: %s\n", p)
		}
	}
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
		entry.Timestamp = c4m.NullTimestamp() // null renders as "-"
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

func storeManifestContent(manifest *c4m.Manifest, baseDir string, scanStart time.Time) {
	s := getOrSetupStore()
	if s == nil {
		return
	}

	// Collect work sequentially — path reconstruction is order-dependent.
	type fileItem struct {
		entry *c4m.Entry
		path  string // relative, for warnings
		full  string
	}
	var files []fileItem
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
			// Directory records are stored by storeManifestSelf after
			// all file entry IDs are final.
			continue
		}
		if entry.IsSequence {
			// A folded entry's name is a pattern, not a file on disk.
			// Expand to the member filenames and store each member's
			// bytes — otherwise a sequence snapshot silently stores
			// nothing for its frames.
			members, err := c4m.ExpandSequencePattern(entry.Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: cannot expand sequence %s: %v\n", entry.Name, err)
				continue
			}
			prefix := strings.Join(dirStack, "")
			for _, m := range members {
				relPath := prefix + m
				files = append(files, fileItem{nil, relPath, filepath.Join(baseDir, relPath)})
			}
			continue
		}
		if entry.C4ID.IsNil() || s.Has(entry.C4ID) {
			continue
		}
		relPath := strings.Join(dirStack, "") + entry.Name
		files = append(files, fileItem{entry, relPath, filepath.Join(baseDir, relPath)})
	}

	// Store file content on a bounded worker pool — objects are
	// independent and store.Put is safe for concurrent use.
	workers := runtime.GOMAXPROCS(0)
	if workers > 16 {
		workers = 16
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, it := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(it fileItem) {
			defer wg.Done()
			defer func() { <-sem }()
			if it.entry == nil {
				storeSequenceMember(s, it.path, it.full)
				return
			}
			storeFileEntry(s, it.entry, it.path, it.full)
		}(it)
	}
	wg.Wait()

	// The snapshot captures itself: directory records (deepest first)
	// plus the root record — stored by storeManifestSelf after all
	// file entry IDs are final. THE snapshot ID is the root ID.
	id, size := storeManifestSelf(s, manifest)

	// Batch barrier: the whole ingest becomes durable in one flush,
	// then the claim is journaled durably. Only after both may the ID
	// be reported — the print barrier.
	syncStore(s)
	journalClaim(s, id, size, claimName(baseDir, true), claimOrigin(baseDir), scanStart)
	reportStored(id)
}

// storeManifestSelf stores a snapshot's self-description: the root's
// one-level canonical record — whose stored bytes hash to the ROOT ID
// (ComputeC4ID), THE snapshot identity — plus a one-level record for
// every directory, deepest first, so the whole tree expands from the
// store alone (`c4 cat -r`, `c4 patch -r`). One description, one ID:
// the same value `-q` prints, the journal claims, and `stored:`
// reports. Returns the root ID and the root record's byte size.
func storeManifestSelf(s store.Store, m *c4m.Manifest) (c4.ID, int64) {
	// Directory records, deepest first so parents reference stored
	// children. Idempotent: existing records are skipped by ID.
	var dirs []*c4m.Entry
	for _, e := range m.Entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool { return dirs[i].Depth > dirs[j].Depth })
	for _, e := range dirs {
		if !e.C4ID.IsNil() && s.Has(e.C4ID) {
			continue
		}
		if id := storeDirectoryC4m(m, e, s); e.C4ID.IsNil() {
			e.C4ID = id
		}
	}

	// Root record: the canonical top-level view. Mirrors ComputeC4ID
	// (copy, canonicalize, canonical text) so the stored bytes hash to
	// the root ID.
	record := m.Copy()
	record.Canonicalize()
	text := record.Canonical()
	if text == "" {
		return c4.Identify(bytes.NewReader(nil)), 0
	}
	id, err := s.Put(strings.NewReader(text))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store root record: %v\n", err)
		return c4.ID{}, 0
	}
	return id, int64(len(text))
}

// reportStored prints the stored-manifest line to stderr. Called after
// the durability barrier so "stored" means durable. Stdout stays pure —
// stdout is the c4m.
func reportStored(id c4.ID) {
	if id.IsNil() {
		return
	}
	fmt.Fprintf(os.Stderr, "stored: %s\n", id)
}

// storeFileEntry stores one file's content, c4m-aware: c4m files are
// canonicalized before storing, and the entry's ID is updated when
// canonicalization changed it.
// storeSequenceMember stores one expanded member of a folded sequence
// entry. Members are raw content — Put computes each member's own ID.
func storeSequenceMember(s store.Store, relPath, fullPath string) {
	f, err := os.Open(fullPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot read sequence member %s: %v\n", relPath, err)
		return
	}
	defer f.Close()
	if _, err := s.Put(f); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store %s: %v\n", relPath, err)
	}
}

func storeFileEntry(s store.Store, entry *c4m.Entry, relPath, fullPath string) {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return // skip files we can't open
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
		return
	}
	if newID != entry.C4ID {
		entry.C4ID = newID
	}
}

// storeDirectoryC4m extracts a directory's direct children from a manifest
// and stores the resulting one-level c4m as content. This enables c4 cat <dir-id>
// to retrieve the directory listing, and c4 cat -r to recursively expand.
// Returns the record's C4 ID — the directory's own ID — which for an empty
// directory is the empty-content ID (nothing stored, matching scan).
//
// The stored c4m is canonical: only direct children at depth 0, sorted.
// This matches how directory C4 IDs are computed (one-level canonical form).
func storeDirectoryC4m(manifest *c4m.Manifest, dirEntry *c4m.Entry, s store.Store) c4.ID {
	children := manifest.Children(dirEntry)
	if len(children) == 0 {
		return c4.Identify(bytes.NewReader(nil))
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
		return c4.ID{}
	}

	id, err := s.Put(strings.NewReader(canonical))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store directory c4m for %s: %v\n", dirEntry.Name, err)
		return c4.ID{}
	}
	return id
}

// ingestStore memoizes the store handle for ingest: every Put in a
// command goes through one instance, so the final Sync barrier covers
// the whole batch. Also avoids re-reading config (and re-prompting)
// per file.
var (
	ingestStore     store.Store
	ingestStoreOpen bool
)

func getOrSetupStore() store.Store {
	if ingestStoreOpen {
		return ingestStore
	}
	ingestStoreOpen = true
	ingestStore = openOrSetupStore()
	return ingestStore
}

func openOrSetupStore() store.Store {
	s, err := store.OpenStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: store error: %v\n", err)
		return nil
	}
	if s != nil {
		return applyIngestSync(s)
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
		return applyIngestSync(s)
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
