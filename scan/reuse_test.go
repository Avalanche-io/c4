package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// TestReuseGuideTrustRule pins the metadata-trusted re-scan: unchanged
// files (size+mtime match, strictly older than the guide's scan start)
// reuse guide IDs without re-reading; changed and racy files re-hash.
func TestReuseGuideTrustRule(t *testing.T) {
	tree := t.TempDir()
	writeFile := func(rel, content string) string {
		p := filepath.Join(tree, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	aPath := writeFile("a.txt", "alpha content")
	bPath := writeFile("sub/b.txt", "bravo content")
	writeFile("sub/c.txt", "charlie content")

	// Age the files well behind the coming scan start.
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	for _, p := range []string{aPath, bPath, filepath.Join(tree, "sub/c.txt")} {
		if err := os.Chtimes(p, past, past); err != nil {
			t.Fatal(err)
		}
	}

	guide, err := Dir(tree, WithMode(ModeFull))
	if err != nil {
		t.Fatal(err)
	}
	scanStart := time.Now().UTC()

	// Change b (bytes AND mtime), make c racy (mtime >= scan start),
	// leave a untouched.
	if err := os.WriteFile(bPath, []byte("bravo CHANGED!"), 0644); err != nil {
		t.Fatal(err)
	}
	future := scanStart.Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(tree, "sub/c.txt"), future, future); err != nil {
		t.Fatal(err)
	}

	gen := NewGeneratorWithOptions(
		WithMode(ModeFull),
		WithReuseGuide(guide, scanStart),
	)
	m, err := gen.GenerateFromPath(tree)
	if err != nil {
		t.Fatal(err)
	}
	reused, rehashed := gen.ReuseStats()
	if reused != 1 {
		t.Fatalf("expected exactly 1 reused (a.txt), got %d", reused)
	}
	if rehashed != 2 {
		t.Fatalf("expected 2 rehashed (changed b, racy c), got %d", rehashed)
	}

	// The reused entry's ID matches the guide; the changed file's ID
	// differs from the guide.
	guidePaths := map[string]string{}
	var stack []string
	for _, e := range guide.Entries {
		if e.Depth < len(stack) {
			stack = stack[:e.Depth]
		}
		if e.IsDir() {
			for len(stack) <= e.Depth {
				stack = append(stack, "")
			}
			stack[e.Depth] = e.Name + "/"
			continue
		}
		guidePaths[joinStack(stack, e.Depth)+e.Name] = e.C4ID.String()
	}
	var newStack []string
	for _, e := range m.Entries {
		if e.Depth < len(newStack) {
			newStack = newStack[:e.Depth]
		}
		if e.IsDir() {
			for len(newStack) <= e.Depth {
				newStack = append(newStack, "")
			}
			newStack[e.Depth] = e.Name + "/"
			continue
		}
		p := joinStack(newStack, e.Depth) + e.Name
		switch p {
		case "a.txt":
			if e.C4ID.String() != guidePaths[p] {
				t.Fatalf("a.txt should reuse guide ID")
			}
		case "sub/b.txt":
			if e.C4ID.String() == guidePaths[p] {
				t.Fatalf("changed b.txt must not reuse guide ID")
			}
		case "sub/c.txt":
			// Racy but unchanged: re-hashed, same bytes → same ID.
			if e.C4ID.String() != guidePaths[p] {
				t.Fatalf("racy c.txt re-hash should yield the same ID for same bytes")
			}
		}
	}
}

func joinStack(stack []string, depth int) string {
	out := ""
	for i := 0; i < depth && i < len(stack); i++ {
		out += stack[i]
	}
	return out
}

// TestReuseGuideAcceptedRisk pins the documented posture: a same-size
// byte change with a restored (older) mtime is INVISIBLE to the
// default re-scan — the guide ID is reused for stale bytes. This is a
// deliberate, stated trade (design/snapshot-loop v8 Amendment 1);
// --verify exists to catch it on demand.
func TestReuseGuideAcceptedRisk(t *testing.T) {
	tree := t.TempDir()
	p := filepath.Join(tree, "x.bin")
	if err := os.WriteFile(p, []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(p, past, past); err != nil {
		t.Fatal(err)
	}

	guide, err := Dir(tree, WithMode(ModeFull))
	if err != nil {
		t.Fatal(err)
	}
	scanStart := time.Now().UTC()

	// The obfuscated change: same size, mtime restored.
	if err := os.WriteFile(p, []byte("9876543210"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, past, past); err != nil {
		t.Fatal(err)
	}

	gen := NewGeneratorWithOptions(WithMode(ModeFull), WithReuseGuide(guide, scanStart))
	m, err := gen.GenerateFromPath(tree)
	if err != nil {
		t.Fatal(err)
	}
	reused, _ := gen.ReuseStats()
	if reused != 1 {
		t.Fatalf("posture: obfuscated change should be reused (invisible), got reused=%d", reused)
	}
	// The stale ID is the guide's — the documented accepted risk.
	if m.Entries[0].C4ID != guide.Entries[0].C4ID {
		t.Fatal("expected the stale guide ID under the posture")
	}
}

// TestProjectContentMatchesGeneration pins eval-side projection: a
// full scan projected with ProjectContent is byte-identical to a
// generation-side ModeContent scan — same canonical text, same root
// ID, on trees with nesting, symlinks, and empty directories.
func TestProjectContentMatchesGeneration(t *testing.T) {
	tree := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(tree, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0640); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "alpha")
	write("sub/b.txt", "bravo")
	write("sub/deep/c.md", "charlie")
	if err := os.MkdirAll(filepath.Join(tree, "emptydir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(tree, "link")); err != nil {
		t.Fatal(err)
	}

	generated, err := Dir(tree, WithMode(ModeContent))
	if err != nil {
		t.Fatal(err)
	}
	full, err := Dir(tree, WithMode(ModeFull))
	if err != nil {
		t.Fatal(err)
	}
	projected := ProjectContent(full)

	if got, want := projected.ComputeC4ID(), generated.ComputeC4ID(); got != want {
		t.Fatalf("projection root ID %s != generation root ID %s", got, want)
	}
	gText := generated.Canonical()
	pText := projected.Canonical()
	if gText != pText {
		t.Fatalf("projection differs from generation:\n--- generated\n%s--- projected\n%s", gText, pText)
	}

	// Idempotence: projecting a projection changes nothing.
	again := ProjectContent(projected)
	if again.ComputeC4ID() != projected.ComputeC4ID() {
		t.Fatal("projection is not idempotent")
	}

	// The projection never mutates its input: the full manifest keeps
	// its modes and timestamps.
	if full.Entries[0].Mode == 0 && full.Entries[0].Timestamp.Equal(c4m.NullTimestamp()) {
		t.Fatal("ProjectContent mutated its input")
	}
}
