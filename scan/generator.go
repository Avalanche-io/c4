package scan

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// ScanMode controls how much information is gathered during a scan.
type ScanMode int

const (
	ModeStructure ScanMode = iota // names and hierarchy only
	ModeMetadata                  // structure + permissions, timestamps, sizes
	ModeFull                      // structure + metadata + C4 IDs
	// ModeContent is the content-only projection (decided 2026-07-09,
	// permanent): names, sizes, symlink targets, and C4 IDs real; mode
	// and timestamp null at EVERY level, directory entries included.
	// Two trees with byte-identical content produce equal IDs on any
	// machine, clock, or umask. An ID does not self-describe its
	// projection; comparisons must be like-mode.
	ModeContent
)

// defaultConcurrencyCap caps the worker pool regardless of GOMAXPROCS so we
// don't oversubscribe storage with hundreds of concurrent readdir calls.
const defaultConcurrencyCap = 16

// ParseScanMode parses a mode string: "s"/"1" → structure, "m"/"2" →
// metadata, "f"/"3" → full, "c"/"4" → content (metadata-independent).
func ParseScanMode(s string) (ScanMode, error) {
	switch strings.ToLower(s) {
	case "s", "1", "structure":
		return ModeStructure, nil
	case "m", "2", "metadata":
		return ModeMetadata, nil
	case "f", "3", "full", "":
		return ModeFull, nil
	case "c", "4", "content":
		return ModeContent, nil
	default:
		return ModeFull, fmt.Errorf("unknown scan mode %q (use s/m/f/c or 1/2/3/4)", s)
	}
}

// Generator creates C4M manifests from filesystem paths
type Generator struct {
	mode            ScanMode
	followSymlinks  bool
	includeHidden   bool
	detectSequences bool
	excludePatterns []string
	excludeFile     string          // explicit exclude file path
	excludeFileName string          // filename to look for in scanned dirs (from env)
	guide           map[string]bool // paths from guide c4m (nil = no guide)
	scanRoot        string
	progress        *progress     // nil = no progress reporting (zero-cost path)
	maxConcurrency  int           // 0 = auto, 1 = sequential, n > 1 = bounded parallel
	sem             chan struct{} // worker-pool slots; nil for sequential

	ctx      context.Context        // cancellation; nil means no cancellation
	streamCB func(*c4m.Entry) error // fires per discovered entry; nil disables streaming
	streamMu sync.Mutex             // serializes streamCB calls under parallel walk

	// dirIdentity, when non-nil, replaces canonicalDirID as the function
	// deriving a directory's C4 ID from its fully-resolved direct children.
	// This is the seam for alternate canonicalizations (e.g. a future
	// content mode hashing null-stat entry lines): install a different
	// function, nothing else in the walk changes.
	dirIdentity func(children []*c4m.Entry) c4.ID

	// Metadata-trusted re-scan (design/snapshot-loop v8 Amendment 1):
	// a file's prior ID is reused iff its path exists in the reuse
	// guide, size and mtime match at stored precision, AND its mtime is
	// strictly older than the guide's own scan start (the racy rule —
	// no invented constants; correctness never depends on timestamp
	// granularity). Reuse extends trust only to the DESCRIPTION of the
	// live tree; it never causes an unverified byte to enter a store.
	reuseGuide     map[string]*c4m.Entry // root-relative file path → guide entry
	reuseScanStart time.Time             // guide's scan start, second precision
	reused         int64                 // atomic
	rehashed       int64                 // atomic
}

// NewGenerator creates a new manifest generator
func NewGenerator() *Generator {
	return &Generator{
		mode:            ModeFull,
		followSymlinks:  false,
		includeHidden:   true,
		detectSequences: false,
		excludeFileName: os.Getenv("C4_EXCLUDE_FILE"),
	}
}

// GeneratorOption configures a Generator
type GeneratorOption func(*Generator)

// WithC4IDs enables/disables C4 ID computation (shorthand for WithMode).
func WithC4IDs(compute bool) GeneratorOption {
	return func(g *Generator) {
		if compute {
			g.mode = ModeFull
		} else {
			g.mode = ModeMetadata
		}
	}
}

// WithMode sets the scan mode (structure, metadata, or full).
func WithMode(mode ScanMode) GeneratorOption {
	return func(g *Generator) {
		g.mode = mode
	}
}

// WithSymlinks enables/disables following symlinks
func WithSymlinks(follow bool) GeneratorOption {
	return func(g *Generator) {
		g.followSymlinks = follow
	}
}

// WithHidden enables/disables including hidden files
func WithHidden(include bool) GeneratorOption {
	return func(g *Generator) {
		g.includeHidden = include
	}
}

// WithSequenceDetection enables/disables sequence detection
func WithSequenceDetection(detect bool) GeneratorOption {
	return func(g *Generator) {
		g.detectSequences = detect
	}
}

// WithExclude adds glob patterns to exclude from scanning.
func WithExclude(patterns []string) GeneratorOption {
	return func(g *Generator) {
		g.excludePatterns = append(g.excludePatterns, patterns...)
	}
}

// WithExcludeFile sets an explicit exclude file to load patterns from.
func WithExcludeFile(path string) GeneratorOption {
	return func(g *Generator) {
		g.excludeFile = path
	}
}

// WithProgress registers a callback that receives periodic scan stats.
// The callback fires at most every 1000 entries or every 250ms, whichever
// comes first, plus once at the end of the scan. The ScanStats argument is a
// value copy — the callback may keep it freely.
func WithProgress(cb func(ScanStats)) GeneratorOption {
	return func(g *Generator) {
		if cb != nil {
			g.progress = newProgress(cb)
		}
	}
}

// WithMaxConcurrency caps the number of concurrent subdirectory walks.
//
//	n =  0: auto (min(GOMAXPROCS, 16))
//	n =  1: purely sequential (preserves pre-parallelism behavior)
//	n >  1: explicit cap
//
// Output is byte-identical regardless of n — children of each parent are
// stitched back in their post-sort order.
func WithMaxConcurrency(n int) GeneratorOption {
	return func(g *Generator) {
		g.maxConcurrency = n
	}
}

// WithContext attaches a context to the scan. Cancellation is observed at
// every directory boundary and between entries within a directory. The
// returned partial manifest from Dir/GenerateFromPath will reflect work
// completed up to the cancellation point.
func WithContext(ctx context.Context) GeneratorOption {
	return func(g *Generator) {
		g.ctx = ctx
	}
}

// WithEntryStream installs a callback that fires once per discovered entry
// before it is added to the manifest. Returning a non-nil error halts the
// scan; that error is returned by Dir/GenerateFromPath alongside the
// partial manifest collected so far.
//
// Under parallel walk (the default) the callback may be invoked from
// multiple goroutines but is serialized by an internal mutex, so the
// callback body itself does not need to be thread-safe. Order is the
// discovery order produced by the worker pool, which is non-deterministic
// across runs; pair with WithMaxConcurrency(1) for a deterministic order.
//
// Directory entries stream after their children (post-order): a directory's
// Size, Timestamp, and C4 ID are resolved bottom-up from its children, so
// emitting it afterwards means every streamed entry is fully resolved.
func WithEntryStream(cb func(*c4m.Entry) error) GeneratorOption {
	return func(g *Generator) {
		g.streamCB = cb
	}
}

// WithGuide sets an existing manifest as a guide. Only entries present
// in the guide will be included in the scan. This enables the
// scan-filter-continue workflow.
func WithGuide(m *Manifest) GeneratorOption {
	return func(g *Generator) {
		g.guide = buildGuideSet(m)
	}
}

// WithReuseGuide enables the metadata-trusted re-scan: files whose
// path, size, and mtime match the guide — and whose mtime is strictly
// older than the guide's scan start — reuse the guide's recorded ID
// without re-reading their bytes. Everything else (new, changed, or
// racy) is re-hashed. The guide must be full-fidelity (recorded sizes
// and mtimes); a content-projection guide reuses nothing.
func WithReuseGuide(m *Manifest, scanStart time.Time) GeneratorOption {
	return func(g *Generator) {
		g.reuseGuide = buildReuseMap(m)
		g.reuseScanStart = scanStart.UTC().Truncate(time.Second)
	}
}

// buildReuseMap maps root-relative file paths to guide entries that
// are trustable: regular files with real IDs, sizes, and timestamps.
func buildReuseMap(m *Manifest) map[string]*c4m.Entry {
	out := make(map[string]*c4m.Entry)
	var dirStack []string
	for _, e := range m.Entries {
		if e.Depth < len(dirStack) {
			dirStack = dirStack[:e.Depth]
		}
		if e.IsDir() {
			for len(dirStack) <= e.Depth {
				dirStack = append(dirStack, "")
			}
			name := e.Name
			if !strings.HasSuffix(name, "/") {
				name += "/"
			}
			dirStack[e.Depth] = name
			continue
		}
		if e.IsSequence || e.Target != "" || e.C4ID.IsNil() ||
			e.Size < 0 || e.Timestamp.Equal(c4m.NullTimestamp()) {
			continue
		}
		out[strings.Join(dirStack[:min(e.Depth, len(dirStack))], "")+e.Name] = e
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// reuseID applies the trust rule for one regular file.
func (g *Generator) reuseID(path string, info os.FileInfo) (c4.ID, bool) {
	if g.reuseGuide == nil {
		return c4.ID{}, false
	}
	e, ok := g.reuseGuide[relFromRoot(g.scanRoot, path)]
	if !ok || e.Size != info.Size() {
		return c4.ID{}, false
	}
	mtime := info.ModTime().UTC().Truncate(time.Second)
	if !mtime.Equal(e.Timestamp.UTC().Truncate(time.Second)) {
		return c4.ID{}, false
	}
	// The racy rule: a file whose mtime is not strictly older than the
	// guide's scan start could have been written during or after that
	// scan within timestamp granularity — re-hash it regardless.
	if g.reuseScanStart.IsZero() || !mtime.Before(g.reuseScanStart) {
		return c4.ID{}, false
	}
	return e.C4ID, true
}

// ReuseStats reports how many files reused guide IDs versus re-hashed
// during the last scan. Zero values when no reuse guide was set.
func (g *Generator) ReuseStats() (reused, rehashed int64) {
	return atomic.LoadInt64(&g.reused), atomic.LoadInt64(&g.rehashed)
}

// buildGuideSet extracts all paths from a manifest into a lookup set.
func buildGuideSet(m *Manifest) map[string]bool {
	set := make(map[string]bool)
	var dirStack []string
	for _, entry := range m.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		var fullPath string
		if entry.Depth > 0 && entry.Depth <= len(dirStack) {
			prefix := ""
			for i := 0; i < entry.Depth; i++ {
				prefix += dirStack[i]
			}
			fullPath = prefix + entry.Name
		} else {
			fullPath = entry.Name
		}
		set[fullPath] = true
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}
	return set
}

// NewGeneratorWithOptions creates a generator with options
func NewGeneratorWithOptions(opts ...GeneratorOption) *Generator {
	g := NewGenerator()
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// clone creates a copy with the same settings but fresh state. Clones are
// used only for symlink-target sub-scans, which are rooted outside the main
// walk. They share the parent's semaphore so the global concurrency cap is
// honored, and drop the progress reporter and the entry-stream callback to
// avoid double-counting / double-emit. The context IS propagated so a single
// cancellation halts the entire scan, including sub-scans.
//
// The guide is deliberately NOT copied: guide paths are relative to the main
// scan root, so applying them to a scan rooted elsewhere would spuriously
// filter every entry and yield empty (wrong) results.
func (g *Generator) clone() *Generator {
	clone := &Generator{
		mode:            g.mode,
		followSymlinks:  g.followSymlinks,
		includeHidden:   g.includeHidden,
		detectSequences: g.detectSequences,
		excludeFile:     g.excludeFile,
		excludeFileName: g.excludeFileName,
		maxConcurrency:  g.maxConcurrency,
		sem:             g.sem,
		ctx:             g.ctx,
		dirIdentity:     g.dirIdentity,
	}
	if len(g.excludePatterns) > 0 {
		clone.excludePatterns = make([]string, len(g.excludePatterns))
		copy(clone.excludePatterns, g.excludePatterns)
	}
	return clone
}

// resolveConcurrency returns the worker-pool size to use.
func (g *Generator) resolveConcurrency() int {
	if g.maxConcurrency == 1 {
		return 1
	}
	if g.maxConcurrency > 1 {
		return g.maxConcurrency
	}
	n := runtime.GOMAXPROCS(0)
	if n > defaultConcurrencyCap {
		n = defaultConcurrencyCap
	}
	if n < 1 {
		n = 1
	}
	return n
}

// ctxErr returns a non-nil error if the attached context (if any) is done.
func (g *Generator) ctxErr() error {
	if g.ctx == nil {
		return nil
	}
	return g.ctx.Err()
}

// emit fires the per-entry stream callback (if any). Serialized via
// streamMu so callers don't need to worry about thread-safety even under
// parallel walk. A non-nil callback error halts the scan; it is propagated
// up to GenerateFromPath which packages it with the partial manifest.
func (g *Generator) emit(e *Entry) error {
	if g.streamCB == nil {
		return nil
	}
	g.streamMu.Lock()
	defer g.streamMu.Unlock()
	return g.streamCB(e)
}

// errReturn packages a partial manifest with an error. When streaming or a
// context is configured, callers want the partial result back so they can
// persist it / inspect it; otherwise we preserve the historical nil-on-
// error contract for backward compatibility.
func (g *Generator) errReturn(m *Manifest, err error) (*Manifest, error) {
	if g.ctx != nil || g.streamCB != nil {
		return m, err
	}
	return nil, err
}

// GenerateFromPath creates a manifest from a filesystem path
func (g *Generator) GenerateFromPath(path string) (*Manifest, error) {
	manifest := NewManifest()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	g.scanRoot = absPath

	// Initialize semaphore for the worker pool. Sub-scans (clone()) inherit
	// this same semaphore so the cap is enforced across the entire walk.
	if g.sem == nil {
		if n := g.resolveConcurrency(); n > 1 {
			g.sem = make(chan struct{}, n)
		}
	}

	// Load exclude patterns from file if specified.
	if g.excludeFile != "" {
		g.loadExcludeFile(g.excludeFile)
	}
	// Load exclude patterns from env-named file in scanned directory.
	if g.excludeFileName != "" && info.IsDir() {
		g.loadExcludeFile(filepath.Join(absPath, g.excludeFileName))
	}

	if info.IsDir() {
		entries, walkErr := g.generateDir(absPath, "", 0)
		// Even on cancel/error, append whatever entries we managed to
		// collect — callers want a partial manifest from a streaming run.
		for _, e := range entries {
			manifest.AddEntry(e)
		}
		if walkErr != nil {
			return g.errReturn(manifest, walkErr)
		}
	} else {
		entry, entryErr := g.generateEntry(absPath, info, 0)
		if entryErr != nil {
			return g.errReturn(manifest, entryErr)
		}
		if emitErr := g.emit(entry); emitErr != nil {
			manifest.AddEntry(entry)
			return g.errReturn(manifest, emitErr)
		}
		manifest.AddEntry(entry)
		if g.progress != nil {
			g.progress.record(absPath, entry.IsDir(), entry.Size)
		}
	}

	if g.progress != nil {
		g.progress.final()
	}

	// Sort entries hierarchically (files before directories at each level)
	manifest.SortEntries()

	// Compute directory sizes from children (OS-reported dir sizes are platform-dependent).
	// Uses the canonical c4m implementation — single-pass, nil-infectious, spec-compliant.
	c4m.PropagateMetadata(manifest.Entries)

	// Detect and collapse file sequences if enabled
	if g.detectSequences {
		collapsed := c4m.DetectSequences(manifest)
		manifest.Entries = collapsed.Entries
	}

	return manifest, nil
}

// generateDir walks dirPath and returns its entries (directory self-entry
// first if dirName is non-empty, then all children in source order). Children
// are produced by recursive calls; subdirectory walks may run on the worker
// pool but their result slices are merged in deterministic order so the
// final entry list is independent of scheduling.
//
// Directory Size, Timestamp, and C4 ID are resolved bottom-up from the
// already-scanned children — a directory is identified by the canonical
// one-level listing of its direct children (see canonicalDirID), never by
// re-scanning the subtree. This keeps the walk linear in the number of
// entries and keeps guided scans correct (guide paths are root-relative and
// only ever matched against the single root-anchored walk).
func (g *Generator) generateDir(dirPath, dirName string, depth int) ([]*Entry, error) {
	if err := g.ctxErr(); err != nil {
		return nil, err
	}
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	out := make([]*Entry, 0, len(dirEntries)+1)
	childDepth := depth

	var dirEntry *Entry
	if dirName != "" {
		dirInfo, err := os.Lstat(dirPath)
		if err != nil {
			return nil, err
		}
		dirEntry, err = g.generateEntry(dirPath, dirInfo, depth)
		if err != nil {
			return nil, err
		}
		dirEntry.Name = dirName + "/"
		// Size/Timestamp resolution and C4 ID computation happen after the
		// children are scanned; the entry is emitted then, fully resolved.
		out = append(out, dirEntry)
		childDepth = depth + 1
	}

	// Load exclude patterns from env-named file in subdirectories.
	if g.excludeFileName != "" && dirName != "" {
		g.loadExcludeFile(filepath.Join(dirPath, g.excludeFileName))
	}

	// Filter and classify children. We need a fixed source order so the
	// final entry list is deterministic regardless of which goroutine
	// completed first.
	type subdir struct {
		name string
		path string
	}
	type slot struct {
		direct     *Entry   // non-nil for files/symlinks scanned inline
		subEntries []*Entry // non-nil once a subdir walk completes
		sub        *subdir  // non-nil for subdirectories pending walk
	}
	slots := make([]slot, 0, len(dirEntries))

	for _, entry := range dirEntries {
		if err := g.ctxErr(); err != nil {
			return out, err
		}
		name := entry.Name()

		if !g.includeHidden && strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(dirPath, name)

		if len(g.excludePatterns) > 0 {
			relPath := relFromRoot(g.scanRoot, fullPath)
			if g.matchExclude(relPath, name, entry.IsDir()) {
				continue
			}
		}

		if g.guide != nil {
			relPath := relFromRoot(g.scanRoot, fullPath)
			guideName := relPath
			if entry.IsDir() {
				guideName += "/"
			}
			if !g.guide[guideName] {
				continue
			}
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("failed to get info for %s: %w", fullPath, err)
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			if g.followSymlinks {
				targetInfo, err := os.Stat(fullPath)
				if err == nil {
					info = targetInfo
				}
			} else {
				md := g.generateMetadata(fullPath, info, childDepth)
				if bmd, ok := md.(*BasicFileMetadata); ok {
					target, err := os.Readlink(fullPath)
					if err == nil {
						bmd.SetTarget(filepath.ToSlash(target))
						if g.computesIDs() {
							id := g.computeSymlinkTargetC4ID(fullPath, target)
							bmd.SetID(id)
						}
					}
				}
				fileEntry := MetadataToEntry(md)
				fileEntry.Name = name
				if err := g.emit(fileEntry); err != nil {
					return out, err
				}
				slots = append(slots, slot{direct: fileEntry})
				if g.progress != nil {
					g.progress.record(fullPath, fileEntry.IsDir(), fileEntry.Size)
				}
				continue
			}
		}

		if info.IsDir() {
			slots = append(slots, slot{sub: &subdir{name: name, path: fullPath}})
			continue
		}

		fileEntry, err := g.generateEntry(fullPath, info, childDepth)
		if err != nil {
			return nil, err
		}
		fileEntry.Name = name
		if err := g.emit(fileEntry); err != nil {
			return out, err
		}
		slots = append(slots, slot{direct: fileEntry})
		if g.progress != nil {
			g.progress.record(fullPath, false, fileEntry.Size)
		}
	}

	// Dispatch subdirectory walks. Each subdir tries to grab a slot from
	// the global semaphore; if all slots are busy we fall through and
	// walk inline on the current goroutine. This bounds wall-clock fan-out
	// without ever blocking — guarantees forward progress under arbitrary
	// tree shapes.
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	recordErr := func(e error) {
		if e == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = e
		}
		errMu.Unlock()
	}

	for i := range slots {
		if slots[i].sub == nil {
			continue
		}
		sub := slots[i].sub
		idx := i

		// Try non-blocking acquire. If the pool is empty (g.sem == nil) or
		// full, fall back to inline walk.
		if g.sem != nil {
			select {
			case g.sem <- struct{}{}:
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() { <-g.sem }()
					childEntries, err := g.generateDir(sub.path, sub.name, childDepth)
					if err != nil {
						recordErr(err)
						return
					}
					slots[idx].subEntries = childEntries
				}()
				continue
			default:
			}
		}

		childEntries, err := g.generateDir(sub.path, sub.name, childDepth)
		if err != nil {
			recordErr(err)
			continue
		}
		slots[idx].subEntries = childEntries
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	// Stitch results in original (source) order — deterministic. Collect
	// the direct children while we're at it: files/symlinks are the direct
	// slots, and a subdirectory walk always returns its self-entry first.
	directChildren := make([]*Entry, 0, len(slots))
	for _, s := range slots {
		if s.direct != nil {
			out = append(out, s.direct)
			directChildren = append(directChildren, s.direct)
			continue
		}
		if len(s.subEntries) > 0 {
			out = append(out, s.subEntries...)
			directChildren = append(directChildren, s.subEntries[0])
		}
	}

	if dirEntry != nil {
		if g.computesIDs() {
			// Resolve this directory's null Size/Timestamp from its direct
			// children — subdirectory children were already resolved by
			// their own generateDir calls, so [self, children...] is all
			// the canonical c4m.PropagateMetadata needs; the final
			// whole-manifest pass in GenerateFromPath then early-outs.
			//
			// ModeFull only: stat always yields real sizes there, so
			// children are never null. In structure/metadata modes a child
			// directory can carry a legitimately-null (nil-infected) Size,
			// which this truncated scope — lacking the child's own
			// descendants — would misread as an empty directory and wrongly
			// resolve to 0. Those modes compute no C4 IDs, so they need no
			// per-directory resolution; the whole-manifest pass handles
			// them with full context, as before.
			scope := make([]*Entry, 0, len(directChildren)+1)
			scope = append(scope, dirEntry)
			scope = append(scope, directChildren...)
			c4m.PropagateMetadata(scope)

			dirEntry.C4ID = g.dirID(directChildren)
		}
		if err := g.emit(dirEntry); err != nil {
			return out, err
		}
		if g.progress != nil {
			g.progress.record(dirPath, true, dirEntry.Size)
		}
	}

	return out, nil
}

// dirID derives a directory's C4 ID from its direct children. It dispatches
// to the installed dirIdentity seam, falling back to the spec's canonical
// one-level listing. Children must be fully resolved (sizes, timestamps,
// and C4 IDs final) before this is called.
func (g *Generator) dirID(children []*Entry) c4.ID {
	if g.dirIdentity != nil {
		return g.dirIdentity(children)
	}
	return g.canonicalDirID(children)
}

// canonicalDirID implements the spec's directory identity: the C4 ID of the
// one-level canonical listing of the directory's direct children — files
// before directories, natural sort, one canonical entry line each. The bytes
// hashed are identical to Manifest.ComputeC4ID over a manifest holding only
// the direct children, and therefore to a fresh scan of the directory itself.
// An empty directory hashes the empty string.
func (g *Generator) canonicalDirID(children []*Entry) c4.ID {
	if g.detectSequences && len(children) > 0 {
		// Fold sequences among the direct children so the listing matches
		// the folded entries the final manifest will carry.
		tmp := c4m.NewManifest()
		for _, c := range children {
			cp := *c
			cp.Depth = 0
			tmp.AddEntry(&cp)
		}
		children = c4m.DetectSequences(tmp).Entries
	}

	sorted := make([]*Entry, len(children))
	copy(sorted, children)
	sort.Slice(sorted, func(i, j int) bool {
		iDir := strings.HasSuffix(sorted[i].Name, "/")
		jDir := strings.HasSuffix(sorted[j].Name, "/")
		if iDir != jDir {
			return !iDir // files first
		}
		return c4m.NaturalLess(sorted[i].Name, sorted[j].Name)
	})

	var buf bytes.Buffer
	for _, e := range sorted {
		buf.WriteString(e.Canonical())
		buf.WriteByte('\n')
	}
	return c4.Identify(&buf)
}

// generateEntry creates an entry from file info
func (g *Generator) generateEntry(path string, info os.FileInfo, depth int) (*Entry, error) {
	md := g.generateMetadata(path, info, depth)

	entry := MetadataToEntry(md)
	// MetadataToEntry adds trailing slash for directories, but we handle that elsewhere
	if entry.IsDir() && strings.HasSuffix(entry.Name, "/") {
		entry.Name = entry.Name[:len(entry.Name)-1]
	}

	// Content projection: mode and timestamp are null at every level —
	// names, sizes, symlink targets, and IDs stay real. The nulling is
	// what makes directory roll-up IDs metadata-independent: canonical
	// entry lines render "-" for both fields, so the one-level listing
	// a directory ID hashes carries no machine-specific state.
	if g.mode == ModeContent {
		entry.Mode = 0
		entry.Timestamp = c4m.NullTimestamp()
	}

	return entry, nil
}

// computesIDs reports whether this scan mode hashes content (files,
// symlink targets, directory roll-ups). Full and content modes hash;
// they differ only in which metadata fields the entries carry.
func (g *Generator) computesIDs() bool {
	return g.mode == ModeFull || g.mode == ModeContent
}

// generateMetadata creates metadata from file info
func (g *Generator) generateMetadata(path string, info os.FileInfo, depth int) FileMetadata {
	if g.mode == ModeStructure {
		return NewStructureMetadata(path, info, depth)
	}

	md := NewFileMetadata(path, info, depth)

	if g.computesIDs() && info.Mode().IsRegular() {
		if id, ok := g.reuseID(path, info); ok {
			md.SetID(id)
			atomic.AddInt64(&g.reused, 1)
		} else {
			id, err := g.computeFileC4ID(path)
			if err == nil {
				md.SetID(id)
			}
			if g.reuseGuide != nil {
				atomic.AddInt64(&g.rehashed, 1)
			}
		}
	}

	return md
}

// computeFileC4ID computes the C4 ID for a file
func (g *Generator) computeFileC4ID(path string) (c4.ID, error) {
	file, err := os.Open(path)
	if err != nil {
		return c4.ID{}, err
	}
	defer file.Close()

	return c4.Identify(file), nil
}

// computeSymlinkTargetC4ID computes the C4 ID for a symlink's target
func (g *Generator) computeSymlinkTargetC4ID(symlinkPath, target string) c4.ID {
	targetPath := target
	if !filepath.IsAbs(target) {
		targetPath = filepath.Join(filepath.Dir(symlinkPath), target)
	}

	targetInfo, err := os.Lstat(targetPath)
	if err != nil {
		return c4.ID{}
	}

	if targetInfo.Mode()&os.ModeSymlink != 0 {
		return c4.ID{}
	}

	if targetInfo.IsDir() {
		subGen := g.clone()
		manifest, err := subGen.GenerateFromPath(targetPath)
		if err != nil {
			return c4.ID{}
		}
		return manifest.ComputeC4ID()
	}

	if targetInfo.Mode().IsRegular() {
		id, err := g.computeFileC4ID(targetPath)
		if err != nil {
			return c4.ID{}
		}
		return id
	}

	return c4.ID{}
}

// matchExclude checks if a path matches any exclude pattern.
// Patterns are matched against both the basename and the relative path from scan root.
func (g *Generator) matchExclude(relPath, name string, isDir bool) bool {
	for _, pattern := range g.excludePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
	}
	return false
}

func relFromRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

// loadExcludeFile reads glob patterns from a file (one per line, # comments, blank lines skipped).
func (g *Generator) loadExcludeFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // file not found is not an error
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		g.excludePatterns = append(g.excludePatterns, line)
	}
}

// Dir scans a directory and returns a c4m manifest. This is the primary
// entry point for the scan package.
//
//	m, err := scan.Dir("/path/to/dir")
//	m, err := scan.Dir("/path", scan.WithMode(scan.ModeMetadata))
func Dir(path string, opts ...GeneratorOption) (*c4m.Manifest, error) {
	return NewGeneratorWithOptions(opts...).GenerateFromPath(path)
}
