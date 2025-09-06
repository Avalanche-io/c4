package c4m

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	
	// Continuation state for regular chunking (Scenario 2)
	// When we chunk in the middle of scanning, we need to preserve parent context
	continuationPath  string   // Path we're continuing from
	continuationDepth int      // Depth we're continuing at
	continuationChain []*Entry // Parent directories to restate
	
	// Collapsed directory state (Scenario 1)
	// When a large directory gets its own isolated chunk series
	collapsedDir     string // Path of the collapsed directory (empty if not in collapsed mode)
	collapsedBaseID  c4.ID  // Base ID for @base directive in collapsed chunks
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
		sbs.currentEntries = 0
		return nil
	}
	
	// Never use @base for regular chunking
	// Regular chunks always start fresh from depth 0
	if err := sbs.bundle.AddProgressChunkWithBase(sbs.scan, sbs.currentManifest, false); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}
	
	if sbs.currentEntries > 0 {
		TimedPrintf("✓ Wrote chunk %d (%d entries, reason: %s)\n", 
			sbs.chunksWritten+1, sbs.currentEntries, reason)
		sbs.chunksWritten++
	}
	
	// Reset for next chunk
	sbs.currentManifest = NewManifest()
	sbs.currentEntries = 0
	sbs.currentSize = 0
	
	return nil
}

// writeCollapsedChunk writes a chunk for a collapsed directory (Scenario 1)
func (sbs *SimpleBundleScanner) writeCollapsedChunk(reason string) error {
	if len(sbs.currentManifest.Entries) == 0 {
		sbs.currentEntries = 0
		return nil
	}
	
	// For collapsed directory chunks, use @base after the first chunk
	// The first chunk starts the collapsed series, subsequent chunks continue it
	includeBase := sbs.collapsedBaseID != c4.ID{}
	
	if err := sbs.bundle.AddProgressChunkWithBase(sbs.scan, sbs.currentManifest, includeBase); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}
	
	if sbs.currentEntries > 0 {
		TimedPrintf("✓ Wrote collapsed chunk %d (%d entries, reason: %s)\n", 
			sbs.chunksWritten+1, sbs.currentEntries, reason)
		sbs.chunksWritten++
	}
	
	// After writing first chunk of collapsed dir, track its ID for @base
	if !includeBase && sbs.collapsedDir != "" {
		sbs.collapsedBaseID = sbs.scan.LastChunkID
	}
	
	// Reset for next chunk
	sbs.currentManifest = NewManifest()
	sbs.currentEntries = 0
	sbs.currentSize = 0
	
	return nil
}

// writeContinuationChunk writes a continuation chunk (Scenario 2)
func (sbs *SimpleBundleScanner) writeContinuationChunk(reason string) error {
	if len(sbs.currentManifest.Entries) == 0 {
		sbs.currentEntries = 0
		return nil
	}
	
	// Write the chunk WITH @base directive
	if err := sbs.bundle.AddProgressChunkWithBase(sbs.scan, sbs.currentManifest, true); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}
	
	if sbs.currentEntries > 0 {
		TimedPrintf("✓ Wrote continuation chunk %d (%d entries, reason: %s)\n", 
			sbs.chunksWritten+1, sbs.currentEntries, reason)
		sbs.chunksWritten++
	}
	
	// Reset for next chunk
	sbs.currentManifest = NewManifest()
	sbs.currentEntries = 0
	sbs.currentSize = 0
	
	// Restate parent context for continuation
	for _, parentEntry := range sbs.continuationChain {
		sbs.currentManifest.Entries = append(sbs.currentManifest.Entries, parentEntry)
		sbs.currentEntries++
	}
	
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
	sbs.scan.Path = scanPath // Store the scan path for reference
	return sbs.scanDirectoryInternal(scanPath, 0, true)
}

// scanDirectory is the public interface that handles non-root directories
func (sbs *SimpleBundleScanner) scanDirectory(path string, depth int) error {
	return sbs.scanDirectoryInternal(path, depth, false)
}

// scanDirectoryInternal does the actual scanning work
func (sbs *SimpleBundleScanner) scanDirectoryInternal(path string, depth int, isRoot bool) error {
	// Check if we're in continuation mode and need to handle chunking
	if sbs.continuationPath != "" && !sbs.canFitDirectory(path) {
		// We're continuing from a previous chunk and this directory won't fit
		// Write a continuation chunk with parent context preserved
		if err := sbs.writeContinuationChunk("directory won't fit"); err != nil {
			return err
		}
	}
	
	// Note: We no longer check for collapsed directories here
	// They are handled after processing files to maintain proper ordering
	
	// Check if this directory fits in current chunk
	if !sbs.canFitDirectory(path) {
		// Need to chunk before this directory
		if sbs.continuationPath != "" {
			// Already in continuation mode
			if err := sbs.writeContinuationChunk("chunk full"); err != nil {
				return err
			}
		} else if sbs.currentEntries > 0 {
			// Start continuation mode - need to preserve context
			sbs.setupContinuation(path)
			if err := sbs.writeContinuationChunk("starting continuation"); err != nil {
				return err
			}
		}
	}
	
	// Add directory entry (but NOT if this is the root directory being scanned)
	var dirEntry *Entry
	if !isRoot {
		dirInfo, err := os.Stat(path)
		if err != nil {
			return err
		}
		
		dirEntry = &Entry{
			Mode:      dirInfo.Mode(),
			Timestamp: dirInfo.ModTime(),
			Size:      sbs.directorySizes[path],
			Name:      filepath.Base(path) + "/",
			Depth:     depth,
		}
		
		sbs.currentManifest.Entries = append(sbs.currentManifest.Entries, dirEntry)
		sbs.currentEntries++
		sbs.totalEntries++
	}
	
	// Read directory contents
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	
	// Separate files and directories
	var files []os.DirEntry
	var dirs []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}
	
	// Sort files by name (natural sort)
	sort.Slice(files, func(i, j int) bool {
		return NaturalLess(files[i].Name(), files[j].Name())
	})
	
	// Sort directories by name (natural sort)
	sort.Slice(dirs, func(i, j int) bool {
		return NaturalLess(dirs[i].Name(), dirs[j].Name())
	})
	
	// Process files first
	for _, entry := range files {
		fullPath := filepath.Join(path, entry.Name())
		if err := sbs.scanFile(fullPath, depth); err != nil {
			// Skip files we can't read
			continue
		}
		
		// Check if we need to chunk after adding each file
		if sbs.currentEntries >= sbs.config.MaxEntriesPerChunk {
			if sbs.collapsedDir != "" {
				// We're in a collapsed directory, use collapsed chunking
				if err := sbs.writeCollapsedChunk("collapsed chunk full"); err != nil {
					return err
				}
			} else if sbs.continuationPath == "" {
				// Not in continuation mode yet - need to set it up
				sbs.setupContinuation(path)
				if err := sbs.writeContinuationChunk("chunk full mid-scan"); err != nil {
					return err
				}
			} else {
				// Already in continuation mode
				if err := sbs.writeContinuationChunk("continuation chunk full"); err != nil {
					return err
				}
			}
		}
	}
	
	// Process subdirectories AFTER all files
	// We need to handle collapsed directories specially to maintain files-before-dirs ordering
	var regularDirs []os.DirEntry
	var collapsedDirEntries []*Entry
	
	for _, entry := range dirs {
		fullPath := filepath.Join(path, entry.Name())
		
		// Check if this should be a collapsed directory
		if sbs.shouldSeparateDirectory(fullPath) {
			// This will be a collapsed directory - prepare its entry but don't scan yet
			dirInfo, _ := os.Stat(fullPath)
			collapsedEntry := &Entry{
				Mode:      dirInfo.Mode(),
				Timestamp: dirInfo.ModTime(),
				Size:      sbs.directorySizes[fullPath],
				Name:      entry.Name() + "/",
				Depth:     depth,
				// C4ID will be set when we actually scan it
			}
			collapsedDirEntries = append(collapsedDirEntries, collapsedEntry)
		} else {
			// Regular directory for recursive scanning
			regularDirs = append(regularDirs, entry)
		}
	}
	
	// First process regular directories (they add their entries when scanned)
	for _, entry := range regularDirs {
		fullPath := filepath.Join(path, entry.Name())
		if err := sbs.scanDirectoryInternal(fullPath, depth+1, false); err != nil {
			// Skip directories we can't process
			continue
		}
	}
	
	// Then handle collapsed directories (add entry then scan)
	for i, entry := range collapsedDirEntries {
		fullPath := filepath.Join(path, strings.TrimSuffix(entry.Name, "/"))
		
		// Write current chunk if needed before starting collapsed directory
		if sbs.currentEntries > 10 {
			if sbs.continuationPath != "" {
				if err := sbs.writeContinuationChunk("before collapsed dir"); err != nil {
					return err
				}
			} else {
				if err := sbs.writeChunk("before collapsed dir"); err != nil {
					return err
				}
			}
			// Clear continuation state
			sbs.continuationPath = ""
			sbs.continuationChain = nil
		}
		
		// Scan the collapsed directory to get its C4 ID
		largeC4ID, err := sbs.scanCollapsedDirectory(fullPath, depth+1)
		if err != nil {
			continue
		}
		
		// Update the entry with the C4 ID and add to manifest
		collapsedDirEntries[i].C4ID = largeC4ID
		sbs.currentManifest.Entries = append(sbs.currentManifest.Entries, collapsedDirEntries[i])
		sbs.currentEntries++
		sbs.totalEntries++
	}
	
	// Calculate directory C4 ID from its contents (only for non-root)
	if !isRoot && dirEntry != nil && sbs.directoryCounts[path] > 1 {
		dirEntry.C4ID = sbs.calculateDirectoryID(path, dirEntry, sbs.currentManifest)
	}
	
	return nil
}

// setupContinuation sets up the continuation state when we need to chunk
func (sbs *SimpleBundleScanner) setupContinuation(currentPath string) {
	// Build the parent chain from scan root to current position
	sbs.continuationChain = make([]*Entry, 0)
	sbs.continuationPath = currentPath
	
	relPath, _ := filepath.Rel(sbs.scan.Path, currentPath)
	if relPath == "." || relPath == "" {
		sbs.continuationDepth = 0
		return
	}
	
	parts := strings.Split(relPath, string(filepath.Separator))
	currentBuildPath := sbs.scan.Path
	
	// Build parent chain (not including the current directory)
	for i := 0; i < len(parts); i++ {
		currentBuildPath = filepath.Join(currentBuildPath, parts[i])
		info, _ := os.Stat(currentBuildPath)
		parentEntry := &Entry{
			Mode:      info.Mode(),
			Timestamp: info.ModTime(),
			Size:      sbs.directorySizes[currentBuildPath],
			Name:      parts[i] + "/",
			Depth:     i,
		}
		sbs.continuationChain = append(sbs.continuationChain, parentEntry)
	}
	sbs.continuationDepth = len(parts)
}

// scanCollapsedDirectory handles directories that get their own isolated chunk series (Scenario 1)
// The directory itself does NOT appear in its chunks - they start at depth 0
// Returns the C4 ID of the last chunk which becomes the directory's ID
func (sbs *SimpleBundleScanner) scanCollapsedDirectory(path string, originalDepth int) (c4.ID, error) {
	TimedPrintf("Collapsed directory %s (%d entries) gets isolated chunk(s)\n", 
		filepath.Base(path), sbs.directoryCounts[path])
	
	// Mark that we're in collapsed directory mode
	sbs.collapsedDir = path
	sbs.collapsedBaseID = c4.ID{} // Will be set after first chunk
	
	// Save current state
	parentManifest := sbs.currentManifest
	parentEntries := sbs.currentEntries
	parentSize := sbs.currentSize
	parentContinuation := sbs.continuationPath
	parentChain := sbs.continuationChain
	
	// Create fresh state for collapsed directory
	// This is an isolated scan starting at depth 0
	sbs.currentManifest = NewManifest()
	sbs.currentEntries = 0
	sbs.currentSize = 0
	sbs.continuationPath = ""
	sbs.continuationChain = nil
	
	// The directory itself doesn't appear in any of its chunks
	// We're doing an isolated scan of its contents
	
	// Read entries
	entries, err := os.ReadDir(path)
	if err != nil {
		return c4.ID{}, err
	}
	
	// Separate and sort entries
	var files []os.DirEntry
	var dirs []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}
	
	// Sort files by name (natural sort)
	sort.Slice(files, func(i, j int) bool {
		return NaturalLess(files[i].Name(), files[j].Name())
	})
	
	// Sort directories by name (natural sort)
	sort.Slice(dirs, func(i, j int) bool {
		return NaturalLess(dirs[i].Name(), dirs[j].Name())
	})
	
	var lastChunkID c4.ID
	
	// Process files first - at depth 0 since this is an isolated scan
	for _, entry := range files {
		fullPath := filepath.Join(path, entry.Name())
		if err := sbs.scanFile(fullPath, 0); err != nil {
			continue
		}
		
		// Check if we need to chunk
		if sbs.currentEntries >= sbs.config.MaxEntriesPerChunk {
			if err := sbs.writeCollapsedChunk("collapsed dir continuation"); err != nil {
				return c4.ID{}, err
			}
		}
	}
	
	// Process subdirectories - at depth 0 since this is an isolated scan  
	for _, entry := range dirs {
		fullPath := filepath.Join(path, entry.Name())
		// Recursively scan subdirectories at depth 0 in collapsed context
		if err := sbs.scanDirectoryInternal(fullPath, 0, false); err != nil {
			continue
		}
	}
	
	// Write final chunk if we have entries
	if sbs.currentEntries > 0 {
		if err := sbs.writeCollapsedChunk("collapsed dir complete"); err != nil {
			return c4.ID{}, err
		}
	}
	
	// Get the C4 ID of the last chunk we wrote (whether we just wrote it or not)
	if len(sbs.scan.ProgressChunks) > 0 {
		lastChunkID = sbs.scan.LastChunkID
	} else {
		// If no chunks were written, the directory is empty
		lastChunkID = c4.ID{}
	}
	
	// Clear collapsed directory state
	sbs.collapsedDir = ""
	sbs.collapsedBaseID = c4.ID{}
	
	// Restore parent context
	sbs.currentManifest = parentManifest
	sbs.currentEntries = parentEntries
	sbs.currentSize = parentSize
	sbs.continuationPath = parentContinuation
	sbs.continuationChain = parentChain
	
	// Return the C4 ID of the last chunk (the collapsed directory's ID)
	return lastChunkID, nil
}

// scanFile adds a file entry (or directory via symlink)
func (sbs *SimpleBundleScanner) scanFile(path string, depth int) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	// Handle the case where a symlink points to a directory
	name := fileInfo.Name()
	if fileInfo.IsDir() {
		name = name + "/"
	}
	
	entry := &Entry{
		Mode:      fileInfo.Mode(),
		Timestamp: fileInfo.ModTime(),
		Size:      fileInfo.Size(),
		Name:      name,
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