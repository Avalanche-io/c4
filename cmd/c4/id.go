package main

import (
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
	"github.com/Avalanche-io/c4/scan"
	"github.com/Avalanche-io/c4/store"
)

func runID(args []string) {
	fs := newFlags("id")
	fs.help(idHelp)
	storeFlag := fs.boolFlag("store", 's', false, "Snapshot into the store while streaming; the final line is the durable claim")
	quiet := fs.boolFlag("quiet", 'q', false, "Print one bare ID line per path (THE identity; byte-pure)")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Output ergonomic form c4m")
	seqFlag := fs.boolFlag("sequence", 'S', false, "Detect and fold file sequences")
	excludeFlags := fs.stringArrayFlag("exclude", "Glob pattern to exclude (repeatable)")
	excludeFileFlag := fs.stringFlag("exclude-file", 0, "", "File of exclude patterns (one per line)")
	modeFlag := fs.stringFlag("mode", 'm', "", "Detail level: s=structure, m=metadata, f=full (default), c=content projection")
	continueFlag := fs.stringFlag("continue", 'c', "", "Reuse guide: trust unchanged size+mtime from this c4m (racy-safe)")
	verifyFlag := fs.boolFlag("verify", 0, false, "Re-hash everything; with -c, report changes hidden under unchanged metadata")
	listFlag := fs.boolFlag("list", 'l', false, "List the .c4m files whose normalised text differs from their bytes; exit 1 if any")
	diffFlag := fs.boolFlag("diff", 'd', false, "Show the diff from each .c4m file's bytes to its normalised text; exit 1 if any")
	fs.parse(args)
	if *listFlag || *diffFlag {
		os.Exit(runIDCheck(fs.args, *listFlag, *diffFlag))
	}

	// -s snapshots at full detail, always. Without -q it streams the
	// listing and closes with the claim line; -s -q is the blessed
	// one-line capture (draft-v9 §3).
	if *storeFlag && *modeFlag != "" {
		fatalf("Error: -s always snapshots at full detail; -m conflicts with -s\n" +
			"For a durable content ID, snapshot first, then project the stored description:\n" +
			"  c4 cat -r \"$SNAP\" | c4 id -q -m c -")
	}

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

	modeStr := *modeFlag
	if modeStr == "" || *storeFlag {
		modeStr = "f"
	}
	mode, err := scan.ParseScanMode(modeStr)
	if err != nil {
		fatalf("Error: %v", err)
	}

	// -q prints THE identity, which only ID-bearing levels carry.
	if *quiet && mode != scan.ModeFull && mode != scan.ModeContent {
		fatalf("Error: -q needs an ID-bearing level: the default f, or c")
	}

	// One path per listing stream: without -q there is exactly one
	// stream and its final line is that path's root ID; with -q, one
	// bare line per succeeded path (pairing valid only on exit 0).
	if !*quiet && len(paths) > 1 {
		fatalf("Error: without -q, id takes one path per invocation — pass -q for one bare ID line per path")
	}

	shouldStore := *storeFlag
	// If -s is requested, ensure the store is configured before scanning.
	// This prompts the user immediately rather than after a long scan.
	// Without a store, -s refuses: through 2026-11-03 it silently fell
	// back to a plain scan when stdin was not a terminal (or the prompt
	// was declined), printing no "stored:" line and exiting 0, so a
	// script believed its content was stored when nothing was.
	if shouldStore {
		if s := getOrSetupStore(); s == nil {
			fatalf("Error: -s needs a content store; %s", noStoreHint())
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
	// so scripts capture THE identity without parsing prose. A partial
	// scan — unreadable entries recorded with nulls — still produces
	// its description and exits 2 (1 beats 2 when both occur).
	combined := c4m.NewManifest()
	failed := false
	partial := false

	for _, p := range paths {
		if p == "-" {
			// Parse stdin as a c4m description; -m projects downward.
			m, dropped := readStdinManifest(true)
			if dropped {
				fmt.Fprintln(os.Stderr, "normalized: -: trailing root ID removed (a c4m file never ends with its own ID)")
			}
			reportNormalized("-", c4m.Normalize(m))
			if mode == scan.ModeContent {
				m = scan.ProjectContent(m)
			}
			if shouldStore {
				if s := getOrSetupStore(); s != nil {
					start := time.Now().UTC()
					id, _ := storeManifestSelf(s, m)
					syncStore(s)
					journalClaim(s, id, start)
					reportStored(id)
				}
			}
			if *quiet {
				fmt.Println(m.ComputeC4ID())
				continue
			}
			// No trailing root ID: a c4m written from this output must
			// not end with its own ID (a file cannot contain its own ID;
			// Joshua, 2026-11-03). -q prints the ID alone.
			outputManifest(m, *ergonomic)
			return
		}

		// An argument whose first slash-separated component is
		// syntactically a C4 ID is a store address — and id never reads
		// the store (prefix ./ to force a filesystem path).
		if first := strings.SplitN(p, "/", 2)[0]; looksLikeC4ID(first) {
			fatalf("Error: %s is a store address and id never reads the store — use: c4 cat -r %s | c4 id -q -", p, first)
		}

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
			scanStart := time.Now().UTC()
			m := scanDirectory(p, mode, *seqFlag, false, scanExcludes, excludeFile, guide, guideStart, *verifyFlag)
			if reportPartial(m, mode) {
				partial = true
			}
			if *quiet {
				// THE ID of a directory scan is the identity of its
				// description — printed only after the claim is durable.
				if shouldStore && storeManifestContent(m, p, scanStart) {
					partial = true
				}
				fmt.Println(m.ComputeC4ID())
				continue
			}
			if *ergonomic {
				// -e implies buffered: aligned flat entries. No root-ID
				// line after them: a file written from this output must
				// not end with its own ID (design/no-self-id.md); -q is
				// the ID, and stored: on stderr is the -s narration.
				if shouldStore && storeManifestContent(m, p, scanStart) {
					partial = true
				}
				outputManifest(m, true)
			} else {
				// The streaming chain shape: the body streams as
				// description and ends on its last entry; nothing is
				// printed after it (design/no-self-id.md). Under -s the
				// claim is the -q line or the journal; stored: on stderr
				// narrates after the barrier. A dead stdout never cancels
				// a snapshot: ingest and journal complete, exit 1.
				//
				// Under -s the store pass runs first and the body is
				// written after it, so what is printed is what was stored:
				// a file the store pass gives up is null in both (decided
				// 2026-11-16; to be revisited for 1.1, since it changes
				// the -s streaming design). The
				// scan is complete before either, so nothing reaches stdout
				// earlier than it could; plain c4 id streams as before.
				// The unbuilt alternative keeps streaming and appends one
				// more patch section nulling the entries the store pass
				// gave up, so the chain resolves to what was stored.
				if shouldStore && storeManifestContent(m, p, scanStart) {
					partial = true
				}
				bodyErr := c4m.WriteChainBody(os.Stdout, m)
				if bodyErr != nil {
					fmt.Fprintf(os.Stderr, "Error: stdout write failed: %v", bodyErr)
					if shouldStore {
						fmt.Fprintf(os.Stderr, " (the snapshot is claimed and journaled; c4 log holds the ID)")
					}
					fmt.Fprintln(os.Stderr)
					os.Exit(1)
				}
			}
			if partial {
				os.Exit(2)
			}
			return
		}

		if strings.HasSuffix(p, ".c4m") {
			// c4m input → normalize to canonical (or ergonomic) form;
			// -m projects downward (a description computes at its own
			// knowledge level; projection only ever removes knowledge).
			// The normaliser is the migration for files that end with a
			// bare ID (written by `c4 diff >>` through 1.0.26): it reads
			// them, drops the line, says so, and writes the valid form.
			m, dropped, err := loadManifestMigrating(p)
			if err != nil {
				if *quiet {
					fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", p, err)
					failed = true
					continue
				}
				fatalf("Error loading %s: %v", p, err)
			}
			if dropped {
				fmt.Fprintf(os.Stderr, "normalized: %s: trailing root ID removed (a c4m file never ends with its own ID)\n", p)
			}
			// Normalise: every directory with listed children is re-derived
			// from them, so a hand edit computes to the library's own ID.
			reportNormalized(p, c4m.Normalize(m))
			if mode == scan.ModeContent {
				m = scan.ProjectContent(m)
			}
			// Store canonical c4m content if -s is set.
			if shouldStore {
				s := getOrSetupStore()
				if s != nil {
					start := time.Now().UTC()
					id, _ := storeManifestSelf(s, m)
					syncStore(s)
					journalClaim(s, id, start)
					reportStored(id)
				}
			}
			if *quiet {
				// THE ID of a description is the ID of its canonical
				// text at its own knowledge level.
				fmt.Println(m.ComputeC4ID())
				continue
			}
			// Resolved FLAT canonical text, the sanctioned flattener. No
			// trailing root ID: a c4m written from this output must not
			// end with its own ID (a file cannot contain its own ID;
			// Joshua, 2026-11-03). -q prints the ID alone.
			outputManifest(m, *ergonomic)
			return
		}

		// Regular file → single-entry c4m. A FILE argument's claim is
		// its bytes: stored, barriered, and journaled per path — the ID
		// prints only after its claim is durable.
		start := time.Now().UTC()
		entry, readErr := identifyFile(p, info, mode, shouldStore)
		if readErr != nil {
			// The file could not be read: it has no ID, and saying "does
			// not hash" would blame the level. A non-local file on a mount
			// fails this way (EIO partway through the read).
			fmt.Fprintf(os.Stderr, "Error: cannot read %s: %v\n", p, readErr)
			failed = true
			continue
		}
		combined.AddEntry(entry)
		if entry.C4ID.IsNil() {
			if *quiet {
				fmt.Fprintf(os.Stderr, "Error: no ID for %s (level %q does not hash)\n", p, modeStr)
				failed = true
				continue
			}
		} else if shouldStore {
			if s := getOrSetupStore(); s != nil {
				syncStore(s)
				journalClaim(s, entry.C4ID, start)
			}
		}
		if *quiet && !entry.C4ID.IsNil() {
			fmt.Println(entry.C4ID)
		}
	}

	if !*quiet {
		outputManifest(combined, *ergonomic)
	}
	if failed {
		os.Exit(1)
	}
	if partial {
		os.Exit(2)
	}
}

// reportPartial declares every entry a scan could not fully read —
// recorded with null fields — on stderr, and reports whether any
// exist. Only ID-bearing levels can tell unread from unrecorded.
func reportPartial(m *c4m.Manifest, mode scan.ScanMode) bool {
	if mode != scan.ModeFull && mode != scan.ModeContent {
		return false
	}
	found := false
	for path, e := range c4m.EntryPaths(m.Entries) {
		if e.IsSequence || e.Target != "" || !e.C4ID.IsNil() {
			continue
		}
		fmt.Fprintf(os.Stderr, "partial: %s recorded with nulls (unreadable)\n", path)
		found = true
	}
	return found
}

// manifestFromStdin parses stdin as a c4m description (`c4 patch -`);
// patch chains resolve to their final state, and a trailing bare ID is
// refused like every strict read.
func manifestFromStdin() *c4m.Manifest {
	m, _ := readStdinManifest(false)
	return m
}

// readStdinManifest parses stdin as a c4m description. With migrate (the
// `c4 id -` normaliser only) a trailing bare ID is verified as a
// checkpoint and dropped, and dropped reports it, exactly as
// `c4 id FILE.c4m` does; otherwise the decoder refuses it.
func readStdinManifest(migrate bool) (m *c4m.Manifest, dropped bool) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatalf("Error reading stdin: %v", err)
	}
	if c4m.IsJournal(data) {
		fatalf("Error: stdin is a store journal, not a description — list it with: c4 log <file>")
	}
	d := c4m.NewDecoder(bytes.NewReader(data))
	if migrate {
		d.AllowTrailingID()
	}
	m, err = d.Decode()
	if err != nil {
		fatalf("Error parsing stdin as c4m: %v", err)
	}
	return m, d.DroppedTrailingID()
}

func doStdin(storeFlag bool) {
	if storeFlag {
		s := getOrSetupStore()
		if s == nil {
			// Never print an ID as if stored when nothing was: a script
			// would believe its content was kept.
			fatalf("Error: storing stdin needs a content store; %s", noStoreHint())
		}
		if s != nil {
			start := time.Now().UTC()
			id, _, err := storeContentBytes(s, os.Stdin)
			if err != nil {
				fatalf("Error storing: %v", err)
			}
			syncStore(s)
			journalClaim(s, id, start)
			fmt.Println(id)
			return
		}
	}

	// Identify means the bytes: stdin is never parsed here (c4 id -
	// parses a description), so a c4m file's ID is the ID that
	// summons it (design/no-self-id.md).
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatalf("Error reading stdin: %v", err)
	}
	id := c4.Identify(bytes.NewReader(data))
	fmt.Println(id)
}

func scanDirectory(dirPath string, mode scan.ScanMode, seqFlag, shouldStore bool, excludes []string, excludeFile string, guide *c4m.Manifest, guideStart time.Time, verify bool) *c4m.Manifest {
	// Eval-side projection (draft-v9 §1): the scanner always captures
	// full fidelity when it hashes; the content level is a projection
	// applied to the result, never a generation mode.
	project := mode == scan.ModeContent
	if project {
		mode = scan.ModeFull
	}
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

	if project {
		manifest = scan.ProjectContent(manifest)
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
		if root, ok := journalRoot(s); ok {
			// A journal that can't be read is an error, not a missing
			// claim: swallowing it would turn every -c warm re-scan into
			// a cold one without a word, and hide a journal of a format
			// this c4 doesn't know (c4 issue #44).
			j := c4m.OpenJournal(root)
			claims, err := j.Claims()
			if err != nil {
				fatalf("Error reading %s: %v", j.Path(), err)
			}
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

func identifyFile(path string, info os.FileInfo, mode scan.ScanMode, shouldStore bool) (*c4m.Entry, error) {
	var readErr error
	entry := &c4m.Entry{
		Name: filepath.Base(path),
	}

	switch mode {
	case scan.ModeContent:
		// The content projection: mode and timestamp null entirely;
		// size is the byte count read and hashed.
		entry.Size = info.Size()
		entry.Timestamp = c4m.NullTimestamp()
	case scan.ModeMetadata, scan.ModeFull:
		entry.Mode = info.Mode()
		entry.Timestamp = info.ModTime().UTC()
		entry.Size = info.Size()
	default:
		entry.Size = -1
		entry.Timestamp = c4m.NullTimestamp() // null renders as "-"
	}

	if mode == scan.ModeFull || mode == scan.ModeContent {
		// A file's ID is the ID of its bytes, a .c4m file included
		// (design/no-self-id.md). A file that cannot be read has no ID:
		// the entry stays null and the error is reported to the caller.
		if shouldStore {
			if s := getOrSetupStore(); s != nil {
				entry.C4ID = storeFileBytes(s, path)
			} else {
				entry.C4ID, readErr = identifyFileBytes(path)
			}
		} else {
			entry.C4ID, readErr = identifyFileBytes(path)
		}
	}

	return entry, readErr
}

// storePassHook runs between the scan and the store pass. A release build
// leaves it empty; a test build (-tags c4testhooks, testhooks.go) sets it
// so a test can change the tree or the store at exactly that point.
var storePassHook = func(baseDir string, s store.Store) {}

// mustPut stores r's bytes or ends the run. A store that cannot take an
// object leaves the description naming a C4 ID the store does not hold,
// so nothing may be claimed, journaled or reported after it: the run dies
// before the print barrier (exit 1). Through 2026-11-16 each store-pass
// Put failure printed a warning and the run went on, journaled nothing
// (the root record was among the failures) and exited 0.
func mustPut(s store.Store, r io.Reader, what string) c4.ID {
	id, err := s.Put(r)
	if err != nil {
		fatalf("Error: failed to store %s; nothing claimed: %v", what, err)
	}
	return id
}

// storeManifestContent stores a scanned tree's content and listings, then
// journals and reports the claim. It reports whether the store pass had to
// give up a file: one gone, unreadable, or no longer the bytes the scan
// hashed. Such a file is recorded as a scan that could not read it records
// it (a null C4 ID, its ancestors re-derived from the listings that hold
// it), each is named on stderr as partial:, and the caller exits 2. The
// description stored, printed and journaled then names no C4 ID the store
// does not hold. Through 2026-11-16 a file the store pass could not read
// was skipped silently, and one that had changed took the new ID unseen,
// so `c4 id -s` exited 0 naming content the store did not have.
func storeManifestContent(manifest *c4m.Manifest, baseDir string, scanStart time.Time) bool {
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
	// Folded entries name an ID-list object built from their members' IDs
	// in range order; that object must be stored too, so members are
	// grouped by sequence and their IDs collected positionally.
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
				// The scan folded this range, so its pattern expands; if
				// it does not, its frames and list cannot be stored and
				// the line would name content the store lacks.
				fatalf("Error: cannot expand the sequence %s: %v; nothing claimed", entry.Name, err)
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
	var lost []lostFile
	for _, it := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(it fileItem) {
			defer wg.Done()
			defer func() { <-sem }()
			if it.entry == nil {
				// Distinct slots, one writer each: no lock needed.
				g := seqs[it.seq]
				g.members[it.pos], g.sizes[it.pos], g.whys[it.pos] = storeSequenceMember(s, it.path, it.full)
				return
			}
			if why := storeFileEntry(s, it.entry, it.path, it.full); why != "" {
				mu.Lock()
				lost = append(lost, lostFile{it.path, why})
				mu.Unlock()
			}
		}(it)
	}
	wg.Wait()

	// Every folded entry's ID-list object is stored after its members.
	// Without it the stored listing names content that is not in the store
	// and the description cannot resolve (release 1.0.x: 7bac715). A range
	// with a member the store pass could not take is unfolded instead.
	unfolded := false
	for _, g := range seqs {
		scanned := rangeMemberIDs(manifest, g.entry, len(g.names))
		for i := range g.names {
			if g.whys[i] == "" && (scanned == nil || g.members[i] != scanned[i]) {
				g.whys[i] = "changed since the scan"
			}
		}
		if scanned == nil && allTaken(g.whys) {
			// No list in the description to check members against: the
			// rebuilt list must be the one the range names.
			if c4.Identify(bytes.NewReader(c4m.IDListBytes(g.members))) == g.entry.C4ID {
				for i := range g.whys {
					g.whys[i] = ""
				}
			}
		}
		if allTaken(g.whys) {
			if err := storeSequenceIDList(s, g.entry, g.members); err != nil {
				// Every member was taken and checked against the scan's
				// list, so this cannot fail; if it does, the range would
				// name a list the store does not hold.
				fatalf("Error: %v; nothing claimed", err)
			}
			continue
		}
		unfoldRange(manifest, g.entry, g.names, g.members, g.sizes, g.whys)
		unfolded = true
		for i, why := range g.whys {
			if why != "" {
				lost = append(lost, lostFile{g.prefix + g.names[i], why})
			}
		}
	}

	// A file given up changes the listings above it: re-derive every
	// directory's size and ID from its children, the one derivation the
	// scanner and the normaliser use, before any listing is stored.
	if unfolded {
		manifest.SortEntries()
		manifest.InvalidateIndex()
	}
	if len(lost) > 0 {
		c4m.Normalize(manifest)
	}
	sort.Slice(lost, func(i, j int) bool { return lost[i].path < lost[j].path })
	for _, l := range lost {
		fmt.Fprintf(os.Stderr, "partial: %s recorded with nulls (%s)\n", l.path, l.why)
	}

	// The snapshot captures itself: directory records (deepest first)
	// plus the root record — stored by storeManifestSelf after all
	// file entry IDs are final. THE snapshot ID is the root ID.
	id, _ := storeManifestSelf(s, manifest)

	// Batch barrier: the whole ingest becomes durable in one flush,
	// then the claim is journaled durably. Only after both may the ID
	// be reported — the print barrier.
	syncStore(s)
	journalClaim(s, id, scanStart)
	reportStored(id)
	return len(lost) > 0
}

// lostFile is a file the store pass had to give up, and why.
type lostFile struct{ path, why string }

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
// 2026-11-16, brief E Q1). Each member takes the range line's mode and
// timestamp, which the folded line states for every frame; a member taken
// keeps its C4 ID and size, one given up has both null. The range's ID list
// goes with the range.
func unfoldRange(m *c4m.Manifest, rangeEntry *c4m.Entry, names []string, ids []c4.ID, sizes []int64, whys []string) {
	at := -1
	for i, e := range m.Entries {
		if e == rangeEntry {
			at = i
			break
		}
	}
	if at < 0 {
		return
	}
	members := make([]*c4m.Entry, len(names))
	for i, name := range names {
		e := &c4m.Entry{Mode: rangeEntry.Mode, Timestamp: rangeEntry.Timestamp, Size: -1, Depth: rangeEntry.Depth, Name: name}
		if whys[i] == "" {
			e.C4ID, e.Size = ids[i], sizes[i]
		}
		members[i] = e
	}
	rest := append([]*c4m.Entry{}, m.Entries[at+1:]...)
	m.Entries = append(append(m.Entries[:at], members...), rest...)
	delete(m.RangeData, rangeEntry.C4ID)
}

// storeManifestSelf stores a snapshot's self-description: the root's
// one-level canonical record — whose stored bytes hash to the ROOT ID
// (ComputeC4ID), THE snapshot identity — plus a one-level record for
// every directory, deepest first, so the whole tree expands from the
// store alone (`c4 cat -r`, `c4 patch -r`). One description, one ID:
// the same value `-q` prints, the journal claims, and `stored:`
// reports. Returns the root ID and the root record's byte size.
func storeManifestSelf(s store.Store, m *c4m.Manifest) (c4.ID, int64) {
	// A folded range's ID list is stored beside the listings, under its
	// own C4 ID, which is the range's: it is linked by ID from its line
	// as a subdirectory's listing is (ruled 2026-11-13 by the
	// maintainer; SPECIFICATION.md "Inline Range Data", "With store").
	// Through 2026-11-13 `c4 id -s FILE.c4m` stored the listings and not
	// the lists.
	storeRangeLists(s, m)

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
	// An empty root's record is the empty content: stored like any other
	// record, so the ID reported for it returns through c4 cat.
	id := mustPut(s, strings.NewReader(text), "the root record")
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
// entry and returns the member's own ID and size, or why the store pass
// could not take it (its ID is then nil).
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

// storeSequenceIDList stores the ID-list object a folded entry names. The
// rebuilt object is verified against the entry's own ID before it is
// stored: a mismatch means the scan and the store disagree about the
// sequence, which is reported rather than stored under another name.
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
	// The bytes, whatever the name: a .c4m file inside a tree is stored
	// as itself, so its entry ID returns it and restore writes it back
	// byte for byte (design/no-self-id.md). Bytes that are not the ones
	// the scan hashed are not stored under the entry: the line would name
	// one C4 ID and the store hold another.
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
	// One derivation of a directory's identity: c4m.DirectoryRecord, the
	// same the scanner and the normaliser use.
	// An empty directory's record is the empty content, and its line names
	// that ID, so it is stored like any other record (approved 2026-11-16,
	// option A). Through 2026-11-16 it was returned unstored, and a tree
	// with an empty directory and no empty file named the empty-content ID
	// without the store holding it.
	canonical, _ := c4m.DirectoryRecord(manifest, dirEntry)
	return mustPut(s, strings.NewReader(canonical), "the listing of "+dirEntry.Name)
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
	// Every path that stores and claims opens its store here first, so a
	// journal of a format this c4 doesn't know is refused before anything
	// is ingested: nothing is stored, and no v1 line is appended under a
	// newer header (c4 issue #44). It is the journal -s writes: a local
	// store's, or a MultiStore's write store's (journalRoot).
	if root, ok := journalRoot(ingestStore); ok {
		if err := c4m.OpenJournal(root).CheckVersion(); err != nil {
			fatalf("Error: %s: %v", c4m.OpenJournal(root).Path(), err)
		}
	}
	return ingestStore
}

// openOrSetupStore opens the configured content store, or returns nil when
// none is configured. It never prompts and never creates a store: a
// composable verb runs the same in a terminal as in a script, and a store
// is configured on purpose (noStoreHint). Through 2026-11-13 it asked, in a
// terminal, whether to create ~/.c4/store, and created it on any answer
// but "n" (decided 2026-11-13 by the release lead; reversible).
func openOrSetupStore() store.Store {
	s, err := store.OpenStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: store error: %v\n", err)
		return nil
	}
	if s != nil {
		return applyIngestSync(s)
	}
	return nil
}

// noStoreHint says how to configure a content store, for the verbs that
// refuse without one.
func noStoreHint() string {
	return "no content store is configured: set C4_STORE=" + store.DefaultStorePath() +
		" (any directory; created on first use) or C4_STORE=s3://bucket/prefix/, or put a store line in ~/.c4/config"
}

func outputManifest(manifest *c4m.Manifest, ergonomic bool) {
	enc := c4m.NewEncoder(os.Stdout)
	if ergonomic {
		enc.SetPretty(true)
	}
	enc.Encode(manifest)
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
		// RangeData is keyed by the list's own ID, so the stored list is
		// the one the line names; if not, the line names a list the store
		// does not hold.
		if id := mustPut(s, strings.NewReader(list), "the ID list of "+e.Name); id != e.C4ID {
			fatalf("Error: the ID list of %s hashes to %s, its line names %s; nothing claimed", e.Name, id, e.C4ID)
		}
	}
}
