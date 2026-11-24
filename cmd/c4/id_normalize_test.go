package main

import (
	"os"
	"path/filepath"
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

	t.Run("fills a null time from the newest child", func(t *testing.T) {
		p := mutated(t, root, "null-time", full, func(m *c4m.Manifest) { entryAt(t, m, "shots/s010/").Timestamp = c4m.NullTimestamp() })
		out, _ := normalise(t, p)
		got := entryAt(t, decodeText(t, out), "shots/s010/")
		want := entryAt(t, fm, "shots/s010/b.exr").Timestamp
		if !got.Timestamp.Equal(want) {
			t.Errorf("s010/ time %s, want its newest child's %s", got.Timestamp, want)
		}
	})

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

	// Not ruled, so kept exactly as 06f5f8f keeps them: a stated size whose
	// children cannot support one is not re-derived to null, and a
	// directory over a null-ID child keeps its stated C4 ID.
	t.Run("left out: a stated size over a null-size child", func(t *testing.T) {
		p := mutated(t, root, "child-null-size", full, func(m *c4m.Manifest) { entryAt(t, m, "shots/s010/a.exr").Size = -1 })
		out, _ := normalise(t, p)
		if got := entryAt(t, decodeText(t, out), "shots/s010/"); got.Size != s010.Size {
			t.Errorf("s010/ size %d, stated %d: re-derived", got.Size, s010.Size)
		}
	})
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
	if code != 1 || !strings.Contains(out, "\n-  drwxr-xr-x ") || !strings.Contains(out, "\n+  drwxr-xr-x ") {
		t.Errorf("-d: exit %d, want 1 and the s010/ line removed and added:\n%s", code, out)
	}
	if after, _ := os.ReadFile(stale); string(after) != string(staleBytes) {
		t.Errorf("-l/-d wrote the file")
	}
}
