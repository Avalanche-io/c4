package progscan

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// entryRec tracks one entry's position in the c4m file and on disk.
type entryRec struct {
	offset  int64  // byte offset of line start in file
	lineLen int    // total line length including newline
	depth   int    // indentation depth
	name    string // c4m name (dirs have trailing /)
	isDir   bool
	fsPath  string // full filesystem path (for stat/hash)
}

// Scanner performs a three-phase progressive scan of a filesystem,
// writing results to a c4m file with fixed-width padded lines.
// The c4m file is always valid at every point — partial resolution
// is represented as null fields, not missing entries.
type Scanner struct {
	root    string
	outPath string
	f       *os.File
	entries []entryRec
	pos     int64 // current write position in file

	// Display is an optional writer that receives display-format lines
	// during Phase 0. When set, entries stream to this writer as they
	// are discovered, providing immediate output. Set before Phase0().
	Display io.Writer

	// Stats
	Dirs  int
	Files int
}

// New creates a scanner for the given root directory, writing to outPath.
func New(root, outPath string) (*Scanner, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return nil, err
	}
	return &Scanner{
		root:    abs,
		outPath: outPath,
		f:       f,
	}, nil
}

// Close closes the underlying file.
func (s *Scanner) Close() error {
	return s.f.Close()
}

// OutPath returns the path to the working c4m file.
func (s *Scanner) OutPath() string {
	return s.outPath
}

// Phase0 performs structure discovery using readdir (no stat).
// Writes padded entries with type-only mode, null timestamps,
// null sizes, and null C4 IDs.
func (s *Scanner) Phase0() error {
	s.entries = s.entries[:0]
	s.pos = 0
	return s.walkDir(s.root, 0)
}

func (s *Scanner) walkDir(dir string, depth int) error {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", dir, err)
	}

	// Separate files and directories, skip .git.
	var files, dirs []os.DirEntry
	for _, e := range dirEntries {
		if e.Name() == ".git" {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	// Sort each group by name (os.ReadDir already sorts, but be explicit).
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })

	// Write files first.
	for _, e := range files {
		name := c4m.SafeName(e.Name())
		mode := typeOnlyMode(e.Type())
		if err := s.writeEntry(depth, mode, name, filepath.Join(dir, e.Name()), false); err != nil {
			return err
		}
		s.Files++
	}

	// Write each directory entry, then recurse into it.
	for _, e := range dirs {
		name := c4m.SafeName(e.Name()) + "/"
		mode := typeOnlyMode(e.Type())
		if err := s.writeEntry(depth, mode, name, filepath.Join(dir, e.Name()), true); err != nil {
			return err
		}
		s.Dirs++

		if err := s.walkDir(filepath.Join(dir, e.Name()), depth+1); err != nil {
			return err
		}
	}

	return nil
}

func (s *Scanner) writeEntry(depth int, mode os.FileMode, name, fsPath string, isDir bool) error {
	line := PaddedLine(depth, mode, time.Time{}, -1, name, c4.ID{})
	rec := entryRec{
		offset:  s.pos,
		lineLen: len(line),
		depth:   depth,
		name:    name,
		isDir:   isDir,
		fsPath:  fsPath,
	}

	if _, err := s.f.Write(line); err != nil {
		return err
	}
	s.pos += int64(len(line))
	s.entries = append(s.entries, rec)

	// Stream display-format line if a display writer is set.
	if s.Display != nil {
		dl := DisplayLine(depth, mode, time.Time{}, -1, name, c4.ID{})
		if _, err := io.WriteString(s.Display, dl); err != nil {
			return err
		}
	}
	return nil
}

// Phase1 performs metadata resolution by stat-ing each entry and
// writing mode, timestamp, and size in place. Workers operate on
// non-overlapping file regions via pwrite — no locking needed.
func (s *Scanner) Phase1() error {
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	work := make(chan int, workers*4)
	var firstErr atomic.Value
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				rec := &s.entries[i]
				fi, err := os.Lstat(rec.fsPath)
				if err != nil {
					continue
				}

				if _, err := s.f.WriteAt(ModeBytes(fi.Mode()), rec.offset+int64(modeOff(rec.depth))); err != nil {
					firstErr.CompareAndSwap(nil, err)
					return
				}
				if _, err := s.f.WriteAt(TSBytes(fi.ModTime()), rec.offset+int64(tsOff(rec.depth))); err != nil {
					firstErr.CompareAndSwap(nil, err)
					return
				}

				size := int64(-1)
				if !rec.isDir {
					size = fi.Size()
				}
				if _, err := s.f.WriteAt(SizeBytes(size), rec.offset+int64(sizeOff(rec.depth))); err != nil {
					firstErr.CompareAndSwap(nil, err)
					return
				}
			}
		}()
	}

	for i := range s.entries {
		work <- i
	}
	close(work)
	wg.Wait()

	if v := firstErr.Load(); v != nil {
		return v.(error)
	}
	return s.f.Sync()
}

// Phase2 computes C4 IDs for regular files and writes them in place.
// Uses a worker pool sized to CPU count and syncs the file to disk
// at least every 60 seconds so progress is durable.
func (s *Scanner) Phase2() error {
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	// Separate small files (IOPS-bound) from large files (bandwidth-bound).
	const sizeThreshold = 64 * 1024
	var small, large []int
	for i := range s.entries {
		rec := &s.entries[i]
		if rec.isDir {
			continue
		}
		// We need file size from Phase 1. Stat again (cheap, cached by OS).
		fi, err := os.Lstat(rec.fsPath)
		if err != nil {
			continue
		}
		if fi.Size() <= sizeThreshold {
			small = append(small, i)
		} else {
			large = append(large, i)
		}
	}

	var firstErr atomic.Value
	var pending atomic.Int64
	lastSync := time.Now()
	var syncMu sync.Mutex

	hashAndWrite := func(idx int) {
		rec := &s.entries[idx]
		id, err := identifyFile(rec.fsPath)
		if err != nil {
			return
		}
		off := rec.offset + int64(c4idOff(rec.depth, len(rec.name)))
		if _, err := s.f.WriteAt(C4IDBytes(id), off); err != nil {
			firstErr.CompareAndSwap(nil, err)
			return
		}
		pending.Add(1)

		// Periodic sync: flush to disk every 60 seconds.
		syncMu.Lock()
		if time.Since(lastSync) >= 60*time.Second {
			s.f.Sync()
			lastSync = time.Now()
			pending.Store(0)
		}
		syncMu.Unlock()
	}

	// Process small files with many workers (IOPS-bound).
	smallWorkers := workers
	if smallWorkers > len(small) {
		smallWorkers = len(small)
	}
	if smallWorkers > 0 {
		work := make(chan int, smallWorkers*4)
		var wg sync.WaitGroup
		for w := 0; w < smallWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range work {
					if firstErr.Load() != nil {
						return
					}
					hashAndWrite(i)
				}
			}()
		}
		for _, i := range small {
			work <- i
		}
		close(work)
		wg.Wait()
	}

	// Process large files with fewer workers (bandwidth-bound).
	largeWorkers := workers / 2
	if largeWorkers < 1 {
		largeWorkers = 1
	}
	if largeWorkers > len(large) {
		largeWorkers = len(large)
	}
	if largeWorkers > 0 {
		work := make(chan int, largeWorkers*2)
		var wg sync.WaitGroup
		for w := 0; w < largeWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range work {
					if firstErr.Load() != nil {
						return
					}
					hashAndWrite(i)
				}
			}()
		}
		for _, i := range large {
			work <- i
		}
		close(work)
		wg.Wait()
	}

	if v := firstErr.Load(); v != nil {
		return v.(error)
	}
	return s.f.Sync()
}

// DecodeWorking decodes the working c4m file and returns the manifest.
func (s *Scanner) DecodeWorking() (*c4m.Manifest, error) {
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	m, err := c4m.NewDecoder(s.f).Decode()
	if err != nil {
		return nil, fmt.Errorf("decode working file: %w", err)
	}
	return m, nil
}

// Compact writes a canonical c4m (no padding, no gaps) to w by parsing
// the working file through the standard c4m decoder and re-encoding.
func (s *Scanner) Compact(w io.Writer) error {
	m, err := s.DecodeWorking()
	if err != nil {
		return err
	}
	enc := c4m.NewEncoder(w)
	return enc.Encode(m)
}

func identifyFile(path string) (c4.ID, error) {
	f, err := os.Open(path)
	if err != nil {
		return c4.ID{}, err
	}
	defer f.Close()
	return c4.Identify(f), nil
}
