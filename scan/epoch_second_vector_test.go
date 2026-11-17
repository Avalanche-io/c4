package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

const epochSecondVector = "../c4m/testdata/epoch-second-vector"

// TestEpochSecondVector is the shared conformance row for a time inside
// the epoch second (c4m/testdata/epoch-second-vector/README.md). The
// format writes the second a time falls in, and the epoch is null, so a
// file at 0.5 s is null: its line is "-". A scan's description ID, the ID
// of its own encoded c4m, and saved.c4m's all agree.
func TestEpochSecondVector(t *testing.T) {
	savedText, err := os.ReadFile(filepath.Join(epochSecondVector, "saved.c4m"))
	if err != nil {
		t.Fatal(err)
	}
	wantID, err := os.ReadFile(filepath.Join(epochSecondVector, "root-id.txt"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	buildVectorTree(t, epochSecondVector, dir)
	scanned, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	text, err := c4m.Marshal(scanned)
	if err != nil {
		t.Fatal(err)
	}
	own, err := c4m.Unmarshal(text)
	if err != nil {
		t.Fatal(err)
	}

	// Every time inside the epoch second is null, in memory and in its line.
	for _, e := range scanned.Entries {
		switch e.Name {
		case "exact.txt", "half.txt", "in.txt":
			if !e.TimestampIsNull() {
				t.Errorf("%s: scanned time %s is not null", e.Name, e.Timestamp)
			}
		case "one.txt", "pre.txt", "sub/":
			if e.TimestampIsNull() {
				t.Errorf("%s: scanned time %s is null", e.Name, e.Timestamp)
			}
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(text)), "\n") {
		f := strings.Fields(line)
		if (f[3] == "half.txt" || f[3] == "in.txt") && f[1] != "-" {
			t.Errorf("%s is written %q, want -", f[3], f[1])
		}
	}

	// One description ID, whichever way it is reached.
	if a, b := scanned.ComputeC4ID(), own.ComputeC4ID(); a != b {
		t.Fatalf("the scan's description ID %s differs from its own c4m's %s", a, b)
	}
	if pr := c4m.PatchDiff(own, scanned); len(pr.Patch.Entries) != 0 {
		t.Fatalf("a diff of the scan against its own c4m carries %d entries, want none", len(pr.Patch.Entries))
	}

	if runtime.GOOS == "windows" {
		return // saved.c4m carries POSIX modes
	}
	if !bytes.Equal(text, savedText) {
		t.Fatalf("the scan encodes differently from saved.c4m:\n%s\nwant:\n%s", text, savedText)
	}
	if got := scanned.ComputeC4ID().String(); got != strings.TrimSpace(string(wantID)) {
		t.Fatalf("the scan's description ID is %s, want root-id.txt's %s", got, wantID)
	}
}
