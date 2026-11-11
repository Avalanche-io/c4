package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
)

// `c4 patch record.c4m dir`, where record.c4m is a one-level record (each
// subdirectory one line carrying its own C4 ID, as c4 1.1 stores a tree),
// emptied every such subdirectory with exit 0 through 1.0.25. A directory
// the record does not list the contents of is left as it is.
func TestPatchOneLevelRecordKeepsSubdirectories(t *testing.T) {
	bin := buildC4(t)
	work := t.TempDir()
	tree := filepath.Join(work, "tree")
	for rel, s := range map[string]string{
		"root.txt": "root", "sub/child.txt": "child", "sub/deep/leaf.txt": "leaf",
	} {
		p := filepath.Join(tree, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	full, err := scan.NewGenerator().GenerateFromPath(tree)
	if err != nil {
		t.Fatal(err)
	}
	record := c4m.NewManifest()
	for _, e := range full.Entries {
		if e.Depth == 0 {
			record.AddEntry(e)
		}
	}
	var b bytes.Buffer
	if err := c4m.NewEncoder(&b).Encode(record); err != nil {
		t.Fatal(err)
	}
	recPath := filepath.Join(work, "record.c4m")
	if err := os.WriteFile(recPath, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errOut, code := runC4(t, bin, "patch", recPath, tree)
	if code != 0 {
		t.Fatalf("c4 patch exited %d: %s", code, errOut)
	}
	for _, rel := range []string{"root.txt", "sub/child.txt", "sub/deep/leaf.txt"} {
		if _, err := os.Stat(filepath.Join(tree, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s removed by c4 patch of a one-level record", rel)
		}
	}
}
