package scan

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// createBenchTree creates a directory tree with the given number of files.
// Files are spread across subdirectories (~50 files per dir) to simulate
// realistic project layouts. Each file contains small random content to
// ensure unique C4 IDs.
func createBenchTree(b *testing.B, numFiles int) string {
	b.Helper()
	dir := b.TempDir()

	filesPerDir := 50
	dirIndex := 0
	currentDir := dir

	for i := 0; i < numFiles; i++ {
		if i%filesPerDir == 0 {
			currentDir = filepath.Join(dir, fmt.Sprintf("dir%04d", dirIndex))
			if err := os.MkdirAll(currentDir, 0755); err != nil {
				b.Fatal(err)
			}
			dirIndex++
		}
		// Small files with unique content (32 bytes random)
		content := make([]byte, 32)
		if _, err := rand.Read(content); err != nil {
			b.Fatal(err)
		}
		path := filepath.Join(currentDir, fmt.Sprintf("file%06d.dat", i))
		if err := os.WriteFile(path, content, 0644); err != nil {
			b.Fatal(err)
		}
	}
	return dir
}

// createBenchTreeMixed creates a tree with varied file sizes to simulate
// real-world projects (many small files, some medium, a few large).
func createBenchTreeMixed(b *testing.B, numFiles int) string {
	b.Helper()
	dir := b.TempDir()

	filesPerDir := 50
	dirIndex := 0
	currentDir := dir

	for i := 0; i < numFiles; i++ {
		if i%filesPerDir == 0 {
			currentDir = filepath.Join(dir, fmt.Sprintf("dir%04d", dirIndex))
			if err := os.MkdirAll(currentDir, 0755); err != nil {
				b.Fatal(err)
			}
			dirIndex++
		}

		// Varied sizes: 90% small (100B), 9% medium (10KB), 1% large (1MB)
		var size int
		switch {
		case i%100 == 0:
			size = 1024 * 1024 // 1MB
		case i%10 == 0:
			size = 10 * 1024 // 10KB
		default:
			size = 100 // 100B
		}

		content := make([]byte, size)
		if _, err := rand.Read(content); err != nil {
			b.Fatal(err)
		}
		path := filepath.Join(currentDir, fmt.Sprintf("file%06d.dat", i))
		if err := os.WriteFile(path, content, 0644); err != nil {
			b.Fatal(err)
		}
	}
	return dir
}

// BenchmarkGeneratorStructure benchmarks the Generator doing structure+metadata
// only (no C4 ID computation). This is the fast path.
func BenchmarkGeneratorStructure(b *testing.B) {
	for _, numFiles := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("files=%d", numFiles), func(b *testing.B) {
			dir := createBenchTree(b, numFiles)
			g := NewGeneratorWithOptions(WithC4IDs(false))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m, err := g.GenerateFromPath(dir)
				if err != nil {
					b.Fatal(err)
				}
				_ = m
			}
			b.ReportMetric(float64(numFiles), "files/op")
		})
	}
}

// BenchmarkGeneratorFull benchmarks the Generator with C4 ID computation.
func BenchmarkGeneratorFull(b *testing.B) {
	for _, numFiles := range []int{100, 1000} {
		b.Run(fmt.Sprintf("files=%d", numFiles), func(b *testing.B) {
			dir := createBenchTree(b, numFiles)
			g := NewGenerator()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m, err := g.GenerateFromPath(dir)
				if err != nil {
					b.Fatal(err)
				}
				_ = m
			}
			b.ReportMetric(float64(numFiles), "files/op")
		})
	}
}

// BenchmarkProgressiveFull benchmarks the ProgressiveScanner with all stages.
func BenchmarkProgressiveFull(b *testing.B) {
	for _, numFiles := range []int{100, 1000} {
		b.Run(fmt.Sprintf("files=%d", numFiles), func(b *testing.B) {
			dir := createBenchTree(b, numFiles)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ps := NewProgressiveScanner(dir)
				if err := ps.Start(); err != nil {
					b.Fatal(err)
				}
				ps.Wait()
				ps.OutputCurrentState(io.Discard)
			}
			b.ReportMetric(float64(numFiles), "files/op")
		})
	}
}

// BenchmarkGeneratorMixed benchmarks with realistic mixed file sizes.
func BenchmarkGeneratorMixed(b *testing.B) {
	for _, numFiles := range []int{100, 1000} {
		b.Run(fmt.Sprintf("files=%d", numFiles), func(b *testing.B) {
			dir := createBenchTreeMixed(b, numFiles)
			g := NewGenerator()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m, err := g.GenerateFromPath(dir)
				if err != nil {
					b.Fatal(err)
				}
				_ = m
			}
			b.ReportMetric(float64(numFiles), "files/op")
		})
	}
}

// BenchmarkProgressiveMixed benchmarks progressive scanner with mixed sizes.
func BenchmarkProgressiveMixed(b *testing.B) {
	for _, numFiles := range []int{100, 1000} {
		b.Run(fmt.Sprintf("files=%d", numFiles), func(b *testing.B) {
			dir := createBenchTreeMixed(b, numFiles)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ps := NewProgressiveScanner(dir)
				if err := ps.Start(); err != nil {
					b.Fatal(err)
				}
				ps.Wait()
				ps.OutputCurrentState(io.Discard)
			}
			b.ReportMetric(float64(numFiles), "files/op")
		})
	}
}
