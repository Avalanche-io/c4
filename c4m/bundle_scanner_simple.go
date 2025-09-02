package c4m

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	
	"github.com/Avalanche-io/c4"
)

// SimpleBundleScanner implements directory-aware chunking with count tracking
// Based on the algorithm: track counts, flush complete directories when possible
type SimpleBundleScanner struct {
	bundle  *Bundle
	scan    *BundleScan
	config  *BundleConfig
	
	// Current accumulation
	currentManifest *Manifest
	currentSize     int64
	currentEntries  int
	
	// Directory tracking (populated in phase 1)
	directoryCounts map[string]int    // Path -> entry count (including subdirs)
	directorySizes  map[string]int64  // Path -> total size
	
	// Statistics
	chunksWritten  int
	totalEntries   int
	incompleteDir  string // Track if we're continuing an incomplete directory
}

// NewSimpleBundleScanner creates a scanner with directory-aware chunking
func NewSimpleBundleScanner(bundle *Bundle, scan *BundleScan, config *BundleConfig) *SimpleBundleScanner {
	if config == nil {
		config = DefaultBundleConfig()
	}
	
	return &SimpleBundleScanner{
		bundle:          bundle,
		scan:            scan,
		config:          config,
		currentManifest: NewManifest(),
		directoryCounts: make(map[string]int),
		directorySizes:  make(map[string]int64),
	}
}

// Phase 1: Count all directories to make informed decisions
func (sbs *SimpleBundleScanner) countDirectory(path string) (entries int, size int64, err error) {
	entries = 1 // Count the directory itself
	
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return 0, 0, err
	}
	
	// Process files
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			entries++
			if info, err := entry.Info(); err == nil {
				size += info.Size()
			}
		}
	}
	
	// Process subdirectories recursively
	for _, entry := range dirEntries {
		if entry.IsDir() {
			subPath := filepath.Join(path, entry.Name())
			subEntries, subSize, err := sbs.countDirectory(subPath)
			if err != nil {
				// Skip directories we can't read
				continue
			}
			entries += subEntries
			size += subSize
		}
	}
	
	// Store the counts
	sbs.directoryCounts[path] = entries
	sbs.directorySizes[path] = size
	
	return entries, size, nil
}

// shouldSeparateDirectory decides if a directory should be its own chunk
func (sbs *SimpleBundleScanner) shouldSeparateDirectory(path string) bool {
	count := sbs.directoryCounts[path]
	
	// If directory has more than 70% of chunk capacity, separate it
	threshold := int(float64(sbs.config.MaxEntriesPerChunk) * 0.7)
	return count > threshold
}

// canFitDirectory checks if directory can fit in current chunk
func (sbs *SimpleBundleScanner) canFitDirectory(path string) bool {
	count := sbs.directoryCounts[path]
	projectedEntries := sbs.currentEntries + count
	
	// Allow up to 125% overage for finding good boundaries
	maxWithOverage := int(float64(sbs.config.MaxEntriesPerChunk) * 1.25)
	return projectedEntries <= maxWithOverage
}

// writeChunk writes the current manifest as a chunk
func (sbs *SimpleBundleScanner) writeChunk(reason string) error {
	if len(sbs.currentManifest.Entries) == 0 {
		return nil
	}
	
	// Determine if we need @base (continuing an incomplete directory)
	includeBase := sbs.chunksWritten > 0 && sbs.incompleteDir != ""
	
	if err := sbs.bundle.AddProgressChunkWithBase(sbs.scan, sbs.currentManifest, includeBase); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}
	
	TimedPrintf("✓ Wrote chunk %d (%d entries, reason: %s)\n", 
		sbs.chunksWritten+1, sbs.currentEntries, reason)
	
	sbs.chunksWritten++
	
	// Reset for next chunk
	sbs.currentManifest = NewManifest()
	sbs.currentEntries = 0
	sbs.currentSize = 0
	
	return nil
}

// ScanPath performs directory-aware scanning with proper chunking
func (sbs *SimpleBundleScanner) ScanPath(scanPath string) error {
	TimedPrintln("Phase 1: Counting directories...")
	
	// Phase 1: Count all directories
	totalEntries, totalSize, err := sbs.countDirectory(scanPath)
	if err != nil {
		return fmt.Errorf("failed to count directories: %w", err)
	}
	
	TimedPrintf("Found %d entries, %d MB total\n", totalEntries, totalSize/(1024*1024))
	TimedPrintln("Phase 2: Scanning with directory-aware chunking...")
	
	// Phase 2: Stream with intelligent chunking
	return sbs.scanDirectory(scanPath, 0, nil)
}

// scanDirectory recursively scans with directory-aware chunking
func (sbs *SimpleBundleScanner) scanDirectory(path string, depth int, parentManifest *Manifest) error {
	// Check if this directory should be separated
	if depth > 0 && sbs.shouldSeparateDirectory(path) {
		// Only write current chunk if it has meaningful content
		// (more than just a parent directory entry)
		if sbs.currentEntries > 10 { // Avoid tiny chunks
			if err := sbs.writeChunk("before large directory"); err != nil {
				return err
			}
			sbs.incompleteDir = ""
		}
		
		// Scan this large directory as its own chunk(s)
		return sbs.scanLargeDirectory(path, depth)
	}
	
	// Check if this directory fits in current chunk
	if !sbs.canFitDirectory(path) {
		// Write current chunk
		if err := sbs.writeChunk("chunk full"); err != nil {
			return err
		}
		// Mark that we're starting fresh (no incomplete directory)
		sbs.incompleteDir = ""
	}
	
	// Add directory entry
	dirInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	dirEntry := &Entry{
		Mode:      dirInfo.Mode(),
		Timestamp: dirInfo.ModTime(),
		Size:      sbs.directorySizes[path],
		Name:      filepath.Base(path),
		Depth:     depth,
	}
	
	sbs.currentManifest.Entries = append(sbs.currentManifest.Entries, dirEntry)
	sbs.currentEntries++
	sbs.totalEntries++
	
	// Read directory contents
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	
	// Process files first (maintaining sort order)
	for _, entry := range entries {
		if !entry.IsDir() {
			fullPath := filepath.Join(path, entry.Name())
			if err := sbs.scanFile(fullPath, depth+1); err != nil {
				// Skip files we can't read
				continue
			}
		}
	}
	
	// Process subdirectories
	for _, entry := range entries {
		if entry.IsDir() {
			fullPath := filepath.Join(path, entry.Name())
			if err := sbs.scanDirectory(fullPath, depth+1, sbs.currentManifest); err != nil {
				// Skip directories we can't process
				continue
			}
		}
	}
	
	// Calculate directory C4 ID from its contents
	if sbs.directoryCounts[path] > 1 {
		dirEntry.C4ID = sbs.calculateDirectoryID(path, dirEntry, sbs.currentManifest)
	}
	
	return nil
}

// scanLargeDirectory handles directories that need their own chunk(s)
func (sbs *SimpleBundleScanner) scanLargeDirectory(path string, depth int) error {
	TimedPrintf("Large directory %s (%d entries) gets separate chunk(s)\n", 
		filepath.Base(path), sbs.directoryCounts[path])
	
	// This directory gets its own chunk series
	// Track that we might need @base for continuation
	sbs.incompleteDir = path
	
	// Create fresh manifest for this directory
	sbs.currentManifest = NewManifest()
	sbs.currentEntries = 0
	sbs.currentSize = 0
	
	// Add directory entry
	dirInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	dirEntry := &Entry{
		Mode:      dirInfo.Mode(),
		Timestamp: dirInfo.ModTime(),
		Size:      sbs.directorySizes[path],
		Name:      filepath.Base(path),
		Depth:     depth,
	}
	
	sbs.currentManifest.Entries = append(sbs.currentManifest.Entries, dirEntry)
	sbs.currentEntries++
	sbs.totalEntries++
	
	// Read entries
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	
	// Process files, checking for chunk boundaries
	for _, entry := range entries {
		if !entry.IsDir() {
			// Check if we need to chunk
			if sbs.currentEntries >= sbs.config.MaxEntriesPerChunk {
				if err := sbs.writeChunk("large dir continuation"); err != nil {
					return err
				}
				// Continue with same incomplete directory
			}
			
			fullPath := filepath.Join(path, entry.Name())
			if err := sbs.scanFile(fullPath, depth+1); err != nil {
				continue
			}
		}
	}
	
	// Process subdirectories
	for _, entry := range entries {
		if entry.IsDir() {
			fullPath := filepath.Join(path, entry.Name())
			// Subdirectories might trigger their own separations
			if err := sbs.scanDirectory(fullPath, depth+1, sbs.currentManifest); err != nil {
				continue
			}
		}
	}
	
	// Calculate directory ID
	if sbs.directoryCounts[path] > 1 {
		dirEntry.C4ID = sbs.calculateDirectoryID(path, dirEntry, sbs.currentManifest)
	}
	
	// Write final chunk for this directory
	if err := sbs.writeChunk("large dir complete"); err != nil {
		return err
	}
	sbs.incompleteDir = ""
	
	return nil
}

// scanFile adds a file entry
func (sbs *SimpleBundleScanner) scanFile(path string, depth int) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	entry := &Entry{
		Mode:      fileInfo.Mode(),
		Timestamp: fileInfo.ModTime(),
		Size:      fileInfo.Size(),
		Name:      fileInfo.Name(),
		Depth:     depth,
	}
	
	// Calculate C4 ID for regular files
	if fileInfo.Mode().IsRegular() {
		file, err := os.Open(path)
		if err == nil {
			defer file.Close()
			entry.C4ID = c4.Identify(file)
		}
	}
	
	sbs.currentManifest.Entries = append(sbs.currentManifest.Entries, entry)
	sbs.currentEntries++
	sbs.currentSize += fileInfo.Size()
	sbs.totalEntries++
	
	return nil
}

// calculateDirectoryID computes C4 ID for a directory from its contents
func (sbs *SimpleBundleScanner) calculateDirectoryID(path string, dirEntry *Entry, manifest *Manifest) c4.ID {
	// Build a sub-manifest for this directory
	var content strings.Builder
	content.WriteString("@c4m 1.0\n")
	
	// Find all entries that belong to this directory
	startDepth := dirEntry.Depth
	started := false
	
	for _, entry := range manifest.Entries {
		// Skip until we find our directory
		if !started {
			if entry == dirEntry {
				started = true
			}
			continue
		}
		
		// Stop when we reach an entry at same or shallower depth
		if entry.Depth <= startDepth {
			break
		}
		
		// Include entries that are direct children
		if entry.Depth == startDepth+1 {
			relativeEntry := *entry
			relativeEntry.Depth = 0
			content.WriteString(relativeEntry.Canonical())
			content.WriteString("\n")
		}
	}
	
	return c4.Identify(strings.NewReader(content.String()))
}

// Complete finishes the scan
func (sbs *SimpleBundleScanner) Complete() error {
	// Write any remaining entries
	if sbs.currentEntries > 0 {
		if err := sbs.writeChunk("final"); err != nil {
			return err
		}
	}
	
	// Mark scan as complete
	return sbs.bundle.CompleteScan(sbs.scan)
}

// GetStatistics returns scan statistics
func (sbs *SimpleBundleScanner) GetStatistics() map[string]interface{} {
	avgEntries := 0
	if sbs.chunksWritten > 0 {
		avgEntries = sbs.totalEntries / sbs.chunksWritten
	}
	
	return map[string]interface{}{
		"total_entries":   sbs.totalEntries,
		"chunks_written":  sbs.chunksWritten,
		"avg_entries":     avgEntries,
		"directory_count": len(sbs.directoryCounts),
	}
}