package c4m

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	
	"github.com/Avalanche-io/c4"
)

// CompartmentBundleScanner creates bundles using compartmentalized chunking
// Large subdirectories become separate chains, small ones stay inline
type CompartmentBundleScanner struct {
	bundle       *Bundle
	scan         *BundleScan
	config       *BundleConfig
	rootPath     string
	
	// Current chunk state
	currentChunk *Manifest
	chunkEntries int
	chunkBytes   int64
	
	// Synchronization
	mu           sync.Mutex
	chunksWritten int
}

// DirScanResult holds the result of scanning a directory
type DirScanResult struct {
	Manifest    *Manifest  // Full manifest if inline
	FinalID     *c4.ID     // Final chunk ID if separate chain
	EntryCount  int
	TotalSize   int64
	WasSeparate bool       // True if created separate chain
}

// NewCompartmentBundleScanner creates a scanner with compartmentalized chunking
func NewCompartmentBundleScanner(scanPath string, config *BundleConfig) (*CompartmentBundleScanner, error) {
	if config == nil {
		config = DefaultBundleConfig()
	}
	
	// Create bundle
	bundle, err := CreateBundle(scanPath, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create bundle: %w", err)
	}
	
	// Start new scan
	scan, err := bundle.NewScan(scanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create scan: %w", err)
	}
	
	return &CompartmentBundleScanner{
		bundle:        bundle,
		scan:          scan,
		config:        config,
		rootPath:      scanPath,
		currentChunk:  NewManifest(),
	}, nil
}

// Scan performs the filesystem scan with compartmentalized chunking
func (cbs *CompartmentBundleScanner) Scan() error {
	// Always scan the root directory inline first to determine structure
	manifest, entryCount, totalSize, err := cbs.scanDirectoryFlat(cbs.rootPath, 0)
	if err != nil {
		return err
	}
	
	// Set the manifest and counts
	cbs.currentChunk = manifest
	cbs.chunkEntries = entryCount
	cbs.chunkBytes = totalSize
	
	// Write the final chunk(s)
	if cbs.chunkEntries > 0 {
		if err := cbs.flushChunk(true); err != nil {
			return err
		}
	}
	
	// Complete the scan
	if err := cbs.bundle.CompleteScan(cbs.scan); err != nil {
		return err
	}
	
	return nil
}

// estimateDirectorySize quickly estimates if a directory will exceed chunk limits
func (cbs *CompartmentBundleScanner) estimateDirectorySize(dirPath string) (entryCount int, totalSize int64, shouldSeparate bool) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0, 0, false
	}
	
	// Count entries and estimate size
	for _, entry := range entries {
		entryCount++
		if info, err := entry.Info(); err == nil {
			totalSize += info.Size()
			
			// If it's a directory, do a rough estimate (don't recurse deeply)
			if entry.IsDir() {
				// Estimate subdirectory might have many files
				// This is a heuristic - we can refine later
				subPath := filepath.Join(dirPath, entry.Name())
				if subEntries, err := os.ReadDir(subPath); err == nil {
					entryCount += len(subEntries)
				}
			}
		}
	}
	
	// Decide on separation when directory exceeds chunk limits
	shouldSeparate = entryCount > cbs.config.MaxEntriesPerChunk ||
	                totalSize > cbs.config.MaxBytesPerChunk
	
	return entryCount, totalSize, shouldSeparate
}

// scanDirectoryCompartment scans a directory and decides whether to inline or create separate chain
func (cbs *CompartmentBundleScanner) scanDirectoryCompartment(dirPath string, depth int) (*DirScanResult, error) {
	// First, estimate if this directory needs its own chain
	_, _, needsSeparateChain := cbs.estimateDirectorySize(dirPath)
	
	if needsSeparateChain {
		// This directory is large - create separate chain
		return cbs.scanLargeDirectory(dirPath, depth)
	}
	
	// This directory is small - scan inline
	return cbs.scanSmallDirectory(dirPath, depth)
}

// scanLargeDirectory creates a separate chain for a large directory
func (cbs *CompartmentBundleScanner) scanLargeDirectory(dirPath string, depth int) (*DirScanResult, error) {
	// Save current state
	oldChunk := cbs.currentChunk
	oldEntries := cbs.chunkEntries
	oldBytes := cbs.chunkBytes
	
	// Reset for new chain
	cbs.currentChunk = NewManifest()
	cbs.chunkEntries = 0
	cbs.chunkBytes = 0
	
	// Scan the directory with proper chunking
	totalSize, err := cbs.scanLargeDirectoryWithChunking(dirPath, 0)
	if err != nil {
		// Restore state on error
		cbs.currentChunk = oldChunk
		cbs.chunkEntries = oldEntries
		cbs.chunkBytes = oldBytes
		return nil, err
	}
	
	// Flush any remaining entries
	var finalID *c4.ID
	if cbs.chunkEntries > 0 {
		if err := cbs.flushChunkWithBase(true, true); err != nil {
			return nil, err
		}
	}
	
	// Get the final chunk ID
	if len(cbs.scan.ProgressChunks) > 0 {
		lastChunk := cbs.scan.ProgressChunks[len(cbs.scan.ProgressChunks)-1]
		if id, err := c4.Parse(lastChunk); err == nil {
			finalID = &id
		}
	}
	
	// Restore previous state
	cbs.currentChunk = oldChunk
	cbs.chunkEntries = oldEntries
	cbs.chunkBytes = oldBytes
	
	return &DirScanResult{
		FinalID:     finalID,
		EntryCount:  1,  // The directory reference counts as 1 entry
		TotalSize:   totalSize,
		WasSeparate: true,
	}, nil
}

// scanSmallDirectory scans a directory that will be included inline
func (cbs *CompartmentBundleScanner) scanSmallDirectory(dirPath string, depth int) (*DirScanResult, error) {
	manifest, entryCount, totalSize, err := cbs.scanDirectoryFlat(dirPath, depth)
	if err != nil {
		return nil, err
	}
	
	return &DirScanResult{
		Manifest:    manifest,
		EntryCount:  entryCount,
		TotalSize:   totalSize,
		WasSeparate: false,
	}, nil
}

// scanDirectoryFlat scans a directory and all its contents into a manifest
func (cbs *CompartmentBundleScanner) scanDirectoryFlat(dirPath string, depth int) (*Manifest, int, int64, error) {
	manifest := NewManifest()
	var entryCount int
	var totalBytes int64
	
	// Read directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}
	
	// Create directory entry
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return nil, 0, 0, err
	}
	
	dirEntry := &Entry{
		Name:      filepath.Base(dirPath),
		Mode:      dirInfo.Mode(),
		Timestamp: dirInfo.ModTime(),
		Depth:     depth,
	}
	
	// Track subdirectory sizes for proper directory size calculation
	var dirSize int64
	
	// Separate files and directories for proper ordering
	var fileEntries []os.DirEntry
	var dirEntries []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			dirEntries = append(dirEntries, entry)
		} else {
			fileEntries = append(fileEntries, entry)
		}
	}
	
	// Process files first
	for _, entry := range fileEntries {
		entryPath := filepath.Join(dirPath, entry.Name())
		
		// Process file
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		fileEntry := &Entry{
			Name:      entry.Name(),
			Mode:      info.Mode(),
			Size:      info.Size(),
			Timestamp: info.ModTime(),
			Depth:     depth + 1,
		}
		
		// Compute C4 ID if regular file
		if info.Mode().IsRegular() {
			file, err := os.Open(entryPath)
			if err == nil {
				id := c4.Identify(file)
				fileEntry.C4ID = id
				file.Close()
			}
		}
		
		manifest.AddEntry(fileEntry)
		entryCount++
		totalBytes += info.Size()
		dirSize += info.Size()
	}
	
	// Now process directories
	for _, entry := range dirEntries {
		entryPath := filepath.Join(dirPath, entry.Name())
		
		// Check if subdirectory should be separate
		_, _, subNeedsSeparate := cbs.estimateDirectorySize(entryPath)
		
		if subNeedsSeparate {
			// Large subdirectory - create separate chain
			subResult, err := cbs.scanLargeDirectory(entryPath, depth+1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}
			
			// Add reference entry
			subInfo, _ := os.Stat(entryPath)
			subEntry := &Entry{
				Name:      entry.Name(),
				Mode:      subInfo.Mode(),
				Size:      subResult.TotalSize,
				Timestamp: subInfo.ModTime(),
				Depth:     depth + 1,
			}
			// Only add C4ID if we have one
			if subResult.FinalID != nil {
				subEntry.C4ID = *subResult.FinalID
			}
			manifest.AddEntry(subEntry)
			entryCount++
			dirSize += subResult.TotalSize
		} else {
			// Small subdirectory - recurse inline
			subManifest, subCount, subSize, err := cbs.scanDirectoryFlat(entryPath, depth+1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}
			
			// Add all entries from subdirectory (including the dir entry itself)
			for _, subEntry := range subManifest.Entries {
				manifest.AddEntry(subEntry)
			}
			entryCount += subCount
			dirSize += subSize
		}
	}
	
	// Set directory size as sum of children
	dirEntry.Size = dirSize
	
	// Add directory entry at the beginning
	newEntries := []*Entry{dirEntry}
	newEntries = append(newEntries, manifest.Entries...)
	manifest.Entries = newEntries
	entryCount++
	
	return manifest, entryCount, dirSize, nil
}

// flushChunk writes the current chunk to the bundle
func (cbs *CompartmentBundleScanner) flushChunk(isFinal bool) error {
	return cbs.flushChunkWithBase(isFinal, false)
}

// flushChunkWithBase writes the current chunk with optional @base reference
func (cbs *CompartmentBundleScanner) flushChunkWithBase(isFinal bool, includeBase bool) error {
	cbs.mu.Lock()
	if cbs.chunkEntries == 0 {
		cbs.mu.Unlock()
		return nil
	}
	
	chunk := cbs.currentChunk
	
	// Reset state
	cbs.currentChunk = NewManifest()
	prevEntries := cbs.chunkEntries
	cbs.chunkEntries = 0
	cbs.chunkBytes = 0
	cbs.chunksWritten++
	chunkNum := cbs.chunksWritten
	cbs.mu.Unlock()
	
	// Write chunk to bundle
	if err := cbs.bundle.AddProgressChunkWithBase(cbs.scan, chunk, includeBase); err != nil {
		return fmt.Errorf("failed to write chunk %d: %w", chunkNum, err)
	}
	
	fmt.Fprintf(os.Stderr, "✓ Wrote chunk %d (%d entries)\n", chunkNum, prevEntries)
	return nil
}

// continuationFlush handles flushing when continuing a flat directory scan
func (cbs *CompartmentBundleScanner) continuationFlush() error {
	// This creates a @base chain continuation
	return cbs.flushChunkWithBase(false, true)
}

// scanDirectoryFlatNoCompartment scans a directory without creating sub-chains
func (cbs *CompartmentBundleScanner) scanDirectoryFlatNoCompartment(dirPath string, depth int) (*Manifest, int, int64, error) {
	manifest := NewManifest()
	var entryCount int
	var totalBytes int64
	
	// Read directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}
	
	// Create directory entry
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return nil, 0, 0, err
	}
	
	dirEntry := &Entry{
		Name:      filepath.Base(dirPath),
		Mode:      dirInfo.Mode(),
		Timestamp: dirInfo.ModTime(),
		Depth:     depth,
	}
	
	// Track subdirectory sizes for proper directory size calculation
	var dirSize int64
	
	// Separate files and directories for proper ordering
	var fileEntries []os.DirEntry
	var dirEntries []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			dirEntries = append(dirEntries, entry)
		} else {
			fileEntries = append(fileEntries, entry)
		}
	}
	
	// Process files first
	for _, entry := range fileEntries {
		entryPath := filepath.Join(dirPath, entry.Name())
		
		if entry.IsDir() {
			// Recurse inline without compartmentalization
			subManifest, subCount, subSize, err := cbs.scanDirectoryFlatNoCompartment(entryPath, depth+1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}
			
			// Merge subdirectory entries into our manifest
			for _, subEntry := range subManifest.Entries {
				manifest.AddEntry(subEntry)
			}
			entryCount += subCount
			dirSize += subSize
		} else {
			// Process file
			info, err := entry.Info()
			if err != nil {
				continue
			}
			
			fileEntry := &Entry{
				Name:      entry.Name(),
				Mode:      info.Mode(),
				Size:      info.Size(),
				Timestamp: info.ModTime(),
				Depth:     depth + 1,
			}
			
			// Compute C4 ID if regular file
			if info.Mode().IsRegular() {
				file, err := os.Open(entryPath)
				if err == nil {
					id := c4.Identify(file)
					fileEntry.C4ID = id
					file.Close()
				}
			}
			
			manifest.AddEntry(fileEntry)
			entryCount++
			totalBytes += info.Size()
			dirSize += info.Size()
		}
	}
	
	// Set directory size as sum of children
	dirEntry.Size = dirSize
	
	// Add directory entry at the beginning
	newEntries := []*Entry{dirEntry}
	newEntries = append(newEntries, manifest.Entries...)
	manifest.Entries = newEntries
	entryCount++
	
	return manifest, entryCount, dirSize, nil
}

// scanLargeDirectoryWithChunking scans a large directory and creates proper chunks with @base chains
func (cbs *CompartmentBundleScanner) scanLargeDirectoryWithChunking(dirPath string, depth int) (int64, error) {
	// Read directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}
	
	// Get directory info
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return 0, err
	}
	
	// Track total size
	var totalDirSize int64
	
	// Create directory entry
	dirEntry := &Entry{
		Name:      filepath.Base(dirPath),
		Mode:      dirInfo.Mode(),
		Timestamp: dirInfo.ModTime(),
		Depth:     depth,
	}
	
	// Process all entries to calculate directory size
	var allEntries []*Entry
	var separateSubdirs []*Entry  // Subdirectories that will get their own chains
	
	for _, entry := range entries {
		entryPath := filepath.Join(dirPath, entry.Name())
		
		if entry.IsDir() {
			// Check if this subdirectory should get its own chain
			_, _, needsSeparate := cbs.estimateDirectorySize(entryPath)
			
			if needsSeparate {
				// Large subdirectory - create separate chain
				subResult, err := cbs.scanLargeDirectory(entryPath, depth+1)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
					continue
				}
				
				// Add reference entry
				subInfo, _ := os.Stat(entryPath)
				subEntry := &Entry{
					Name:      entry.Name(),
					Mode:      subInfo.Mode(),
					Size:      subResult.TotalSize,
					Timestamp: subInfo.ModTime(),
					Depth:     depth + 1,
				}
				if subResult.FinalID != nil {
					subEntry.C4ID = *subResult.FinalID
				}
				separateSubdirs = append(separateSubdirs, subEntry)
				totalDirSize += subResult.TotalSize
			} else {
				// Small subdirectory - include inline
				subManifest, _, subSize, err := cbs.scanDirectoryFlatNoCompartment(entryPath, depth+1)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
					continue
				}
				
				// Add all entries from subdirectory (they already have correct depth)
				allEntries = append(allEntries, subManifest.Entries...)
				totalDirSize += subSize
			}
		} else {
			// Process file
			info, err := entry.Info()
			if err != nil {
				continue
			}
			
			fileEntry := &Entry{
				Name:      entry.Name(),
				Mode:      info.Mode(),
				Size:      info.Size(),
				Timestamp: info.ModTime(),
				Depth:     depth + 1,
			}
			
			// Compute C4 ID if regular file
			if info.Mode().IsRegular() {
				file, err := os.Open(entryPath)
				if err == nil {
					id := c4.Identify(file)
					fileEntry.C4ID = id
					file.Close()
				}
			}
			
			allEntries = append(allEntries, fileEntry)
			totalDirSize += info.Size()
		}
	}
	
	// Set directory size
	dirEntry.Size = totalDirSize
	
	// Add directory entry first
	cbs.currentChunk.AddEntry(dirEntry)
	cbs.chunkEntries++
	
	// Now process all entries, chunking as needed
	hasWrittenFirstChunk := false
	for _, entry := range allEntries {
		// Check if we need to flush
		willExceed := cbs.chunkEntries >= cbs.config.MaxEntriesPerChunk ||
		             cbs.chunkBytes >= cbs.config.MaxBytesPerChunk
		
		if willExceed && cbs.chunkEntries > 0 {
			// Flush with @base for continuation
			if err := cbs.flushChunkWithBase(false, hasWrittenFirstChunk); err != nil {
				return 0, err
			}
			hasWrittenFirstChunk = true
		}
		
		// Add entry to current chunk
		cbs.currentChunk.AddEntry(entry)
		cbs.chunkEntries++
		if entry.Size > 0 {
			cbs.chunkBytes += entry.Size
		}
	}
	
	// Add references to large subdirectories at the end
	// These are just references with C4 IDs, not the actual content
	for _, subdir := range separateSubdirs {
		// Check if we need to flush before adding reference
		if cbs.chunkEntries >= cbs.config.MaxEntriesPerChunk {
			if err := cbs.flushChunkWithBase(false, hasWrittenFirstChunk); err != nil {
				return 0, err
			}
			hasWrittenFirstChunk = true
		}
		
		// Add subdirectory reference
		cbs.currentChunk.AddEntry(subdir)
		cbs.chunkEntries++
	}
	
	return totalDirSize, nil
}

// GetBundlePath returns the path to the created bundle
func (cbs *CompartmentBundleScanner) GetBundlePath() string {
	return cbs.bundle.Path
}

// GetChunksWritten returns the number of chunks written
func (cbs *CompartmentBundleScanner) GetChunksWritten() int {
	cbs.mu.Lock()
	defer cbs.mu.Unlock()
	return cbs.chunksWritten
}