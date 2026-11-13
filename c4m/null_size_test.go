package c4m

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// A size of 0 beside the C4 ID of non-empty content is a null size
// (ruled 2026-11-08): a filesystem surface has no "-", so a null size
// materialises as 0, and only the empty content truthfully has size 0.
// A 0 with no ID, a 0 with the empty content's ID, and a symlink's 0 are
// all known zeros.
func TestNullSizeZero(t *testing.T) {
	ts := "2026-03-06T12:00:00Z"
	hello := c4.Identify(strings.NewReader("hello")).String()
	empty := EmptyContentID.String()

	cases := []struct {
		name     string
		line     string
		wantNull bool
	}{
		{"zero with real content ID is null", "-rw-r--r-- " + ts + " 0 f " + hello, true},
		{"zero with empty-content ID is known", "-rw-r--r-- " + ts + " 0 f " + empty, false},
		{"zero with no ID is known (structure scan)", "-rw-r--r-- " + ts + " 0 f -", false},
		{"symlink zero is its own size", "lrwxr-xr-x " + ts + " 0 l -> f " + hello, false},
		{"directory zero with listing ID is known", "drwxr-xr-x " + ts + " 0 d/ " + empty, false},
		{"directory zero with other ID is null", "drwxr-xr-x " + ts + " 0 d/ " + hello, true},
		{"non-zero is known", "-rw-r--r-- " + ts + " 5 f " + hello, false},
		{"dash is null", "-rw-r--r-- " + ts + " - f " + hello, true},
	}
	for _, tc := range cases {
		m, err := Unmarshal([]byte(tc.line + "\n"))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		e := m.Entries[0]
		if e.SizeIsNull() != tc.wantNull {
			t.Errorf("%s: SizeIsNull=%v, want %v (Size %d)", tc.name, e.SizeIsNull(), tc.wantNull, e.Size)
		}
		if tc.wantNull && e.Size != -1 {
			t.Errorf("%s: decoder must hold a null size as -1, got %d", tc.name, e.Size)
		}
		if !tc.wantNull && e.Size != 0 && !strings.Contains(tc.name, "non-zero") {
			t.Errorf("%s: known zero must stay 0, got %d", tc.name, e.Size)
		}
		// The canonical text writes the null as "-", the known zero as "0".
		fields := strings.Fields(e.Canonical())
		if got := fields[2]; (got == "-") != tc.wantNull {
			t.Errorf("%s: canonical size %q", tc.name, got)
		}
	}
}

// The same description, written with "0" or with "-", has one ID: the
// rule is applied in canonical form, whichever way the entry was built.
func TestNullSizeZeroOneID(t *testing.T) {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	id := c4.Identify(strings.NewReader("hello"))

	fromZero, err := Unmarshal([]byte("-rw-r--r-- 2026-03-06T12:00:00Z 0 f " + id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	fromDash, err := Unmarshal([]byte("-rw-r--r-- 2026-03-06T12:00:00Z - f " + id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if fromZero.ComputeC4ID() != fromDash.ComputeC4ID() {
		t.Fatal("0 and - beside a real content ID must be one description")
	}

	// Built through the API with Size 0 (as a scan over a mount would
	// build it), the entry still renders and hashes as null.
	built := &Manifest{Version: "1.0", Entries: []*Entry{
		{Name: "f", Mode: 0644, Timestamp: ts, Size: 0, C4ID: id},
	}}
	if built.ComputeC4ID() != fromDash.ComputeC4ID() {
		t.Fatal("an API-built Size 0 beside a real content ID must hash as null")
	}
	var buf bytes.Buffer
	NewEncoder(&buf).Encode(built)
	if !strings.Contains(buf.String(), " - f ") {
		t.Fatalf("encoder must write the null as -, got %q", buf.String())
	}
	if !built.Entries[0].HasNullValues() {
		t.Fatal("HasNullValues must report the null size")
	}
}

// A null written as 0 infects the parent directory's size like any null.
func TestNullSizeZeroPropagates(t *testing.T) {
	id := c4.Identify(strings.NewReader("hello"))
	m, err := Unmarshal([]byte(strings.Join([]string{
		"drwxr-xr-x 2026-03-06T12:00:00Z - d/ -",
		"  -rw-r--r-- 2026-03-06T12:00:00Z 0 f " + id.String(),
		"",
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	PropagateMetadata(m.Entries)
	for _, e := range m.Entries {
		if e.IsDir() && !e.SizeIsNull() {
			t.Fatalf("directory size must be null over a null-size child, got %d", e.Size)
		}
	}
}
