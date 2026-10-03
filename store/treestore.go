package store

import (
	"crypto/sha512"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Avalanche-io/c4"
)

// DefaultSplitThreshold is the maximum number of content files in a leaf
// directory before it splits into 2-char subdirectories.
const DefaultSplitThreshold = 4096

// TreeStore is a content-addressed store that uses adaptive trie sharding.
// Every C4 ID starts with "c4", so the store always has exactly one
// top-level directory: c4/. Real fanout begins at characters 3-4.
//
// Directories are either leaves (contain content files) or interior nodes
// (contain 2-char subdirectories). When a leaf exceeds SplitThreshold
// files, it splits into subdirectories based on the next 2 characters.
type TreeStore struct {
	root           string
	splitThreshold int
	syncMode       SyncMode
	mu             sync.Mutex
	counts         map[string]int // leaf dir → file count (split accounting)
	dirty          bool           // batch writes awaiting a Sync barrier
	// pending holds complete batch-mode objects awaiting the barrier,
	// keyed by ID, valued by their temp path. Objects are published at
	// their hash names only AFTER Sync's device barrier, so presence at
	// a hash name always implies bytes on stable media — a power cut
	// can lose a pending object (safe: it was never claimed durable)
	// but can never leave a torn object at a valid name for a later
	// run's write-skip to adopt.
	pending map[c4.ID]string
	// stat is a test seam for trie lookups; nil means os.Stat.
	stat func(string) (os.FileInfo, error)
}

var _ Store = (*TreeStore)(nil)
var _ Syncer = (*TreeStore)(nil)

// NewTreeStore creates a TreeStore rooted at the given directory.
// The directory is created if it does not exist.
func NewTreeStore(root string) (*TreeStore, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create store root: %w", err)
	}
	return &TreeStore{root: root, splitThreshold: DefaultSplitThreshold}, nil
}

// Root returns the root directory of the store.
func (s *TreeStore) Root() string {
	return s.root
}

// SetSplitThreshold sets the maximum file count before a leaf directory
// splits. This is primarily useful for testing.
func (s *TreeStore) SetSplitThreshold(n int) {
	s.splitThreshold = n
}

// SetSyncMode selects the write-durability policy. The default is
// SyncEach: every object durable before it lands. Set the mode before
// writing, not concurrently with writes.
func (s *TreeStore) SetSyncMode(mode SyncMode) {
	s.syncMode = mode
}

// Sync makes every object written so far durable. Under SyncEach each
// write is already durable and under SyncNone durability is waived, so
// both are no-ops. Under SyncBatch this is the batch barrier: one
// device-cache flush (F_FULLFSYNC on darwin) covering every object
// written since the last Sync — issued BEFORE pending objects are
// renamed to their hash names, so a rename is only ever visible for
// bytes that are already on stable media.
func (s *TreeStore) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncMode != SyncBatch || (!s.dirty && len(s.pending) == 0) {
		return nil
	}
	if err := SyncDir(s.root); err != nil {
		return err
	}
	for id, tmpName := range s.pending {
		p, ok := s.locate(id)
		if ok {
			os.Remove(tmpName)
			delete(s.pending, id)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		if err := os.Rename(tmpName, p); err != nil {
			return fmt.Errorf("rename: %w", err)
		}
		s.noteAdd(filepath.Dir(p))
		delete(s.pending, id)
	}
	s.dirty = false
	return nil
}

// Has reports whether the store contains content for the given ID.
func (s *TreeStore) Has(id c4.ID) bool {
	s.mu.Lock()
	_, ok := s.pending[id]
	s.mu.Unlock()
	if ok {
		return true
	}
	_, ok = s.locate(id)
	return ok
}

// Open opens the content for reading.
func (s *TreeStore) Open(id c4.ID) (io.ReadCloser, error) {
	s.mu.Lock()
	tmpName, ok := s.pending[id]
	s.mu.Unlock()
	if ok {
		return os.Open(tmpName)
	}
	f, err := os.Open(s.path(id))
	if err == nil || !os.IsNotExist(err) {
		return f, err
	}
	if p, ok := s.foldFind(id); ok {
		return os.Open(p)
	}
	return nil, err
}

// Create creates a new entry for writing. The caller must know the ID
// in advance. Writes go to a temp file; Close flushes per the store's
// sync mode and renames atomically.
func (s *TreeStore) Create(id c4.ID) (io.WriteCloser, error) {
	p := s.path(id)
	if s.Has(id) {
		return nil, &os.PathError{Op: "create", Path: p, Err: os.ErrExist}
	}

	// Batch mode: write to a temp file and defer publication to the
	// Sync barrier, same as Put — a rename must never precede the
	// durability of the bytes it exposes.
	if s.syncMode == SyncBatch {
		tmp, err := os.CreateTemp(s.root, ".ingest.*")
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return &pendingWriter{f: tmp, s: s, id: id}, nil
	}

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	w, err := NewDurableWriter(p)
	if err != nil {
		return nil, err
	}
	w.sync = s.syncMode
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
	return w, nil
}

// pendingWriter is Create's batch-mode writer: bytes land in a temp
// file with a cheap flush on Close, and the object is registered as
// pending so it publishes at its hash name only after the Sync barrier.
type pendingWriter struct {
	f  *os.File
	s  *TreeStore
	id c4.ID
}

func (w *pendingWriter) Write(b []byte) (int, error) {
	return w.f.Write(b)
}

func (w *pendingWriter) Close() error {
	tmpName := w.f.Name()
	if err := flushFile(w.f, SyncBatch); err != nil {
		w.f.Close()
		os.Remove(tmpName)
		return err
	}
	if err := w.f.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	if _, ok := w.s.pending[w.id]; ok {
		os.Remove(tmpName)
		return nil
	}
	if _, ok := w.s.locate(w.id); ok {
		os.Remove(tmpName)
		return nil
	}
	if w.s.pending == nil {
		w.s.pending = make(map[c4.ID]string)
	}
	w.s.pending[w.id] = tmpName
	return nil
}

// Put reads all content from r, computes its C4 ID, stores it, and returns
// the ID. If the content already exists the write is skipped. Put is safe
// for concurrent use: hashing, temp writes, and flushes overlap; only the
// publish step (exists-check, rename, split accounting) is serialized.
func (s *TreeStore) Put(r io.Reader) (c4.ID, error) {
	// Write to a temp file while computing the C4 ID.
	tmp, err := os.CreateTemp(s.root, ".ingest.*")
	if err != nil {
		return c4.ID{}, fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	keepTmp := false
	defer func() {
		if !keepTmp {
			os.Remove(tmpName) // clean up on any error path
		}
	}()

	h := sha512.New()
	w := io.MultiWriter(tmp, h)
	if _, err := io.Copy(w, r); err != nil {
		tmp.Close()
		return c4.ID{}, fmt.Errorf("copy: %w", err)
	}
	if err := flushFile(tmp, s.syncMode); err != nil {
		tmp.Close()
		return c4.ID{}, fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return c4.ID{}, fmt.Errorf("close: %w", err)
	}

	var id c4.ID
	copy(id[:], h.Sum(nil))

	// Publish under the lock: path resolution, rename, and split
	// accounting must not interleave with a concurrent split.
	s.mu.Lock()
	defer s.mu.Unlock()

	// If content already exists, skip the rename.
	p, ok := s.locate(id)
	if ok {
		return id, nil
	}

	// Batch mode: defer publication. The object must not appear at its
	// hash name until the barrier has made its bytes durable — a rename
	// visible after a power cut must always imply good bytes, or a
	// later run's presence-gated write-skip adopts a torn object into a
	// printed snapshot.
	if s.syncMode == SyncBatch {
		if _, ok := s.pending[id]; ok {
			return id, nil
		}
		if s.pending == nil {
			s.pending = make(map[c4.ID]string)
		}
		s.pending[id] = tmpName
		keepTmp = true
		s.dirty = true
		return id, nil
	}

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return c4.ID{}, fmt.Errorf("mkdir: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return c4.ID{}, fmt.Errorf("rename: %w", err)
	}
	s.dirty = true

	// Check if the leaf directory needs splitting.
	s.noteAdd(filepath.Dir(p))

	return id, nil
}

// ContentPath returns the local filesystem path for id when present.
// A pending batch object's temp path is returned: the file is complete
// (written and closed), just not yet published at its hash name.
func (s *TreeStore) ContentPath(id c4.ID) (string, bool) {
	s.mu.Lock()
	tmpName, ok := s.pending[id]
	s.mu.Unlock()
	if ok {
		return tmpName, true
	}
	p, ok := s.locate(id)
	if !ok {
		return "", false
	}
	return p, true
}

// Remove deletes the content for the given ID.
func (s *TreeStore) Remove(id c4.ID) error {
	err := os.Remove(s.path(id))
	if err == nil || !os.IsNotExist(err) {
		return err
	}
	if p, ok := s.foldFind(id); ok {
		return os.Remove(p)
	}
	return err
}

// Walk enumerates every object in the store, calling fn with each object's
// ID and size. Files whose names do not parse as C4 IDs (temp files, stray
// files) are skipped. A non-nil error from fn stops the walk.
func (s *TreeStore) Walk(fn func(id c4.ID, size int64) error) error {
	return filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		id, perr := c4.Parse(d.Name())
		if perr != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(id, info.Size())
	})
}

// path resolves the storage path for an ID by walking the trie.
// It follows 2-char prefix subdirectories until reaching a leaf.
func (s *TreeStore) path(id c4.ID) string {
	str := id.String()
	dir := s.root
	for i := 0; i+2 <= len(str); i += 2 {
		sub := filepath.Join(dir, str[i:i+2])
		info, err := s.statPath(sub)
		if err != nil || !info.IsDir() {
			break
		}
		dir = sub
	}
	return filepath.Join(dir, str)
}

// locate returns the path of id's object file and true when it exists.
// The exact-case trie path is checked first with a single stat, exactly
// as before; only on a miss is foldFind consulted. On a miss the
// exact-case path is returned as the write target.
func (s *TreeStore) locate(id c4.ID) (string, bool) {
	p := s.path(id)
	if _, err := s.statPath(p); err == nil {
		return p, true
	}
	if fp, ok := s.foldFind(id); ok {
		return fp, true
	}
	return p, false
}

// foldFind searches for id's object file through shard directories whose
// names equal the ID's segments under case folding. C4 IDs are base58
// and case-sensitive, but a store built on a case-insensitive filesystem
// (default macOS APFS) files IDs differing only in case at some level
// under one shard directory named by whichever came first ("c4/1J"
// holding "c41j..."). On a case-sensitive filesystem the exact-case
// probe misses those objects; this finds them without moving anything.
// Each level probes at most four case variants (exact first) by stat
// rather than listing the directory, so a miss never reads a full leaf.
func (s *TreeStore) foldFind(id c4.ID) (string, bool) {
	return s.foldSearch(s.root, id.String(), 0)
}

func (s *TreeStore) foldSearch(dir, str string, i int) (string, bool) {
	if i+2 <= len(str) {
		var seen []os.FileInfo
		for _, seg := range foldVariants(str[i : i+2]) {
			info, err := s.statPath(filepath.Join(dir, seg))
			if err != nil || !info.IsDir() || sameAsAny(info, seen) {
				continue
			}
			seen = append(seen, info)
			if p, ok := s.foldSearch(filepath.Join(dir, seg), str, i+2); ok {
				return p, true
			}
		}
		if len(seen) > 0 {
			return "", false // interior node: objects live in the leaves
		}
	}
	p := filepath.Join(dir, str)
	if _, err := s.statPath(p); err != nil {
		return "", false
	}
	return p, true
}

// sameAsAny reports whether info is the same directory as any in seen.
// On a case-insensitive filesystem every case variant resolves to one
// directory; this keeps the search from walking it more than once.
func sameAsAny(info os.FileInfo, seen []os.FileInfo) bool {
	for _, o := range seen {
		if os.SameFile(info, o) {
			return true
		}
	}
	return false
}

// foldVariants returns every ASCII case variant of seg, seg itself first.
func foldVariants(seg string) []string {
	out := []string{seg}
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') {
			continue
		}
		for _, v := range out[:len(out):len(out)] {
			b := []byte(v)
			b[i] ^= 0x20
			out = append(out, string(b))
		}
	}
	return out
}

// noteAdd records one new file in a leaf directory and splits the leaf
// when it exceeds the threshold. The count is cached so the split check
// is O(1) per Put instead of a ReadDir; the cache is seeded by one
// ReadDir on first touch and invalidated on split. External writers may
// skew the cache — the only consequence is a slightly early or late
// split. Caller must hold s.mu.
func (s *TreeStore) noteAdd(dir string) {
	n, ok := s.counts[dir]
	if !ok {
		n = countFiles(dir) // includes the file just renamed in
	} else {
		n++
	}
	if s.counts == nil {
		s.counts = make(map[string]int)
	}
	s.counts[dir] = n

	if n <= s.splitThreshold {
		return
	}
	s.split(dir)
	delete(s.counts, dir)
}

// countFiles counts regular (non-temp) files in dir.
func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var n int
	for _, e := range entries {
		if !e.IsDir() && !isTemp(e.Name()) {
			n++
		}
	}
	return n
}

// split redistributes a leaf directory's files into 2-char
// subdirectories based on the next prefix segment. Caller must hold s.mu.
func (s *TreeStore) split(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Determine the prefix depth of this directory relative to root.
	depth := s.prefixDepth(dir)

	for _, e := range entries {
		if e.IsDir() || isTemp(e.Name()) {
			continue
		}
		name := e.Name()
		if len(name) <= depth+2 {
			continue // ID too short for another level (shouldn't happen)
		}
		sub := name[depth : depth+2]
		subDir := filepath.Join(dir, sub)
		os.MkdirAll(subDir, 0755)
		os.Rename(filepath.Join(dir, name), filepath.Join(subDir, name))
	}
}

// prefixDepth returns how many characters of the ID are consumed by the
// directory path from root to dir. Each trie level consumes 2 characters.
func (s *TreeStore) prefixDepth(dir string) int {
	rel, err := filepath.Rel(s.root, dir)
	if err != nil {
		return 0
	}
	if rel == "." {
		return 0
	}
	parts := filepath.SplitList(rel)
	if len(parts) == 1 {
		parts = splitPath(rel)
	}
	return len(parts) * 2
}

// splitPath splits a path into its components.
func splitPath(p string) []string {
	var parts []string
	for {
		dir, file := filepath.Split(p)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		if dir == "" || dir == p {
			break
		}
		p = filepath.Clean(dir)
	}
	return parts
}

// isTemp returns true for temp files created during ingestion.
func isTemp(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

// statPath stats p through the test seam when one is set.
func (s *TreeStore) statPath(p string) (os.FileInfo, error) {
	if s.stat != nil {
		return s.stat(p)
	}
	return os.Stat(p)
}
