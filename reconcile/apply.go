package reconcile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"

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

	for _, op := range plan.Operations {
		switch op.Type {
		case OpMkdir:
			if err := r.applyMkdir(op, res); err != nil {
				res.Errors = append(res.Errors, err)
			}
			dirOps = append(dirOps, op)
		case OpCreate:
			if err := r.applyCreate(op, res); err != nil {
				res.Errors = append(res.Errors, err)
			}
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

	// Post-pass: set directory metadata. Process deepest first so that
	// setting a child directory's timestamp doesn't reset its parent's.
	if !r.dryRun {
		sort.Slice(dirOps, func(i, j int) bool {
			return len(dirOps[i].Path) > len(dirOps[j].Path) // deepest first
		})
		for _, op := range dirOps {
			r.setMetadata(op.Path, op.Entry)
		}
	}

	return res, nil
}

func (r *Reconciler) applyMkdir(op Operation, res *Result) error {
	info, err := os.Stat(op.Path)
	if err == nil && info.IsDir() {
		res.Skipped++
		return nil
	}
	if r.dryRun {
		res.Created++
		return nil
	}
	mode := os.FileMode(0755)
	if op.Entry != nil && op.Entry.Mode != 0 {
		mode = op.Entry.Mode.Perm() | os.ModeDir
	}
	if err := os.MkdirAll(op.Path, mode.Perm()); err != nil {
		return err
	}
	res.Created++
	return nil
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

	rc, err := r.openContent(op.ContentID)
	if err != nil {
		return err
	}
	defer rc.Close()

	dw, err := store.NewDurableWriter(op.Path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dw, rc); err != nil {
		return err
	}
	if err := dw.Close(); err != nil {
		return err
	}

	// Set metadata.
	r.setMetadata(op.Path, op.Entry)
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
			if err := copyFile(op.SrcPath, op.Path); err != nil {
				return err
			}
			if err := os.Remove(op.SrcPath); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else {
			return err
		}
	}

	r.setMetadata(op.Path, op.Entry)
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
	res.Created++
	return nil
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
func (r *Reconciler) setMetadata(path string, entry *c4m.Entry) {
	if entry == nil {
		return
	}
	if entry.Mode != 0 {
		os.Chmod(path, entry.Mode.Perm())
	}
	if !entry.Timestamp.Equal(c4m.NullTimestamp()) {
		os.Chtimes(path, entry.Timestamp, entry.Timestamp)
	}
}

// copyFile copies src to dst using a durable writer.
func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	dw, err := store.NewDurableWriter(dst)
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
