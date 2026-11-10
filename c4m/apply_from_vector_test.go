package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// The shared conformance vector for ApplyPatchFrom: testdata/apply-from-vector,
// one folder per case with base.c4m, target.c4m, patch.c4m (the patch
// diffed from base, in patch form: the base's ID as boundary, then the
// entries), expected.c4m and expected-root-id.txt. Every file is made by
// the library (set C4M_UPDATE_VECTORS=1 to rewrite); each case is one the
// operator's cross-check or a review found breaking an implementation
// (2026-11-09). A conforming implementation applies patch.c4m, read
// against base.c4m, to target.c4m and gets expected.c4m exactly.
type applyFromCase struct {
	name                string
	base, target, after *Manifest // after: the state the patch was diffed to
	check               func(t *testing.T, got *Manifest)
}

func applyFromCases() []applyFromCase {
	t1 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	t3 := t1.Add(2 * time.Hour)
	file := func(name, content string, depth int, mode os.FileMode, ts time.Time) *Entry {
		return &Entry{Name: name, Mode: mode, Size: int64(len(content)), Depth: depth,
			Timestamp: ts, C4ID: c4.Identify(strings.NewReader(content))}
	}
	dir := func(name string, depth int, ts time.Time) *Entry {
		return &Entry{Name: name, Mode: 0755 | os.ModeDir, Size: -1, Depth: depth, Timestamp: ts}
	}
	m := func(entries ...*Entry) *Manifest {
		out := NewManifest()
		for _, e := range entries {
			cp := *e
			out.AddEntry(&cp)
		}
		Normalize(out)
		return out
	}
	has := func(got *Manifest, path string) *Entry { return EntryPaths(got.Entries)[path] }

	return []applyFromCase{
		{
			// Both sides add the same path with the same bytes. Plain
			// ApplyPatch on the target reads the add as a removal.
			name:   "same-add",
			base:   m(file("keep.txt", "K", 0, 0644, t1)),
			after:  m(file("keep.txt", "K", 0, 0644, t1), file("out.txt", "same bytes", 0, 0644, t2)),
			target: m(file("keep.txt", "K", 0, 0644, t1), file("out.txt", "same bytes", 0, 0644, t2)),
			check: func(t *testing.T, got *Manifest) {
				if has(got, "out.txt") == nil {
					t.Fatal("same-add: the path must stay")
				}
			},
		},
		{
			// base d/f; the patch removes f; the target no longer has d/.
			name:   "removal-under-removed-dir",
			base:   m(dir("d/", 0, t1), file("f", "F", 1, 0644, t1), file("other.txt", "T", 0, 0644, t1)),
			after:  m(dir("d/", 0, t2), file("other.txt", "T", 0, 0644, t1)),
			target: m(file("other.txt", "T", 0, 0644, t1)),
			check: func(t *testing.T, got *Manifest) {
				if has(got, "d/") != nil {
					t.Fatal("removal-under-removed-dir: d/ must not be recreated")
				}
			},
		},
		{
			// The target's model.bin is executable, so its weights/ line
			// differs from the base's; the patch removes model.bin.
			name:   "emptied-dir-differing-line",
			base:   m(dir("weights/", 0, t1), file("model.bin", "weights", 1, 0644, t1)),
			after:  m(dir("weights/", 0, t2)),
			target: m(dir("weights/", 0, t1), file("model.bin", "weights", 1, 0755, t1)),
			check: func(t *testing.T, got *Manifest) {
				w := has(got, "weights/")
				if w == nil || has(got, "weights/model.bin") != nil {
					t.Fatal("emptied-dir-differing-line: weights/ stays, model.bin goes")
				}
				if w.Size != 0 || w.C4ID != c4.Identify(bytes.NewReader(nil)) {
					t.Fatalf("emptied-dir-differing-line: weights/ must be the empty directory, got %d %s", w.Size, w.C4ID)
				}
			},
		},
		{
			// Both wrote one path differently: the patch's entry wins.
			name:   "later-write-wins",
			base:   m(file("p.txt", "v0", 0, 0644, t1)),
			after:  m(file("p.txt", "ours", 0, 0644, t2)),
			target: m(file("p.txt", "theirs", 0, 0644, t2)),
			check: func(t *testing.T, got *Manifest) {
				if e := has(got, "p.txt"); e == nil || e.C4ID != c4.Identify(strings.NewReader("ours")) {
					t.Fatal("later-write-wins: the patch's entry must win")
				}
			},
		},
		{
			// The target is the base: the result is ApplyPatch's.
			name:   "own-base",
			base:   m(file("a.txt", "A", 0, 0644, t1), file("gone.txt", "G", 0, 0644, t1), dir("d/", 0, t1), file("old.txt", "O", 1, 0644, t1)),
			after:  m(file("a.txt", "A2", 0, 0644, t2), file("new.txt", "N", 0, 0644, t2), dir("d/", 0, t2), file("old.txt", "O", 1, 0644, t1), file("deep.txt", "D", 1, 0644, t2)),
			target: m(file("a.txt", "A", 0, 0644, t1), file("gone.txt", "G", 0, 0644, t1), dir("d/", 0, t1), file("old.txt", "O", 1, 0644, t1)),
			check: func(t *testing.T, got *Manifest) {
				if has(got, "gone.txt") != nil || has(got, "new.txt") == nil || has(got, "d/deep.txt") == nil || has(got, "d/old.txt") == nil {
					t.Fatal("own-base: all four meanings must apply")
				}
			},
		},
		{
			// A directory whose own time differs in target and base keeps
			// the target's line (its observed time); size and ID derive.
			name:   "dir-own-line-kept",
			base:   m(dir("d/", 0, t1), file("f", "F", 1, 0644, t1)),
			after:  m(dir("d/", 0, t2), file("f", "F", 1, 0644, t1), file("g", "G", 1, 0644, t2)),
			target: m(dir("d/", 0, t3), file("f", "F", 1, 0644, t1), file("x", "X", 1, 0644, t3)),
			check: func(t *testing.T, got *Manifest) {
				d := has(got, "d/")
				if d == nil || !d.Timestamp.Equal(t3) {
					t.Fatalf("dir-own-line-kept: the target's directory time must be kept, got %v", d)
				}
				if has(got, "d/f") == nil || has(got, "d/g") == nil || has(got, "d/x") == nil {
					t.Fatal("dir-own-line-kept: d/ must hold f, g and x")
				}
			},
		},
		{
			// A name is one thing. The target made directory x/ (with
			// x/y.txt) and file d; the patch writes file x and deepens d/
			// with a write under it. The later write wins by kind as it
			// does by bytes: x/ and its subtree go, file d goes. Before
			// the rule the result held both a file and a directory of one
			// name (75 of 601 applications with kind swaps, the storage
			// project's review, 2026-11-09).
			name:   "kind-conflict",
			base:   m(file("keep.txt", "K", 0, 0644, t1)),
			after:  m(file("keep.txt", "K", 0, 0644, t1), file("x", "file now", 0, 0644, t2), dir("d/", 0, t2), file("n.txt", "N", 1, 0644, t2)),
			target: m(file("keep.txt", "K", 0, 0644, t1), dir("x/", 0, t2), file("y.txt", "Y", 1, 0644, t2), file("d", "was a file", 0, 0644, t2)),
			check: func(t *testing.T, got *Manifest) {
				if has(got, "x") == nil || has(got, "x/") != nil || has(got, "x/y.txt") != nil {
					t.Fatal("kind-conflict: writing file x must remove directory x/ and its subtree")
				}
				if has(got, "d/") == nil || has(got, "d/n.txt") == nil || has(got, "d") != nil {
					t.Fatal("kind-conflict: a write under d/ must remove file d")
				}
				for _, e := range got.Entries {
					other := e.Name + "/"
					if e.IsDir() {
						other = strings.TrimSuffix(e.Name, "/")
					}
					if e.Depth == 0 && has(got, other) != nil {
						t.Fatalf("kind-conflict: %q and %q both present", e.Name, other)
					}
				}
			},
		},
	}
}

func TestApplyFromVector(t *testing.T) {
	root := filepath.Join("testdata", "apply-from-vector")
	update := os.Getenv("C4M_UPDATE_VECTORS") != ""
	for _, tc := range applyFromCases() {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(root, tc.name)
			pr := PatchDiff(tc.base, tc.after)
			var patch bytes.Buffer // as EncodePatch writes it: the base's ID, then the entries
			if err := NewEncoder(&patch).EncodePatch(pr); err != nil {
				t.Fatal(err)
			}
			expected := ApplyPatchFrom(tc.target, tc.base, pr.Patch)
			tc.check(t, expected)

			files := map[string][]byte{
				"base.c4m":             encodeCanonical(t, tc.base),
				"target.c4m":           encodeCanonical(t, tc.target),
				"patch.c4m":            patch.Bytes(),
				"expected.c4m":         encodeCanonical(t, expected),
				"expected-root-id.txt": []byte(expected.ComputeC4ID().String() + "\n"),
			}
			if update {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				for name, data := range files {
					if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			for name, want := range files {
				got, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s/%s differs from what the library writes:\n--- file\n%s--- library\n%s", tc.name, name, got, want)
				}
			}

			// A conforming reader: decode the three files, apply, compare.
			base := decodeFile(t, filepath.Join(dir, "base.c4m"))
			target := decodeFile(t, filepath.Join(dir, "target.c4m"))
			sections, err := DecodePatchChain(bytes.NewReader(files["patch.c4m"]))
			if err != nil || len(sections) != 1 || sections[0].BaseID != base.ComputeC4ID() {
				t.Fatalf("patch.c4m must be one section over the base's ID: %d sections, %v", len(sections), err)
			}
			patchM := &Manifest{Version: "1.0", Entries: sections[0].Entries}
			got := ApplyPatchFrom(target, base, patchM)
			if got.Canonical() != expected.Canonical() {
				t.Fatalf("applied from files differs:\n%s\nvs\n%s", got.Canonical(), expected.Canonical())
			}
		})
	}
}

func encodeCanonical(t *testing.T, m *Manifest) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(m); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeFile(t *testing.T, path string) *Manifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
