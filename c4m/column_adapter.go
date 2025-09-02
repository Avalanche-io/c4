package c4m

import (
	"sync"
	"time"
)

// ColumnAdapter manages adaptive C4 ID column positioning with lookahead
type ColumnAdapter struct {
	// Configuration
	initialDelay   time.Duration // Max time to wait before starting output
	minSpacing     int           // Minimum spaces between content and C4 ID
	columnInterval int           // Column boundary interval (e.g., 10)
	
	// State
	currentColumn int       // Current C4 ID column position
	maxLineLength int       // Maximum line length seen so far
	started       bool      // Whether output has started
	startTime     time.Time // When scanning started
	
	// Synchronization
	mu         sync.RWMutex
	scanChan   chan *Entry    // Channel for entries to scan
	updateChan chan lineUpdate // Channel for line length updates
	done       chan struct{}  // Signal completion
	wg         sync.WaitGroup
}

// lineUpdate represents a discovered line length
type lineUpdate struct {
	entry  *Entry
	length int
	depth  int
}

// NewColumnAdapter creates a new adaptive column manager
func NewColumnAdapter(initialDelay time.Duration) *ColumnAdapter {
	if initialDelay == 0 {
		initialDelay = 500 * time.Millisecond
	}
	
	return &ColumnAdapter{
		initialDelay:   initialDelay,
		minSpacing:     10,
		columnInterval: 10,
		currentColumn:  80, // Start at column 80
		scanChan:       make(chan *Entry, 100),
		updateChan:     make(chan lineUpdate, 100),
		done:           make(chan struct{}),
	}
}

// Start begins the adaptive column detection
func (ca *ColumnAdapter) Start() {
	ca.mu.Lock()
	ca.startTime = time.Now()
	ca.started = false
	ca.mu.Unlock()
	
	// Start scanner goroutine
	ca.wg.Add(1)
	go ca.scanner()
	
	// Start updater goroutine
	ca.wg.Add(1)
	go ca.updater()
}

// Stop gracefully shuts down the adapter
func (ca *ColumnAdapter) Stop() {
	close(ca.done)
	ca.wg.Wait()
}

// ScanEntry submits an entry for column width calculation
func (ca *ColumnAdapter) ScanEntry(entry *Entry) {
	select {
	case ca.scanChan <- entry:
	case <-ca.done:
	}
}

// GetColumn returns the current C4 ID column position
// It may block up to initialDelay on first call
func (ca *ColumnAdapter) GetColumn() int {
	ca.mu.RLock()
	
	// If we haven't started output yet
	if !ca.started {
		ca.mu.RUnlock()
		
		// Wait up to initialDelay for better column estimate
		elapsed := time.Since(ca.startTime)
		if elapsed < ca.initialDelay {
			remaining := ca.initialDelay - elapsed
			timer := time.NewTimer(remaining)
			defer timer.Stop()
			
			select {
			case <-timer.C:
			case <-ca.done:
			}
		}
		
		// Mark as started
		ca.mu.Lock()
		ca.started = true
		ca.mu.Unlock()
		
		ca.mu.RLock()
	}
	
	col := ca.currentColumn
	ca.mu.RUnlock()
	return col
}

// ForceUpdateColumn allows manual column updates during output
func (ca *ColumnAdapter) ForceUpdateColumn(lineLength int) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	
	if lineLength > ca.maxLineLength {
		ca.maxLineLength = lineLength
		ca.updateColumnLocked()
	}
}

// scanner goroutine processes entries to calculate line lengths
func (ca *ColumnAdapter) scanner() {
	defer ca.wg.Done()
	
	for {
		select {
		case entry := <-ca.scanChan:
			if entry != nil {
				length := ca.calculateLineLength(entry)
				select {
				case ca.updateChan <- lineUpdate{
					entry:  entry,
					length: length,
					depth:  entry.Depth,
				}:
				case <-ca.done:
					return
				}
			}
		case <-ca.done:
			return
		}
	}
}

// updater goroutine processes line length updates
func (ca *ColumnAdapter) updater() {
	defer ca.wg.Done()
	
	for {
		select {
		case update := <-ca.updateChan:
			ca.processUpdate(update)
		case <-ca.done:
			return
		}
	}
}

// processUpdate handles a line length update
func (ca *ColumnAdapter) processUpdate(update lineUpdate) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	
	// Track maximum line length
	if update.length > ca.maxLineLength {
		ca.maxLineLength = update.length
		
		// Update column if needed (only move right, never left)
		ca.updateColumnLocked()
	}
}

// updateColumnLocked updates the column position based on max line length
// Must be called with lock held
func (ca *ColumnAdapter) updateColumnLocked() {
	// Calculate required column
	requiredColumn := ca.maxLineLength + ca.minSpacing
	
	// Round up to next column interval
	if requiredColumn > ca.currentColumn {
		// Round up to next boundary
		newColumn := ((requiredColumn / ca.columnInterval) + 1) * ca.columnInterval
		if newColumn > ca.currentColumn {
			ca.currentColumn = newColumn
		}
	}
}

// calculateLineLength computes the display length of an entry line
func (ca *ColumnAdapter) calculateLineLength(entry *Entry) int {
	// This calculation must match the actual formatting logic
	// For now, use a simplified version - this should be refined
	
	indentWidth := 2 // Default indent width
	indent := entry.Depth * indentWidth
	
	// Mode: 10 chars
	modeLen := 10
	
	// Timestamp: ~24 chars for pretty format
	timeLen := 24
	
	// Size: variable, estimate based on value
	sizeLen := ca.estimateSizeLength(entry.Size)
	
	// Name: actual length (accounting for quotes if needed)
	nameLen := len(formatName(entry.Name))
	
	// Target: if symlink
	targetLen := 0
	if entry.Target != "" {
		targetLen = 4 + len(entry.Target) // " -> " + target
	}
	
	// Total: indent + mode + space + time + space + size + space + name + target
	total := indent + modeLen + 1 + timeLen + 1 + sizeLen + 1 + nameLen + targetLen
	
	return total
}

// estimateSizeLength estimates the display length of a formatted size
func (ca *ColumnAdapter) estimateSizeLength(size int64) int {
	if size < 0 {
		return 1 // "-" for null size
	}
	
	// Count digits and add commas
	if size == 0 {
		return 1
	}
	
	digits := 0
	temp := size
	for temp > 0 {
		digits++
		temp /= 10
	}
	
	// Add comma separators
	commas := (digits - 1) / 3
	return digits + commas
}

// AdaptiveGeneratorOptions extends generator options with adaptive column support
type AdaptiveGeneratorOptions struct {
	InitialDelay   time.Duration // Max time to wait before first output
	EnableAdaptive bool          // Whether to use adaptive columns
}

// GeneratorWithAdapter wraps a generator with adaptive column support
type GeneratorWithAdapter struct {
	*Generator
	adapter *ColumnAdapter
	options AdaptiveGeneratorOptions
}

// NewGeneratorWithAdapter creates a generator with adaptive column support
func NewGeneratorWithAdapter(opts AdaptiveGeneratorOptions) *GeneratorWithAdapter {
	gen := NewGenerator()
	
	var adapter *ColumnAdapter
	if opts.EnableAdaptive {
		adapter = NewColumnAdapter(opts.InitialDelay)
	}
	
	return &GeneratorWithAdapter{
		Generator: gen,
		adapter:   adapter,
		options:   opts,
	}
}

// GenerateFromPath generates a manifest with adaptive column support
func (g *GeneratorWithAdapter) GenerateFromPath(path string) (*Manifest, error) {
	if g.adapter != nil {
		g.adapter.Start()
		defer g.adapter.Stop()
	}
	
	// Use the underlying generator
	manifest, err := g.Generator.GenerateFromPath(path)
	if err != nil {
		return nil, err
	}
	
	// If adaptive columns are enabled, scan all entries
	if g.adapter != nil {
		for _, entry := range manifest.Entries {
			g.adapter.ScanEntry(entry)
		}
	}
	
	return manifest, nil
}

// GetC4IDColumn returns the current C4 ID column position
func (g *GeneratorWithAdapter) GetC4IDColumn() int {
	if g.adapter != nil {
		return g.adapter.GetColumn()
	}
	// Fallback to static calculation
	return 80
}