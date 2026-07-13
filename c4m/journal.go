package c4m

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// The store journal is the store's root record: one line per claim,
// append-only, travelling with the store. It is NOT a c4m document —
// its first line is an @-directive, which every conforming c4m parser
// MUST reject, so misinterpretation is structurally impossible in
// both directions (design/snapshot-loop/round-3/draft-v9.md §5).
//
// Grammar:
//
//	@c4 journal 1
//	<scan-start RFC3339 UTC seconds> <claim C4 ID>
//	...
//
// Two fields per line, both load-bearing: the claim ID is the root
// (recovery handle, gc root set) and the scan start anchors the
// re-scan racy rule. Ordering is line position (append order).
// Roots are purely virtual: no names, no sizes, no origins — claim
// provenance is testimony-layer work (design/testimony-record.md).

// JournalName is the journal's filename inside a store root.
const JournalName = "journal"

// journalMagic is the required first line (with a format version).
const journalMagic = "@c4 journal 1"

// journalTimeFormat is RFC 3339 at second precision, always UTC.
const journalTimeFormat = "2006-01-02T15:04:05Z"

// Journal is a store's claims log.
type Journal struct {
	path string
	root string
}

// Claim is one journal line: a claimed root ID and the instant the
// claiming scan began (the walk's start, never the append time — the
// racy rule measures against it).
type Claim struct {
	ScanStart time.Time
	ID        c4.ID
}

// OpenJournal returns the journal for a store root. The file is
// created on first append.
func OpenJournal(storeRoot string) *Journal {
	return &Journal{path: filepath.Join(storeRoot, JournalName), root: storeRoot}
}

// Path returns the journal file's path.
func (j *Journal) Path() string { return j.path }

// Line renders the claim as its journal line (without newline) — the
// exact text the journal records; what c4 log reprints.
func (c Claim) Line() string {
	return c.ScanStart.UTC().Truncate(time.Second).Format(journalTimeFormat) +
		" " + c.ID.String()
}

// Append writes one claim line to the journal and returns only after
// the bytes are durable: the append is part of the print barrier — a
// caller may print the claimed ID the moment Append returns, and
// never before.
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

	record := c.Line() + "\n"
	if end == 0 {
		record = journalMagic + "\n" + record
	}
	if _, err := f.WriteAt([]byte(record), end); err != nil {
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
	// (~112 bytes), so one tail read suffices.
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

// ParseClaimLine parses one journal claim line (two fields:
// scan-start, ID).
func ParseClaimLine(line string) (Claim, error) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return Claim{}, fmt.Errorf("journal line: want 2 fields, got %d", len(fields))
	}
	ts, err := time.Parse(journalTimeFormat, fields[0])
	if err != nil {
		return Claim{}, fmt.Errorf("journal scan-start: %w", err)
	}
	id, err := c4.Parse(fields[1])
	if err != nil {
		return Claim{}, fmt.Errorf("journal claim ID: %w", err)
	}
	return Claim{ScanStart: ts, ID: id}, nil
}

// IsJournal reports whether data begins with the journal magic line
// (any version).
func IsJournal(data []byte) bool {
	return bytes.HasPrefix(data, []byte("@c4 journal"))
}

// Claims reads every complete claim line, ignoring a torn tail (the
// torn line belongs to an append that never reported success). An
// absent journal is an empty history.
func (j *Journal) Claims() ([]Claim, error) {
	data, err := os.ReadFile(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return DecodeJournal(data)
}

// DecodeJournal parses journal bytes (a store's journal file, or a
// copy of one): the magic header, then one claim per complete line.
// A torn (unterminated) final line is dropped.
func DecodeJournal(data []byte) ([]Claim, error) {
	// Drop a torn tail before parsing.
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
	if !IsJournal(data) {
		return nil, fmt.Errorf("journal: missing %q header", journalMagic)
	}

	var claims []Claim
	lines := strings.Split(string(data), "\n")
	for i, line := range lines[1:] { // skip the magic line
		if line == "" {
			continue
		}
		c, err := ParseClaimLine(line)
		if err != nil {
			return nil, fmt.Errorf("journal line %d: %w", i+2, err)
		}
		claims = append(claims, c)
	}
	return claims, nil
}
