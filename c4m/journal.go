package c4m

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Avalanche-io/c4"
)

// The store journal is an ordinary c4m patch chain at <store>/log.c4m:
// one section per claim, where a section is a single canonical entry
// line (null mode; the timestamp is the SCAN START per v8 Amendment 2;
// origin recorded as an inbound flow link) followed by the claimed
// object's bare ID as the section boundary. Appends touch only the
// tail; the file never rewrites. See design/snapshot-loop/round-1/
// draft-v7.md and round-2/draft-v8.md.

// JournalName is the journal's filename inside a store root.
const JournalName = "log.c4m"

// Claim is one journal section: the record that the object named by ID
// was durably stored, under what name, from where, and when the scan
// that produced it began.
type Claim struct {
	ScanStart time.Time // UTC; recorded at walk start, not append time
	Size      int64     // byte size of the object the ID names
	Name      string    // final path component; ".c4m" suffix for descriptions
	Origin    string    // "<host>:<abs-path>" as given; empty for stdin
	ID        c4.ID
}

// Journal is an append-only claim log inside a store root.
type Journal struct {
	path string
	root string
}

// OpenJournal returns the journal for a store root. The file is
// created on first append.
func OpenJournal(storeRoot string) *Journal {
	return &Journal{path: filepath.Join(storeRoot, JournalName), root: storeRoot}
}

// Path returns the journal file's path.
func (j *Journal) Path() string { return j.path }

// entryLine renders the claim as a canonical c4m entry line.
func (c Claim) entryLine() string {
	e := &Entry{
		Timestamp: c.ScanStart.UTC().Truncate(time.Second),
		Size:      c.Size,
		Name:      c.Name,
		C4ID:      c.ID,
	}
	if c.Origin != "" {
		e.FlowDirection = FlowInbound
		e.FlowTarget = c.Origin
	}
	return e.Canonical()
}

// Append writes one claim section (entry line + bare-ID boundary) to
// the journal and returns only after the bytes are durable: the append
// is part of the print barrier — a caller may print the claimed ID the
// moment Append returns, and never before.
//
// Mechanics per the design pins: the append serializes under an OS
// lock that dies with its holder; a torn tail (a crashed writer's
// partial line) is truncated before appending; durability is the
// file's own Sync (F_FULLFSYNC on darwin, fsync elsewhere,
// write-through semantics on Windows).
func (j *Journal) Append(c Claim) error {
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("journal open: %w", err)
	}
	defer f.Close()

	if err := lockFile(f); err != nil {
		return fmt.Errorf("journal lock: %w", err)
	}
	// The lock releases when f closes (deferred above) — and the OS
	// releases it if this process dies, so a crashed writer never
	// wedges the store.

	end, err := truncateTornTail(f)
	if err != nil {
		return fmt.Errorf("journal tail: %w", err)
	}

	section := c.entryLine() + "\n" + c.ID.String() + "\n"
	if _, err := f.WriteAt([]byte(section), end); err != nil {
		return fmt.Errorf("journal write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("journal sync: %w", err)
	}
	return nil
}

// truncateTornTail ensures the file ends at a line boundary, removing
// a crashed writer's partial final line. Returns the append offset.
func truncateTornTail(f *os.File) (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	if size == 0 {
		return 0, nil
	}

	// Read backward in one bounded chunk: journal lines are short
	// (< 4KB even with long origins), so one tail read suffices.
	const tail = 4096
	off := size - tail
	if off < 0 {
		off = 0
	}
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return 0, err
	}
	if buf[len(buf)-1] == '\n' {
		return size, nil
	}
	cut := bytes.LastIndexByte(buf, '\n')
	var newSize int64
	if cut < 0 {
		newSize = off // no newline in tail window: torn from window start
		if off != 0 {
			// Pathological (>4KB torn line): scan further back.
			all := make([]byte, size)
			if _, err := f.ReadAt(all, 0); err != nil {
				return 0, err
			}
			c := bytes.LastIndexByte(all, '\n')
			newSize = int64(c + 1) // c==-1 → 0
		}
	} else {
		newSize = off + int64(cut) + 1
	}
	if err := f.Truncate(newSize); err != nil {
		return 0, err
	}
	return newSize, nil
}

// Claims reads every complete claim section, ignoring a torn tail
// (the torn line belongs to an append that never reported success).
// The journal being an ordinary patch chain, sections are decoded with
// the standard chain decoder.
func (j *Journal) Claims() ([]Claim, error) {
	data, err := os.ReadFile(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// Drop a torn tail before decoding.
	if n := len(data); n > 0 && data[n-1] != '\n' {
		if cut := bytes.LastIndexByte(data, '\n'); cut >= 0 {
			data = data[:cut+1]
		} else {
			data = nil
		}
	}
	if len(data) == 0 {
		return nil, nil
	}

	sections, err := DecodePatchChain(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("journal decode: %w", err)
	}
	var claims []Claim
	for _, sec := range sections {
		for _, e := range sec.Entries {
			c := Claim{
				ScanStart: e.Timestamp,
				Size:      e.Size,
				Name:      e.Name,
				ID:        e.C4ID,
			}
			if e.FlowDirection == FlowInbound {
				c.Origin = e.FlowTarget
			}
			claims = append(claims, c)
		}
	}
	return claims, nil
}
