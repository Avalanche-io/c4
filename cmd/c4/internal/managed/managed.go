// Package managed implements c4-tracked directories.
//
// A managed directory has a .c4/ subdirectory containing:
//   - snapshots/ — c4m files stored by their C4 ID
//   - history    — newline-separated C4 IDs, newest first
//   - redo       — redo stack (C4 IDs of undone states)
//   - tags/      — named references to snapshot C4 IDs
//   - ignore     — exclusion patterns in c4m all-nil entry format
//
// The `:` colon notation in the CLI maps to managed directory operations.
// Read through `:` is implicit. Write through `:` requires establishment
// via `c4 mk :`.
package managed

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

// atomicWriteFile writes data to path atomically via temp file + fsync + rename.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp.*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

const metaDir = ".c4"

// Dir represents a c4-managed directory.
type Dir struct {
	root string // the managed directory path
	meta string // .c4/ directory path
}

// IsManaged returns true if the given directory is c4-managed.
func IsManaged(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(abs, metaDir, "history"))
	return err == nil
}

// Open opens an existing managed directory.
func Open(path string) (*Dir, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	meta := filepath.Join(abs, metaDir)
	if _, err := os.Stat(filepath.Join(meta, "history")); err != nil {
		return nil, fmt.Errorf("not a managed directory: %s", abs)
	}
	return &Dir{root: abs, meta: meta}, nil
}

// Init establishes a directory for c4 tracking. Scans the directory
// and stores the initial snapshot.
func Init(path string, excludePatterns []string) (*Dir, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	meta := filepath.Join(abs, metaDir)
	if _, err := os.Stat(filepath.Join(meta, "history")); err == nil {
		return nil, fmt.Errorf("already managed: %s", abs)
	}

	// Create directory structure
	for _, sub := range []string{"snapshots", "tags"} {
		if err := os.MkdirAll(filepath.Join(meta, sub), 0755); err != nil {
			return nil, fmt.Errorf("create %s: %w", sub, err)
		}
	}

	d := &Dir{root: abs, meta: meta}

	// Write ignore patterns if provided
	if len(excludePatterns) > 0 {
		if err := d.writeIgnore(excludePatterns); err != nil {
			return nil, fmt.Errorf("write ignore: %w", err)
		}
	}

	// Scan and store initial snapshot
	manifest, err := d.scan()
	if err != nil {
		return nil, fmt.Errorf("initial scan: %w", err)
	}

	id, err := d.storeSnapshot(manifest)
	if err != nil {
		return nil, fmt.Errorf("store snapshot: %w", err)
	}

	// Initialize history with the first snapshot
	if err := d.writeHistory([]string{id.String()}); err != nil {
		return nil, fmt.Errorf("write history: %w", err)
	}

	// Clear redo stack
	if err := atomicWriteFile(filepath.Join(d.meta, "redo"), nil, 0644); err != nil {
		return nil, fmt.Errorf("write redo: %w", err)
	}

	return d, nil
}

// Root returns the managed directory path.
func (d *Dir) Root() string { return d.root }

// Current returns the current (head) snapshot manifest.
func (d *Dir) Current() (*c4m.Manifest, error) {
	history, err := d.readHistory()
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, fmt.Errorf("empty history")
	}
	return d.loadSnapshot(history[0])
}

// Snapshot captures the current directory state and pushes it onto history.
// Clears the redo stack (new change after undo = forward history detaches).
// Returns the C4 ID of the new snapshot.
func (d *Dir) Snapshot() (c4.ID, error) {
	unlock, err := d.lock()
	if err != nil {
		return c4.ID{}, fmt.Errorf("lock: %w", err)
	}
	defer unlock()

	manifest, err := d.scan()
	if err != nil {
		return c4.ID{}, fmt.Errorf("scan: %w", err)
	}

	id, err := d.storeSnapshot(manifest)
	if err != nil {
		return c4.ID{}, fmt.Errorf("store: %w", err)
	}

	history, err := d.readHistory()
	if err != nil {
		return c4.ID{}, err
	}

	history = append([]string{id.String()}, history...)

	// Prune history if retention limit is set
	if retain := d.Retain(); retain > 0 && len(history) > retain {
		history = history[:retain]
	}

	if err := d.writeHistory(history); err != nil {
		return c4.ID{}, err
	}

	// Clear redo stack — new change detaches forward history
	if err := atomicWriteFile(filepath.Join(d.meta, "redo"), nil, 0644); err != nil {
		return c4.ID{}, fmt.Errorf("clear redo: %w", err)
	}

	// Clean up orphaned snapshot files
	d.pruneSnapshots()

	return id, nil
}

// GetSnapshot returns the Nth ancestor snapshot (0 = current).
func (d *Dir) GetSnapshot(n int) (*c4m.Manifest, error) {
	history, err := d.readHistory()
	if err != nil {
		return nil, err
	}
	if n < 0 || n >= len(history) {
		return nil, fmt.Errorf("snapshot ~%d does not exist (history has %d entries)", n, len(history))
	}
	return d.loadSnapshot(history[n])
}

// HistoryEntry represents one snapshot in the history chain.
type HistoryEntry struct {
	Index     int
	ID        string
	Timestamp time.Time
}

// History returns the full snapshot history, newest first.
func (d *Dir) History() ([]HistoryEntry, error) {
	history, err := d.readHistory()
	if err != nil {
		return nil, err
	}
	entries := make([]HistoryEntry, len(history))
	for i, id := range history {
		entries[i] = HistoryEntry{Index: i, ID: id}
		snapPath := filepath.Join(d.meta, "snapshots", id)
		if info, serr := os.Stat(snapPath); serr == nil {
			entries[i].Timestamp = info.ModTime().UTC()
		}
	}
	return entries, nil
}

// HistoryLen returns the number of snapshots in history.
func (d *Dir) HistoryLen() (int, error) {
	history, err := d.readHistory()
	if err != nil {
		return 0, err
	}
	return len(history), nil
}

// Undo reverts to the previous snapshot. The current state moves to
// the redo stack.
func (d *Dir) Undo() error {
	unlock, err := d.lock()
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer unlock()

	history, err := d.readHistory()
	if err != nil {
		return err
	}
	if len(history) < 2 {
		return fmt.Errorf("nothing to undo")
	}

	// Pop current from history, push to redo
	undone := history[0]
	history = history[1:]

	redo, err := d.readRedo()
	if err != nil {
		return err
	}
	redo = append([]string{undone}, redo...)

	if err := d.writeHistory(history); err != nil {
		return err
	}
	return d.writeRedo(redo)
}

// Redo re-applies the last undone snapshot.
func (d *Dir) Redo() error {
	unlock, err := d.lock()
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer unlock()

	redo, err := d.readRedo()
	if err != nil {
		return err
	}
	if len(redo) == 0 {
		return fmt.Errorf("nothing to redo")
	}

	// Pop from redo, push to history
	redone := redo[0]
	redo = redo[1:]

	history, err := d.readHistory()
	if err != nil {
		return err
	}
	history = append([]string{redone}, history...)

	if err := d.writeHistory(history); err != nil {
		return err
	}
	return d.writeRedo(redo)
}

// GetTag returns the snapshot for a named tag.
func (d *Dir) GetTag(name string) (*c4m.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(d.meta, "tags", name))
	if err != nil {
		return nil, fmt.Errorf("tag %q not found", name)
	}
	idStr := strings.TrimSpace(string(data))
	return d.loadSnapshot(idStr)
}

// SetTag creates or updates a named tag pointing to a snapshot C4 ID.
func (d *Dir) SetTag(name, c4id string) error {
	unlock, err := d.lock()
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer unlock()

	// Verify the snapshot exists
	snapPath := filepath.Join(d.meta, "snapshots", c4id)
	if _, err := os.Stat(snapPath); err != nil {
		return fmt.Errorf("snapshot %s does not exist", c4id)
	}
	return atomicWriteFile(filepath.Join(d.meta, "tags", name), []byte(c4id+"\n"), 0644)
}

// RemoveTag removes a named tag.
func (d *Dir) RemoveTag(name string) error {
	unlock, err := d.lock()
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer unlock()
	return os.Remove(filepath.Join(d.meta, "tags", name))
}

// SetRetain sets the maximum number of snapshots to keep in history.
// On each new snapshot, older entries beyond this limit are pruned.
// Set to 0 to disable retention pruning (keep all).
func (d *Dir) SetRetain(n int) error {
	if n <= 0 {
		return os.Remove(filepath.Join(d.meta, "retain"))
	}
	return atomicWriteFile(filepath.Join(d.meta, "retain"), []byte(strconv.Itoa(n)+"\n"), 0644)
}

// Retain returns the configured retention limit, or 0 if unlimited.
func (d *Dir) Retain() int {
	data, err := os.ReadFile(filepath.Join(d.meta, "retain"))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// ListTags returns all tag names and their C4 IDs.
func (d *Dir) ListTags() (map[string]string, error) {
	entries, err := os.ReadDir(filepath.Join(d.meta, "tags"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	tags := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(d.meta, "tags", e.Name()))
		if err != nil {
			continue
		}
		tags[e.Name()] = strings.TrimSpace(string(data))
	}
	return tags, nil
}

// AddIgnorePatterns appends exclusion patterns.
func (d *Dir) AddIgnorePatterns(patterns []string) error {
	unlock, err := d.lock()
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer unlock()

	existing, _ := d.readIgnorePatterns()
	combined := append(existing, patterns...)
	return d.writeIgnore(combined)
}

// IgnorePatterns returns the current exclusion patterns.
func (d *Dir) IgnorePatterns() ([]string, error) {
	return d.readIgnorePatterns()
}

// RemoveIgnorePattern removes a single exclusion pattern.
func (d *Dir) RemoveIgnorePattern(pattern string) error {
	unlock, err := d.lock()
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer unlock()

	existing, err := d.readIgnorePatterns()
	if err != nil {
		return err
	}
	var filtered []string
	for _, p := range existing {
		if p != pattern {
			filtered = append(filtered, p)
		}
	}
	return d.writeIgnore(filtered)
}

// Teardown removes the .c4/ directory, stopping tracking.
// The filesystem is left untouched.
func (d *Dir) Teardown() error {
	return os.RemoveAll(d.meta)
}

// pruneSnapshots removes snapshot files not referenced by history, redo, or tags.
func (d *Dir) pruneSnapshots() {
	// Collect all referenced snapshot IDs
	referenced := make(map[string]struct{})

	history, _ := d.readHistory()
	for _, id := range history {
		referenced[id] = struct{}{}
	}
	redo, _ := d.readRedo()
	for _, id := range redo {
		referenced[id] = struct{}{}
	}
	tags, _ := d.ListTags()
	for _, id := range tags {
		referenced[id] = struct{}{}
	}

	// Walk snapshots/ and remove unreferenced files
	entries, err := os.ReadDir(filepath.Join(d.meta, "snapshots"))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := referenced[e.Name()]; ok {
			continue
		}
		os.Remove(filepath.Join(d.meta, "snapshots", e.Name()))
	}
}

// --- Internal methods ---

// scan produces a c4m manifest of the managed directory, excluding .c4/.
func (d *Dir) scan() (*c4m.Manifest, error) {
	excludes := []string{metaDir}
	patterns, _ := d.readIgnorePatterns()
	excludes = append(excludes, patterns...)

	gen := scan.NewGeneratorWithOptions(
		scan.WithC4IDs(true),
		scan.WithExclude(excludes),
	)
	return gen.GenerateFromPath(d.root)
}

// storeSnapshot encodes a manifest to canonical c4m, computes its C4 ID,
// and stores it in snapshots/.
func (d *Dir) storeSnapshot(manifest *c4m.Manifest) (c4.ID, error) {
	var buf bytes.Buffer
	enc := c4m.NewEncoder(&buf)
	if err := enc.Encode(manifest); err != nil {
		return c4.ID{}, fmt.Errorf("encode: %w", err)
	}

	data := buf.Bytes()
	id := c4.Identify(bytes.NewReader(data))

	snapPath := filepath.Join(d.meta, "snapshots", id.String())
	if _, err := os.Stat(snapPath); err == nil {
		return id, nil // already stored (idempotent)
	}
	if err := atomicWriteFile(snapPath, data, 0644); err != nil {
		return c4.ID{}, fmt.Errorf("write snapshot: %w", err)
	}
	return id, nil
}

// loadSnapshot reads and decodes a snapshot by its C4 ID string.
func (d *Dir) loadSnapshot(idStr string) (*c4m.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(d.meta, "snapshots", idStr))
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", idStr, err)
	}
	return c4m.NewDecoder(bytes.NewReader(data)).Decode()
}

func (d *Dir) readHistory() ([]string, error) {
	return d.readIDList(filepath.Join(d.meta, "history"))
}

func (d *Dir) writeHistory(ids []string) error {
	return d.writeIDList(filepath.Join(d.meta, "history"), ids)
}

func (d *Dir) readRedo() ([]string, error) {
	return d.readIDList(filepath.Join(d.meta, "redo"))
}

func (d *Dir) writeRedo(ids []string) error {
	return d.writeIDList(filepath.Join(d.meta, "redo"), ids)
}

func (d *Dir) readIDList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}

func (d *Dir) writeIDList(path string, ids []string) error {
	content := ""
	if len(ids) > 0 {
		content = strings.Join(ids, "\n") + "\n"
	}
	return atomicWriteFile(path, []byte(content), 0644)
}

func (d *Dir) readIgnorePatterns() ([]string, error) {
	data, err := os.ReadFile(filepath.Join(d.meta, "ignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}

func (d *Dir) writeIgnore(patterns []string) error {
	content := ""
	if len(patterns) > 0 {
		content = strings.Join(patterns, "\n") + "\n"
	}
	return atomicWriteFile(filepath.Join(d.meta, "ignore"), []byte(content), 0644)
}
