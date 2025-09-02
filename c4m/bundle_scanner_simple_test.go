package c4m

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestSimpleScanner validates the directory-aware chunking algorithm
func TestSimpleScanner(t *testing.T) {
	tests := []struct {
		name            string
		setupFunc       func(string) error
		expectedChunks  []int   // Acceptable range of chunks
		minUtilization float64 // Minimum average chunk utilization
	}{
		{
			name: "small_100_files",
			setupFunc: func(dir string) error {
				// Create 100 files in root
				for i := 0; i < 100; i++ {
					path := filepath.Join(dir, fmt.Sprintf("file%03d.txt", i))
					if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
						return err
					}
				}
				return nil
			},
			expectedChunks: []int{1}, // Should be 1 chunk
			minUtilization: 0.0, // No minimum for small directories
		},
		{
			name: "medium_10k_files",
			setupFunc: func(dir string) error {
				// Create 10,000 files across subdirs
				for d := 0; d < 10; d++ {
					subdir := filepath.Join(dir, fmt.Sprintf("dir%d", d))
					os.MkdirAll(subdir, 0755)
					for i := 0; i < 1000; i++ {
						path := filepath.Join(subdir, fmt.Sprintf("file%04d.txt", i))
						if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
							return err
						}
					}
				}
				return nil
			},
			expectedChunks: []int{1}, // Should fit in 1 chunk (10k < 100k)
			minUtilization: 0.1,      // At least 10% utilization
		},
		{
			name: "large_1m_files",
			setupFunc: func(dir string) error {
				// Create 1M files (way over chunk size)
				// Use smaller test for speed: 200k files
				for d := 0; d < 200; d++ {
					subdir := filepath.Join(dir, fmt.Sprintf("dir%03d", d))
					os.MkdirAll(subdir, 0755)
					for i := 0; i < 1000; i++ {
						path := filepath.Join(subdir, fmt.Sprintf("file%04d.txt", i))
						if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
							return err
						}
					}
				}
				return nil
			},
			expectedChunks: []int{2, 3}, // 200k entries → 2-3 chunks
			minUtilization: 0.7,         // Should have good utilization
		},
		{
			name: "large_single_dir",
			setupFunc: func(dir string) error {
				// Single directory with 80k files (>70% threshold)
				for i := 0; i < 80000; i++ {
					path := filepath.Join(dir, fmt.Sprintf("file%06d.txt", i))
					if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
						return err
					}
				}
				return nil
			},
			expectedChunks: []int{1}, // Should be 1 chunk (80k < 100k)
			minUtilization: 0.8,      // High utilization
		},
		{
			name: "mixed_hierarchy",
			setupFunc: func(dir string) error {
				// Large shallow dir (should separate)
				largeDir := filepath.Join(dir, "large")
				os.MkdirAll(largeDir, 0755)
				for i := 0; i < 75000; i++ {
					path := filepath.Join(largeDir, fmt.Sprintf("file%05d.txt", i))
					os.WriteFile(path, []byte("data"), 0644)
				}
				
				// Small dirs (should stay inline)
				for d := 0; d < 5; d++ {
					smallDir := filepath.Join(dir, fmt.Sprintf("small%d", d))
					os.MkdirAll(smallDir, 0755)
					for i := 0; i < 100; i++ {
						path := filepath.Join(smallDir, fmt.Sprintf("file%03d.txt", i))
						os.WriteFile(path, []byte("tiny"), 0644)
					}
				}
				
				return nil
			},
			expectedChunks: []int{1, 2}, // Large dir might trigger separation
			minUtilization: 0.35, // Lower because of mixed sizes
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test directory
			testDir, err := os.MkdirTemp("", "simple_test_*")
			if err != nil {
				t.Fatalf("Failed to create test dir: %v", err)
			}
			defer os.RemoveAll(testDir)
			
			// Setup test files
			if err := tt.setupFunc(testDir); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			
			// Create bundle with dev config for smaller chunks in tests
			config := DevBundleConfig()
			config.MaxEntriesPerChunk = 100000 // Use standard size for tests
			config.BundleDir = testDir
			
			bundle, err := CreateBundle(testDir, config)
			if err != nil {
				t.Fatalf("Failed to create bundle: %v", err)
			}
			
			// Start scan
			scan, err := bundle.NewScan(testDir)
			if err != nil {
				t.Fatalf("Failed to create scan: %v", err)
			}
			
			// Create simple scanner
			scanner := NewSimpleBundleScanner(bundle, scan, config)
			
			// Run scan
			if err := scanner.ScanPath(testDir); err != nil {
				t.Fatalf("Scan failed: %v", err)
			}
			
			// Complete scan
			if err := scanner.Complete(); err != nil {
				t.Fatalf("Failed to complete scan: %v", err)
			}
			
			// Get statistics
			stats := scanner.GetStatistics()
			
			chunksWritten := stats["chunks_written"].(int)
			totalEntries := stats["total_entries"].(int)
			avgEntries := stats["avg_entries"].(int)
			dirCount := stats["directory_count"].(int)
			
			// Validate chunk count
			validChunkCount := false
			for _, expected := range tt.expectedChunks {
				if chunksWritten == expected {
					validChunkCount = true
					break
				}
			}
			if !validChunkCount {
				t.Errorf("Expected chunks %v, got %d", tt.expectedChunks, chunksWritten)
			}
			
			// Validate utilization
			if chunksWritten > 0 && tt.minUtilization > 0 {
				utilization := float64(avgEntries) / float64(config.MaxEntriesPerChunk)
				if utilization < tt.minUtilization {
					t.Errorf("Expected min utilization %.2f, got %.2f (avg %d entries)",
						tt.minUtilization, utilization, avgEntries)
				}
			}
			
			// Log results
			t.Logf("✓ Results: %d entries in %d chunks (avg %d/chunk, %.1f%% utilization)",
				totalEntries, chunksWritten, avgEntries,
				float64(avgEntries)/float64(config.MaxEntriesPerChunk)*100)
			t.Logf("  Directories analyzed: %d", dirCount)
			
			// Success criteria validation
			if chunksWritten > 0 {
				// Check for reasonable chunk sizes (not tiny)
				if avgEntries < 10 && totalEntries > 100 {
					t.Errorf("❌ Chunks too small: avg %d entries", avgEntries)
				}
				
				// Check for not massive chunks
				if avgEntries > int(float64(config.MaxEntriesPerChunk)*1.25) {
					t.Errorf("❌ Chunks too large: avg %d entries (>125%% of max)", avgEntries)
				}
			}
		})
	}
}

// TestChunkingDecisions validates the chunking decision logic
func TestChunkingDecisions(t *testing.T) {
	config := &BundleConfig{
		MaxEntriesPerChunk: 100000,
	}
	
	scanner := &SimpleBundleScanner{
		config: config,
		directoryCounts: map[string]int{
			"/small":  1000,   // 1% - should stay inline
			"/medium": 50000,  // 50% - should stay inline
			"/large":  75000,  // 75% - should separate
			"/huge":   150000, // 150% - definitely separate
		},
	}
	
	tests := []struct {
		path         string
		shouldSep    bool
		description  string
	}{
		{"/small", false, "Small dir stays inline"},
		{"/medium", false, "Medium dir stays inline"},
		{"/large", true, "Large dir (>70%) separates"},
		{"/huge", true, "Huge dir definitely separates"},
	}
	
	for _, tt := range tests {
		result := scanner.shouldSeparateDirectory(tt.path)
		if result != tt.shouldSep {
			t.Errorf("%s: expected %v, got %v", tt.description, tt.shouldSep, result)
		}
	}
}