package c4m

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	
	"github.com/Avalanche-io/c4"
)

// SimpleBundleScanner creates bundles by scanning filesystems
type SimpleBundleScanner struct {
	bundle       *Bundle
	scan         *BundleScan
	config       *BundleConfig
	rootPath     string
	
	// Current chunk state
	currentChunk *Manifest
	chunkEntries int
	chunkBytes   int64
	lastChunkTime time.Time
	
	// Synchronization
	mu           sync.Mutex
	chunksWritten int
}

// NewSimpleBundleScanner creates a scanner that outputs to a bundle
func NewSimpleBundleScanner(scanPath string, config *BundleConfig) (*SimpleBundleScanner, error) {
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
	
	return &SimpleBundleScanner{
		bundle:        bundle,
		scan:          scan,
		config:        config,
		rootPath:      scanPath,
		currentChunk:  NewManifest(),
		lastChunkTime: time.Now(),
	}, nil
}

// Scan performs the filesystem scan and outputs to bundle
func (sbs *SimpleBundleScanner) Scan() error {
	// Scan the filesystem recursively, accumulating entries
	if err := sbs.scanDirectory(sbs.rootPath, 0); err != nil {
		return err
	}
	
	// Flush final chunk if any entries remain
	if sbs.chunkEntries > 0 {
		if err := sbs.flushChunk(); err != nil {
			return err
		}
	}
	
	// Complete the scan
	if err := sbs.bundle.CompleteScan(sbs.scan); err != nil {
		return err
	}
	
	return nil
}

// scanDirectory recursively scans a directory and adds entries
func (sbs *SimpleBundleScanner) scanDirectory(dirPath string, depth int) error {
	// Read directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}
	
	// Create directory entry
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return err
	}
	
	dirEntry := &Entry{
		Name:      filepath.Base(dirPath),
		Mode:      dirInfo.Mode(),
		Timestamp: dirInfo.ModTime(),
		Depth:     depth,
	}
	
	// Add to current chunk
	sbs.addEntry(dirEntry)
	
	// Don't flush after directory entries - only after accumulating content
	
	// Process entries
	for _, entry := range entries {
		entryPath := filepath.Join(dirPath, entry.Name())
		
		if entry.IsDir() {
			// Recurse into subdirectory
			if err := sbs.scanDirectory(entryPath, depth+1); err != nil {
				// Log error but continue
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
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
			
			// Add to current chunk
			sbs.addEntry(fileEntry)
			
			// Only check flush periodically to avoid overhead
			sbs.mu.Lock()
			shouldCheck := sbs.chunkEntries >= sbs.config.MaxEntriesPerChunk ||
				           sbs.chunkBytes >= sbs.config.MaxBytesPerChunk
			sbs.mu.Unlock()
			
			if shouldCheck {
				if err := sbs.flushChunk(); err != nil {
					return err
				}
			}
		}
	}
	
	return nil
}

// addEntry adds an entry to the current chunk
func (sbs *SimpleBundleScanner) addEntry(entry *Entry) {
	sbs.mu.Lock()
	defer sbs.mu.Unlock()
	
	sbs.currentChunk.AddEntry(entry)
	sbs.chunkEntries++
	if entry.Size > 0 {
		sbs.chunkBytes += entry.Size
	}
}

// shouldFlushByCount checks if we've hit the entry limit
func (sbs *SimpleBundleScanner) shouldFlushByCount() bool {
	sbs.mu.Lock()
	defer sbs.mu.Unlock()
	return sbs.chunkEntries >= sbs.config.MaxEntriesPerChunk
}

// shouldFlushBySize checks if we've hit the size limit
func (sbs *SimpleBundleScanner) shouldFlushBySize() bool {
	sbs.mu.Lock()
	defer sbs.mu.Unlock()
	return sbs.chunkBytes >= sbs.config.MaxBytesPerChunk
}

// Note: Removed time-based flushing as it was causing excessive small chunks
// Time-based flushing should only be used for long-running operations
// where no new entries are being added, not during active scanning

// flushChunk writes the current chunk to the bundle
func (sbs *SimpleBundleScanner) flushChunk() error {
	sbs.mu.Lock()
	if sbs.chunkEntries == 0 {
		sbs.mu.Unlock()
		return nil
	}
	
	chunk := sbs.currentChunk
	sbs.currentChunk = NewManifest()
	sbs.chunkEntries = 0
	sbs.chunkBytes = 0
	sbs.lastChunkTime = time.Now()
	sbs.chunksWritten++
	chunkNum := sbs.chunksWritten
	sbs.mu.Unlock()
	
	// Write chunk to bundle
	if err := sbs.bundle.AddProgressChunk(sbs.scan, chunk); err != nil {
		return fmt.Errorf("failed to write chunk %d: %w", chunkNum, err)
	}
	
	fmt.Fprintf(os.Stderr, "✓ Wrote chunk %d\n", chunkNum)
	return nil
}

// GetBundlePath returns the path to the created bundle
func (sbs *SimpleBundleScanner) GetBundlePath() string {
	return sbs.bundle.Path
}

// GetChunksWritten returns the number of chunks written
func (sbs *SimpleBundleScanner) GetChunksWritten() int {
	sbs.mu.Lock()
	defer sbs.mu.Unlock()
	return sbs.chunksWritten
}