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
	
	// Use 50% threshold to decide on separation
	// This gives room for metadata and avoids edge cases
	shouldSeparate = entryCount > cbs.config.MaxEntriesPerChunk/2 ||
	                totalSize > cbs.config.MaxBytesPerChunk/2
	
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
	// Creating separate chain for large directory
	
	// Scan the directory flat (no separate chains for sub-subdirectories)
	manifest, entryCount, totalSize, err := cbs.scanDirectoryFlatNoCompartment(dirPath, 0)
	if err != nil {
		return nil, err
	}
	
	// Save current state
	oldChunk := cbs.currentChunk
	oldEntries := cbs.chunkEntries
	oldBytes := cbs.chunkBytes
	
	// Set the manifest for this directory
	cbs.currentChunk = manifest
	cbs.chunkEntries = entryCount
	cbs.chunkBytes = totalSize
	
	// Write chunk(s) for this directory
	var finalID *c4.ID
	if cbs.chunkEntries > 0 {
		if err := cbs.flushChunk(true); err != nil {
			return nil, err
		}
		// Get the final chunk ID
		if len(cbs.scan.ProgressChunks) > 0 {
			lastChunk := cbs.scan.ProgressChunks[len(cbs.scan.ProgressChunks)-1]
			// The lastChunk is already a string ID, not content to hash
			// Just parse it directly
			if id, err := c4.Parse(lastChunk); err == nil {
				finalID = &id
			}
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
	
	// Process entries
	for _, entry := range entries {
		entryPath := filepath.Join(dirPath, entry.Name())
		
		if entry.IsDir() {
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
				
				// Merge subdirectory entries into our manifest
				for _, subEntry := range subManifest.Entries {
					manifest.AddEntry(subEntry)
				}
				entryCount += subCount
				dirSize += subSize
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

// flushChunk writes the current chunk to the bundle
func (cbs *CompartmentBundleScanner) flushChunk(isFinal bool) error {
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
	if err := cbs.bundle.AddProgressChunk(cbs.scan, chunk); err != nil {
		return fmt.Errorf("failed to write chunk %d: %w", chunkNum, err)
	}
	
	fmt.Fprintf(os.Stderr, "✓ Wrote chunk %d (%d entries)\n", chunkNum, prevEntries)
	return nil
}

// continuationFlush handles flushing when continuing a flat directory scan
func (cbs *CompartmentBundleScanner) continuationFlush() error {
	// This creates a @base chain continuation
	return cbs.flushChunk(false)
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
	
	// Process entries
	for _, entry := range entries {
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