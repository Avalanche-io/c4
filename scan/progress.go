package scan

import (
	"sync"
	"sync/atomic"
	"time"
)

// ScanStats is a snapshot of scan progress passed to the WithProgress callback.
type ScanStats struct {
	Entries int64         // entries scanned so far
	Bytes   int64         // bytes scanned (file content sizes summed)
	Files   int64         // file entries scanned
	Dirs    int64         // directory entries scanned
	Current string        // path currently being processed
	Elapsed time.Duration // wall-clock time since scan start
}

// progress tracks running totals and rate-limits callback invocations.
// Fires whichever comes first: every entryThreshold entries or every timeInterval.
// time.Now() is only consulted every timeCheckEvery entries to avoid clock overhead.
//
// All counters are accessed atomically so record() is safe from multiple
// goroutines under parallel walk. fire() takes a mutex so callbacks observe
// a consistent snapshot.
type progress struct {
	cb      func(ScanStats)
	start   time.Time
	entries int64
	bytes   int64
	files   int64
	dirs    int64

	mu           sync.Mutex
	current      string
	lastEntries  int64
	lastFireTime time.Time
}

const (
	entryThreshold = 1000
	timeInterval   = 250 * time.Millisecond
	timeCheckEvery = 100
)

func newProgress(cb func(ScanStats)) *progress {
	now := time.Now()
	return &progress{cb: cb, start: now, lastFireTime: now}
}

// record updates running totals for one entry and fires the callback if the
// thresholds dictate. size < 0 (e.g. directories) is treated as 0 bytes.
// Safe for concurrent use.
func (p *progress) record(path string, isDir bool, size int64) {
	n := atomic.AddInt64(&p.entries, 1)
	if isDir {
		atomic.AddInt64(&p.dirs, 1)
	} else {
		atomic.AddInt64(&p.files, 1)
		if size > 0 {
			atomic.AddInt64(&p.bytes, size)
		}
	}

	// Entry-count threshold — cheap branch, no clock call.
	p.mu.Lock()
	p.current = path
	if n-p.lastEntries >= entryThreshold {
		p.fireLocked()
		p.mu.Unlock()
		return
	}
	// Time-based threshold — sample sparsely to avoid time.Now() per entry.
	if n%timeCheckEvery == 0 && time.Since(p.lastFireTime) >= timeInterval {
		p.fireLocked()
	}
	p.mu.Unlock()
}

// fireLocked snapshots counters and invokes the callback. Caller holds p.mu.
func (p *progress) fireLocked() {
	p.lastEntries = atomic.LoadInt64(&p.entries)
	p.lastFireTime = time.Now()
	// Pass a value copy so the callback can't observe mid-update mutation.
	p.cb(ScanStats{
		Entries: atomic.LoadInt64(&p.entries),
		Bytes:   atomic.LoadInt64(&p.bytes),
		Files:   atomic.LoadInt64(&p.files),
		Dirs:    atomic.LoadInt64(&p.dirs),
		Current: p.current,
		Elapsed: time.Since(p.start),
	})
}

// final fires a last callback so callers see the completed totals.
func (p *progress) final() {
	p.mu.Lock()
	p.fireLocked()
	p.mu.Unlock()
}
