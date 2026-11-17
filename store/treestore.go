package store

import (
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	// rename is a test seam for the publishing renames (Put, Sync and
	// Create's writer); nil means os.Rename.
	rename func(oldpath, newpath string) error
	// open is a test seam for Open's opens of object files; nil means
	// os.Open.
	open func(string) (*os.File, error)
	// touched holds directories whose entries changed during publication
	// (rename destinations and split sources and targets), synced and
	// cleared at the end of Sync.
	touched map[string]bool
}

var _ Store = (*TreeStore)(nil)
var _ Syncer = (*TreeStore)(nil)

// NewTreeStore creates a TreeStore rooted at the given directory.
// The directory is created if it does not exist.
func NewTreeStore(root string) (*TreeStore, error) {
	// The root is kept as given but cleaned (no trailing slash, no "./",
	// no doubled separators), so paths built under it with filepath.Join
	// compare and relate to it exactly. It is not made absolute: a
	// relative root means "relative to the working directory" for the
	// life of the process, as it always has.
	root = filepath.Clean(root)
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
	// The barrier, in order:
	//  1. one device flush (SyncDir on darwin is F_FULLFSYNC): every
	//     pending object's bytes are on stable media before any rename
	//     makes a name visible, so a name always implies durable bytes;
	//  2. the renames, with any new shard directories;
	//  3. a plain fsync of every directory a rename or mkdir touched, and
	//     of its ancestors up to the store root: the new names and
	//     directories reach the filesystem's log;
	//  4. one more device flush, so that log reaches the device before
	//     Sync returns and a claim is journaled or printed.
	// Before 2026-11-09 steps 3 and 4 did not exist: the directory entries
	// were left to whatever flush happened next (the journal's), which
	// POSIX does not promise carries them and which filesystems handle
	// differently (found in the storage project's review). The per
	// directory sync is a plain fsync so the device flush is paid twice
	// per batch, not once per directory.
	if err := syncDirFull(s.root); err != nil {
		return err
	}
	for id, tmpName := range s.pending {
		p, ok := s.locate(id)
		if ok {
			os.Remove(tmpName)
			delete(s.pending, id)
			s.markTouched(filepath.Dir(p)) // a skip rests on that name: sync it (holdName)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		published, at, err := s.publish(tmpName, p, id)
		if err != nil {
			return fmt.Errorf("rename: %w", err)
		}
		delete(s.pending, id)
		if !published {
			s.markTouched(filepath.Dir(at)) // another writer published it first; sync its name
			continue
		}
		s.markTouched(filepath.Dir(p))
		s.noteAdd(filepath.Dir(p)) // may split, which marks the directories it moves into
	}
	touched := s.touchedDirs()
	for d := range touched {
		if err := syncDirEntries(d); err != nil {
			return fmt.Errorf("sync %s: %w", d, err)
		}
	}
	if len(touched) > 0 {
		if err := syncDirFull(s.root); err != nil {
			return err
		}
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
	f, err := s.openPath(s.path(id))
	if err == nil || !os.IsNotExist(err) && !moving(err) {
		return f, err
	}
	// The search found the file at one level, and another process's split
	// can move it down before the open: search again.
	//
	// The bound is openFollows(id) passes, one search and one open each,
	// and Open stops at the first of: the search finds no copy (not
	// found, the first open's error); an open succeeds, or fails other
	// than not-exist (that result); the bound (not found). A file only
	// moves deeper, at most one level per two characters of the ID, so a
	// file that is only being moved is followed well within the bound; it
	// is reached only if the ID is removed and stored again on every
	// pass, and then it was absent at some instant during this Open, so
	// not found is a true answer (TestTreeStoreOpenFollowsAMove,
	// TestTreeStoreOpenRemovedMidSearch, TestTreeStoreOpenRetryIsBounded).
	//
	// On Windows an open can also meet the split's rename itself: the
	// rename holds the file, and the open fails with a sharing violation
	// (moving). Open waits movingPause and searches again, within the same
	// bound; if the bound runs out with the file still held, it returns
	// that error, never not-found. Measured 2026-11-16 on master's code
	// with this lookup (c4 #186): 4 to 9 such failures in 6 root splits on
	// every Windows leg (TestTreeStoreOpenWaitsOutARename).
	var held error
	for i := 0; i < openFollows(id); i++ {
		p, ok := s.foldFind(id)
		if !ok {
			if moving(err) { // it was moving, and is gone
				return nil, &os.PathError{Op: "open", Path: s.path(id), Err: os.ErrNotExist}
			}
			return nil, err
		}
		f, oerr := s.openPath(p)
		if oerr == nil {
			return f, nil
		}
		if moving(oerr) {
			held = oerr
			time.Sleep(movingPause)
			continue
		}
		if !os.IsNotExist(oerr) {
			return nil, oerr
		}
		held = nil
	}
	if held != nil {
		return nil, held
	}
	return nil, err
}

// openPath opens an object file through the test seam when one is set.
func (s *TreeStore) openPath(p string) (*os.File, error) {
	if s.open != nil {
		return s.open(p)
	}
	return os.Open(p)
}

// moving reports an open that failed because another process's rename
// holds the file at this instant: a sharing violation, which only Windows
// produces. The file is there, at this level or the next one down.
func moving(err error) bool { return errors.Is(err, errSharingViolation) }

// movingPause is how long Open waits before it looks again for a file
// whose open met a rename in progress.
var movingPause = time.Millisecond

// openFollows is how many times Open searches again for a file that was
// gone when it opened what the search found: one per character of the
// ID, twice the deepest a file can move.
func openFollows(id c4.ID) int { return len(id.String()) }

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
	w.rename = s.rename
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
	return &createWriter{DurableWriter: w, s: s, id: id}, nil
}

// createWriter is Create's writer outside batch mode. Its rename is the
// same publish as Put's and fails the same way on Windows when another
// writer published the ID first (see publish); the object is then stored,
// and Close reports success. Only a failed rename is forgiven, never a
// failed flush, and only when the object is present; the DurableWriter
// has already removed the temp file. DurableWriter itself keeps failing:
// its other users write paths that are not content-addressed, where an
// existing file is not the same bytes.
type createWriter struct {
	*DurableWriter
	s  *TreeStore
	id c4.ID
}

func (w *createWriter) Close() error {
	err := w.DurableWriter.Close()
	var le *os.LinkError
	if err != nil && !(errors.As(err, &le) && w.s.Has(w.id)) {
		return err
	}
	at := w.DurableWriter.final
	if err != nil { // another writer published it first: hold that copy
		if p, ok := w.s.ContentPath(w.id); ok {
			at = p
		}
	}
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	return w.s.holdName(filepath.Dir(at))
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
	if found, ok := w.s.locate(w.id); ok {
		os.Remove(tmpName)
		return w.s.holdName(filepath.Dir(found))
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

	// If content already exists, skip the rename. The skip rests on that
	// name, which another writer may not have synced yet: hold it
	// (holdName).
	p, ok := s.locate(id)
	if ok {
		if err := s.holdName(filepath.Dir(p)); err != nil {
			return c4.ID{}, err
		}
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
	published, at, err := s.publish(tmpName, p, id)
	if err != nil {
		return c4.ID{}, fmt.Errorf("rename: %w", err)
	}
	// Published here or by another writer first, the name is held the
	// same way: under SyncEach its directory is synced before Put returns.
	if err := s.holdName(filepath.Dir(at)); err != nil {
		return c4.ID{}, err
	}
	if !published {
		return id, nil // another writer published the same bytes first
	}
	s.dirty = true

	// Check if the leaf directory needs splitting.
	s.noteAdd(filepath.Dir(p))

	return id, nil
}

// publish renames a complete temp file, whose bytes hash to id, to the
// object's name p, and reports whether this rename is what published it.
//
// The store is content-addressed: a name is only ever given to bytes that
// hash to it, so the same name means the same bytes. When the rename
// fails and the object is now present, another writer (another process,
// or another TreeStore on the same root) published the same bytes between
// this writer's look and its rename. The object is stored; the temp file
// is removed and the failure is not one. On POSIX the rename replaces the
// other copy and does not fail. On Windows it replaces with MoveFileEx,
// which can refuse ("Access is denied") while the other writer's rename
// or a reader holds the name: measured 2026-11-16 on release/1.0.27
// 9175b56 before this change, 3% of concurrent same-ID Puts across eight
// instances failed on every Windows leg and none elsewhere (c4 #181; on
// master #177; TestTreeStoreSameIDPublishers). A failure with the object
// still absent is returned as before.
func (s *TreeStore) publish(tmpName, p string, id c4.ID) (published bool, at string, err error) {
	rename := s.rename
	if rename == nil {
		rename = os.Rename
	}
	err = rename(tmpName, p)
	if err == nil {
		return true, p, nil
	}
	if found, ok := s.locate(id); ok {
		os.Remove(tmpName)
		return false, found, nil
	}
	return false, "", err
}

// holdName makes the directory entry of an object this writer relies on,
// in dir, durable by the time the caller's claim can be made: an object
// it just published, or one it skipped because it was already there.
//
// Through 2026-11-16 every skip returned without syncing anything, and a
// batch whose objects were all present left Sync with nothing to do. The
// name it skipped on may be another writer's, not yet synced (a SyncNone
// writer, or one that died before its barrier), so a power cut could
// leave a journaled claim whose content no store finds (a claim never
// rests on a name that isn't durable;
// TestSkippedObjectSurvivesAPowerCut). A SyncEach publish synced the
// file's bytes and not its name either.
//
// Under SyncBatch the directory is marked touched and the batch made
// dirty, so the barrier syncs it, and its ancestors to the root, once
// per batch however many objects rest on it. Under SyncEach the chain is
// synced, then the device flushed, before the caller returns. Under
// SyncNone nothing is done: atomic, not durable, by stated choice.
// Caller holds s.mu.
func (s *TreeStore) holdName(dir string) error {
	switch s.syncMode {
	case SyncBatch:
		s.markTouched(dir)
		s.dirty = true
	case SyncEach:
		for _, d := range dirsWithinRoot(s.root, dir) {
			if err := syncDirEntries(d); err != nil {
				return fmt.Errorf("sync %s: %w", d, err)
			}
		}
		if err := syncDirFull(s.root); err != nil {
			return err
		}
	}
	return nil
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

// Remove deletes the content for the given ID: every copy of it on the
// ID's path. A write that raced another process's split can leave a
// second copy one level up beside the one at its place (the same bytes,
// since a name is only ever given to bytes that hash to it); through
// 2026-11-16 Remove deleted the first copy it found and Has went on
// reporting the other (TestTreeStoreStaleLevelWriters).
func (s *TreeStore) Remove(id c4.ID) error {
	err := os.Remove(s.path(id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	removed := err == nil
	// Each pass removes one copy or finds one moved deeper, and a path has
	// at most one level per two characters of the ID, so the bound is
	// never reached unless another process keeps publishing the ID.
	for i := 0; i < len(id.String()); i++ {
		p, ok := s.foldFind(id)
		if !ok {
			break
		}
		if rerr := os.Remove(p); rerr != nil {
			if os.IsNotExist(rerr) {
				continue // another process removed or moved it meanwhile
			}
			return rerr
		}
		removed = true
	}
	if removed {
		return nil
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
// as before; only on a miss is foldFind consulted, which looks at every
// level of the trie on the way down. On a miss the exact-case path is
// returned as the write target.
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

// foldFind searches for id's object file from the root down, looking for
// the file at every level before descending, through shard directories
// whose names equal the ID's segments exactly or under case folding.
//
// Every level, because a split moves a leaf's files one level down one
// rename at a time, after making the subdirectories, and another process
// may be splitting right now: from a subdirectory's mkdir until a file's
// own rename the file is still one level up, and a writer that resolved
// its target before another process's split lands there for good, as
// does every file a split cut short by a crash had not moved yet. Through
// 2026-11-16 a lookup descended into any subdirectory that existed and
// looked nowhere else, so those objects read as absent. Looking top down
// is enough, with no lock across processes: a file only ever moves
// deeper and shard directories are never removed, so a file that exists
// is either at the level being looked at, or below it under a directory
// that already exists and is descended into next.
//
// The two premises, as of 2026-11-16, and what pins them:
//   - An object only ever moves deeper. The only renames of an existing
//     object are split's (dir/name to dir/XX/name) and Settle's (only
//     when deeper reports the target below). Every other rename publishes
//     a new object from a temp file: Put, Sync, and the DurableWriter
//     Create hands out.
//   - No shard directory is ever removed. There is no RemoveAll in this
//     package. Every os.Remove names a temp file or an object file:
//     Remove(id) removes path(id) or what foldFind found, both ending in
//     the ID; the rest are temp-file clean-ups. Nothing in cmd/c4 removes
//     inside a store.
//
// TestTreeStoreSplitLeavesItsLevel checks both over real splits; a change
// that breaks either breaks it, and TestTreeStoreSplitKeepsObjectsVisible
// with it.
//
// Case folding, because C4 IDs are base58 and case-sensitive, but a store
// built on a case-insensitive filesystem (default macOS APFS) files IDs
// differing only in case at some level under one shard directory named
// by whichever came first ("c4/1J" holding "c41j..."). On a
// case-sensitive filesystem the exact-case probe misses those objects;
// this finds them without moving anything. Each level probes the file
// and at most four case variants (exact first) by stat rather than
// listing the directory, so a miss never reads a full leaf.
func (s *TreeStore) foldFind(id c4.ID) (string, bool) {
	return s.foldSearch(s.root, id.String(), 0)
}

func (s *TreeStore) foldSearch(dir, str string, i int) (string, bool) {
	// The file at this level first, then the levels below: the order the
	// files move in (see foldFind).
	here := filepath.Join(dir, str)
	if _, err := s.statPath(here); err == nil {
		return here, true
	}
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
	}
	return "", false
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

// split redistributes a leaf directory's objects into 2-char
// subdirectories based on the next prefix segment. Caller must hold s.mu.
//
// Only objects move: files whose names are C4 IDs. Anything else in the
// leaf stays where it is. Through 2026-11-16 a split moved every non-dot
// file by its first two characters: a config file in a store rooted at
// ~/.c4, or a file a user left there, was filed under a two-character
// directory and no longer found where it was
// (TestTreeStoreSplitMovesOnlyObjects).
//
// Each move goes one level down, never up, and no shard directory is ever
// removed: the two premises the top-down lookup rests on (foldFind;
// TestTreeStoreSplitKeepsObjectsVisible, TestTreeStoreSplitLeavesItsLevel).
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
		if _, err := c4.Parse(name); err != nil {
			continue // not an object: anything else kept here stays
		}
		if len(name) <= depth+2 {
			continue // ID too short for another level (shouldn't happen)
		}
		sub := name[depth : depth+2]
		subDir := filepath.Join(dir, sub)
		os.MkdirAll(subDir, 0755)
		os.Rename(filepath.Join(dir, name), filepath.Join(subDir, name))
		// The split moves names out of dir and into subDir: both changed.
		s.markTouched(subDir)
	}
	s.markTouched(dir)
}

// markTouched records a directory whose entries the barrier changed (a
// rename landed in it, or a split moved names in or out), for the entry
// sync at the end of Sync. Caller holds s.mu.
func (s *TreeStore) markTouched(dir string) {
	if s.touched == nil {
		s.touched = make(map[string]bool)
	}
	s.touched[dir] = true
}

// touchedDirs returns every marked directory and its ancestors within
// the root, and clears the marks. Caller holds s.mu.
func (s *TreeStore) touchedDirs() map[string]bool {
	out := make(map[string]bool)
	for d := range s.touched {
		for _, a := range dirsWithinRoot(s.root, d) {
			out[a] = true
		}
	}
	s.touched = nil
	return out
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

// syncDirFull and syncDirEntries are the barrier's two directory syncs,
// as package variables so tests can observe which directories were
// synced, in what order, and that nothing outside the store was.
var (
	syncDirFull    = SyncDir
	syncDirEntries = fsyncDir
)

// dirsWithinRoot returns dir and each of its ancestors that lie inside
// root, root itself included, nearest first. The bound is the relation
// to the root, not string equality, so no spelling of the root (a
// trailing slash, a "./") can make the walk leave the store: with a
// root stored unclean, d == root was never true and the walk reached
// the filesystem root, where opening an ancestor the user cannot read
// failed the barrier and, since the barrier is fatal, stopped the store
// from storing (found in the fleet's review, 2026-11-10).
func dirsWithinRoot(root, dir string) []string {
	var out []string
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		rel, err := filepath.Rel(root, d)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return out
		}
		out = append(out, d)
		if rel == "." || d == filepath.Dir(d) {
			return out
		}
	}
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
