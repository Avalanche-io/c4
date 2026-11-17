package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/internal/racy"
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
	// Without a store, -s refuses: through 1.0.23 it silently fell back
	// to a plain scan when stdin was not a terminal (or the prompt was
	// declined), printing no "stored:" line and exiting 0, so a script
	// believed its content was stored when nothing was.
	if shouldStore {
		if s := getOrSetupStore(); s == nil {
			fatalf("Error: -s needs a content store and none is configured: set C4_STORE=/path/to/store or s3://bucket/prefix, or run c4 id -s in a terminal to create %s", store.DefaultStorePath())
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
	start := racy.Start() // before any file argument is observed

	// On this line -q means "quiet": nothing is printed to stdout. On 1.1
	// it means "the ID only" and prints one line. A script moving between
	// the two gets silence with exit 0 here, so say so once on stderr
	// (found by absfs, 2026-11-09). With -s the stored: line already
	// narrates, so the silence is not silent.
	if *quiet && !shouldStore {
		fmt.Fprintln(os.Stderr, "c4 id -q: output suppressed; on 1.0.x -q prints nothing by design (on 1.1 it prints the ID; here the ID is the stored: line of c4 id -s)")
	}

	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil {
			fatalf("Error: %v", err)
		}

		if info.IsDir() {
			m := scanDirectory(p, mode, *seqFlag, false, scanExcludes, excludeFile, guide)
			partial := reportPartial(m, mode)
			// The store pass runs after the scan's own partial: lines, so a
			// file it gives up is named once, with why; it runs before the
			// description is printed, so what is printed is what was stored.
			if shouldStore && storeManifestContent(m, p) {
				partial = true
			}
			if !*quiet {
				outputManifest(m, *ergonomic)
			}
			// A scan that could not read every file is partial knowledge:
			// recorded with nulls, declared on stderr, and exit 2, so a
			// script can tell it from a complete scan. Through 1.0.23 it
			// exited 0 (measured on a mount with a non-local file,
			// 2026-11-07).
			if partial {
				os.Exit(2)
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
					id := storeManifestSelf(s, m)
					syncStore(s)
					reportStored(id)
				}
			}
			if !*quiet {
				outputManifest(m, *ergonomic)
			}
			return
		}

		// Regular file → single-entry c4m
		entry := identifyFile(p, info, mode, shouldStore, start)
		combined.AddEntry(entry)
	}

	if shouldStore {
		if s := getOrSetupStore(); s != nil {
			id := storeManifestSelf(s, combined)
			syncStore(s)
			reportStored(id)
		}
	}

	if !*quiet {
		outputManifest(combined, *ergonomic)
	}
}

func doStdin(storeFlag bool) {
	if storeFlag {
		s := getOrSetupStore()
		if s != nil {
			id, err := storeContentBytes(s, os.Stdin)
			if err != nil {
				fatalf("Error storing: %v", err)
			}
			syncStore(s)
			fmt.Println(id)
			return
		}
	}

	// The ID of exactly the bytes read: stdin holding c4m text is no
	// exception, so the ID summons that text (design/no-self-id.md on
	// master). Through 1.0.26 c4m text was identified re-rendered.
	fmt.Println(c4.Identify(os.Stdin))
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

// identifyFile describes one file argument. A regular file whose mtime
// second is at or after start is racy (see internal/racy): it is waited
// out and re-observed, and recorded with a null timestamp if it never
// settles, exactly as a directory scan records its files.
func identifyFile(path string, info os.FileInfo, mode scan.ScanMode, shouldStore bool, start time.Time) *c4m.Entry {
	entry := &c4m.Entry{
		Name: filepath.Base(path),
	}
	describeFile(entry, path, info, mode, shouldStore)
	if mode < scan.ModeMetadata || !info.Mode().IsRegular() || !racy.Is(info.ModTime(), start) {
		return entry
	}
	unsettled := racy.Settle(1,
		func(int) time.Time { return entry.Timestamp },
		func(int) (time.Time, bool) {
			before, err := os.Lstat(path)
			if err != nil || !before.Mode().IsRegular() {
				entry.Timestamp = c4m.NullTimestamp()
				return time.Time{}, true
			}
			describeFile(entry, path, before, mode, shouldStore)
			after, err := os.Lstat(path)
			if err != nil || !after.Mode().IsRegular() {
				entry.Timestamp = c4m.NullTimestamp()
				return time.Time{}, true
			}
			return after.ModTime(), after.Size() == before.Size() && after.ModTime().Equal(before.ModTime())
		})
	if len(unsettled) > 0 {
		entry.Timestamp = c4m.NullTimestamp()
	}
	return entry
}

// describeFile records one observation of a file argument in entry.
func describeFile(entry *c4m.Entry, path string, info os.FileInfo, mode scan.ScanMode, shouldStore bool) {
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
				entry.C4ID = storeFileBytes(s, path)
			} else {
				entry.C4ID = identifyFileBytes(path)
			}
		} else {
			entry.C4ID = identifyFileBytes(path)
		}
	}
}

// storePassHook runs between the scan and the store pass. A release build
// leaves it empty; a test build (-tags c4testhooks, testhooks.go) sets it
// so a test can change the tree or the store at exactly that point.
var storePassHook = func(baseDir string, s store.Store) {}

// mustPut stores r's bytes or ends the run. A store that cannot take an
// object leaves the description naming a C4 ID the store does not hold,
// so nothing may be reported after it (exit 1). Through 2026-11-16 each
// store-pass Put failure printed a warning and the run went on to exit 0.
func mustPut(s store.Store, r io.Reader, what string) c4.ID {
	id, err := s.Put(r)
	if err != nil {
		fatalf("Error: failed to store %s; nothing claimed: %v", what, err)
	}
	return id
}

// storeManifestContent stores a scanned tree's content and listings, then
// reports the stored description. It reports whether the store pass had to
// give up a file: one gone, unreadable, or no longer the bytes the scan
// hashed. Such a file is recorded as a scan that could not read it records
// it (a null C4 ID, its directories re-derived from the listings that hold
// it), each is named on stderr as partial:, and the caller exits 2. The
// description stored and printed then names no C4 ID the store does not
// hold. Through 2026-11-16 a file the store pass could not read was skipped
// silently and the run exited 0; one that had changed was nulled, but the
// directories above it kept IDs derived from its old bytes.
func storeManifestContent(manifest *c4m.Manifest, baseDir string) bool {
	s := getOrSetupStore()
	if s == nil {
		return false
	}
	storePassHook(baseDir, s)

	// Collect work sequentially — path reconstruction is order-dependent.
	type fileItem struct {
		entry *c4m.Entry
		path  string // relative, for warnings
		full  string
		seq   int // index into seqs, or -1 when not a sequence member
		pos   int // member position within that sequence, in range order
	}
	// Folded entries name an ID-list object built from their members'
	// IDs in range order; that object must be stored too, so members
	// are grouped by sequence and their IDs collected positionally.
	type seqGroup struct {
		entry   *c4m.Entry
		prefix  string   // the directory path the members are in
		names   []string // member file names, in range order
		members []c4.ID  // what the store pass stored for each
		sizes   []int64
		whys    []string // why a member could not be taken, or ""
	}
	var seqs []*seqGroup
	var files []fileItem
	var dirs []*c4m.Entry
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

			// The directory's c4m is stored as content (enables c4 cat -r).
			if !entry.C4ID.IsNil() && !s.Has(entry.C4ID) {
				dirs = append(dirs, entry)
			}
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
			g := &seqGroup{entry: entry, prefix: prefix, names: members,
				members: make([]c4.ID, len(members)), sizes: make([]int64, len(members)), whys: make([]string, len(members))}
			seqs = append(seqs, g)
			gi := len(seqs) - 1
			for i, m := range members {
				relPath := prefix + m
				files = append(files, fileItem{nil, relPath, filepath.Join(baseDir, relPath), gi, i})
			}
			continue
		}
		if entry.C4ID.IsNil() || s.Has(entry.C4ID) {
			continue
		}
		relPath := strings.Join(dirStack, "") + entry.Name
		files = append(files, fileItem{entry, relPath, filepath.Join(baseDir, relPath), -1, 0})
	}

	// Store file content on a bounded worker pool — objects are
	// independent and store.Put is safe for concurrent use.
	workers := runtime.GOMAXPROCS(0)
	if workers > 16 {
		workers = 16
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	lost := map[*c4m.Entry]string{}
	for _, it := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(it fileItem) {
			defer wg.Done()
			defer func() { <-sem }()
			if it.entry == nil {
				// Distinct slots, one writer each — no lock needed.
				g := seqs[it.seq]
				g.members[it.pos], g.sizes[it.pos], g.whys[it.pos] = storeSequenceMember(s, it.path, it.full)
				return
			}
			if why := storeFileEntry(s, it.entry, it.path, it.full); why != "" {
				mu.Lock()
				lost[it.entry] = why
				mu.Unlock()
			}
		}(it)
	}
	wg.Wait()

	// Every folded entry's ID-list object is stored after its members.
	// Without it the stored listing names content that is not in the
	// store and the description cannot resolve. A range with a member the
	// store pass could not take is unfolded instead.
	unfolded := false
	for _, g := range seqs {
		scanned := rangeMemberIDs(manifest, g.entry, len(g.names))
		for i := range g.names {
			if g.whys[i] == "" && (scanned == nil || g.members[i] != scanned[i]) {
				g.whys[i] = "changed since the scan"
			}
		}
		if scanned == nil && allTaken(g.whys) {
			if c4.Identify(bytes.NewReader(c4m.IDListBytes(g.members))) == g.entry.C4ID {
				for i := range g.whys {
					g.whys[i] = ""
				}
			}
		}
		if allTaken(g.whys) {
			if err := storeSequenceIDList(s, g.entry, g.members); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			continue
		}
		for e, why := range unfoldRange(manifest, g.entry, g.names, g.members, g.sizes, g.whys) {
			lost[e] = why
		}
		unfolded = true
	}
	if unfolded {
		manifest.SortEntries()
		manifest.InvalidateIndex()
	}
	if len(lost) > 0 {
		rederiveAbove(manifest, lost)
	}
	var lines []string
	for path, e := range c4m.EntryPaths(manifest.Entries) {
		if why, ok := lost[e]; ok {
			lines = append(lines, fmt.Sprintf("partial: %s recorded with nulls (%s)", path, why))
		}
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Fprintln(os.Stderr, l)
	}

	// Store directory c4m objects after all file entry IDs are final.
	for _, entry := range dirs {
		storeDirectoryC4m(manifest, entry, s)
	}

	// The snapshot captures itself: manifest text + root record.
	id := storeManifestSelf(s, manifest)

	// Batch barrier: the whole ingest becomes durable in one flush.
	syncStore(s)
	reportStored(id)
	return len(lost) > 0
}

func allTaken(whys []string) bool {
	for _, w := range whys {
		if w != "" {
			return false
		}
	}
	return true
}

var c4IDText = regexp.MustCompile(`c4[1-9A-HJ-NP-Za-km-z]{88}`)

// rangeMemberIDs returns the member C4 IDs a folded entry's ID list names,
// in range order: what the scan hashed. Nil when the description carries
// no list for the range, or one that does not hold n IDs.
func rangeMemberIDs(m *c4m.Manifest, e *c4m.Entry, n int) []c4.ID {
	list, ok := m.RangeData[e.C4ID]
	if !ok {
		return nil
	}
	texts := c4IDText.FindAllString(list, -1)
	if len(texts) != n {
		return nil
	}
	ids := make([]c4.ID, n)
	for i, t := range texts {
		id, err := c4.Parse(t)
		if err != nil {
			return nil
		}
		ids[i] = id
	}
	return ids
}

// unfoldRange replaces a folded entry, one of whose members the store pass
// could not take, with its members, as a scan that could not read that
// member records them: a member with no C4 ID blocks the fold (the
// maintainer's rule), and a range's frame IDs are never dropped (ruled
// 2026-11-16). Each member takes the range line's mode and timestamp,
// which the folded line states for every frame; a member taken keeps its
// C4 ID and size, one given up has both null. The range's ID list goes with
// the range. It returns the members given up, and why.
func unfoldRange(m *c4m.Manifest, rangeEntry *c4m.Entry, names []string, ids []c4.ID, sizes []int64, whys []string) map[*c4m.Entry]string {
	given := map[*c4m.Entry]string{}
	at := -1
	for i, e := range m.Entries {
		if e == rangeEntry {
			at = i
			break
		}
	}
	if at < 0 {
		return given
	}
	members := make([]*c4m.Entry, len(names))
	for i, name := range names {
		e := &c4m.Entry{Mode: rangeEntry.Mode, Timestamp: rangeEntry.Timestamp, Size: -1, Depth: rangeEntry.Depth, Name: name}
		if whys[i] == "" {
			e.C4ID, e.Size = ids[i], sizes[i]
		} else {
			given[e] = whys[i]
		}
		members[i] = e
	}
	rest := append([]*c4m.Entry{}, m.Entries[at+1:]...)
	m.Entries = append(append(m.Entries[:at], members...), rest...)
	delete(m.RangeData, rangeEntry.C4ID)
	return given
}

// rederiveAbove re-derives the size and C4 ID of every directory above an
// entry the store pass changed, deepest first, so each is derived from
// children already final: its size by PropagateMetadata over its own
// subtree, its ID from its listing, as the scanner derives them. A stated
// directory timestamp is kept. This line has no normaliser that re-derives
// stated aggregates (master's c4m.Normalize).
func rederiveAbove(m *c4m.Manifest, changed map[*c4m.Entry]string) {
	type span struct{ start, end int }
	var stack []int // indexes of the open directories
	above := map[int]bool{}
	for i, e := range m.Entries {
		for len(stack) > 0 && m.Entries[stack[len(stack)-1]].Depth >= e.Depth {
			stack = stack[:len(stack)-1]
		}
		if _, ok := changed[e]; ok {
			for _, d := range stack {
				above[d] = true
			}
		}
		if e.IsDir() {
			stack = append(stack, i)
		}
	}
	var dirs []span
	for i := range above {
		end := i + 1
		for end < len(m.Entries) && m.Entries[end].Depth > m.Entries[i].Depth {
			end++
		}
		dirs = append(dirs, span{i, end})
	}
	sort.Slice(dirs, func(a, b int) bool { return m.Entries[dirs[a].start].Depth > m.Entries[dirs[b].start].Depth })
	for _, d := range dirs {
		dir := m.Entries[d.start]
		dir.Size = -1
		c4m.PropagateMetadata(m.Entries[d.start:d.end])
		_, dir.C4ID = directoryRecord(m, dir)
	}
}

// storeManifestSelf stores a manifest's own description: its canonical
// text (retrievable by the manifest's C4 ID) and the scan root's
// one-level directory record (retrievable by the root directory's
// C4 ID, i.e. ComputeC4ID). Together with the per-directory records
// this makes a snapshot self-contained — content, structure, and the
// description itself all live in the store. Returns the manifest's ID.
func storeManifestSelf(s store.Store, m *c4m.Manifest) c4.ID {
	// A folded range's ID list is stored beside the listings, under its
	// own C4 ID, which is the range's: it is linked by ID from its line
	// as a subdirectory's listing is (ruled 2026-11-13 by the
	// maintainer; SPECIFICATION.md "Inline Range Data", "With store").
	// Through 2026-11-13 `c4 id -s FILE.c4m` did not store the lists.
	storeRangeLists(s, m)

	// A one-level record for every directory, deepest first, so the whole
	// tree expands from the store alone, as a scan with -s leaves it
	// (master's storeManifestSelf). Through 1.0.26 `c4 id -s FILE.c4m`
	// stored the file's text and the root record only, so no nested
	// listing could be fetched by its ID: "it would be an error to create
	// a c4m tree and drop the subdirectories" (the maintainer).
	// Idempotent: a record already in the store is skipped by ID. The
	// description itself is not changed; a directory with no children
	// listed stores nothing here, and its stated ID is kept as written.
	storeDirectoryRecords(s, m)

	data, err := c4m.Marshal(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to encode manifest: %v\n", err)
		return c4.ID{}
	}
	id := mustPut(s, bytes.NewReader(data), "the description")

	// Root record: the canonical top-level view. Mirrors ComputeC4ID
	// (copy, canonicalize, canonical text) so the stored bytes hash to
	// the root directory's ID.
	record := m.Copy()
	record.Canonicalize()
	// An empty root's record is the empty content, stored like any other.
	mustPut(s, strings.NewReader(record.Canonical()), "the root record")
	return id
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

// storeSequenceMember stores one expanded member of a folded sequence
// entry and returns the member's own ID. A nil ID means the member
// could not be stored, which makes the folded entry's ID-list
// unbuildable.
func storeSequenceMember(s store.Store, relPath, fullPath string) (c4.ID, int64, string) {
	f, err := os.Open(fullPath)
	if err != nil {
		return c4.ID{}, -1, unreadableWhy(err)
	}
	defer f.Close()
	cr := &countingReader{r: f}
	id := mustPut(s, cr, relPath)
	if cr.err != nil {
		return c4.ID{}, -1, "unreadable at the store pass"
	}
	return id, cr.n, ""
}

// countingReader counts the bytes read through it and keeps the first
// read error, so a member cut short is not mistaken for a whole one.
type countingReader struct {
	r   io.Reader
	n   int64
	err error
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if err != nil && err != io.EOF && c.err == nil {
		c.err = err
	}
	return n, err
}

// unreadableWhy names why the store pass could not open a file.
func unreadableWhy(err error) string {
	if os.IsNotExist(err) {
		return "gone at the store pass"
	}
	return "unreadable at the store pass"
}

// storeSequenceIDList stores the ID-list object a folded entry names.
// A folded entry's C4ID is the ID of its members' ID list, so without
// this object the stored listing references content that is not in the
// store and the description cannot resolve.
//
// memberIDs must be in the folded range's order. The rebuilt object is
// verified against the entry's own ID before it is trusted: a mismatch
// means the scan and the store disagree about the sequence, which is
// reported rather than silently stored under a different name.
func storeSequenceIDList(s store.Store, entry *c4m.Entry, memberIDs []c4.ID) error {
	for i, id := range memberIDs {
		if id.IsNil() {
			return fmt.Errorf("sequence %s: member %d was not stored", entry.Name, i+1)
		}
	}
	data := c4m.IDListBytes(memberIDs)
	if got := c4.Identify(bytes.NewReader(data)); got != entry.C4ID {
		return fmt.Errorf("sequence %s: rebuilt ID list is %s, entry names %s", entry.Name, got, entry.C4ID)
	}
	mustPut(s, bytes.NewReader(data), "the ID list of "+entry.Name)
	return nil
}

// storeFileEntry stores one file's content. A file inside a scanned
// tree is bytes, whatever its name and whatever its content parses as:
// storing a transformed copy loses the original and leaves the entry
// naming content that is not on disk.
// storeFileEntry stores the bytes a file entry names. It returns "" when
// the store holds them, else why it does not: the file is gone, unreadable,
// or no longer the bytes the scan hashed. The entry's C4 ID is then null,
// as a scan that could not read the file records it.
func storeFileEntry(s store.Store, entry *c4m.Entry, relPath, fullPath string) string {
	// A symlink to a directory carries the directory's ID, and the
	// directory's own entry stores that record; the link path has no
	// bytes of its own to store (reading it as a file is "is a directory").
	info, err := os.Stat(fullPath)
	if err == nil && info.IsDir() {
		return ""
	}
	if err != nil {
		entry.C4ID = c4.ID{}
		return unreadableWhy(err)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		entry.C4ID = c4.ID{}
		return unreadableWhy(err)
	}
	// The scan hashed these bytes; if they hash differently now, the file
	// changed under the scan. Record the entry unread rather than claiming
	// an ID for bytes that are no longer there.
	if c4.Identify(bytes.NewReader(data)) != entry.C4ID {
		entry.C4ID = c4.ID{}
		return "changed since the scan"
	}
	mustPut(s, bytes.NewReader(data), relPath)
	return ""
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
	// An empty directory's record is the empty content, and its line names
	// that ID, so it is stored like any other record (approved 2026-11-16,
	// option A). Through 2026-11-16 it was returned unstored, and a tree
	// with an empty directory and no empty file named the empty-content ID
	// without the store holding it.
	canonical, id := directoryRecord(manifest, dirEntry)
	if id.IsNil() {
		return id
	}
	return mustPut(s, strings.NewReader(canonical), "the listing of "+dirEntry.Name)
}

// directoryRecord returns a directory's one-level canonical record from its
// direct children in manifest, and the record's C4 ID, the directory's own.
// No children gives the empty record and the empty-content ID.
func directoryRecord(manifest *c4m.Manifest, dirEntry *c4m.Entry) (string, c4.ID) {
	children := manifest.Children(dirEntry)
	if len(children) == 0 {
		return "", c4.Identify(bytes.NewReader(nil))
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
		return "", c4.ID{}
	}
	return canonical, c4.Identify(strings.NewReader(canonical))
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
	return openOrSetupStoreFrom(os.Stdin, isTerminal())
}

// openOrSetupStoreFrom opens the configured store or, when none is
// configured and terminal says someone can answer, asks on stderr whether
// to create the default one, reading the answer from in. A typed line
// answers: Enter, "y" or "yes" is yes, anything else no. End of input with
// nothing read is no: stdin at /dev/null (cron, launchd, an agent) is a
// character device, so isTerminal reports it as a terminal, and through
// 1.0.26 its empty read counted as Enter and silently created ~/.c4/store
// and a config pointing at it.
func openOrSetupStoreFrom(in io.Reader, terminal bool) store.Store {
	s, err := store.OpenStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: store error: %v\n", err)
		return nil
	}
	if s != nil {
		return applyIngestSync(s)
	}

	// No store configured — offer to create default (local only).
	if !terminal {
		return nil
	}

	fmt.Fprintf(os.Stderr, "No content store configured. Create %s? [Y/n] ", store.DefaultStorePath())
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && answer == "" {
		// Nobody answered: create nothing.
		fmt.Fprintln(os.Stderr)
		return nil
	}
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

// reportPartial declares every file a scan could not read, recorded with
// a null ID, on stderr, and reports whether any exist. Only the full
// level can tell unread from unrecorded. (1.0.x: through 1.0.23 such a
// scan exited 0, so a script could not tell it from a complete one.)
func reportPartial(m *c4m.Manifest, mode scan.ScanMode) bool {
	if mode != scan.ModeFull {
		return false
	}
	found := false
	for path, e := range c4m.EntryPaths(m.Entries) {
		if e.IsDir() || e.IsSequence || e.Target != "" || !e.C4ID.IsNil() {
			continue
		}
		fmt.Fprintf(os.Stderr, "partial: %s recorded with nulls (unreadable)\n", path)
		found = true
	}
	return found
}

// storeRangeLists stores the ID list of every folded range m holds that m
// carries inline. A list m does not carry is not invented; a range line
// without its list stays valid, as a directory line in one-level form is.
func storeRangeLists(s store.Store, m *c4m.Manifest) {
	for _, e := range m.Entries {
		if !e.IsSequence || e.C4ID.IsNil() || s.Has(e.C4ID) {
			continue
		}
		list, ok := m.RangeData[e.C4ID]
		if !ok {
			continue
		}
		if id := mustPut(s, strings.NewReader(list), "the ID list of "+e.Name); id != e.C4ID {
			fmt.Fprintf(os.Stderr, "Warning: the ID list of %s hashes to %s, its line names %s\n", e.Name, id, e.C4ID)
		}
	}
}

// storeDirectoryRecords stores the one-level record of every directory in
// m that lists children, deepest first. A record already in the store is
// skipped by ID.
func storeDirectoryRecords(s store.Store, m *c4m.Manifest) {
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
		storeDirectoryC4m(m, e, s)
	}
}
