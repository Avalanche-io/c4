package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// Scanner is a simplified, correct implementation of the bundle scanner
type BundleScannerImpl struct {
	bundle  *Bundle
	scan    *BundleScan
	config  *BundleConfig
	
	// Directory statistics (populated in phase 1)
	dirCounts map[string]int   // Path -> total entry count
	dirSizes  map[string]int64 // Path -> total size
	
	// Statistics
	totalEntries  int
	chunksWritten int
	
	// Fast mode flag - skip C4 ID computation for structural testing
	SkipC4IDs bool
}

// NewScanner creates a new simplified scanner
func NewBundleScannerImpl(bundle *Bundle, scan *BundleScan, config *BundleConfig) *BundleScannerImpl {
	if config == nil {
		config = DefaultBundleConfig()
	}
	
	return &BundleScannerImpl{
		bundle:    bundle,
		scan:      scan,
		config:    config,
		dirCounts: make(map[string]int),
		dirSizes:  make(map[string]int64),
	}
}

// DirectoryPlan represents a plan for processing a directory
type DirectoryPlan struct {
	path          string
	depth         int
	isRoot        bool
	files         []os.DirEntry
	regularDirs   []string      // Paths of regular subdirs
	collapsedDirs []string      // Paths of dirs to collapse
}

// ScanPath is the main entry point
func (s *BundleScannerImpl) ScanPath(scanPath string) error {
	TimedPrintln("Phase 1: Counting directories...")
	
	// First pass: count everything
	if err := s.countDirectory(scanPath); err != nil {
		return fmt.Errorf("failed to count directories: %w", err)
	}
	
	TimedPrintf("Found %d total entries\n", s.dirCounts[scanPath])
	TimedPrintln("Phase 2: Scanning with proper ordering...")
	
	// Second pass: scan with proper ordering
	entries, err := s.scanDirectory(scanPath, 0, true)
	if err != nil {
		return fmt.Errorf("failed to scan: %w", err)
	}
	
	TimedPrintln("Phase 3: Chunking entries...")
	
	// Third pass: chunk the entries
	if err := s.chunkAndWrite(entries, false); err != nil {
		return fmt.Errorf("failed to chunk: %w", err)
	}
	
	return nil
}

// countDirectory counts all entries recursively (phase 1)
func (s *BundleScannerImpl) countDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	
	count := 1 // Directory itself
	var size int64
	
	for _, entry := range entries {
		if entry.IsDir() {
			subPath := filepath.Join(path, entry.Name())
			if err := s.countDirectory(subPath); err == nil {
				count += s.dirCounts[subPath]
				size += s.dirSizes[subPath]
			}
		} else {
			count++
			if info, err := entry.Info(); err == nil {
				size += info.Size()
			}
		}
	}
	
	s.dirCounts[path] = count
	s.dirSizes[path] = size
	return nil
}

// planDirectory creates an execution plan for a directory
func (s *BundleScannerImpl) planDirectory(path string, depth int, isRoot bool) (*DirectoryPlan, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	
	plan := &DirectoryPlan{
		path:   path,
		depth:  depth,
		isRoot: isRoot,
	}
	
	// Separate files and directories
	var dirEntries []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			dirEntries = append(dirEntries, entry)
		} else {
			plan.files = append(plan.files, entry)
		}
	}
	
	// Sort files naturally
	sort.Slice(plan.files, func(i, j int) bool {
		return NaturalLess(plan.files[i].Name(), plan.files[j].Name())
	})
	
	// Sort directories naturally
	sort.Slice(dirEntries, func(i, j int) bool {
		return NaturalLess(dirEntries[i].Name(), dirEntries[j].Name())
	})
	
	// Determine which directories should be collapsed
	// Always check for collapsing, even in fast mode (we need C4 IDs for references)
	threshold := int(float64(s.config.MaxEntriesPerChunk) * 0.7)
	for _, dir := range dirEntries {
		dirPath := filepath.Join(path, dir.Name())
		if s.dirCounts[dirPath] > threshold {
			plan.collapsedDirs = append(plan.collapsedDirs, dirPath)
		} else {
			plan.regularDirs = append(plan.regularDirs, dirPath)
		}
	}
	
	return plan, nil
}

// scanDirectory recursively scans and returns properly ordered entries
func (s *BundleScannerImpl) scanDirectory(path string, depth int, isRoot bool) ([]*Entry, error) {
	var result []*Entry
	
	// Get the plan
	plan, err := s.planDirectory(path, depth, isRoot)
	if err != nil {
		return nil, err
	}
	
	// Add directory entry (unless it's the root)
	if !isRoot {
		dirInfo, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		
		dirEntry := &Entry{
			Mode:      dirInfo.Mode(),
			Timestamp: dirInfo.ModTime(),
			Size:      s.dirSizes[path],
			Name:      filepath.Base(path) + "/",
			Depth:     depth,
		}
		result = append(result, dirEntry)
		s.totalEntries++
	}
	
	// Process files first (maintaining order)
	for _, file := range plan.files {
		filePath := filepath.Join(path, file.Name())
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}
		
		// Determine actual depth for files
		fileDepth := depth
		if !isRoot {
			fileDepth = depth + 1
		}
		
		// Handle symlinks that might point to directories
		linkInfo, err := os.Lstat(filePath)
		if err == nil && linkInfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink
			// Symlinks are always treated as symlinks, not what they point to
			// They should NOT have trailing slashes even if they point to directories
			entry := &Entry{
				Mode:      linkInfo.Mode(),
				Timestamp: linkInfo.ModTime(),
				Size:      0,  // Symlinks have size 0
				Name:      file.Name(),  // No trailing slash for symlinks
				Depth:     fileDepth,
			}
			
			// Optionally add target information
			if target, err := os.Readlink(filePath); err == nil {
				entry.Target = target
			}
			
			result = append(result, entry)
		} else {
			// Regular file
			entry := &Entry{
				Mode:      fileInfo.Mode(),
				Timestamp: fileInfo.ModTime(),
				Size:      fileInfo.Size(),
				Name:      file.Name(),
				Depth:     fileDepth,
			}
			
			// Compute C4 ID for regular files (skip if SkipC4IDs is true)
			if !s.SkipC4IDs {
				fileC4, err := s.computeFileC4(filePath)
				if err == nil {
					entry.C4ID = fileC4
				}
			}
			
			result = append(result, entry)
		}
		s.totalEntries++
	}
	
	// Process all subdirectories in sorted order (both regular and collapsed)
	// First, combine and sort all directories
	allDirs := make([]string, 0, len(plan.regularDirs)+len(plan.collapsedDirs))
	allDirs = append(allDirs, plan.regularDirs...)
	allDirs = append(allDirs, plan.collapsedDirs...)
	
	// Sort all directories naturally by name
	sort.Slice(allDirs, func(i, j int) bool {
		return NaturalLess(filepath.Base(allDirs[i]), filepath.Base(allDirs[j]))
	})
	
	// Determine if directory should be collapsed
	isCollapsed := make(map[string]bool)
	for _, dir := range plan.collapsedDirs {
		isCollapsed[dir] = true
	}
	
	// Process directories in sorted order
	subdirDepth := depth + 1
	if isRoot {
		subdirDepth = depth
	}
	
	for _, dirPath := range allDirs {
		if isCollapsed[dirPath] {
			// Handle collapsed directory
			dirInfo, err := os.Stat(dirPath)
			if err != nil {
				continue
			}
			
			dirEntry := &Entry{
				Mode:      dirInfo.Mode(),
				Timestamp: dirInfo.ModTime(),
				Size:      s.dirSizes[dirPath],
				Name:      filepath.Base(dirPath) + "/",
				Depth:     subdirDepth,
			}
			
			// Scan collapsed directory in isolation
			collapsedID, err := s.scanCollapsedDirectory(dirPath)
			if err != nil {
				continue
			}
			dirEntry.C4ID = collapsedID
			result = append(result, dirEntry)
			s.totalEntries++
		} else {
			// Handle regular directory (recursive scan)
			subEntries, err := s.scanDirectory(dirPath, subdirDepth, false)
			if err != nil {
				continue
			}
			result = append(result, subEntries...)
		}
	}
	
	// Update directory C4 ID if needed
	// Always compute for directories (even in fast mode) as they're needed for references
	if !isRoot && len(result) > 1 {
		// First entry is the directory itself
		result[0].C4ID = s.computeDirectoryC4(result)
	}
	
	return result, nil
}

// scanCollapsedDirectory handles a collapsed directory as an isolated unit
// It performs a separate root scan of the directory and returns the last chunk's C4 ID
func (s *BundleScannerImpl) scanCollapsedDirectory(path string) (c4.ID, error) {
	TimedPrintf("Scanning collapsed directory: %s (%d entries)\n",
		filepath.Base(path), s.dirCounts[path])
	
	// Create a separate scan context - this is like starting a new root scan
	// but reusing the same bundle for unified output
	collapsedScan := &BundleScan{
		Path:      path,
		StartTime: time.Now(),
	}
	
	// Create an independent scanner for this collapsed directory
	// It shares the bundle (for unified output) and cached counts (to avoid re-scanning)
	collapsedScanner := &BundleScannerImpl{
		bundle:    s.bundle,           // Same bundle - unified output
		scan:      collapsedScan,       // New scan context - independent chunks
		config:    s.config,
		dirCounts: s.dirCounts,         // Reuse structure already discovered
		dirSizes:  s.dirSizes,          // No need to recount
		SkipC4IDs: s.SkipC4IDs,         // Inherit the skip flag
	}
	
	// Scan as if this directory is a root (depth 0, isRoot=true)
	// This creates a valid, independent manifest starting at depth 0
	entries, err := collapsedScanner.scanDirectory(path, 0, true)
	if err != nil {
		return c4.ID{}, err
	}
	
	// In fast mode, we can compute the C4 ID without writing chunks
	// This avoids creating chunks that might confuse validation
	if s.SkipC4IDs {
		// Compute directory C4 ID from the entries without writing chunks
		if len(entries) > 0 {
			return collapsedScanner.computeDirectoryC4(entries), nil
		}
		// Fallback: create ID from metadata
		idString := fmt.Sprintf("collapsed:%s:size:%d:count:%d",
			path, s.dirSizes[path], s.dirCounts[path])
		return c4.Identify(strings.NewReader(idString)), nil
	}
	
	// Normal mode: write chunks and return the last chunk ID
	// These chunks form an independent series starting at depth 0
	if err := collapsedScanner.chunkAndWrite(entries, false); err != nil {
		return c4.ID{}, err
	}
	
	// Return the last chunk's ID - this is what the parent directory entry will reference
	return collapsedScan.LastChunkID, nil
}

// chunkAndWrite splits entries into chunks and writes them
func (s *BundleScannerImpl) chunkAndWrite(entries []*Entry, useBase bool) error {
	manifest := NewManifest()
	entryCount := 0
	firstChunk := true
	
	for _, entry := range entries {
		manifest.Entries = append(manifest.Entries, entry)
		entryCount++
		
		// Check if we need to write a chunk
		if entryCount >= s.config.MaxEntriesPerChunk {
			// Write chunk with appropriate @base handling
			if useBase && !firstChunk {
				if err := s.bundle.AddProgressChunkWithBase(s.scan, manifest, true); err != nil {
					return err
				}
			} else {
				if err := s.bundle.AddProgressChunk(s.scan, manifest); err != nil {
					return err
				}
			}
			
			TimedPrintf("Wrote chunk %d (%d entries)\n", s.chunksWritten+1, entryCount)
			s.chunksWritten++
			firstChunk = false
			
			// Reset for next chunk
			manifest = NewManifest()
			entryCount = 0
		}
	}
	
	// Write final chunk if there are remaining entries
	if entryCount > 0 {
		if useBase && !firstChunk {
			if err := s.bundle.AddProgressChunkWithBase(s.scan, manifest, true); err != nil {
				return err
			}
		} else {
			if err := s.bundle.AddProgressChunk(s.scan, manifest); err != nil {
				return err
			}
		}
		
		TimedPrintf("Wrote chunk %d (%d entries)\n", s.chunksWritten+1, entryCount)
		s.chunksWritten++
	}
	
	return nil
}

// computeFileC4 computes the C4 ID of a file
func (s *BundleScannerImpl) computeFileC4(path string) (c4.ID, error) {
	// Return empty ID immediately if skipping C4 IDs
	if s.SkipC4IDs {
		return c4.ID{}, nil
	}
	
	file, err := os.Open(path)
	if err != nil {
		return c4.ID{}, err
	}
	defer file.Close()
	
	return c4.Identify(file), nil
}

// computeDirectoryC4 computes the C4 ID from directory entries
func (s *BundleScannerImpl) computeDirectoryC4(entries []*Entry) c4.ID {
	// Return empty ID immediately if skipping C4 IDs
	if s.SkipC4IDs {
		return c4.ID{}
	}
	
	// Create a virtual manifest of the directory contents
	manifest := NewManifest()
	
	// Skip the first entry (the directory itself) and add all children
	for i := 1; i < len(entries); i++ {
		// Adjust depth to be relative to directory
		entryCopy := *entries[i]
		if entries[0].Depth < entryCopy.Depth {
			entryCopy.Depth = entryCopy.Depth - entries[0].Depth - 1
		}
		manifest.Entries = append(manifest.Entries, &entryCopy)
	}
	
	// Compute C4 of the manifest
	manifestStr := manifest.AllEntriesString()
	return c4.Identify(strings.NewReader(manifestStr))
}

// Complete finalizes the scan
func (s *BundleScannerImpl) Complete() error {
	now := time.Now()
	s.scan.CompletedAt = &now
	
	TimedPrintf("\nScan complete: %d entries in %d chunks\n", 
		s.totalEntries, s.chunksWritten)
	
	// The bundle tracks its own metadata
	return nil
}

// GetStatistics returns scan statistics
func (s *BundleScannerImpl) GetStatistics() map[string]interface{} {
	avgEntries := 0
	if s.chunksWritten > 0 {
		avgEntries = s.totalEntries / s.chunksWritten
	}
	
	return map[string]interface{}{
		"total_entries":  s.totalEntries,
		"chunks_written": s.chunksWritten,
		"avg_entries":    avgEntries,
		"scan_path":      s.scan.Path,
	}
}