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
				m.Sort()
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
				Name: fmt.Sprintf("dir%03d/file%02d.txt", i, j),
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := *manifest // Copy
		m.SortSiblingsHierarchically()
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

// BenchmarkUnion tests union operation performance
func BenchmarkUnion(b *testing.B) {
	benchmarks := []struct {
		name     string
		sets     int
		sizeEach int
	}{
		{"2Sets-100Each", 2, 100},
		{"3Sets-100Each", 3, 100},
		{"2Sets-1000Each", 2, 1000},
		{"5Sets-1000Each", 5, 1000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			sources := make([]Source, bm.sets)

			for s := 0; s < bm.sets; s++ {
				manifest := NewManifest()
				for i := 0; i < bm.sizeEach; i++ {
					manifest.AddEntry(&Entry{
						Name: fmt.Sprintf("set%d_file%06d.txt", s, i),
						C4ID: c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("content%d_%d", s, i)))),
					})
				}
				sources[s] = ManifestSource{manifest}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Union(sources...)
			}
		})
	}
}

// BenchmarkIntersect tests intersect operation performance
func BenchmarkIntersect(b *testing.B) {
	benchmarks := []struct {
		name    string
		size    int
		overlap int // percentage of overlap
	}{
		{"Small-50%Overlap", 100, 50},
		{"Small-NoOverlap", 100, 0},
		{"Medium-50%Overlap", 1000, 50},
		{"Medium-10%Overlap", 1000, 10},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create first manifest
			manifest1 := NewManifest()
			for i := 0; i < bm.size; i++ {
				manifest1.AddEntry(&Entry{
					Name: fmt.Sprintf("file%06d.txt", i),
					C4ID: c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("content%d", i)))),
				})
			}

			// Create second manifest with overlap
			manifest2 := NewManifest()
			overlapCount := bm.size * bm.overlap / 100
			// Add overlapping entries
			for i := 0; i < overlapCount; i++ {
				manifest2.AddEntry(&Entry{
					Name: fmt.Sprintf("file%06d.txt", i),
					C4ID: c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("content%d", i)))),
				})
			}
			// Add unique entries
			for i := bm.size; i < bm.size+(bm.size-overlapCount); i++ {
				manifest2.AddEntry(&Entry{
					Name: fmt.Sprintf("file%06d.txt", i),
					C4ID: c4.Identify(bytes.NewReader([]byte(fmt.Sprintf("content%d", i)))),
				})
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Intersect(ManifestSource{manifest1}, ManifestSource{manifest2})
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
			buf.WriteString("@c4m 1.0\n")

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
			Name:      fmt.Sprintf("dir%03d/file%03d.txt", i/10, i),
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