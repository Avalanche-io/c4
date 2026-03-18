package reconcile

import (
	"io"
	"os"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// ContentSource provides read access to content by C4 ID.
type ContentSource interface {
	Has(id c4.ID) bool
	Open(id c4.ID) (io.ReadCloser, error)
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

		fullPath := baseDir + "/" + relPath
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
		if _, err := os.Stat(p); err == nil {
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
	SrcPath   string     // source path for move
	Entry     *c4m.Entry // target entry metadata
	ContentID c4.ID      // content to write
}

// Plan is an ordered list of operations with a content availability check.
type Plan struct {
	Operations []Operation
	Missing    []c4.ID
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
}

// Saver stores content by C4 ID. Used to preserve content before removal.
type Saver interface {
	Has(id c4.ID) bool
	Put(r io.Reader) (c4.ID, error)
}

// Reconciler orchestrates filesystem reconciliation.
type Reconciler struct {
	sources       []ContentSource
	dryRun        bool
	storeRemovals Saver // if set, store content before removing files
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

// New creates a Reconciler with the given options.
func New(opts ...Option) *Reconciler {
	r := &Reconciler{}
	for _, o := range opts {
		o(r)
	}
	return r
}

// openContent searches all sources for the given C4 ID and returns a reader.
func (r *Reconciler) openContent(id c4.ID) (io.ReadCloser, error) {
	for _, src := range r.sources {
		if src.Has(id) {
			return src.Open(id)
		}
	}
	return nil, os.ErrNotExist
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
	actual := c4.Identify(f)
	return actual == id
}
