package c4m

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// BundleScanner wraps ProgressiveScanner to output to bundles
type BundleScanner struct {
	scanner      *ProgressiveScanner
	bundle       *Bundle
	scan         *BundleScan
	config       *BundleConfig
	
	// Chunking state
	currentChunk *Manifest
	chunkEntries int
	chunkBytes   int64
	lastChunkTime time.Time
	
	// Synchronization
	mu           sync.Mutex
	outputChan   chan *Entry
	done         chan struct{}
	wg           sync.WaitGroup
	
	// Stats
	totalChunks  int32
}

// NewBundleScanner creates a scanner that outputs to a bundle
func NewBundleScanner(scanPath string, config *BundleConfig) (*BundleScanner, error) {
	if config == nil {
		config = DefaultBundleConfig()
	}
	
	// Create or open bundle
	bundle, err := CreateBundle(scanPath, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create bundle: %w", err)
	}
	
	// Start new scan
	scan, err := bundle.NewScan(scanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create scan: %w", err)
	}
	
	// Create a custom progressive scanner that sends entries to us
	scanner := NewProgressiveScanner(scanPath)
	
	bs := &BundleScanner{
		scanner:       scanner,
		bundle:        bundle,
		scan:          scan,
		config:        config,
		currentChunk:  NewManifest(),
		lastChunkTime: time.Now(),
		outputChan:    make(chan *Entry, 10000),
		done:          make(chan struct{}),
	}
	
	// Hook into scanner output
	scanner.outputHook = bs.captureEntry
	
	return bs, nil
}

// ResumeBundleScanner resumes scanning from an existing bundle
func ResumeBundleScanner(bundlePath string, config *BundleConfig) (*BundleScanner, error) {
	if config == nil {
		config = DefaultBundleConfig()
	}
	
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bundle: %w", err)
	}
	
	scan, err := bundle.ResumeScan()
	if err != nil {
		// No incomplete scan, start new one
		scan, err = bundle.NewScan(bundle.ScanPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create new scan: %w", err)
		}
	}
	
	scanner := NewProgressiveScanner(bundle.ScanPath)
	
	bs := &BundleScanner{
		scanner:       scanner,
		bundle:        bundle,
		scan:          scan,
		config:        config,
		currentChunk:  NewManifest(),
		lastChunkTime: time.Now(),
		outputChan:    make(chan *Entry, 10000),
		done:          make(chan struct{}),
	}
	
	// Hook into scanner output
	scanner.outputHook = bs.captureEntry
	
	// TODO: Load last chunk state for resume
	
	return bs, nil
}

// Start begins the scanning process
func (bs *BundleScanner) Start() error {
	// Start the underlying scanner
	if err := bs.scanner.Start(); err != nil {
		return err
	}
	
	// Start output processor
	bs.wg.Add(1)
	go bs.processOutput()
	
	return nil
}

// processOutput handles entries from scanner and chunks them
func (bs *BundleScanner) processOutput() {
	defer bs.wg.Done()
	
	ticker := time.NewTicker(bs.config.MaxChunkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case entry, ok := <-bs.outputChan:
			if !ok {
				// Channel closed, flush final chunk
				if bs.chunkEntries > 0 {
					bs.flushChunk()
				}
				return
			}
			
			// Add entry to current chunk
			bs.mu.Lock()
			bs.currentChunk.AddEntry(entry)
			bs.chunkEntries++
			if entry.Size > 0 {
				bs.chunkBytes += entry.Size
			}
			
			// Check if we should flush
			shouldFlush := bs.chunkEntries >= bs.config.MaxEntriesPerChunk ||
				bs.chunkBytes >= bs.config.MaxBytesPerChunk
			bs.mu.Unlock()
			
			if shouldFlush {
				bs.flushChunk()
			}
			
		case <-ticker.C:
			// Periodic flush
			bs.mu.Lock()
			if bs.chunkEntries > 0 && 
			   time.Since(bs.lastChunkTime) >= bs.config.MaxChunkInterval {
				bs.mu.Unlock()
				bs.flushChunk()
			} else {
				bs.mu.Unlock()
			}
			
		case <-bs.done:
			// Graceful shutdown
			close(bs.outputChan)
		}
	}
}

// flushChunk writes the current chunk to the bundle
func (bs *BundleScanner) flushChunk() error {
	bs.mu.Lock()
	if bs.chunkEntries == 0 {
		bs.mu.Unlock()
		return nil
	}
	
	chunk := bs.currentChunk
	bs.currentChunk = NewManifest()
	bs.chunkEntries = 0
	bs.chunkBytes = 0
	bs.lastChunkTime = time.Now()
	bs.mu.Unlock()
	
	// Write chunk to bundle
	if err := bs.bundle.AddProgressChunk(bs.scan, chunk); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}
	
	atomic.AddInt32(&bs.totalChunks, 1)
	return nil
}

// Wait waits for scanning to complete
func (bs *BundleScanner) Wait() error {
	// Start goroutine to stream entries from scanner to bundle
	entriesDone := make(chan error, 1)
	go func() {
		// Output current state which will trigger our hook
		err := bs.scanner.OutputCurrentState(io.Discard)
		close(bs.outputChan) // Signal no more entries
		entriesDone <- err
	}()
	
	// Wait for scanner to complete in background
	go func() {
		bs.scanner.Wait()
	}()
	
	// Wait for output processing to complete
	bs.wg.Wait()
	
	// Get any error from output
	outputErr := <-entriesDone
	
	// Mark scan as complete
	if err := bs.bundle.CompleteScan(bs.scan); err != nil {
		return fmt.Errorf("failed to complete scan: %w", err)
	}
	
	return outputErr
}

// Stop gracefully stops the scanner
func (bs *BundleScanner) Stop() {
	bs.scanner.Stop()
	close(bs.done)
	bs.wg.Wait()
	
	// Flush any remaining chunk
	if bs.chunkEntries > 0 {
		bs.flushChunk()
	}
}

// GetStatus returns scan status
func (bs *BundleScanner) GetStatus() *ScanStatus {
	status := bs.scanner.RequestStatus()
	if status != nil {
		// Add bundle-specific stats
		status.ChunksWritten = int64(atomic.LoadInt32(&bs.totalChunks))
	}
	return status
}

// StreamEntries connects scanner output to bundle input
func (bs *BundleScanner) StreamEntries() error {
	// This would be called by the scanner to send entries
	// For now, we'll integrate differently
	return nil
}

// captureEntry receives entries from the scanner
func (bs *BundleScanner) captureEntry(entry *Entry) {
	select {
	case bs.outputChan <- entry:
	case <-bs.done:
		// Shutting down
	}
}

// OutputReader returns a reader for the complete output
func (bs *BundleScanner) OutputReader() (io.Reader, error) {
	// This would create a reader that follows the @base chain
	// through all chunks to produce the complete manifest
	return nil, fmt.Errorf("not implemented yet")
}