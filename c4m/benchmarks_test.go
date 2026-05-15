package c4m

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// BenchmarkManifestSort tests sorting performance
func BenchmarkManifestSort(b *testing.B) {
	benchmarks := []struct {
		name string
		size int
	}{
		{"Small-100", 100},
		{"Medium-1000", 1000},
		{"Large-10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create manifest with random order
			manifest := NewManifest()
			for i := bm.size; i > 0; i-- {
				manifest.AddEntry(&Entry{
					Name: fmt.Sprintf("file%06d.txt", i),
				})
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m := *manifest // Copy
				m.SortEntries()
			}
		})
	}
}

// BenchmarkSortEntries_Pathological measures sortSiblingsHierarchically on
// a single chain of N nested directories (depth == N). The prior recursive
// implementation rescanned the tail at each level, giving O(N^2). The
// current single-pass index makes this O(N log N) — verify near-linear
// scaling: each 10x bump in depth should cost ~10x time, not ~100x.
func BenchmarkSortEntries_Pathological(b *testing.B) {
	sizes := []int{1000, 10000, 100000}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("Chain-%d", n), func(b *testing.B) {
			template := make([]*Entry, n)
			for i := 0; i < n; i++ {
				template[i] = &Entry{
					Name:  fmt.Sprintf("d%d/", i),
					Mode:  os.ModeDir,
					Depth: i,
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m := NewManifest()
				m.Entries = append(m.Entries[:0], template...)
				m.SortEntries()
			}
		})
	}
}

// BenchmarkSortEntries_NormalTree provides a comparison shape: a balanced
// two-level tree with sqrt(N) directories each holding sqrt(N) files.
// Sort cost is dominated by the per-parent sort and should scale
// near-linearly with N.
func BenchmarkSortEntries_NormalTree(b *testing.B) {
	sizes := []int{1000, 10000, 100000}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("Tree-%d", n), func(b *testing.B) {
			fanout := 1
			for fanout*fanout < n {
				fanout++
			}
			template := make([]*Entry, 0, n+fanout)
			for d := 0; len(template) < n; d++ {
				template = append(template, &Entry{
					Name:  fmt.Sprintf("dir%05d/", d),
					Mode:  os.ModeDir,
					Depth: 0,
				})
				for f := 0; f < fanout && len(template) < n; f++ {
					template = append(template, &Entry{
						Name:  fmt.Sprintf("file%05d.txt", f),
						Depth: 1,
					})
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m := NewManifest()
				m.Entries = append(m.Entries[:0], template...)
				m.SortEntries()
			}
		})
	}
}

// BenchmarkHierarchicalSort tests hierarchical sorting performance
func BenchmarkHierarchicalSort(b *testing.B) {
	// Create manifest with mixed files and directories
	manifest := NewManifest()

	// Add entries in reverse order to force sorting
	for i := 100; i > 0; i-- {
		// Add directory
		manifest.AddEntry(&Entry{
			Name: fmt.Sprintf("dir%03d/", i),
			Mode: os.ModeDir,
		})
		// Add files in directory
		for j := 10; j > 0; j-- {
			manifest.AddEntry(&Entry{
				Name:  fmt.Sprintf("file%02d.txt", j),
				Depth: 1,
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := *manifest // Copy
		m.SortEntries()
	}
}

// BenchmarkDiff tests diff operation performance
func BenchmarkDiff(b *testing.B) {
	benchmarks := []struct {
		name     string
		size     int
		changes  int // percentage of changes
	}{
		{"Small-NoChanges", 100, 0},
		{"Small-10%Changes", 100, 10},
		{"Medium-NoChanges", 1000, 0},
		{"Medium-10%Changes", 1000, 10},
		{"Large-NoChanges", 10000, 0},
		{"Large-10%Changes", 10000, 10},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create source manifest
			source := NewManifest()
			for i := 0; i < bm.size; i++ {
				source.AddEntry(&Entry{
					Name: fmt.Sprintf("file%06d.txt", i),
					C4ID: c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("content%d", i)))),
				})
			}

			// Create target manifest with changes
			target := NewManifest()
			changedCount := bm.size * bm.changes / 100
			for i := 0; i < bm.size; i++ {
				entry := &Entry{
					Name: fmt.Sprintf("file%06d.txt", i),
				}
				if i < changedCount {
					// Modified content
					entry.C4ID = c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("modified%d", i))))
				} else {
					// Same content
					entry.C4ID = c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("content%d", i))))
				}
				target.AddEntry(entry)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Diff(ManifestSource{source}, ManifestSource{target})
			}
		})
	}
}

// BenchmarkParsing tests C4M parsing performance
func BenchmarkParsing(b *testing.B) {
	benchmarks := []struct {
		name string
		size int
	}{
		{"Small-100", 100},
		{"Medium-1000", 1000},
		{"Large-10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create C4M content
			var buf bytes.Buffer

			for i := 0; i < bm.size; i++ {
				// Write entry
				fmt.Fprintf(&buf, "-rw-r--r-- %s %d file%06d.txt %s\n",
					time.Now().Format(time.RFC3339),
					1024,
					i,
					"c41qhJmEJCcRxvK3LKSjN9HYYKVVoXZQZzV2UkdHcRfL3vMqFVVaGUeEKNGCfkKr2mD9wcxJyiRzqXcH2g5jCMQHj7",
				)
			}

			content := buf.Bytes()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				decoder := NewDecoder(bytes.NewReader(content))
				_, _ = decoder.Decode()
			}
		})
	}
}

// BenchmarkWriting tests C4M writing performance
func BenchmarkWriting(b *testing.B) {
	benchmarks := []struct {
		name string
		size int
	}{
		{"Small-100", 100},
		{"Medium-1000", 1000},
		{"Large-10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create manifest
			manifest := NewManifest()
			for i := 0; i < bm.size; i++ {
				manifest.AddEntry(&Entry{
					Name:      fmt.Sprintf("file%06d.txt", i),
					Size:      1024,
					Timestamp: time.Now(),
					Mode:      0644,
					C4ID:      c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("content%d", i)))),
				})
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var buf bytes.Buffer
				_ = NewEncoder(&buf).Encode(manifest)
			}
		})
	}
}

// BenchmarkValidation tests C4M validation performance
func BenchmarkValidation(b *testing.B) {
	// Create test manifest
	manifest := NewManifest()
	for i := 0; i < 1000; i++ {
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("file%03d.txt", i),
			Size:      1024,
			Timestamp: time.Now(),
			Mode:      0644,
		})
	}

	// Write to buffer
	var buf bytes.Buffer
	_ = NewEncoder(&buf).Encode(manifest)
	content := buf.Bytes()

	validator := NewValidator(false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateManifest(bytes.NewReader(content))
	}
}

// BenchmarkNaturalSortPerformance tests natural sorting algorithm performance
func BenchmarkNaturalSortPerformance(b *testing.B) {
	// Create list of filenames with mixed numeric patterns
	names := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		names[i] = fmt.Sprintf("file%d_v%02d_frame%04d.txt", i%10, i%5, i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		namesCopy := make([]string, len(names))
		copy(namesCopy, names)

		for j := 0; j < len(namesCopy)-1; j++ {
			_ = NaturalLess(namesCopy[j], namesCopy[j+1])
		}
	}
}

// buildSyntheticManifest creates a filesystem-walk-ordered slice of entries
// approximating a real source tree: a tunable directory ratio, modest fanout,
// every directory's null Size and Timestamp, every file's known Size and
// Timestamp. Used by BenchmarkPropagateMetadata_Linear to assert scaling.
//
// The shape: a depth-`maxDepth` tree where every directory has `dirFanout`
// child directories plus `fileFanout` child files, truncated once we have
// `n` entries total. Entries are emitted in depth-first walk order — files
// before directories at each level — which matches what scan.GenerateFromPath
// produces post-SortEntries.
func buildSyntheticManifest(n, dirFanout, fileFanout, maxDepth int) []*Entry {
	entries := make([]*Entry, 0, n)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var emit func(prefix string, depth int)
	emit = func(prefix string, depth int) {
		if len(entries) >= n {
			return
		}
		// Files first.
		for i := 0; i < fileFanout && len(entries) < n; i++ {
			entries = append(entries, &Entry{
				Name:      fmt.Sprintf("%sfile%d.txt", prefix, i),
				Mode:      0644,
				Timestamp: now,
				Size:      int64(1024 + i),
				Depth:     depth,
			})
		}
		if depth >= maxDepth {
			return
		}
		// Then subdirectories — null Size and Timestamp so propagation has work to do.
		for i := 0; i < dirFanout && len(entries) < n; i++ {
			entries = append(entries, &Entry{
				Name:      fmt.Sprintf("%sdir%d/", prefix, i),
				Mode:      os.ModeDir | 0755,
				Timestamp: time.Unix(0, 0).UTC(),
				Size:      -1,
				Depth:     depth,
			})
			emit(fmt.Sprintf("%sdir%d_", prefix, i), depth+1)
		}
	}
	emit("", 0)
	return entries
}

// BenchmarkPropagateMetadata_Linear asserts that c4m.PropagateMetadata
// scales near-linearly with entry count. The previous quadratic algorithm
// (linear getDirectoryChildren scan per null directory) blew up to
// minutes-long latency on 1M-entry trees; the single-pass depth-stack
// algorithm should stay near 1.0× per doubling of input. If the ratio
// between adjacent sizes ever climbs much above ~1.3×, something has
// regressed.
//
// Run:
//   go test -run='^$' -bench=BenchmarkPropagateMetadata_Linear -benchtime=3x ./c4m
func BenchmarkPropagateMetadata_Linear(b *testing.B) {
	sizes := []int{10_000, 100_000, 1_000_000}
	for _, n := range sizes {
		template := buildSyntheticManifest(n, 4, 4, 12)
		b.Run(fmt.Sprintf("entries=%d", len(template)), func(b *testing.B) {
			b.ReportAllocs()
			b.StopTimer()
			for i := 0; i < b.N; i++ {
				entries := make([]*Entry, len(template))
				for j, e := range template {
					cp := *e
					entries[j] = &cp
				}
				b.StartTimer()
				PropagateMetadata(entries)
				b.StopTimer()
			}
		})
	}
}

// BenchmarkComputeC4ID_LargeManifest measures allocs/op for
// Manifest.ComputeC4ID on a 100K-entry flat manifest. The previous
// implementation built the entire canonical text as a single allocated
// string before passing it to c4.Identify; the streaming path (writing
// each entry's canonical line through an io.Pipe into c4.Identify)
// eliminates that single multi-MB allocation. The remaining allocs/op
// are dominated by per-Entry Canonical() string construction, which is
// out of scope here.
//
// Run:
//
//	go test -run='^$' -bench=BenchmarkComputeC4ID_LargeManifest -benchtime=5x ./c4m
func BenchmarkComputeC4ID_LargeManifest(b *testing.B) {
	const n = 100_000
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	entries := make([]*Entry, n)
	for i := 0; i < n; i++ {
		entries[i] = &Entry{
			Name:      fmt.Sprintf("file%06d.txt", i),
			Mode:      0644,
			Timestamp: now,
			Size:      int64(1024 + i),
			Depth:     0,
		}
	}
	m := &Manifest{Version: "1.0", Entries: entries}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.ComputeC4ID()
	}
}

// BenchmarkC4IDComparison tests C4 ID comparison performance
func BenchmarkC4IDComparison(b *testing.B) {
	// Create IDs
	ids := make([]c4.ID, 1000)
	for i := 0; i < 1000; i++ {
		ids[i] = c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("content%d", i))))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(ids)-1; j++ {
			_ = ids[j].String() == ids[j+1].String()
		}
	}
}