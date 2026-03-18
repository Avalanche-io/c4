package scan

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Avalanche-io/c4"
)

// ScanStage represents the level of detail in scanning
type ScanStage int

const (
	StageStructure ScanStage = iota // Just file/dir differentiation (fastest)
	StageMetadata                    // Add size, permissions, timestamps
	StageC4ID                        // Compute content hashes (slowest)
	StageComplete                    // All scanning complete
)

// ScanEntry represents a filesystem entry with progressive detail
type ScanEntry struct {
	// Embed FileMetadata for standard file properties
	FileMetadata
	
	// Additional scan-specific fields
	Path      string
	Stage     ScanStage    // Current scan stage completed
	
	// Internal
	parent    *ScanEntry
	children  []*ScanEntry
	mu        sync.RWMutex
}

// ProgressiveScanner performs multi-stage filesystem scanning
type ProgressiveScanner struct {
	// Configuration
	rootPath        string
	numWorkers      int
	includeHidden   bool
	followSymlinks  bool
	c4Workers       int
	slowMode        bool  // Add artificial delays for testing
	
	// Scan state
	stage           ScanStage
	entries         sync.Map // path -> *ScanEntry
	rootEntry       *ScanEntry
	
	// Progress tracking
	totalFound      int64
	metadataScanned int64
	c4Computed      int64
	regularFiles    int64 // Number of regular files found
	
	// Work tracking for proper completion
	structurePending int32  // directories being processed
	metadataPending  int32  // metadata jobs pending
	c4Pending        int32  // C4 computations pending
	
	// Channels for work distribution
	structureChan   chan string
	metadataChan    chan *ScanEntry
	c4Chan          chan *ScanEntry
	
	// Control
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	done            chan struct{} // Signal when scanning is complete
	
	// Signal handling
	signalChan      chan os.Signal
	statusChan      chan chan *ScanStatus
	
	// Output
	outputMu        sync.Mutex
	lastOutput      time.Time
	minOutputDelay  time.Duration
	outputHook      func(*Entry) // Hook for bundle mode
}

// ScanStatus represents the current scan progress
type ScanStatus struct {
	Stage           ScanStage
	TotalFound      int64
	MetadataScanned int64
	C4Computed      int64
	RegularFiles    int64
	ElapsedTime     time.Duration
	StartTime       time.Time
	ChunksWritten   int64 // For bundle mode
}

// NewProgressiveScanner creates a new progressive filesystem scanner
func NewProgressiveScanner(rootPath string) *ProgressiveScanner {
	ctx, cancel := context.WithCancel(context.Background())
	
	numCPU := runtime.NumCPU()
	
	return &ProgressiveScanner{
		rootPath:       rootPath,
		numWorkers:     numCPU * 2,  // For I/O bound structure/metadata scanning
		c4Workers:      numCPU,       // For CPU bound C4 computation
		includeHidden:  false,
		followSymlinks: false,
		minOutputDelay: 100 * time.Millisecond,
		
		structureChan:  make(chan string, 10000),      // Increased buffer for large directories
		metadataChan:   make(chan *ScanEntry, 10000),  // Increased buffer for large directories
		c4Chan:         make(chan *ScanEntry, 1000),   // Increased buffer
		
		signalChan:     make(chan os.Signal, 1),
		statusChan:     make(chan chan *ScanStatus, 10),
		done:           make(chan struct{}),
		
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins the progressive scanning process
func (ps *ProgressiveScanner) Start() error {
	// Set up signal handling
	// On macOS/BSD, SIGINFO (Ctrl+T) is perfect for status updates
	signals := []os.Signal{syscall.SIGINT, syscall.SIGTERM}

	// Add platform-specific signals
	if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" {
		signals = append(signals, SIGINFO)
	} else if runtime.GOOS != "windows" {
		// SIGUSR1 on Linux/Unix (but not Windows)
		signals = append(signals, syscall.Signal(10)) // SIGUSR1
	}
	
	signal.Notify(ps.signalChan, signals...)
	
	// Start signal handler
	ps.wg.Add(1)
	go ps.signalHandler()
	
	// Initialize root entry
	rootInfo, err := os.Lstat(ps.rootPath)
	if err != nil {
		return fmt.Errorf("failed to stat root path: %w", err)
	}
	rootMD := NewFileMetadata(ps.rootPath, rootInfo, 0)
	ps.rootEntry = &ScanEntry{
		FileMetadata: rootMD,
		Path:        ps.rootPath,
		Stage:       StageStructure,
	}
	ps.entries.Store(ps.rootPath, ps.rootEntry)
	
	// Start scanning stages
	ps.wg.Add(1)
	go ps.runStructureScan()
	
	return nil
}

// signalHandler processes signals for status and interrupts
func (ps *ProgressiveScanner) signalHandler() {
	defer ps.wg.Done()
	
	for {
		select {
		case sig := <-ps.signalChan:
			switch sig {
			case SIGINFO:
				// Output current status without stopping
				// SIGINFO: Ctrl+T on macOS/BSD, SIGUSR1 on Linux
				ps.OutputCurrentState(os.Stdout) // Don't run in goroutine for SIGINFO
				
			case syscall.SIGINT, syscall.SIGTERM:
				// Output what we have and stop
				fmt.Fprintf(os.Stderr, "\n# Interrupted - outputting partial results\n")
				ps.OutputCurrentState(os.Stdout) // Output synchronously before stopping
				ps.cancel()
				// Exit the process immediately after canceling (but not during tests)
				if os.Getenv("GO_TEST") != "1" {
					os.Exit(0)
				}
			}
			
		case <-ps.done:
			// Scanning complete
			return
			
		case <-ps.ctx.Done():
			return
		}
	}
}

// RequestStatus requests current scan status
func (ps *ProgressiveScanner) RequestStatus() *ScanStatus {
	// Return status directly without channel communication
	return &ScanStatus{
		Stage:           ps.stage,
		TotalFound:      atomic.LoadInt64(&ps.totalFound),
		MetadataScanned: atomic.LoadInt64(&ps.metadataScanned),
		C4Computed:      atomic.LoadInt64(&ps.c4Computed),
		RegularFiles:    atomic.LoadInt64(&ps.regularFiles),
		StartTime:       time.Now(), // Should be tracked separately
		ElapsedTime:     time.Duration(0), // Not used currently
	}
}

// runStructureScan performs Stage 1: Fast directory structure scanning
func (ps *ProgressiveScanner) runStructureScan() {
	defer ps.wg.Done()
	ps.stage = StageStructure
	
	// Start all workers
	for i := 0; i < ps.numWorkers; i++ {
		ps.wg.Add(1)
		go ps.structureWorker()
	}
	
	for i := 0; i < ps.numWorkers; i++ {
		ps.wg.Add(1)
		go ps.metadataWorker()
	}
	
	for i := 0; i < ps.c4Workers; i++ {
		ps.wg.Add(1)
		go ps.c4Worker()
	}
	
	// Seed with root path - increment BEFORE starting monitor
	atomic.AddInt32(&ps.structurePending, 1)
	ps.structureChan <- ps.rootPath
	
	// Monitor completion - start AFTER seeding
	ps.wg.Add(1)
	go ps.completionMonitor()
}

// structureWorker processes directories for structure scanning
func (ps *ProgressiveScanner) structureWorker() {
	defer ps.wg.Done()
	
	for {
		select {
		case dirPath, ok := <-ps.structureChan:
			if !ok {
				return // Channel closed
			}
			ps.scanDirectory(dirPath)
			// Decrement pending count after processing
			atomic.AddInt32(&ps.structurePending, -1)
			
		case <-ps.ctx.Done():
			return
		}
	}
}

// scanDirectory performs fast directory scanning using optimized syscalls
func (ps *ProgressiveScanner) scanDirectory(dirPath string) {
	// Open directory
	dir, err := os.Open(dirPath)
	if err != nil {
		return
	}
	defer dir.Close()
	
	// Get parent entry
	parentEntry, _ := ps.entries.Load(dirPath)
	parent := parentEntry.(*ScanEntry)
	
	// Read directory entries - using Readdirnames for speed
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return
	}
	
	// Process each entry
	for _, name := range names {
		// Skip hidden files if configured
		if !ps.includeHidden && name[0] == '.' {
			continue
		}
		
		fullPath := filepath.Join(dirPath, name)
		
		// Get file info to create metadata
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		
		// Create metadata and scan entry
		parentDepth := 0
		if parent.FileMetadata != nil {
			parentDepth = parent.FileMetadata.Depth()
		}
		md := NewFileMetadata(fullPath, info, parentDepth+1)
		
		entry := &ScanEntry{
			FileMetadata: md,
			Path:        fullPath,
			Stage:       StageStructure,
			parent:      parent,
		}
		
		// Store entry
		ps.entries.Store(fullPath, entry)
		atomic.AddInt64(&ps.totalFound, 1)
		
		// Add to parent's children
		parent.mu.Lock()
		parent.children = append(parent.children, entry)
		parent.mu.Unlock()
		
		// Artificial delay for slow mode testing
		if ps.slowMode {
			time.Sleep(10 * time.Millisecond) // Slow down structure scanning
		}
		
		// Queue for metadata scanning
		atomic.AddInt32(&ps.metadataPending, 1)
		select {
		case ps.metadataChan <- entry:
		case <-ps.ctx.Done():
			atomic.AddInt32(&ps.metadataPending, -1)
			return
		}
		
		// If directory, queue for recursion
		if entry.FileMetadata.IsDir() {
			atomic.AddInt32(&ps.structurePending, 1)
			select {
			case ps.structureChan <- fullPath:
			case <-ps.ctx.Done():
				atomic.AddInt32(&ps.structurePending, -1)
				return
			}
		}
	}
}

// completionMonitor monitors all stages and closes channels when work is done
func (ps *ProgressiveScanner) completionMonitor() {
	defer ps.wg.Done()
	
	// Brief wait to ensure initial work is queued
	time.Sleep(5 * time.Millisecond)
	
	ticker := time.NewTicker(50 * time.Millisecond) // More responsive
	defer ticker.Stop()
	
	structureClosed := false
	metadataClosed := false
	c4Closed := false
	
	// Track when we last saw work to detect stalls
	lastWorkTime := time.Now()
	var lastStructPending, lastMetaPending, lastC4Pending int32
	
	for {
		select {
		case <-ticker.C:
			structPending := atomic.LoadInt32(&ps.structurePending)
			metaPending := atomic.LoadInt32(&ps.metadataPending)
			c4Pending := atomic.LoadInt32(&ps.c4Pending)
			
			// Check if work is progressing
			if structPending != lastStructPending || metaPending != lastMetaPending || c4Pending != lastC4Pending {
				lastWorkTime = time.Now()
				lastStructPending = structPending
				lastMetaPending = metaPending
				lastC4Pending = c4Pending
			}
			
			// Debug output - enable via environment variable
			if os.Getenv("C4_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "\rPending: struct=%d meta=%d c4=%d closed=%v/%v/%v idle=%.1fs      ",
					structPending, metaPending, c4Pending,
					structureClosed, metadataClosed, c4Closed,
					time.Since(lastWorkTime).Seconds())
			}
			
			// Check structure completion
			if !structureClosed && structPending == 0 {
				close(ps.structureChan)
				structureClosed = true
				ps.stage = StageMetadata
			}
			
			// Check metadata completion
			if !metadataClosed && structureClosed && metaPending == 0 {
				close(ps.metadataChan)
				metadataClosed = true
				ps.stage = StageC4ID
			}
			
			// Check C4 completion
			if !c4Closed && metadataClosed && c4Pending == 0 {
				close(ps.c4Chan)
				c4Closed = true
				// Only set complete when all workers are done
				// Workers will finish processing what's in their channels
				ps.stage = StageComplete
				close(ps.done) // Signal completion
				return // All done!
			}
			
		case <-ps.ctx.Done():
			if !structureClosed {
				close(ps.structureChan)
			}
			if !metadataClosed {
				close(ps.metadataChan)
			}
			if !c4Closed {
				close(ps.c4Chan)
			}
			select {
			case <-ps.done:
				// Already closed
			default:
				close(ps.done)
			}
			return
		}
	}
}

// metadataWorker processes entries for metadata collection
func (ps *ProgressiveScanner) metadataWorker() {
	defer ps.wg.Done()
	
	for {
		select {
		case entry, ok := <-ps.metadataChan:
			if !ok {
				return // Channel closed
			}
			if entry != nil {
				ps.collectMetadata(entry)
				// Decrement pending count after processing
				atomic.AddInt32(&ps.metadataPending, -1)
			}
			
		case <-ps.ctx.Done():
			return
		}
	}
}

// collectMetadata marks the metadata as collected
// (metadata is already gathered when FileMetadata is created)
func (ps *ProgressiveScanner) collectMetadata(entry *ScanEntry) {
	// Handle symlinks - update target if needed
	if entry.FileMetadata.Mode()&os.ModeSymlink != 0 {
		if bmd, ok := entry.FileMetadata.(*BasicFileMetadata); ok {
			if target, err := os.Readlink(entry.Path); err == nil {
				bmd.SetTarget(filepath.ToSlash(target))
			}
		}
	}
	
	entry.mu.Lock()
	entry.Stage = StageMetadata
	entry.mu.Unlock()
	
	atomic.AddInt64(&ps.metadataScanned, 1)
	
	// Artificial delay for slow mode testing
	if ps.slowMode {
		time.Sleep(15 * time.Millisecond) // Slow down metadata scanning
	}
	
	// Queue regular files for C4 computation
	if entry.FileMetadata.Mode().IsRegular() {
		atomic.AddInt64(&ps.regularFiles, 1)
		atomic.AddInt32(&ps.c4Pending, 1)
		select {
		case ps.c4Chan <- entry:
		case <-ps.ctx.Done():
			atomic.AddInt32(&ps.c4Pending, -1)
			return
		}
	}
}



// c4Worker computes C4 IDs for files
func (ps *ProgressiveScanner) c4Worker() {
	defer ps.wg.Done()
	
	for {
		select {
		case entry, ok := <-ps.c4Chan:
			if !ok {
				return // Channel closed
			}
			if entry != nil {
				ps.computeC4ID(entry)
				// Decrement pending count after processing
				atomic.AddInt32(&ps.c4Pending, -1)
			}
			
		case <-ps.ctx.Done():
			return
		}
	}
}

// computeC4ID calculates the C4 ID for a file
func (ps *ProgressiveScanner) computeC4ID(entry *ScanEntry) {
	file, err := os.Open(entry.Path)
	if err != nil {
		return
	}
	defer file.Close()
	
	id := c4.Identify(file)
	
	// Set the C4 ID in FileMetadata
	entry.FileMetadata.SetID(id)
	
	entry.mu.Lock()
	entry.Stage = StageC4ID
	entry.mu.Unlock()
	
	atomic.AddInt64(&ps.c4Computed, 1)
	
	// Artificial delay for slow mode testing
	if ps.slowMode {
		time.Sleep(30 * time.Millisecond) // Slow down C4 computation significantly
	}
}

// OutputCurrentState outputs the current scan state as a C4M manifest
func (ps *ProgressiveScanner) OutputCurrentState(w io.Writer) error {
	ps.outputMu.Lock()
	defer ps.outputMu.Unlock()
	
	// Rate limit output
	if time.Since(ps.lastOutput) < ps.minOutputDelay {
		return nil
	}
	ps.lastOutput = time.Now()
	
	// Build manifest from current state
	manifest := NewManifest()
	
	// Add comment about scan status
	status := ps.RequestStatus()
	if status != nil {
		fmt.Fprintf(w, "# Progressive scan status: Stage %d, Found %d, Metadata %d, C4 %d\n",
			status.Stage, status.TotalFound, status.MetadataScanned, status.C4Computed)
	}
	
	// Add root's children directly (don't include root itself)
	if ps.rootEntry != nil {
		ps.rootEntry.mu.RLock()
		children := make([]*ScanEntry, len(ps.rootEntry.children))
		copy(children, ps.rootEntry.children)
		ps.rootEntry.mu.RUnlock()
		
		// Sort children using natural sort
		sortScanEntries(children)
		
		for _, child := range children {
			if child.FileMetadata.IsDir() {
				ps.addEntriesToManifest(manifest, child, 0)
			} else {
				childEntry := ps.scanEntryToEntry(child, 0)
				manifest.AddEntry(childEntry)
			}
		}
	}
	
	// Output manifest
	return NewEncoder(w).Encode(manifest)
}

// addEntriesToManifest recursively adds entries to manifest
func (ps *ProgressiveScanner) addEntriesToManifest(manifest *Manifest, scanEntry *ScanEntry, depth int) {
	// Convert ScanEntry to Entry
	entry := ps.scanEntryToEntry(scanEntry, depth)
	manifest.AddEntry(entry)
	
	// Send to hook if available (for bundle mode)
	if ps.outputHook != nil {
		ps.outputHook(entry)
	}
	
	// Add children
	scanEntry.mu.RLock()
	children := make([]*ScanEntry, len(scanEntry.children))
	copy(children, scanEntry.children)
	scanEntry.mu.RUnlock()
	
	// Sort children using natural sort
	sortScanEntries(children)
	
	for _, child := range children {
		if child.FileMetadata.IsDir() {
			ps.addEntriesToManifest(manifest, child, depth+1)
		} else {
			childEntry := ps.scanEntryToEntry(child, depth+1)
			manifest.AddEntry(childEntry)
			// Send to hook if available (for bundle mode)
			if ps.outputHook != nil {
				ps.outputHook(childEntry)
			}
		}
	}
}

// scanEntryToEntry converts a ScanEntry to a manifest Entry
func (ps *ProgressiveScanner) scanEntryToEntry(se *ScanEntry, depth int) *Entry {
	se.mu.RLock()
	defer se.mu.RUnlock()
	
	// Convert FileMetadata to Entry
	entry := MetadataToEntry(se.FileMetadata)
	entry.Depth = depth // Override depth with the passed value
	
	// For root directory at depth 0, use basename of path
	if depth == 0 && entry.Name == "" {
		entry.Name = filepath.Base(se.Path)
	}
	
	// MetadataToEntry already adds directory suffix
	// but check if we need to handle incomplete scans
	if se.Stage < StageMetadata {
		// Use zero values for incomplete scan
		entry.Mode = 0
		entry.Size = -1
		entry.Timestamp = time.Unix(0, 0).UTC()
	}
	
	// Add C4 ID if available
	if se.Stage >= StageC4ID {
		entry.C4ID = se.FileMetadata.ID()
	}
	
	return entry
}

// Wait waits for the scan to complete or be interrupted
func (ps *ProgressiveScanner) Wait() {
	// Wait for either completion or cancellation
	<-ps.done
	// The done channel is closed, scanning is complete
	// Now wait for all goroutines to finish
	ps.wg.Wait()
}

// Stop gracefully stops the scanner
func (ps *ProgressiveScanner) Stop() {
	ps.cancel()
	ps.wg.Wait()
}

// sortScanEntries sorts scan entries using natural sort
func sortScanEntries(entries []*ScanEntry) {
	// Use Go's sort.Slice for O(n log n) performance instead of O(n²)
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		// Files before directories
		aIsDir := a.FileMetadata.IsDir()
		bIsDir := b.FileMetadata.IsDir()
		if aIsDir != bIsDir {
			return !aIsDir // files first
		}
		return NaturalLess(a.FileMetadata.Name(), b.FileMetadata.Name())
	})
}

