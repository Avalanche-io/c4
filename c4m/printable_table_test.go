package c4m

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// The Go table and the shared text table every implementation embeds are
// the same ranges.
func TestPrintableTableMatchesSharedFile(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "safename-vector", "printable-"+printableUnicodeVersion+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var got [][2]rune
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "..", 2)
		if len(parts) != 2 {
			t.Fatalf("bad line %q", line)
		}
		lo, err1 := strconv.ParseUint(parts[0], 16, 32)
		hi, err2 := strconv.ParseUint(parts[1], 16, 32)
		if err1 != nil || err2 != nil {
			t.Fatalf("bad line %q", line)
		}
		got = append(got, [2]rune{rune(lo), rune(hi)})
	}
	if len(got) != len(printableRanges) {
		t.Fatalf("shared file has %d ranges, Go table %d", len(got), len(printableRanges))
	}
	for i := range got {
		if got[i] != printableRanges[i] {
			t.Fatalf("range %d: file %X..%X, table %X..%X", i, got[i][0], got[i][1], printableRanges[i][0], printableRanges[i][1])
		}
	}
}

// When this Go carries the pinned Unicode version, the table is exactly
// unicode.IsPrint. On any other version the table, not the runtime, decides.
func TestPrintableTableIsPinnedIsPrint(t *testing.T) {
	if unicode.Version != printableUnicodeVersion {
		t.Skipf("this Go has Unicode %s; the table is pinned to %s", unicode.Version, printableUnicodeVersion)
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if isPrintable(r) != unicode.IsPrint(r) {
			t.Fatalf("U+%04X: table %v, unicode.IsPrint %v", r, isPrintable(r), unicode.IsPrint(r))
		}
	}
}

// A code point's encoding does not depend on the Go that builds the
// binary. U+0870 was assigned in Unicode 14.0: Go 1.16's tables (13.0)
// called it unprintable, so SafeName escaped it there and wrote it plainly
// on newer Go, and a name holding it had two IDs. U+0378 is unassigned in
// 17.0.0 and stays escaped even on a runtime that assigns it later.
func TestSafeNameDoesNotFollowTheRuntimeUnicode(t *testing.T) {
	if got := SafeName("aࡰb"); got != "aࡰb" {
		t.Errorf("U+0870 is printable in Unicode 17.0.0 and is written as itself, got %q", got)
	}
	if got := SafeName("a͸b"); got == "a͸b" {
		t.Errorf("U+0378 is unassigned in Unicode 17.0.0 and must be encoded, got %q", got)
	}
	if UnsafeName(SafeName("a͸b")) != "a͸b" {
		t.Error("the encoded U+0378 must decode back")
	}
}
