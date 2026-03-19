package reconcile

import (
	"crypto/sha512"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// Target is a destination for distributed content.
type Target interface {
	// Kind returns a description of the target (path or store name).
	Kind() string
}

// DirTarget writes files to a directory on disk.
type DirTarget struct {
	Path string
}

func (d DirTarget) Kind() string { return d.Path }

// StoreTarget stores content by C4 ID in a content store.
type StoreTarget struct {
	Store store.Store
}

func (s StoreTarget) Kind() string { return "store" }

// ToDir creates a directory target.
func ToDir(path string) DirTarget { return DirTarget{Path: path} }

// ToStore creates a store target.
func ToStore(s store.Store) StoreTarget { return StoreTarget{Store: s} }

// DistributeResult holds the outcome of a Distribute operation.
type DistributeResult struct {
	Manifest *c4m.Manifest // scanned manifest with C4 IDs
	Targets  []TargetResult
}

// TargetResult reports per-target outcomes.
type TargetResult struct {
	Target  string  // target description
	Created int     // files/objects created
	Errors  []error // per-file errors (non-fatal)
}

// Distribute reads a source directory once and writes to all targets
// simultaneously. Content is hashed during the read — no second pass.
// Returns the manifest (with C4 IDs) and per-target results.
func Distribute(srcPath string, targets ...Target) (*DistributeResult, error) {
	srcPath, err := filepath.Abs(srcPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrInvalid
	}

	// Precompute target indices.
	targetIdx := make(map[Target]int, len(targets))
	var dirTargets []DirTarget
	var storeTargets []StoreTarget
	results := make([]TargetResult, len(targets))
	for i, t := range targets {
		targetIdx[t] = i
		results[i].Target = t.Kind()
		switch tt := t.(type) {
		case DirTarget:
			dirTargets = append(dirTargets, tt)
		case StoreTarget:
			storeTargets = append(storeTargets, tt)
		}
	}

	manifest := c4m.NewManifest()

	// Track directories for post-pass timestamp setting.
	type dirMeta struct {
		rel   string
		entry *c4m.Entry
	}
	var dirs []dirMeta

	// Walk the source directory.
	err = filepath.Walk(srcPath, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		rel, err := filepath.Rel(srcPath, path)
		if err != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		depth := strings.Count(rel, "/")

		if fi.IsDir() {
			entry := &c4m.Entry{
				Name:      filepath.Base(rel) + "/",
				Mode:      fi.Mode(),
				Timestamp: fi.ModTime().UTC(),
				Size:      -1,
				Depth:     depth,
			}
			manifest.AddEntry(entry)

			// Create directory in all dir targets.
			for _, dt := range dirTargets {
				dstDir := filepath.Join(dt.Path, filepath.FromSlash(rel))
				if mkErr := os.MkdirAll(dstDir, fi.Mode().Perm()); mkErr != nil {
					results[targetIdx[dt]].Errors = append(results[targetIdx[dt]].Errors, mkErr)
				}
			}
			dirs = append(dirs, dirMeta{rel, entry})
			return nil
		}

		if !fi.Mode().IsRegular() {
			return nil
		}

		// Open source file.
		src, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer src.Close()

		// Hash writer — always present.
		hasher := sha512.New()

		// Per-dir-target writers. Each writes independently so one
		// failure doesn't poison the others.
		type targetWriter struct {
			dw   *store.DurableWriter
			idx  int
			path string
		}
		var tws []targetWriter
		for _, dt := range dirTargets {
			dstPath := filepath.Join(dt.Path, filepath.FromSlash(rel))
			if mkErr := os.MkdirAll(filepath.Dir(dstPath), 0755); mkErr != nil {
				results[targetIdx[dt]].Errors = append(results[targetIdx[dt]].Errors, mkErr)
				continue
			}
			dw, dwErr := store.NewDurableWriter(dstPath)
			if dwErr != nil {
				results[targetIdx[dt]].Errors = append(results[targetIdx[dt]].Errors, dwErr)
				continue
			}
			tws = append(tws, targetWriter{dw, targetIdx[dt], dstPath})
		}

		// Temp file for store targets (if any).
		var storeTmp *os.File
		if len(storeTargets) > 0 {
			storeTmp, err = os.CreateTemp("", "c4dist.*")
			if err != nil {
				// Can't create temp — store targets will fail.
				for _, st := range storeTargets {
					results[targetIdx[st]].Errors = append(results[targetIdx[st]].Errors, err)
				}
			}
		}

		// Build the MultiWriter: hasher + all healthy dir writers + store temp.
		writers := make([]io.Writer, 0, 1+len(tws)+1)
		writers = append(writers, hasher)
		for _, tw := range tws {
			writers = append(writers, tw.dw)
		}
		if storeTmp != nil {
			writers = append(writers, storeTmp)
		}

		mw := io.MultiWriter(writers...)
		_, copyErr := io.Copy(mw, src)

		// Close dir target writers.
		for _, tw := range tws {
			if closeErr := tw.dw.Close(); closeErr != nil {
				results[tw.idx].Errors = append(results[tw.idx].Errors, closeErr)
			} else if copyErr == nil {
				results[tw.idx].Created++
				// Set file metadata (permissions, timestamps).
				os.Chmod(tw.path, fi.Mode().Perm())
				os.Chtimes(tw.path, fi.ModTime(), fi.ModTime())
			}
		}

		// Compute C4 ID.
		var id c4.ID
		copy(id[:], hasher.Sum(nil))

		// Store to all store targets.
		if storeTmp != nil && copyErr == nil {
			for _, st := range storeTargets {
				if st.Store.Has(id) {
					results[targetIdx[st]].Created++ // already present counts
					continue
				}
				if _, seekErr := storeTmp.Seek(0, 0); seekErr != nil {
					results[targetIdx[st]].Errors = append(results[targetIdx[st]].Errors, seekErr)
					continue
				}
				if _, putErr := st.Store.Put(storeTmp); putErr != nil {
					results[targetIdx[st]].Errors = append(results[targetIdx[st]].Errors, putErr)
				} else {
					results[targetIdx[st]].Created++
				}
			}
		}
		if storeTmp != nil {
			storeTmp.Close()
			os.Remove(storeTmp.Name())
		}

		// Build manifest entry only if the copy succeeded.
		if copyErr == nil {
			entry := &c4m.Entry{
				Name:      filepath.Base(rel),
				Mode:      fi.Mode(),
				Timestamp: fi.ModTime().UTC(),
				Size:      fi.Size(),
				C4ID:      id,
				Depth:     depth,
			}
			manifest.AddEntry(entry)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Propagate directory sizes/timestamps and sort.
	manifest.Canonicalize()

	// Post-pass: set directory timestamps on dir targets (deepest first).
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i].rel) > len(dirs[j].rel)
	})
	for _, di := range dirs {
		for _, dt := range dirTargets {
			dstDir := filepath.Join(dt.Path, filepath.FromSlash(di.rel))
			if di.entry != nil {
				if !di.entry.Timestamp.Equal(c4m.NullTimestamp()) {
					os.Chtimes(dstDir, di.entry.Timestamp, di.entry.Timestamp)
				}
				if di.entry.Mode != 0 {
					os.Chmod(dstDir, di.entry.Mode.Perm())
				}
			}
		}
	}

	return &DistributeResult{
		Manifest: manifest,
		Targets:  results,
	}, nil
}
