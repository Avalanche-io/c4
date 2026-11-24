package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// normFixture builds a settled nested tree, docs/readme.txt and
// shots/{s010/{a.exr, b.exr}, s020/c.exr}, each folder's time its own and
// older than its children's, and returns its scan (`c4 id DIR`).
func normFixture(t *testing.T, bin string, env map[string]string, root string) string {
	t.Helper()
	dir := filepath.Join(root, "tree")
	for _, d := range []string{"docs", "shots/s010", "shots/s020"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	at := func(y int) time.Time { return time.Date(y, 1, 2, 3, 4, 5, 0, time.UTC) }
	for _, f := range []struct {
		path, body string
		year       int
	}{
		{"docs/readme.txt", "readme\n", 2020},
		{"shots/s010/a.exr", "frame a\n", 2021},
		{"shots/s010/b.exr", "frame b\n", 2022},
		{"shots/s020/c.exr", "frame c\n", 2024},
	} {
		p := filepath.Join(dir, f.path)
		if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(p, at(f.year), at(f.year))
	}
	for _, d := range []struct {
		path string
		year int
	}{{"shots/s010", 2017}, {"shots/s020", 2016}, {"shots", 2018}, {"docs", 2019}, {".", 2015}} {
		p := filepath.Join(dir, d.path)
		os.Chtimes(p, at(d.year), at(d.year))
	}
	out, stderr, code := runC4WithEnv(t, bin, env, "id", dir)
	if code != 0 {
		t.Fatalf("c4 id DIR: exit %d\n%s", code, stderr)
	}
	return out
}

// mutated returns full with the entry at path changed by f, through the
// c4m package, written to a file under root.
func mutated(t *testing.T, root, name, full string, f func(m *c4m.Manifest)) string {
	t.Helper()
	m, err := c4m.Unmarshal([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	f(m)
	text, err := c4m.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, name+".c4m")
	if err := os.WriteFile(p, text, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func entryAt(t *testing.T, m *c4m.Manifest, path string) *c4m.Entry {
	t.Helper()
	e := c4m.EntryPaths(m.Entries)[path]
	if e == nil {
		t.Fatalf("no %s", path)
	}
	return e
}

func decodeText(t *testing.T, text string) *c4m.Manifest {
	t.Helper()
	m, err := c4m.Unmarshal([]byte(text))
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, text)
	}
	return m
}

// `c4 id FILE.c4m` does the tree walk: a null or stale directory value its
// listed children determine is filled or corrected, a correction is
// narrated on stderr, a stated directory time and a collapsed directory
// are kept, and the result is idempotent. Through 06f5f8f it fixed form
// only and passed every value through.
func TestNormaliserTreeWalk(t *testing.T) {
	bin := buildC4(t)
	root := t.TempDir()
	env := map[string]string{"HOME": filepath.Join(root, "home"), "USERPROFILE": filepath.Join(root, "home"), "C4_STORE": filepath.Join(root, "store")}
	os.MkdirAll(env["HOME"], 0o755)
	full := normFixture(t, bin, env, root)
	fm := decodeText(t, full)
	s010 := entryAt(t, fm, "shots/s010/")

	normalise := func(t *testing.T, path string) (string, string) {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, "id", path)
		if code != 0 {
			t.Fatalf("c4 id %s: exit %d\n%s", filepath.Base(path), code, stderr)
		}
		// Idempotent: the output normalises to itself, and -l agrees.
		again := filepath.Join(root, "again-"+filepath.Base(path))
		os.WriteFile(again, []byte(out), 0o644)
		if out2, _, _ := runC4WithEnv(t, bin, env, "id", again); out2 != out {
			t.Errorf("a second run changes the output:\n%s\n---\n%s", out, out2)
		}
		if _, _, code := runC4WithEnv(t, bin, env, "id", "-l", again); code != 0 {
			t.Errorf("c4 id -l on the normalised output exits %d, want 0", code)
		}
		return out, stderr
	}

	t.Run("scan passes through unchanged", func(t *testing.T) {
		p := filepath.Join(root, "full.c4m")
		os.WriteFile(p, []byte(full), 0o644)
		out, stderr := normalise(t, p)
		if out != full || stderr != "" {
			t.Errorf("the scan changed (stderr %q):\n%s", stderr, out)
		}
	})

	fills := []struct {
		name, path string
		f          func(e *c4m.Entry)
	}{
		{"null size", "shots/s010/", func(e *c4m.Entry) { e.Size = -1 }},
		{"null C4 ID", "shots/s010/", func(e *c4m.Entry) { e.C4ID = c4.ID{} }},
		{"null size above a derived folder", "shots/", func(e *c4m.Entry) { e.Size = -1 }},
	}
	for _, c := range fills {
		t.Run("fills "+c.name, func(t *testing.T) {
			p := mutated(t, root, strings.ReplaceAll(c.name, " ", "-"), full, func(m *c4m.Manifest) { c.f(entryAt(t, m, c.path)) })
			out, stderr := normalise(t, p)
			if out != full {
				t.Errorf("not filled back to the scan:\n%s", out)
			}
			if stderr != "" {
				t.Errorf("a filled null is narrated as a change: %q", stderr)
			}
		})
	}

	// A folder's time is never derived from its contents (the maintainer,
	// 2026-11-24: "the mod time should NEVER be based on contents, that
	// has bad sideffects."). A null one stays null, and each parent's
	// size and C4 ID are those of its listing as written, the null line
	// included: nothing is hashed or sized over a filled time.
	for _, c := range []struct {
		name  string
		nulls []string
	}{
		{"keeps a null time null", []string{"shots/s010/"}},
		{"keeps nested null times null", []string{"shots/", "shots/s010/"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := mutated(t, root, strings.ReplaceAll(c.name, " ", "-"), full, func(m *c4m.Manifest) {
				for _, d := range c.nulls {
					entryAt(t, m, d).Timestamp = c4m.NullTimestamp()
				}
			})
			out, _ := normalise(t, p)
			om := decodeText(t, out)
			for _, d := range c.nulls {
				if got := entryAt(t, om, d); !got.TimestampIsNull() {
					t.Errorf("%s time %s: filled, want null", d, got.Timestamp)
				}
			}
			for _, d := range []string{"shots/s010/", "shots/"} {
				e := entryAt(t, om, d)
				listing, id := directoryRecord(om, e)
				if e.C4ID != id {
					t.Errorf("%s C4 ID is not its listing's as written (stale hash)", d)
				}
				if want := listedSize(om.Children(e), listing); e.Size != want {
					t.Errorf("%s size %d, its listing as written gives %d (stale size)", d, e.Size, want)
				}
			}
		})
	}

	stale := []struct {
		name, path, field string
		f                 func(e *c4m.Entry)
	}{
		{"stale size", "shots/s010/", "size", func(e *c4m.Entry) { e.Size += 100 }},
		{"stale C4 ID", "shots/s010/", "id", func(e *c4m.Entry) { e.C4ID = entryAt(t, fm, "docs/").C4ID }},
	}
	for _, c := range stale {
		t.Run("corrects "+c.name, func(t *testing.T) {
			p := mutated(t, root, strings.ReplaceAll(c.name, " ", "-"), full, func(m *c4m.Manifest) { c.f(entryAt(t, m, c.path)) })
			out, stderr := normalise(t, p)
			if out != full {
				t.Errorf("not corrected to the scan:\n%s", out)
			}
			if !strings.Contains(stderr, "normalized: "+p+": shots/s010/: "+c.field+" ") {
				t.Errorf("the correction is not narrated on stderr: %q", stderr)
			}
		})
	}

	t.Run("keeps a stated directory time", func(t *testing.T) {
		old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
		p := mutated(t, root, "stated-time", full, func(m *c4m.Manifest) { entryAt(t, m, "shots/s010/").Timestamp = old })
		out, _ := normalise(t, p)
		if got := entryAt(t, decodeText(t, out), "shots/s010/").Timestamp; !got.Equal(old) {
			t.Errorf("stated s010/ time replaced by %s", got)
		}
	})

	t.Run("keeps a collapsed directory", func(t *testing.T) {
		p := mutated(t, root, "collapsed", full, func(m *c4m.Manifest) {
			drop := map[*c4m.Entry]bool{}
			for p, e := range c4m.EntryPaths(m.Entries) {
				if strings.HasPrefix(p, "shots/s010/") && p != "shots/s010/" {
					drop[e] = true
				}
			}
			var kept []*c4m.Entry
			for _, e := range m.Entries {
				if !drop[e] {
					kept = append(kept, e)
				}
			}
			m.Entries = kept
			m.InvalidateIndex()
			entryAt(t, m, "shots/s010/").Size = 7 // stated, and nothing listed can say otherwise
		})
		out, _ := normalise(t, p)
		got := entryAt(t, decodeText(t, out), "shots/s010/")
		if got.Size != 7 || got.C4ID != s010.C4ID || !got.Timestamp.Equal(s010.Timestamp) {
			t.Errorf("collapsed s010/ not kept as stated: %s", got.Canonical())
		}
	})

	// Ruled 2026-11-03 ("Re-derive to null"): a stated size whose children
	// cannot support one is re-derived to null, and reported.
	t.Run("re-derives a stated size over a null-size child to null", func(t *testing.T) {
		p := mutated(t, root, "child-null-size", full, func(m *c4m.Manifest) { entryAt(t, m, "shots/s010/a.exr").Size = -1 })
		out, stderr := normalise(t, p)
		om := decodeText(t, out)
		for _, d := range []string{"shots/s010/", "shots/"} {
			if got := entryAt(t, om, d); got.Size != -1 {
				t.Errorf("%s size %d, want null: a child's size is null", d, got.Size)
			}
		}
		want := fmt.Sprintf("normalized: %s: shots/s010/: size %d -> -", p, s010.Size)
		if !strings.Contains(stderr, want) {
			t.Errorf("the replacement is not narrated: want %q in %q", want, stderr)
		}
	})
	// Not ruled, so kept exactly as 06f5f8f keeps it: a directory over a
	// null-ID child keeps its stated C4 ID.
	t.Run("left out: the C4 ID over a null-ID child", func(t *testing.T) {
		p := mutated(t, root, "child-null-id", full, func(m *c4m.Manifest) { entryAt(t, m, "shots/s010/a.exr").C4ID = c4.ID{} })
		out, _ := normalise(t, p)
		if got := entryAt(t, decodeText(t, out), "shots/s010/"); got.C4ID != s010.C4ID {
			t.Errorf("s010/ C4 ID re-derived over a null-ID child: %s", got.C4ID)
		}
	})
}

// -l lists the files the normaliser would change and -d shows the diff,
// gofmt-style: exit 1 when a file would change, 0 when none would, and
// the files are never written.
func TestNormaliserListAndDiff(t *testing.T) {
	bin := buildC4(t)
	root := t.TempDir()
	env := map[string]string{"HOME": filepath.Join(root, "home"), "USERPROFILE": filepath.Join(root, "home"), "C4_STORE": filepath.Join(root, "store")}
	os.MkdirAll(env["HOME"], 0o755)
	full := normFixture(t, bin, env, root)
	clean := filepath.Join(root, "clean.c4m")
	os.WriteFile(clean, []byte(full), 0o644)
	stale := mutated(t, root, "stale", full, func(m *c4m.Manifest) { entryAt(t, m, "shots/s010/").Size += 100 })
	staleBytes, _ := os.ReadFile(stale)

	if out, stderr, code := runC4WithEnv(t, bin, env, "id", "-l", clean); code != 0 || out != "" {
		t.Errorf("-l on a normalised file: exit %d, stdout %q, stderr %q", code, out, stderr)
	}
	if out, _, code := runC4WithEnv(t, bin, env, "id", "-l", clean, stale); code != 1 || out != stale+"\n" {
		t.Errorf("-l: exit %d, stdout %q; want exit 1 and the stale file only", code, out)
	}
	out, _, code := runC4WithEnv(t, bin, env, "id", "-d", stale)
	removed, added := false, false
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, " s010/ ") {
			continue
		}
		removed = removed || strings.HasPrefix(l, "-  d")
		added = added || strings.HasPrefix(l, "+  d")
	}
	if code != 1 || !removed || !added {
		t.Errorf("-d: exit %d, want 1 and the s010/ line removed and added:\n%s", code, out)
	}
	if after, _ := os.ReadFile(stale); string(after) != string(staleBytes) {
		t.Errorf("-l/-d wrote the file")
	}

	// -l checks against the canonical form, so the pretty form is listed
	// (documented in the flag help and docs/cli-reference.md).
	prettyText, _, _ := runC4WithEnv(t, bin, env, "id", "-e", filepath.Join(root, "tree"))
	pretty := filepath.Join(root, "pretty.c4m")
	os.WriteFile(pretty, []byte(prettyText), 0o644)
	if out, _, code := runC4WithEnv(t, bin, env, "id", "-l", pretty); code != 1 || out != pretty+"\n" {
		t.Errorf("-l on the pretty form: exit %d, stdout %q; want it listed, exit 1", code, out)
	}
}

// `c4 id -l` never lists a correct file: every description this line's own
// tools write passes, on a settled tree holding nesting, an empty folder, a
// zero-byte file, links to a file and to a folder, a 0750 folder with an
// old time, and a three-frame sequence. Measured on 257c7dc, and pinned
// here.
func TestNormaliserPassesTheLinesOwnOutputs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	bin := buildC4(t)
	root := t.TempDir()
	env := map[string]string{"HOME": filepath.Join(root, "home"), "USERPROFILE": filepath.Join(root, "home"), "C4_STORE": filepath.Join(root, "store")}
	os.MkdirAll(env["HOME"], 0o755)
	dir := filepath.Join(root, "tree")
	for _, d := range []string{"docs", "empty", "shots/s010", "plates"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	at := func(y int) time.Time { return time.Date(y, 1, 2, 3, 4, 5, 0, time.UTC) }
	for _, f := range []struct {
		path, body string
		year       int
	}{
		{"docs/readme.txt", "readme\n", 2020},
		{"docs/zero.bin", "", 2021},
		{"shots/s010/a.exr", "frame a\n", 2022},
		{"plates/f.001.exr", "frame 1\n", 2023},
		{"plates/f.002.exr", "frame 2\n", 2023},
		{"plates/f.003.exr", "frame 3\n", 2023},
	} {
		p := filepath.Join(dir, f.path)
		if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(p, at(f.year), at(f.year))
	}
	for _, l := range []struct{ target, path string }{{"readme.txt", "docs/to-file"}, {"../shots", "docs/to-folder"}} {
		p := filepath.Join(dir, l.path)
		if err := os.Symlink(l.target, p); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("touch", "-h", "-t", at(2019).Format("200601021504.05"), p)
		cmd.Env = append(os.Environ(), "TZ=UTC")
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}
	os.Chmod(filepath.Join(dir, "shots"), 0o750)
	for _, d := range []struct {
		path string
		year int
	}{{"shots/s010", 2017}, {"shots", 2010}, {"plates", 2016}, {"empty", 2014}, {"docs", 2018}, {".", 2015}} {
		os.Chtimes(filepath.Join(dir, d.path), at(d.year), at(d.year))
	}

	for _, scan := range [][]string{{"id", dir}, {"id", "-S", dir}, {"id", "-m", "m", dir}, {"id", "-m", "s", dir}, {"id", "-s", dir}} {
		name := strings.Join(scan[:len(scan)-1], " ")
		t.Run(name, func(t *testing.T) {
			out, stderr, code := runC4WithEnv(t, bin, env, scan...)
			if code != 0 {
				t.Fatalf("c4 %s: exit %d\n%s", name, code, stderr)
			}
			scanned := filepath.Join(root, strings.ReplaceAll(name, " ", "")+".c4m")
			os.WriteFile(scanned, []byte(out), 0o644)
			// What each level states for sizes, measured on this line: a
			// metadata scan (-m m) records file and folder sizes with null
			// C4 IDs; a structure scan (-m s) records every file's size
			// null, so each folder holding one is the null-size case the
			// normaliser re-derives to null, and must already be null (an
			// empty folder is size 0).
			sm := decodeText(t, out)
			for p, e := range c4m.EntryPaths(sm.Entries) {
				switch {
				case name == "id -m s" && p != "empty/" && e.Size != -1:
					t.Errorf("-m s: %s states size %d; this test expects it null", p, e.Size)
				case name == "id -m m" && e.Size < 0:
					t.Errorf("-m m: %s states a null size; this test expects sizes recorded", p)
				}
			}
			flat, stderr, code := runC4WithEnv(t, bin, env, "patch", scanned)
			if code != 0 {
				t.Fatalf("c4 patch %s: exit %d\n%s", filepath.Base(scanned), code, stderr)
			}
			flattened := filepath.Join(root, strings.ReplaceAll(name, " ", "")+"-flat.c4m")
			os.WriteFile(flattened, []byte(flat), 0o644)
			for _, f := range []string{scanned, flattened} {
				if out, stderr, code := runC4WithEnv(t, bin, env, "id", "-l", f); code != 0 {
					t.Errorf("c4 id -l %s: exit %d, a correct file is listed\n%s%s", filepath.Base(f), code, out, stderr)
				}
			}
		})
	}
}

// The scan and the store pass keep their released time handling
// (06f5f8f): a folder whose own mtime is the epoch, so reads as null, is
// written with its newest child's time, filled by PropagateMetadata, in
// the scan and again in the store pass's re-derivation above a file it
// gives up. That is a time derived from contents, which the maintainer
// ruled on 2026-11-24 must never be ("the mod time should NEVER be based
// on contents, that has bad sideffects."), but changing a released
// writer is his to order. This pins the line as it is until he does.
func TestStorePassFolderTimeIsAsReleased(t *testing.T) {
	bin := buildC4Hooked(t)
	tree := filepath.Join(t.TempDir(), "sp")
	writeTree(t, tree, map[string]string{"a.txt": "alpha", "sub/b.txt": "beta", "sub/c.txt": "gamma"})
	older := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	newest := time.Date(2021, 2, 3, 4, 5, 6, 0, time.UTC)
	for p, at := range map[string]time.Time{"a.txt": older, "sub/b.txt": older, "sub/c.txt": newest, "sub": time.Unix(0, 0), "": older} {
		if err := os.Chtimes(filepath.Join(tree, p), at, at); err != nil {
			t.Fatal(err)
		}
	}
	env := storePassEnv(t)
	env["C4_TEST_STORE_PASS"] = "write sub/b.txt bytes the scan never saw"
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 2 {
		t.Fatalf("exit %d, want 2 (a file given up)\n%s", code, stderr)
	}
	sub := entryAt(t, decodeText(t, out), "sub/")
	if !sub.Timestamp.Equal(newest) {
		t.Errorf("sub/ time %s; on 06f5f8f it is its newest child's, %s", sub.Timestamp, newest)
	}
}
