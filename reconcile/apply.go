package reconcile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// Apply executes the plan against the filesystem.
// Each operation checks if already done before executing (idempotent).
func (r *Reconciler) Apply(plan *Plan, dirPath string) (*Result, error) {
	dirPath, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, err
	}

	res := &Result{}

	// Collect directory operations for a post-pass. Directory metadata
	// (especially timestamps) must be set AFTER all children are written,
	// because writing children updates the directory mtime.
	var dirOps []Operation

	// Consecutive creates batch for parallel execution. The batch flushes
	// before any other operation type, preserving plan order between
	// creates and everything else.
	var creates []Operation
	// Directories this apply made: a null timestamp on one of them
	// becomes the epoch, so the tree reads back at the target's ID; on
	// a directory that already existed a null never overwrites.
	freshDirs := make(map[string]bool)
	flush := func() {
		r.applyCreates(creates, res)
		creates = creates[:0]
	}

	for _, op := range plan.Operations {
		if op.Type == OpCreate {
			creates = append(creates, op)
			continue
		}
		flush()
		switch op.Type {
		case OpMkdir:
			made, err := r.applyMkdir(op, res)
			if err != nil {
				res.Errors = append(res.Errors, err)
			}
			if made {
				freshDirs[op.Path] = true
			}
			dirOps = append(dirOps, op)
		case OpMove:
			if err := r.applyMove(op, res); err != nil {
				res.Errors = append(res.Errors, err)
			}
		case OpSymlink:
			if err := r.applySymlink(op, res); err != nil {
				res.Errors = append(res.Errors, err)
			}
		case OpChmod:
			if err := r.applyChmod(op, res); err != nil {
				res.Errors = append(res.Errors, err)
			}
		case OpChtimes:
			// Defer directory chtimes to the post-pass (children may update mtime).
			if op.Entry != nil && op.Entry.IsDir() {
				dirOps = append(dirOps, op)
			} else {
				if err := r.applyChtimes(op, res); err != nil {
					res.Errors = append(res.Errors, err)
				}
			}
		case OpRemove:
			if err := r.applyRemove(op, res); err != nil {
				res.Errors = append(res.Errors, err)
			}
		case OpRmdir:
			if err := r.applyRmdir(op, res); err != nil {
				res.Errors = append(res.Errors, err)
			}
		}
	}
	flush()

	// Post-pass: set directory metadata. Process deepest first so that
	// setting a child directory's timestamp doesn't reset its parent's.
	if !r.dryRun {
		sort.Slice(dirOps, func(i, j int) bool {
			return len(dirOps[i].Path) > len(dirOps[j].Path) // deepest first
		})
		for _, op := range dirOps {
			r.setMetadata(op.Path, op.Entry, freshDirs[op.Path])
		}
	}

	// Batch barrier: created files were handed to the device with cheap
	// flushes; one device-cache flush makes the whole batch durable
	// before Apply returns. Skipped when nothing was written. No-op on
	// Windows, where per-file Sync already reaches stable storage.
	if !r.dryRun && r.syncMode == store.SyncBatch && r.wroteFiles {
		if err := store.SyncDir(dirPath); err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("durability barrier: %w", err))
		}
	}

	return res, nil
}

// applyMkdir makes the directory; made reports whether it did (false
// when it already existed or in a dry run).
func (r *Reconciler) applyMkdir(op Operation, res *Result) (made bool, err error) {
	info, err := os.Stat(op.Path)
	if err == nil && info.IsDir() {
		res.Skipped++
		return false, nil
	}
	if r.dryRun {
		res.Created++
		return false, nil
	}
	mode := os.FileMode(0755)
	if op.Entry != nil && op.Entry.Mode != 0 {
		mode = op.Entry.Mode.Perm() | os.ModeDir
	}
	if err := os.MkdirAll(op.Path, mode.Perm()); err != nil {
		return false, err
	}
	res.Created++
	return true, nil
}

// applyCreates executes create operations, in parallel when the worker
// count allows. Counters and errors merge in operation order regardless
// of completion order, so results are deterministic.
func (r *Reconciler) applyCreates(ops []Operation, res *Result) {
	if len(ops) > 0 && !r.dryRun {
		r.wroteFiles = true
	}
	workers := r.workers(len(ops))
	if workers <= 1 || r.dryRun {
		for _, op := range ops {
			if err := r.applyCreate(op, res); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("create %s: %w", op.Path, err))
			}
		}
		return
	}

	results := make([]Result, len(ops))
	errs := make([]error, len(ops))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range ops {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer func() { <-sem; wg.Done() }()
			errs[i] = r.applyCreate(ops[i], &results[i])
		}(i)
	}
	wg.Wait()

	for i := range ops {
		res.Created += results[i].Created
		res.Skipped += results[i].Skipped
		if errs[i] != nil {
			res.Errors = append(res.Errors, fmt.Errorf("create %s: %w", ops[i].Path, errs[i]))
		}
	}
}

func (r *Reconciler) applyCreate(op Operation, res *Result) error {
	// Idempotent: check if file already has correct content.
	size := int64(-1)
	if op.Entry != nil {
		size = op.Entry.Size
	}
	if !op.ContentID.IsNil() && fileMatchesID(op.Path, op.ContentID, size) {
		res.Skipped++
		return nil
	}
	if r.dryRun {
		res.Created++
		return nil
	}

	// A create planned from a move's destination (one content ID wanted at
	// two paths) copies from there; the move has already run.
	if op.SrcPath != "" {
		if err := r.copyFile(op.SrcPath, op.Path); err == nil {
			r.setMetadata(op.Path, op.Entry, true)
			res.Created++
			return nil
		}
	}
	// Local fast path: copy file-to-file from a LocalSource, falling back
	// to streaming on any failure.
	if src, ok := r.localPath(op.ContentID); ok {
		if err := r.copyFile(src, op.Path); err == nil {
			r.setMetadata(op.Path, op.Entry, true)
			res.Created++
			return nil
		}
	}

	rc, err := r.openContent(op.ContentID)
	if err != nil {
		return err
	}
	defer rc.Close()

	dw, err := r.newWriter(op.Path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dw, rc); err != nil {
		return err
	}
	if err := dw.Close(); err != nil {
		return err
	}

	// Set metadata: the file is new, so a null timestamp becomes the epoch.
	r.setMetadata(op.Path, op.Entry, true)
	res.Created++
	return nil
}

func (r *Reconciler) applyMove(op Operation, res *Result) error {
	// Idempotent: check if dest already correct.
	size := int64(-1)
	if op.Entry != nil {
		size = op.Entry.Size
	}
	if !op.ContentID.IsNil() && fileMatchesID(op.Path, op.ContentID, size) {
		res.Skipped++
		return nil
	}
	if r.dryRun {
		res.Moved++
		return nil
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(op.Path), 0755); err != nil {
		return err
	}

	err := os.Rename(op.SrcPath, op.Path)
	if err != nil {
		// Cross-device: fall back to copy + remove.
		if isEXDEV(err) {
			if err := r.copyFile(op.SrcPath, op.Path); err != nil {
				return err
			}
			if err := os.Remove(op.SrcPath); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else {
			return err
		}
	}

	// Moved content existed before: a null never overwrites its time.
	r.setMetadata(op.Path, op.Entry, false)
	res.Moved++
	return nil
}

func (r *Reconciler) applySymlink(op Operation, res *Result) error {
	if op.Entry == nil {
		return nil
	}
	target, err := os.Readlink(op.Path)
	if err == nil && target == op.Entry.Target {
		res.Skipped++
		return nil
	}
	if r.dryRun {
		res.Created++
		return nil
	}

	if err := os.Remove(op.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(op.Path), 0755); err != nil {
		return err
	}
	if err := os.Symlink(op.Entry.Target, op.Path); err != nil {
		return err
	}
	// A link's own time, as setMetadata does for a file: the entry's time,
	// or the epoch for a link created from a null time (scans back null).
	// os.Chtimes would follow the link and change its target instead.
	t := op.Entry.Timestamp
	if err := r.setLinkTimeOrRecord(op.Path, t, res); err != nil {
		return err
	}
	res.Created++
	return nil
}

// setLinkTimeOrRecord sets a symlink's own time. Where the platform cannot,
// the path is recorded in res.LinkTimesUnset rather than silently left.
func (r *Reconciler) setLinkTimeOrRecord(path string, t time.Time, res *Result) error {
	if !linkTimeSupported {
		res.LinkTimesUnset = append(res.LinkTimesUnset, path)
		return nil
	}
	return setLinkTime(path, t)
}

func (r *Reconciler) applyChmod(op Operation, res *Result) error {
	if op.Entry == nil {
		return nil
	}
	info, err := os.Stat(op.Path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() == op.Entry.Mode.Perm() {
		res.Skipped++
		return nil
	}
	if r.dryRun {
		res.Updated++
		return nil
	}
	if err := os.Chmod(op.Path, op.Entry.Mode.Perm()); err != nil {
		return err
	}
	res.Updated++
	return nil
}

func (r *Reconciler) applyChtimes(op Operation, res *Result) error {
	if op.Entry == nil {
		return nil
	}
	if op.Entry.Timestamp.Equal(c4m.NullTimestamp()) {
		res.Skipped++
		return nil
	}
	if r.dryRun {
		res.Updated++
		return nil
	}
	if op.Entry.IsSymlink() {
		if err := r.setLinkTimeOrRecord(op.Path, op.Entry.Timestamp, res); err != nil {
			return err
		}
		res.Updated++
		return nil
	}
	if err := os.Chtimes(op.Path, op.Entry.Timestamp, op.Entry.Timestamp); err != nil {
		return err
	}
	res.Updated++
	return nil
}

func (r *Reconciler) applyRemove(op Operation, res *Result) error {
	info, err := os.Lstat(op.Path)
	if os.IsNotExist(err) {
		res.Skipped++
		return nil
	}
	if r.dryRun {
		res.Removed++
		return nil
	}
	// Store content before removing if requested.
	if r.storeRemovals != nil && info != nil && info.Mode().IsRegular() {
		if !op.ContentID.IsNil() && !r.storeRemovals.Has(op.ContentID) {
			f, err := os.Open(op.Path)
			if err == nil {
				if _, err := r.storeRemovals.Put(f); err != nil {
					res.Errors = append(res.Errors, fmt.Errorf("store removal %s: %w", op.Path, err))
				}
				f.Close()
			}
		}
	}
	if err := os.Remove(op.Path); err != nil {
		return err
	}
	res.Removed++
	return nil
}

func (r *Reconciler) applyRmdir(op Operation, res *Result) error {
	entries, err := os.ReadDir(op.Path)
	if err != nil {
		res.Skipped++
		return nil
	}
	if len(entries) != 0 {
		res.Skipped++
		return nil
	}
	if r.dryRun {
		res.Removed++
		return nil
	}
	if err := os.Remove(op.Path); err != nil {
		return err
	}
	res.Removed++
	return nil
}

// setMetadata applies mode and timestamp from an entry to a file path.
// A null mode or timestamp never overwrites what the path already
// has. When the path is fresh (made by this apply), a null timestamp
// becomes the epoch, which scans back as null, so a tree restored
// from a null-dated description reads back at the description's ID.
func (r *Reconciler) setMetadata(path string, entry *c4m.Entry, fresh bool) {
	if entry == nil {
		return
	}
	if entry.Mode != 0 {
		os.Chmod(path, entry.Mode.Perm())
	}
	switch {
	case !entry.Timestamp.Equal(c4m.NullTimestamp()):
		os.Chtimes(path, entry.Timestamp, entry.Timestamp)
	case fresh:
		os.Chtimes(path, c4m.NullTimestamp(), c4m.NullTimestamp())
	}
}

// copyFile copies src to dst atomically, attempting a copy-on-write
// clone first. The byte-copy fallback goes file-to-file so the OS can
// accelerate it (see store.DurableWriter.ReadFrom).
func (r *Reconciler) copyFile(src, dst string) error {
	if err := cloneFile(src, dst); err == nil {
		return nil
	}

	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	dw, err := r.newWriter(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dw, sf); err != nil {
		return err
	}
	return dw.Close()
}

// isEXDEV returns true if err is an EXDEV (cross-device link) error.
func isEXDEV(err error) bool {
	lerr, ok := err.(*os.LinkError)
	if !ok {
		return false
	}
	errno, ok := lerr.Err.(syscall.Errno)
	return ok && errno == syscall.EXDEV
}
