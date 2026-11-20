package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/internal/rangevector"
)

// The folded-range round trip (ruled 2026-11-20 by the maintainer: "We do
// however NEED to have proper round trip for folded ranges"): c4 id -S -s
// of a tree, c4 patch that description into an empty folder, c4 id -S
// again: the same description ID. Run on every tree of the shared vector
// (c4m/testdata/range-roundtrip-vector), with the list inline (the
// vector's c4m file) and with the list only in the store (the stored
// description, c4 cat -r, which carries no inline list).
//
// On Windows a file's mode cannot hold the vector's permission bits, so
// the round trip is measured from the platform's own scan of the tree;
// everywhere else the scan must also equal the vector's recorded ID.
// (On this line c4 id -q prints nothing, so description IDs come from the
// library.)
func TestPatchRoundTripsFoldedRanges(t *testing.T) {
	bin := buildC4(t)
	vec := filepath.Join("..", "..", "c4m", "testdata", "range-roundtrip-vector")
	cases, err := rangevector.Read(filepath.Join(vec, "trees.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := readVectorIDs(t, filepath.Join(vec, "ids.txt"))

	for _, name := range rangevector.Names(cases) {
		if _, ok := want[name]; !ok {
			continue // the link case: see TestPatchRefusesAFoldedLinkRange
		}
		name := name
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
			if err := os.MkdirAll(env["C4_STORE"], 0o755); err != nil {
				t.Fatal(err)
			}
			src := filepath.Join(dir, "src")
			if err := rangevector.Build(src, cases[name]); err != nil {
				t.Fatal(err)
			}
			desc, stderr, code := runC4WithEnv(t, bin, env, "id", "-S", "-s", src)
			if code != 0 {
				t.Fatalf("c4 id -S -s: exit %d: %s", code, stderr)
			}
			id := descriptionID(t, desc)
			target := filepath.Join(dir, "desc.c4m")
			if err := os.WriteFile(target, []byte(desc), 0o644); err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" {
				if id != want[name] {
					t.Fatalf("c4 id -S of the vector's tree is %s, the vector records %s", id, want[name])
				}
				target = filepath.Join(vec, name+".c4m") // the vector's own bytes
			}

			// The list inline: the description file.
			dest := filepath.Join(dir, "dest")
			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatal(err)
			}
			if _, stderr, code = runC4WithEnv(t, bin, env, "patch", target, dest); code != 0 {
				t.Fatalf("patch %s: exit %d: %s", filepath.Base(target), code, stderr)
			}
			if got := scanID(t, bin, env, dest); got != id {
				t.Errorf("patched folder scans (-S) as %s, want %s", got, id)
			}

			// The list only in the store: the stored description, expanded
			// level by level from the store (c4 cat -r), which carries no
			// inline list.
			listing, stderr, code := runC4WithEnv(t, bin, env, "cat", "-r", id)
			if code != 0 {
				t.Fatalf("c4 cat %s: exit %d: %s", id, code, stderr)
			}
			for _, line := range strings.Split(listing, "\n") {
				if len(line) >= 270 && strings.HasPrefix(line, "c4") {
					t.Fatalf("the stored description carries an inline list:\n%s", listing)
				}
			}
			oneLevel := filepath.Join(dir, "one-level.c4m")
			if err := os.WriteFile(oneLevel, []byte(listing), 0o644); err != nil {
				t.Fatal(err)
			}
			dest2 := filepath.Join(dir, "dest2")
			if err := os.MkdirAll(dest2, 0o755); err != nil {
				t.Fatal(err)
			}
			if _, stderr, code = runC4WithEnv(t, bin, env, "patch", oneLevel, dest2); code != 0 {
				t.Fatalf("patch of the stored description: exit %d: %s", code, stderr)
			}
			if got := scanID(t, bin, env, dest2); got != id {
				t.Errorf("folder patched from the store scans (-S) as %s, want %s", got, id)
			}
		})
	}
}

// With neither an inline list nor the list in the store, a range is the
// ordinary missing-content case: exit 1, nothing written, and the message
// names the list, which is what is missing.
func TestPatchNamesAMissingRangeList(t *testing.T) {
	bin := buildC4(t)
	vec := filepath.Join("..", "..", "c4m", "testdata", "range-roundtrip-vector")
	cases, err := rangevector.Read(filepath.Join(vec, "trees.txt"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	full := map[string]string{"C4_STORE": filepath.Join(dir, "full"), "HOME": filepath.Join(dir, "home")}
	bare := map[string]string{"C4_STORE": filepath.Join(dir, "bare"), "HOME": filepath.Join(dir, "home")}
	for _, s := range []string{full["C4_STORE"], bare["C4_STORE"]} {
		if err := os.MkdirAll(s, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(dir, "src")
	if err := rangevector.Build(src, cases["one"]); err != nil {
		t.Fatal(err)
	}
	desc, stderr, code := runC4WithEnv(t, bin, full, "id", "-S", "-s", src)
	if code != 0 {
		t.Fatalf("c4 id -S -s: exit %d: %s", code, stderr)
	}
	listing, stderr, code := runC4WithEnv(t, bin, full, "cat", descriptionID(t, desc))
	if code != 0 {
		t.Fatalf("c4 cat: exit %d: %s", code, stderr)
	}
	oneLevel := filepath.Join(dir, "one-level.c4m")
	if err := os.WriteFile(oneLevel, []byte(listing), 0o644); err != nil {
		t.Fatal(err)
	}
	rangeID := ""
	for _, line := range strings.Split(listing, "\n") {
		if f := strings.Fields(line); len(f) > 0 && strings.Contains(line, "[0001-0003]") {
			rangeID = f[len(f)-1]
		}
	}
	if rangeID == "" {
		t.Fatalf("no range line in the stored listing:\n%s", listing)
	}

	dest := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := runC4WithEnv(t, bin, bare, "patch", oneLevel, dest)
	if code != 1 {
		t.Fatalf("patch with no list anywhere: exit %d, want 1: %s", code, stderr)
	}
	if !strings.Contains(stderr, "the ID list of f.[0001-0003].exr ("+rangeID+")") {
		t.Errorf("the message should name the missing list:\n%s", stderr)
	}
	if out != "" {
		t.Errorf("nothing should be printed before the refusal; stdout:\n%s", out)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("nothing should be written; dest holds %d entries", len(entries))
	}
}

// A folded range of symbolic links is not written: the folded line holds
// the members' targets as one range expression, not one per member (held until the
// maintainer rules on folding links). Exit 1, nothing written, the true
// reason given. The detector is unchanged.
func TestPatchRefusesAFoldedLinkRange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	bin := buildC4(t)
	vec := filepath.Join("..", "..", "c4m", "testdata", "range-roundtrip-vector")
	cases, err := rangevector.Read(filepath.Join(vec, "trees.txt"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	if err := os.MkdirAll(env["C4_STORE"], 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "src")
	if err := rangevector.Build(src, cases["links"]); err != nil {
		t.Fatal(err)
	}
	desc, stderr, code := runC4WithEnv(t, bin, env, "id", "-S", "-s", src)
	if code != 0 {
		t.Fatalf("c4 id -S -s: exit %d: %s", code, stderr)
	}
	if !strings.Contains(desc, "l.[0001-0003].exr") {
		t.Fatalf("the links should fold:\n%s", desc)
	}
	descFile := filepath.Join(dir, "desc.c4m")
	if err := os.WriteFile(descFile, []byte(desc), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runC4WithEnv(t, bin, env, "patch", descFile, dest)
	if code != 1 {
		t.Fatalf("patch of a folded link range: exit %d, want 1: %s", code, stderr)
	}
	if !strings.Contains(stderr, "l.[0001-0003].exr: a folded range of symbolic links records its members' targets as one range expression (or ... when they differ), not one target per member; this version does not write links from it") {
		t.Errorf("the message should give the true reason:\n%s", stderr)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("nothing should be written; dest holds %d entries", len(entries))
	}
}

func readVectorIDs(t *testing.T, file string) map[string]string {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ids := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if f := strings.Split(line, "\t"); len(f) == 2 {
			ids[f[0]] = f[1]
		}
	}
	return ids
}

// descriptionID is the library's description ID of c4m text.
func descriptionID(t *testing.T, text string) string {
	t.Helper()
	m, err := c4m.Unmarshal([]byte(text))
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, text)
	}
	return m.ComputeC4ID().String()
}

// scanID is the description ID of a folder scanned with folding.
func scanID(t *testing.T, bin string, env map[string]string, dir string) string {
	t.Helper()
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-S", dir)
	if code != 0 {
		t.Fatalf("c4 id -S %s: exit %d: %s", dir, code, stderr)
	}
	return descriptionID(t, out)
}

// Reverting to a stored folded description (c4 patch -r ID) reaches the
// planner without the up-front expansion; a range it cannot write is
// reported there with the same true reason, and nothing is written.
func TestPatchRevertNamesAFoldedLinkRange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	bin := buildC4(t)
	vec := filepath.Join("..", "..", "c4m", "testdata", "range-roundtrip-vector")
	cases, err := rangevector.Read(filepath.Join(vec, "trees.txt"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	if err := os.MkdirAll(env["C4_STORE"], 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "src")
	if err := rangevector.Build(src, cases["links"]); err != nil {
		t.Fatal(err)
	}
	desc, stderr, code := runC4WithEnv(t, bin, env, "id", "-S", "-s", src)
	if code != 0 {
		t.Fatalf("c4 id -S -s: exit %d: %s", code, stderr)
	}
	dest := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runC4WithEnv(t, bin, env, "patch", "-r", descriptionID(t, desc), dest)
	if code != 1 {
		t.Fatalf("patch -r to a folded link range: exit %d, want 1: %s", code, stderr)
	}
	if !strings.Contains(stderr, "l.[0001-0003].exr: a folded range of symbolic links records its members' targets as one range expression (or ... when they differ), not one target per member; this version does not write links from it") {
		t.Errorf("the message should give the true reason:\n%s", stderr)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("nothing should be written; dest holds %d entries", len(entries))
	}
}
