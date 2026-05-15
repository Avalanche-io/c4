package scan

import "time"

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
type progress struct {
	cb           func(ScanStats)
	start        time.Time
	entries      int64
	bytes        int64
	files        int64
	dirs         int64
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
func (p *progress) record(path string, isDir bool, size int64) {
	p.entries++
	p.current = path
	if isDir {
		p.dirs++
	} else {
		p.files++
		if size > 0 {
			p.bytes += size
		}
	}

	// Entry-count threshold — cheap branch, no clock call.
	if p.entries-p.lastEntries >= entryThreshold {
		p.fire()
		return
	}
	// Time-based threshold — sample sparsely to avoid time.Now() per entry.
	if p.entries%timeCheckEvery == 0 && time.Since(p.lastFireTime) >= timeInterval {
		p.fire()
	}
}

func (p *progress) fire() {
	p.lastEntries = p.entries
	p.lastFireTime = time.Now()
	// Pass a value copy so the callback can't observe mid-update mutation.
	p.cb(ScanStats{
		Entries: p.entries,
		Bytes:   p.bytes,
		Files:   p.files,
		Dirs:    p.dirs,
		Current: p.current,
		Elapsed: time.Since(p.start),
	})
}

// final fires a last callback so callers see the completed totals.
func (p *progress) final() {
	p.fire()
}
