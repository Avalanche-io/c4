package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A folded sequence (`c4 id -S`) names its members' ID list. Master's
// `c4 id -s -S` stored the members but not that list, so `c4 cat <folded
// ID>` found nothing (release 1.0.x: 7bac715). And once the list was in
// the store, `c4 restore --force` of the folded description deleted the
// real members and wrote the ID list as a file named
// `frame.[0001-0003].exr` (release 1.0.x: 7afd4d8). Now the list is
// stored, and restore refuses a folded entry, naming it, with the members
// untouched.
func TestFoldedSequenceStoresItsListAndRestoreRefusesIt(t *testing.T) {
	bin := buildC4(t)
	work := t.TempDir()
	seq := filepath.Join(work, "seq")
	if err := os.MkdirAll(seq, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		p := filepath.Join(seq, fmt.Sprintf("frame.000%d.exr", i))
		if err := os.WriteFile(p, []byte(fmt.Sprintf("f%d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"HOME": filepath.Join(work, "home"), "C4_STORE": filepath.Join(work, "store")}

	out, errOut, code := runC4WithEnv(t, bin, env, "id", "-s", "-S", seq)
	if code != 0 {
		t.Fatalf("c4 id -s -S exited %d: %s", code, errOut)
	}
	var folded string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "frame.[0001-0003].exr") {
			f := strings.Fields(line)
			folded = f[len(f)-1]
		}
	}
	if folded == "" {
		t.Fatalf("no folded entry in:\n%s", out)
	}
	list, errOut, code := runC4WithEnv(t, bin, env, "cat", folded)
	if code != 0 || len(list) != 3*90 {
		t.Fatalf("c4 cat <folded ID>: exit %d, %d bytes, want the 3-member ID list (270 bytes): %s", code, len(list), errOut)
	}

	foldedC4m := filepath.Join(work, "folded.c4m")
	if err := os.WriteFile(foldedC4m, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code = runC4WithEnv(t, bin, env, "restore", "--force", foldedC4m, seq)
	if code != 1 || !strings.Contains(errOut, "frame.[0001-0003].exr") {
		t.Errorf("restore of a folded description: exit %d, stderr %.300s; want exit 1 naming the folded entry", code, errOut)
	}
	entries, _ := os.ReadDir(seq)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, " ") != "frame.0001.exr frame.0002.exr frame.0003.exr" {
		t.Errorf("seq/ after restore holds %v; the members must be untouched", names)
	}
}
