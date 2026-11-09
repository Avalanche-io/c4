package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// Through 1.0.24 the decoder read a pretty (-e) line whose C4 ID column
// is "-" with its alignment padding inside the name: "a.txt" became
// "a.txt" plus forty spaces. A names-only pretty description therefore
// named nothing that existed, and `c4 patch -q` of it onto its own tree
// removed every real file (measured 2026-11-09). These tests pin the
// fix with the tool's own structure scan and with the real 1.0.21 file.

func assertNamesClean(t *testing.T, m *c4m.Manifest, form string) {
	t.Helper()
	for _, e := range m.Entries {
		if strings.HasSuffix(e.Name, " ") || strings.Contains(e.Name, "  ") {
			t.Fatalf("%s: name %q carries alignment padding", form, e.Name)
		}
	}
}

func TestPrettyNullIDNamesDoNotAbsorbPadding(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "A\n", "sub/b.txt": "B\n", "name with space.txt": "C\n"})

	// The tool's own pretty, names-only description.
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-e", "-m", "s", tree)
	if code != 0 {
		t.Fatalf("id -e -m s: exit %d: %s", code, stderr)
	}
	m, err := c4m.Unmarshal([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	assertNamesClean(t, m, "structure pretty")
	names := map[string]bool{}
	for _, e := range m.Entries {
		names[e.Name] = true
	}
	for _, want := range []string{"a.txt", "sub/", "b.txt", "name with space.txt"} {
		if !names[want] {
			t.Fatalf("structure pretty: %q not decoded; names: %v", want, names)
		}
	}

	// Applied onto its own tree, it removes nothing.
	desc := filepath.Join(dir, "s.c4m")
	if err := os.WriteFile(desc, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runC4WithEnv(t, bin, env, "patch", "-q", desc, tree); code != 0 {
		t.Fatalf("patch -q: exit %d: %s", code, stderr)
	}
	for _, p := range []string{"a.txt", "sub/b.txt", "name with space.txt"} {
		if _, err := os.Stat(filepath.Join(tree, p)); err != nil {
			t.Fatalf("%s was removed by a names-only description of its own tree: %v", p, err)
		}
	}
}

func TestLegacy1021PrettyFileNamesAndPatch(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	legacy := "../../c4m/testdata/legacy/ten-dash-null-1.0.21.c4m"
	data, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	m, err := c4m.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	assertNamesClean(t, m, "legacy 1.0.21")
	names := map[string]bool{}
	for _, e := range m.Entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["d/"] || !names["b.txt"] {
		t.Fatalf("legacy names not decoded: %v", names)
	}

	// The tree the legacy file describes; applying the file keeps it.
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "A\n", "d/b.txt": "B\n"})
	if _, stderr, code := runC4WithEnv(t, bin, env, "patch", "-q", legacy, tree); code != 0 {
		t.Fatalf("patch -q legacy: exit %d: %s", code, stderr)
	}
	for _, p := range []string{"a.txt", "d/b.txt"} {
		if _, err := os.Stat(filepath.Join(tree, p)); err != nil {
			t.Fatalf("%s was removed by the legacy description of its own tree: %v", p, err)
		}
	}
}
