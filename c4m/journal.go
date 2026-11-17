package c4m

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
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

// journalPrefix opens every journal, whatever its format version.
const journalPrefix = "@c4 journal"

// JournalVersion is the one journal format this package reads and writes.
const JournalVersion = 1

// journalMagic is the required first line: the prefix and the version.
const journalMagic = "@c4 journal 1"

// checkJournalVersion refuses a journal whose header names a format this
// package doesn't know. A journal written by a newer format is never
// read as this one and never appended to, since v1 lines under its header
// would corrupt it (c4 issue #44). It reads only the first line; data that
// isn't a journal at all is left to the caller.
func checkJournalVersion(data []byte) error {
	if !IsJournal(data) {
		return nil
	}
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	if string(line) == journalMagic {
		return nil
	}
	found := strings.TrimSpace(strings.TrimPrefix(string(line), journalPrefix))
	return fmt.Errorf("journal: format version %q is not one this c4 knows (it reads and writes version %d); refusing to read or append to it — use a c4 that knows version %s",
		found, JournalVersion, found)
}

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

	// The version is checked before the torn tail is touched: a journal
	// of a format this package doesn't know is left byte for byte as it is.
	if err := checkJournalHead(f); err != nil {
		return err
	}

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
	// The first append makes the file (or finds it empty, left so by an
	// appender that died before writing). Its name is an entry in the
	// store root, durable only once the root is synced; through
	// 2026-11-16 nothing synced it, so a power cut after the first claim
	// was reported could lose the journal and the claim with it
	// (TestJournalFirstAppendSurvivesAPowerCut). Later appends write into
	// a file whose name is already durable.
	if end == 0 {
		if err := syncJournalDir(filepath.Dir(j.path)); err != nil {
			return fmt.Errorf("journal sync %s: %w", filepath.Dir(j.path), err)
		}
	}
	return nil
}

// checkJournalHead reads the first line of an open journal and refuses a
// format version this package doesn't know. An empty file passes.
func checkJournalHead(f *os.File) error {
	head := make([]byte, 256)
	n, err := f.ReadAt(head, 0)
	if n == 0 && err != nil {
		return nil // empty: nothing written yet
	}
	return checkJournalVersion(head[:n])
}

// CheckVersion refuses a journal whose header names a format this package
// doesn't know, before anything reads or appends to it. An absent journal
// passes. Callers that are about to store and claim use it to refuse
// before any ingest.
func (j *Journal) CheckVersion() error {
	f, err := os.Open(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	return checkJournalHead(f)
}

// syncJournalDir makes the store root's entry for a new journal durable.
// A package variable so tests can observe it.
var syncJournalDir = store.SyncDir

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
// (any version), so a journal of any format is recognised as a journal
// and never parsed as a description. Reading it is DecodeJournal's job,
// which refuses a version it doesn't know.
func IsJournal(data []byte) bool {
	return bytes.HasPrefix(data, []byte(journalPrefix))
}

// Claims reads every complete claim line, ignoring a torn tail (the
// torn line belongs to an append that never reported success). An
// absent journal is an empty history.
//
// A journal a store's root split displaced is read first, where it lies:
// through 2026-11-16 a tree store's split moved every file in the root,
// so once a store passed 4,096 objects its journal sat at jo/journal and
// appends started a new one at the root (store split race; decided
// 2026-11-16 by the release lead; reversible). Every claim before the
// split is in the displaced file and every claim after it in the root's,
// so the two read in that order are the store's history in order. The
// displaced file is never written: every append goes to the root.
func (j *Journal) Claims() ([]Claim, error) {
	var claims []Claim
	displaced := filepath.Join(j.root, displacedJournal)
	if data, err := os.ReadFile(displaced); err == nil && IsJournal(data) {
		old, err := DecodeJournal(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", displaced, err)
		}
		claims = old
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	data, err := os.ReadFile(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return claims, nil
		}
		return nil, err
	}
	cur, err := DecodeJournal(data)
	if err != nil {
		return nil, err
	}
	return append(claims, cur...), nil
}

// displacedJournal is where a tree store's root split put the journal
// before 2026-11-16: the split filed every name by its first two
// characters. No object name starts "jo" (every C4 ID starts "c4"), so
// nothing else is ever filed there and the directory never splits.
var displacedJournal = filepath.Join(JournalName[:2], JournalName)

// DecodeJournal parses journal bytes (a store's journal file, or a
// copy of one): the magic header, then one claim per complete line.
// A torn (unterminated) final line is dropped. A journal whose header
// names a format version other than JournalVersion is refused, never
// read as this one.
func DecodeJournal(data []byte) ([]Claim, error) {
	if err := checkJournalVersion(data); err != nil {
		return nil, err
	}
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
