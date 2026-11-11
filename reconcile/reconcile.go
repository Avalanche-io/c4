package reconcile

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// ContentSource provides read access to content by C4 ID.
// Sources must tolerate concurrent Open calls when Apply runs with
// concurrency greater than one (all in-package sources do).
type ContentSource interface {
	Has(id c4.ID) bool
	Open(id c4.ID) (io.ReadCloser, error)
}

// LocalSource is a ContentSource whose content lives in the local
// filesystem. ContentPath returns a path holding the content for id,
// and whether one is available. Apply prefers local paths: file-to-file
// copies use OS acceleration and, where supported, copy-on-write clones.
type LocalSource interface {
	ContentPath(id c4.ID) (string, bool)
}

// DirSource wraps a directory as a ContentSource using a C4 ID to path index.
type DirSource struct {
	index map[c4.ID][]string
}

// NewDirSource builds a content source from a manifest and its base directory.
// It indexes every non-directory entry with a non-nil C4 ID to its filesystem path.
func NewDirSource(m *c4m.Manifest, baseDir string) *DirSource {
	ds := &DirSource{
		index: make(map[c4.ID][]string),
	}

	// Reconstruct full paths using the same depth-based algorithm as EntryPaths.
	stack := make([]string, 0, 8)
	for _, e := range m.Entries {
		for len(stack) > e.Depth {
			stack = stack[:len(stack)-1]
		}

		var sb strings.Builder
		for _, s := range stack {
			sb.WriteString(s)
		}
		sb.WriteString(e.Name)
		relPath := sb.String()

		if e.IsDir() {
			stack = append(stack, e.Name)
			continue
		}

		if e.C4ID.IsNil() {
			continue
		}

		fullPath := filepath.Join(baseDir, filepath.FromSlash(relPath))
		ds.index[e.C4ID] = append(ds.index[e.C4ID], fullPath)
	}

	return ds
}

// Has returns true if the content for id is available.
func (ds *DirSource) Has(id c4.ID) bool {
	paths, ok := ds.index[id]
	if !ok || len(paths) == 0 {
		return false
	}
	// Verify at least one path still exists.
	for _, p := range paths {
		if _, err := os.Lstat(p); err == nil {
			return true
		}
	}
	return false
}

// Open returns a reader for the content identified by id.
func (ds *DirSource) Open(id c4.ID) (io.ReadCloser, error) {
	paths := ds.index[id]
	for _, p := range paths {
		f, err := os.Open(p)
		if err == nil {
			return f, nil
		}
	}
	return nil, os.ErrNotExist
}

// ContentPath returns a local filesystem path holding the content for id.
func (ds *DirSource) ContentPath(id c4.ID) (string, bool) {
	for _, p := range ds.index[id] {
		info, err := os.Stat(p)
		if err == nil && info.Mode().IsRegular() {
			return p, true
		}
	}
	return "", false
}

// Op identifies the type of filesystem operation.
type Op int

const (
	OpMkdir   Op = iota // Create directory
	OpCreate            // Create or overwrite file
	OpMove              // Rename file
	OpSymlink           // Create symlink
	OpChmod             // Change permissions
	OpChtimes           // Change timestamps
	OpRemove            // Remove file
	OpRmdir             // Remove empty directory
)

// Operation is a single atomic filesystem change.
type Operation struct {
	Type      Op
	Path      string     // absolute target path
	SrcPath   string     // source path for a move; for a create, a local file holding the content (a move's destination)
	Entry     *c4m.Entry // target entry metadata
	ContentID c4.ID      // content to write
}

// Plan is an ordered list of operations with a content availability check.
type Plan struct {
	Operations []Operation
	Missing    []c4.ID
	// Refused lists the target entries this platform cannot hold under
	// their names (see nameRefusal), in path order. Nothing is planned for
	// them or under them, and nothing on disk is removed on their account;
	// the rest of the target is planned as usual. It also lists the
	// entries the destination folds onto a sibling's name (measured there;
	// see foldCollisions).
	Refused []Refusal
	// MayCollide lists, in a dry run, sibling names that differ only in
	// case: a dry run writes nothing, so whether the destination folds them
	// is not measured.
	MayCollide []Collision
	// Unmeasured lists sibling names a real run could not measure (the
	// probe could not run); nothing is refused for them.
	Unmeasured []Collision
	// Respell lists the byte-order last of each measured collision set, and
	// each target entry the folder already held under another spelling:
	// after Apply, each is listed under exactly its own name.
	Respell []string
}

// Collision is two sibling target names that may name one entry in the
// destination folder.
type Collision struct {
	Path string // the name that would be refused
	With string // the byte-order last, which would be written
	Err  string // why it was not measured, for Unmeasured
}

// Refusal is one target entry a platform cannot hold as named: a
// directory is reported once, for its whole subtree.
type Refusal struct {
	Path    string // slash-separated path relative to the destination; a directory ends in "/"
	Reason  string // why, citing git's rule or a measurement
	Entries int    // entries refused with it: 1, or the directory and everything under it
}

// IsComplete returns true when all required content is available.
func (p *Plan) IsComplete() bool { return len(p.Missing) == 0 }

// Result reports what happened during Apply.
type Result struct {
	Created int
	Moved   int
	Removed int
	Updated int
	Skipped int
	Errors  []error
	// LinkTimesUnset lists symlinks whose own time the plan wanted set but
	// this platform cannot set without following the link (see
	// linkTimeSupported). Each keeps the time it was created with; the
	// caller reports them and must not verify their timestamps as set.
	LinkTimesUnset []string
}

// Saver stores content by C4 ID. Used to preserve content before removal.
type Saver interface {
	Has(id c4.ID) bool
	Put(r io.Reader) (c4.ID, error)
}

// Reconciler orchestrates filesystem reconciliation.
type Reconciler struct {
	sources        []ContentSource
	dryRun         bool
	storeRemovals  Saver                 // if set, store content before removing files
	syncMode       store.SyncMode        // created-file durability (default SyncEach)
	wroteFiles     bool                  // any create executed; gates the batch barrier
	maxConcurrency int                   // Apply worker cap: 0 = auto, 1 = sequential
	trustMetadata  bool                  // Plan may reuse target IDs on size+mtime match
	known          map[string]*c4m.Entry // the caller's own hashed scan of the directory, by relative path
	knownStart     time.Time             // when that scan started; files not strictly older are rehashed
	root           string                // Apply's directory: no write may escape it
	goos           string                // platform whose name rules the plan applies; "" = runtime.GOOS (a test seam)
}

// Option configures a Reconciler.
type Option func(*Reconciler)

// WithSource adds a content source.
func WithSource(src ContentSource) Option {
	return func(r *Reconciler) {
		r.sources = append(r.sources, src)
	}
}

// WithDryRun controls whether Apply actually modifies the filesystem.
func WithDryRun(v bool) Option {
	return func(r *Reconciler) {
		r.dryRun = v
	}
}

// WithStoreRemovals causes Apply to store file content before removing files.
// This preserves content that would otherwise be lost, making the operation
// reversible.
func WithStoreRemovals(s Saver) Option {
	return func(r *Reconciler) {
		r.storeRemovals = s
	}
}

// WithSyncMode selects how created files reach stable storage, mirroring
// the store's write-durability policy. SyncEach (the default) makes each
// file durable before it is renamed into place. SyncBatch hands each file
// to the device with a cheap flush and issues one device-cache barrier at
// the end of Apply — every write is atomic (complete or absent) during
// the run, and the whole batch is durable when Apply returns. SyncNone
// skips flushing entirely (scratch materialization only).
func WithSyncMode(m store.SyncMode) Option {
	return func(r *Reconciler) {
		r.syncMode = m
	}
}

// WithMaxConcurrency caps the workers Apply uses for file creation.
// 0 = auto (min(GOMAXPROCS, 16)), 1 = sequential, n > 1 = explicit.
func WithMaxConcurrency(n int) Option {
	return func(r *Reconciler) {
		r.maxConcurrency = n
	}
}

// WithTrustedMetadata lets Plan reuse the target entry's C4 ID when an
// existing file's size and mtime (second precision) match, skipping the
// content hash — the same trust contract as guided scanning. Default
// false: every existing file is hashed. A file whose content changed
// while preserving size and mtime is treated as unchanged when enabled.
func WithTrustedMetadata(v bool) Option {
	return func(r *Reconciler) {
		r.trustMetadata = v
	}
}

// WithKnownState gives Plan the caller's own scan of the directory, taken
// at scanStart with every file hashed, so Plan reads no file twice. A
// regular file reuses the scan's C4 ID when its size and mtime equal the
// scan entry's and its mtime is strictly older than scanStart (second
// precision, c4 id -c's racy rule); any other file is hashed. Unlike
// WithTrustedMetadata, what is trusted is a scan of this directory taken
// moments ago by the caller, not a description from the past: restore's
// pre-image scan already hashed every byte, and the plan reuses that
// work instead of repeating it.
func WithKnownState(m *c4m.Manifest, scanStart time.Time) Option {
	return func(r *Reconciler) {
		if m == nil {
			return
		}
		r.known = c4m.EntryPaths(m.Entries)
		r.knownStart = scanStart.UTC()
	}
}

// New creates a Reconciler with the given options.
func New(opts ...Option) *Reconciler {
	r := &Reconciler{} // zero syncMode = store.SyncEach: durable per file
	for _, o := range opts {
		o(r)
	}
	return r
}

// openContent searches all sources for the given C4 ID and returns a reader.
func (r *Reconciler) openContent(id c4.ID) (io.ReadCloser, ContentSource, error) {
	for _, src := range r.sources {
		rc, err := src.Open(id)
		if err == nil {
			return rc, src, nil
		}
	}
	return nil, nil, os.ErrNotExist
}

// localPath searches all sources for a local filesystem path holding the
// content for id.
func (r *Reconciler) localPath(id c4.ID) (string, bool) {
	for _, src := range r.sources {
		ls, ok := src.(LocalSource)
		if !ok {
			continue
		}
		if p, ok := ls.ContentPath(id); ok {
			return p, true
		}
	}
	return "", false
}

// newWriter returns a writer for path per WithSyncMode.
func (r *Reconciler) newWriter(path string) (*store.DurableWriter, error) {
	switch r.syncMode {
	case store.SyncBatch:
		return store.NewBatchWriter(path)
	case store.SyncNone:
		return store.NewAtomicWriter(path)
	}
	return store.NewDurableWriter(path)
}

// workers returns the effective worker count for a batch of n operations.
func (r *Reconciler) workers(n int) int {
	w := r.maxConcurrency
	if w <= 0 {
		w = runtime.GOMAXPROCS(0)
		if w > 16 {
			w = 16
		}
	}
	if w > n {
		w = n
	}
	return w
}

// fileMatchesID returns true if the file at path has the expected C4 ID.
// It compares size first as a fast-path rejection.
func fileMatchesID(path string, id c4.ID, expectedSize int64) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if expectedSize >= 0 && info.Size() != expectedSize {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	actual, err := c4.IdentifyReader(f)
	return err == nil && actual == id
}
