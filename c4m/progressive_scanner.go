package c4m

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
	Path      string
	Name      string
	Depth     int
	IsDir     bool
	Stage     ScanStage    // Current scan stage completed
	
	// Stage 2: Metadata
	Mode      os.FileMode
	Size      int64
	Timestamp time.Time
	Target    string       // For symlinks
	
	// Stage 3: Content
	C4ID      c4.ID
	
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
	signals := []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1}
	
	// Add SIGINFO on systems that support it (macOS/BSD)
	// SIGINFO is triggered by Ctrl+T and is meant for status updates
	if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" {
		signals = append(signals, SIGINFO)
	}
	
	signal.Notify(ps.signalChan, signals...)
	
	// Start signal handler
	ps.wg.Add(1)
	go ps.signalHandler()
	
	// Initialize root entry
	ps.rootEntry = &ScanEntry{
		Path:  ps.rootPath,
		Name:  filepath.Base(ps.rootPath),
		Depth: 0,
		IsDir: true,
		Stage: StageStructure,
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
			case syscall.SIGUSR1, SIGINFO:
				// Output current status without stopping
				// SIGUSR1: manual signal via kill command
				// SIGINFO: Ctrl+T on macOS/BSD
				ps.OutputCurrentState(os.Stdout) // Don't run in goroutine for SIGINFO
				
			case syscall.SIGINT, syscall.SIGTERM:
				// Output what we have and stop
				fmt.Fprintf(os.Stderr, "\n# Interrupted - outputting partial results\n")
				ps.OutputCurrentState(os.Stdout) // Output synchronously before stopping
				ps.cancel()
				// Exit the process immediately after canceling
				os.Exit(0)
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
		
		// Create scan entry with minimal info
		entry := &ScanEntry{
			Path:   fullPath,
			Name:   name,
			Depth:  parent.Depth + 1,
			Stage:  StageStructure,
			parent: parent,
		}
		
		// Use lstat to determine if directory (fast, no following symlinks)
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		
		entry.IsDir = info.IsDir()
		
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
		if entry.IsDir {
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
	
	// Wait a bit before starting to monitor to ensure initial work is queued
	time.Sleep(100 * time.Millisecond) // Initial wait before monitoring
	
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

// collectMetadata gathers full metadata for an entry
func (ps *ProgressiveScanner) collectMetadata(entry *ScanEntry) {
	info, err := os.Lstat(entry.Path)
	if err != nil {
		return
	}
	
	entry.mu.Lock()
	entry.Mode = info.Mode()
	entry.Size = info.Size()
	entry.Timestamp = info.ModTime().UTC()
	
	// Handle symlinks
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(entry.Path); err == nil {
			entry.Target = target
		}
	}
	
	entry.Stage = StageMetadata
	entry.mu.Unlock()
	
	atomic.AddInt64(&ps.metadataScanned, 1)
	
	// Artificial delay for slow mode testing
	if ps.slowMode {
		time.Sleep(15 * time.Millisecond) // Slow down metadata scanning
	}
	
	// Queue regular files for C4 computation
	if info.Mode().IsRegular() {
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
	
	entry.mu.Lock()
	entry.C4ID = id
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
			if child.IsDir {
				ps.addEntriesToManifest(manifest, child, 0)
			} else {
				childEntry := ps.scanEntryToEntry(child, 0)
				manifest.AddEntry(childEntry)
			}
		}
	}
	
	// Output manifest
	_, err := manifest.WriteTo(w)
	return err
}

// addEntriesToManifest recursively adds entries to manifest
func (ps *ProgressiveScanner) addEntriesToManifest(manifest *Manifest, scanEntry *ScanEntry, depth int) {
	// Convert ScanEntry to Entry
	entry := ps.scanEntryToEntry(scanEntry, depth)
	manifest.AddEntry(entry)
	
	// Add children
	scanEntry.mu.RLock()
	children := make([]*ScanEntry, len(scanEntry.children))
	copy(children, scanEntry.children)
	scanEntry.mu.RUnlock()
	
	// Sort children using natural sort
	sortScanEntries(children)
	
	for _, child := range children {
		if child.IsDir {
			ps.addEntriesToManifest(manifest, child, depth+1)
		} else {
			childEntry := ps.scanEntryToEntry(child, depth+1)
			manifest.AddEntry(childEntry)
		}
	}
}

// scanEntryToEntry converts a ScanEntry to a manifest Entry
func (ps *ProgressiveScanner) scanEntryToEntry(se *ScanEntry, depth int) *Entry {
	se.mu.RLock()
	defer se.mu.RUnlock()
	
	// For root directory at depth 0, use basename of path
	name := se.Name
	if depth == 0 && name == "" {
		name = filepath.Base(se.Path)
	}
	
	entry := &Entry{
		Name:  name,
		Depth: depth,
	}
	
	// Add directory suffix
	if se.IsDir {
		entry.Name += "/"
	}
	
	// Add metadata if available
	if se.Stage >= StageMetadata {
		entry.Mode = se.Mode
		entry.Size = se.Size
		entry.Timestamp = se.Timestamp
		entry.Target = se.Target
	} else {
		// Use zero values for incomplete scan
		entry.Mode = 0
		entry.Size = -1
		entry.Timestamp = time.Unix(0, 0).UTC()
	}
	
	// Add C4 ID if available
	if se.Stage >= StageC4ID {
		entry.C4ID = se.C4ID
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
		if a.IsDir != b.IsDir {
			return !a.IsDir // files first
		}
		return NaturalLess(a.Name, b.Name)
	})
}

