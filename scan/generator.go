package scan

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// ScanMode controls how much information is gathered during a scan.
type ScanMode int

const (
	ModeStructure ScanMode = iota // names and hierarchy only
	ModeMetadata                  // structure + permissions, timestamps, sizes
	ModeFull                      // structure + metadata + C4 IDs
)

// defaultConcurrencyCap caps the worker pool regardless of GOMAXPROCS so we
// don't oversubscribe storage with hundreds of concurrent readdir calls.
const defaultConcurrencyCap = 16

// ParseScanMode parses a mode string: "s"/"1" → structure, "m"/"2" → metadata, "f"/"3" → full.
func ParseScanMode(s string) (ScanMode, error) {
	switch strings.ToLower(s) {
	case "s", "1", "structure":
		return ModeStructure, nil
	case "m", "2", "metadata":
		return ModeMetadata, nil
	case "f", "3", "full", "":
		return ModeFull, nil
	default:
		return ModeFull, fmt.Errorf("unknown scan mode %q (use s/m/f or 1/2/3)", s)
	}
}

// Generator creates C4M manifests from filesystem paths
type Generator struct {
	mode            ScanMode
	followSymlinks  bool
	includeHidden   bool
	detectSequences bool
	excludePatterns []string
	excludeFile     string // explicit exclude file path
	excludeFileName string // filename to look for in scanned dirs (from env)
	guide           map[string]bool // paths from guide c4m (nil = no guide)
	scanRoot        string
	progress        *progress // nil = no progress reporting (zero-cost path)
	maxConcurrency  int       // 0 = auto, 1 = sequential, n > 1 = bounded parallel
	sem             chan struct{} // worker-pool slots; nil for sequential

	ctx      context.Context        // cancellation; nil means no cancellation
	streamCB func(*c4m.Entry) error // fires per discovered entry; nil disables streaming
	streamMu sync.Mutex             // serializes streamCB calls under parallel walk
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
// value copy — the callback may keep it freely. Sub-scans triggered to compute
// directory C4 IDs do not report progress (they would double-count entries).
func WithProgress(cb func(ScanStats)) GeneratorOption {
	return func(g *Generator) {
		if cb != nil {
			g.progress = newProgress(cb)
		}
	}
}

// WithMaxConcurrency caps the number of concurrent subdirectory walks.
//   n =  0: auto (min(GOMAXPROCS, 16))
//   n =  1: purely sequential (preserves pre-parallelism behavior)
//   n >  1: explicit cap
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
// across runs; pair with WithMaxConcurrency(1) if strict walk-order is
// required.
//
// The callback is not invoked for entries emitted by internal sub-scans
// used to compute directory C4 IDs in ModeFull — only top-level walk
// entries are streamed.
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

// clone creates a copy with the same settings but fresh state.
// Sub-scans triggered for directory C4 ID computation share the parent's
// semaphore so the global concurrency cap is honored across the whole walk.
// They drop the progress reporter and the entry-stream callback to avoid
// double-counting / double-emit. The context IS propagated so a single
// cancellation halts the entire scan, including sub-scans.
func (g *Generator) clone() *Generator {
	clone := &Generator{
		mode:            g.mode,
		followSymlinks:  g.followSymlinks,
		includeHidden:   g.includeHidden,
		detectSequences: g.detectSequences,
		excludeFile:     g.excludeFile,
		excludeFileName: g.excludeFileName,
		guide:           g.guide,
		maxConcurrency:  g.maxConcurrency,
		sem:             g.sem,
		ctx:             g.ctx,
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

	if dirName != "" {
		dirInfo, err := os.Lstat(dirPath)
		if err != nil {
			return nil, err
		}
		dirEntry, err := g.generateEntry(dirPath, dirInfo, depth)
		if err != nil {
			return nil, err
		}
		dirEntry.Name = dirName + "/"

		// For directories, compute C4 ID from their recursive manifest.
		// The sub-generator inherits the shared semaphore so concurrency
		// stays bounded globally.
		if g.mode == ModeFull && dirInfo.IsDir() {
			subGen := g.clone()
			subManifest, err := subGen.GenerateFromPath(dirPath)
			if err == nil {
				dirEntry.C4ID = subManifest.ComputeC4ID()
			}
		}

		if err := g.emit(dirEntry); err != nil {
			return out, err
		}
		out = append(out, dirEntry)
		if g.progress != nil {
			g.progress.record(dirPath, true, dirEntry.Size)
		}
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
		direct   *Entry   // non-nil for files/symlinks scanned inline
		subEntries []*Entry // non-nil once a subdir walk completes
		sub      *subdir  // non-nil for subdirectories pending walk
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
						if g.mode == ModeFull {
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

	// Stitch results in original (source) order — deterministic.
	for _, s := range slots {
		if s.direct != nil {
			out = append(out, s.direct)
			continue
		}
		if s.subEntries != nil {
			out = append(out, s.subEntries...)
		}
	}

	return out, nil
}

// generateEntry creates an entry from file info
func (g *Generator) generateEntry(path string, info os.FileInfo, depth int) (*Entry, error) {
	md := g.generateMetadata(path, info, depth)

	entry := MetadataToEntry(md)
	// MetadataToEntry adds trailing slash for directories, but we handle that elsewhere
	if entry.IsDir() && strings.HasSuffix(entry.Name, "/") {
		entry.Name = entry.Name[:len(entry.Name)-1]
	}

	return entry, nil
}

// generateMetadata creates metadata from file info
func (g *Generator) generateMetadata(path string, info os.FileInfo, depth int) FileMetadata {
	if g.mode == ModeStructure {
		return NewStructureMetadata(path, info, depth)
	}

	md := NewFileMetadata(path, info, depth)

	if g.mode == ModeFull && info.Mode().IsRegular() {
		id, err := g.computeFileC4ID(path)
		if err == nil {
			md.SetID(id)
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
