package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// TestLogLongChainLinear is the regression test for the super-quadratic
// `c4 log`: the shipped implementation resolved the whole chain from
// scratch for every section (500 sections took ~60s; 1000 never
// finished). The incremental implementation applies one patch per
// section; 1000 sections must complete comfortably within the timeout.
func TestLogLongChainLinear(t *testing.T) {
	const sections = 1000

	// Build a chain: a base with one entry, then N patch sections each
	// adding one file. Format per c4m chain tests: base entries, base
	// ID line, patch entries, patch ID line, ..., ending on the last
	// patch's entries (a chain never ends with its own ID).
	var chain bytes.Buffer
	base := c4m.NewManifest()
	base.AddEntry(&c4m.Entry{
		Name: "file0000.txt",
		Size: 5,
		C4ID: c4.Identify(strings.NewReader("content-0")),
	})
	var baseBuf bytes.Buffer
	c4m.NewEncoder(&baseBuf).Encode(base)
	chain.Write(baseBuf.Bytes())
	chain.WriteString(base.ComputeC4ID().String() + "\n")

	resolved := base
	for i := 1; i < sections; i++ {
		if i > 1 {
			// Each patch is preceded by the accumulated state's bare ID
			// line — the chain-format section boundary.
			chain.WriteString(resolved.ComputeC4ID().String() + "\n")
		}
		patch := c4m.NewManifest()
		patch.AddEntry(&c4m.Entry{
			Name: fmt.Sprintf("file%04d.txt", i),
			Size: 5,
			C4ID: c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i))),
		})
		c4m.NewEncoder(&chain).Encode(patch)
		resolved = c4m.ApplyPatch(resolved, patch)
	}

	dir := t.TempDir()
	chainPath := filepath.Join(dir, "history.c4m")
	if err := os.WriteFile(chainPath, chain.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildC4(t)
	start := time.Now()
	stdout, stderr, code := runC4(t, bin, "log", chainPath)
	elapsed := time.Since(start)
	if code != 0 {
		t.Fatalf("c4 log exit %d: %s", code, stderr)
	}
	lines := strings.Count(strings.TrimSpace(stdout), "\n") + 1
	if lines != sections {
		t.Fatalf("expected %d log lines, got %d", sections, lines)
	}
	// Generous bound: incremental resolution finishes in a few seconds;
	// the quadratic implementation did not finish 1000 sections in
	// minutes. The bound catches a complexity regression, not noise.
	if elapsed > 60*time.Second {
		t.Fatalf("c4 log took %v for %d sections — complexity regression", elapsed, sections)
	}
	t.Logf("c4 log: %d sections in %v", sections, elapsed)
}
