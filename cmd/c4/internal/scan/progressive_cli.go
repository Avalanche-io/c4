package scan

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

// ProgressiveCLI provides a command-line interface for the progressive scanner
type ProgressiveCLI struct {
	scanner     *ProgressiveScanner
	writer      io.Writer
	errWriter   io.Writer
	verbose     bool
	showProgress bool
	slowMode    bool // Add artificial delays for testing
}

// NewProgressiveCLI creates a new CLI interface
func NewProgressiveCLI(rootPath string, options ...CLIOption) *ProgressiveCLI {
	cli := &ProgressiveCLI{
		scanner:      NewProgressiveScanner(rootPath),
		writer:       os.Stdout,
		errWriter:    os.Stderr,
		showProgress: true,
	}
	
	for _, opt := range options {
		opt(cli)
	}
	
	return cli
}

// CLIOption configures the CLI
type CLIOption func(*ProgressiveCLI)

// WithOutput sets custom output writers
func WithOutput(w, errW io.Writer) CLIOption {
	return func(c *ProgressiveCLI) {
		c.writer = w
		c.errWriter = errW
	}
}

// WithVerbose enables verbose output
func WithVerbose(v bool) CLIOption {
	return func(c *ProgressiveCLI) {
		c.verbose = v
	}
}

// WithProgress enables/disables progress reporting
func WithProgress(p bool) CLIOption {
	return func(c *ProgressiveCLI) {
		c.showProgress = p
	}
}

// WithWorkers sets the number of workers
func WithWorkers(n int) CLIOption {
	return func(c *ProgressiveCLI) {
		c.scanner.numWorkers = n
	}
}

// WithC4Workers sets the number of C4 computation workers
func WithC4Workers(n int) CLIOption {
	return func(c *ProgressiveCLI) {
		c.scanner.c4Workers = n
	}
}

// WithHiddenFiles includes hidden files in the scan
func WithHiddenFiles(h bool) CLIOption {
	return func(c *ProgressiveCLI) {
		c.scanner.includeHidden = h
	}
}

// WithSlowMode enables artificial delays for testing progress display
func WithSlowMode(s bool) CLIOption {
	return func(c *ProgressiveCLI) {
		c.slowMode = s
		c.scanner.slowMode = s
	}
}

// Run starts the progressive scan with CLI features
func (pc *ProgressiveCLI) Run() error {
	// Print instructions
	if pc.verbose {
		fmt.Fprintln(pc.errWriter, "# Starting progressive filesystem scan")
		fmt.Fprintln(pc.errWriter, "# Press Ctrl+C to stop and output results")
		
		// Platform-specific status instructions
		if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" {
			fmt.Fprintln(pc.errWriter, "# Press Ctrl+T for status update (or kill -USR1", os.Getpid(), ")")
		} else {
			fmt.Fprintln(pc.errWriter, "# Send USR1 signal for status: kill -USR1", os.Getpid())
		}
		fmt.Fprintln(pc.errWriter, "#")
	}
	
	// Start scanner
	if err := pc.scanner.Start(); err != nil {
		return err
	}
	
	// Start progress reporter if enabled
	if pc.showProgress {
		go pc.progressReporter()
	}
	
	// Wait for completion or interrupt
	pc.scanner.Wait()
	
	// Output final scan summary if progress reporting was enabled
	if pc.showProgress && !pc.verbose {
		status := pc.scanner.RequestStatus()
		if status != nil && status.Stage == StageComplete {
			// Use a reasonable fallback for elapsed time
			fmt.Fprintf(pc.errWriter, "\n✓ Scan complete: %d items, %d files processed\n",
				status.TotalFound, status.C4Computed)
		}
	}
	
	// Output final results
	if pc.verbose {
		fmt.Fprintln(pc.errWriter, "\n# Scan complete, outputting manifest")
	}
	
	return pc.scanner.OutputCurrentState(pc.writer)
}

// progressReporter periodically reports scan progress
func (pc *ProgressiveCLI) progressReporter() {
	ticker := time.NewTicker(100 * time.Millisecond) // Update more frequently for responsiveness
	defer ticker.Stop()
	
	startTime := time.Now()
	lastStage := ScanStage(-1)
	var stageStartTime time.Time
	
	// Track regular files that need C4 computation
	var regularFileCount int64
	
	// Track stage start values for average rate calculation
	var stageStartStructureCount, stageStartMetadataCount, stageStartC4Count int64
	
	// Initial stage
	stageStartTime = time.Now()
	
	for {
		select {
		case <-ticker.C:
			status := pc.scanner.RequestStatus()
			if status == nil {
				continue
			}
			
			// Calculate overall elapsed time
			elapsed := time.Since(startTime).Seconds()
			
			// Determine which stage to show based on what's actively happening
			// Stages can overlap, so show the one with the most activity
			currentStage := status.Stage
			if status.Stage == StageComplete {
				// Don't show complete until we're really done
				if status.C4Computed < status.RegularFiles {
					currentStage = StageC4ID
				}
			} else if status.Stage >= StageMetadata && status.MetadataScanned < status.TotalFound {
				// Still doing metadata even if some C4 has started
				currentStage = StageMetadata
			} else if status.Stage >= StageC4ID && status.C4Computed < status.RegularFiles {
				// Doing C4 computation
				currentStage = StageC4ID
			}
			
			// Detect stage transitions (use currentStage not status.Stage)
			if currentStage != lastStage {
				// Print summary of completed stage
				if lastStage == StageStructure && currentStage > StageStructure {
					duration := time.Since(stageStartTime).Seconds()
					avgRate := float64(status.TotalFound - stageStartStructureCount) / duration
					fmt.Fprintf(pc.errWriter, "\n✓ Stage 1 complete: Found %d items in %.1fs (%.0f items/s)\n",
						status.TotalFound, duration, avgRate)
					// Only reset if we're actually starting Stage 2 fresh
					if currentStage == StageMetadata {
						stageStartMetadataCount = status.MetadataScanned
						stageStartTime = time.Now()
					}
				} else if lastStage == StageMetadata && currentStage > StageMetadata {
					// Use actual regular file count from scanner
					regularFileCount = status.RegularFiles
					duration := time.Since(stageStartTime).Seconds()
					if duration > 0 && status.MetadataScanned > stageStartMetadataCount {
						avgRate := float64(status.MetadataScanned - stageStartMetadataCount) / duration
						fmt.Fprintf(pc.errWriter, "\n✓ Stage 2 complete: Scanned metadata for %d items, found %d files in %.1fs (%.0f items/s)\n",
							status.MetadataScanned, regularFileCount, duration, avgRate)
					} else {
						fmt.Fprintf(pc.errWriter, "\n✓ Stage 2 complete: Scanned metadata for %d items, found %d files\n",
							status.MetadataScanned, regularFileCount)
					}
					// Only reset if we're actually starting Stage 3 fresh
					if currentStage == StageC4ID {
						stageStartC4Count = status.C4Computed
						stageStartTime = time.Now()
					}
				} else if lastStage == StageC4ID && currentStage > StageC4ID {
					duration := time.Since(stageStartTime).Seconds()
					if duration > 0 && status.C4Computed > stageStartC4Count {
						avgRate := float64(status.C4Computed - stageStartC4Count) / duration
						fmt.Fprintf(pc.errWriter, "\n✓ Stage 3 complete: Computed %d C4 IDs in %.1fs (%.0f files/s)\n",
							status.C4Computed, duration, avgRate)
					} else {
						fmt.Fprintf(pc.errWriter, "\n✓ Stage 3 complete: Computed %d C4 IDs\n",
							status.C4Computed)
					}
				}
				
				lastStage = currentStage
				// Only reset start time if not already reset above
				if lastStage == StageStructure || 
				   (lastStage != StageMetadata && currentStage != StageMetadata) ||
				   (lastStage != StageC4ID && currentStage != StageC4ID) {
					stageStartTime = time.Now()
				}
			}
			
			// Clear current line
			fmt.Fprintf(pc.errWriter, "\r\033[K")
			
			switch currentStage {
			case StageStructure:
				// Stage 1: Structure scanning - use average rate since stage start
				duration := time.Since(stageStartTime).Seconds()
				if duration > 0.1 { // Only show rate after 100ms
					avgRate := float64(status.TotalFound - stageStartStructureCount) / duration
					fmt.Fprintf(pc.errWriter, 
						"Stage 1: Scanning structure... Found: %d items (%.0f/s avg)",
						status.TotalFound, avgRate)
				} else {
					fmt.Fprintf(pc.errWriter, 
						"Stage 1: Scanning structure... Found: %d items",
						status.TotalFound)
				}
					
			case StageMetadata:
				// Stage 2: Metadata collection - use average rate since stage start
				duration := time.Since(stageStartTime).Seconds()
				pct := 0.0
				if status.TotalFound > 0 {
					pct = float64(status.MetadataScanned) * 100.0 / float64(status.TotalFound)
				}
				if duration > 0 {
					avgRate := float64(status.MetadataScanned - stageStartMetadataCount) / duration
					fmt.Fprintf(pc.errWriter,
						"Stage 2: Reading metadata... %d/%d items (%.0f%%, %.0f/s avg)",
						status.MetadataScanned, status.TotalFound, pct, avgRate)
				} else {
					fmt.Fprintf(pc.errWriter,
						"Stage 2: Reading metadata... %d/%d items (%.0f%%)",
						status.MetadataScanned, status.TotalFound, pct)
				}
					
			case StageC4ID:
				// Stage 3: C4 ID computation with progress bar - use average rate
				duration := time.Since(stageStartTime).Seconds()
				var avgRate float64
				if duration > 0 {
					avgRate = float64(status.C4Computed - stageStartC4Count) / duration
				}
				
				// Calculate progress - use actual regular file count
				filesToProcess := status.RegularFiles
				if filesToProcess == 0 && regularFileCount > 0 {
					filesToProcess = regularFileCount
				}
				
				pct := 0.0
				if filesToProcess > 0 {
					pct = float64(status.C4Computed) * 100.0 / float64(filesToProcess)
					if pct > 100 {
						pct = 100
					}
				}
				
				// Calculate ETA based on average rate
				etaStr := ""
				if avgRate > 0 && filesToProcess > status.C4Computed {
					remaining := filesToProcess - status.C4Computed
					etaSecs := float64(remaining) / avgRate
					if etaSecs < 60 {
						etaStr = fmt.Sprintf(" ETA: %.0fs", etaSecs)
					} else {
						etaStr = fmt.Sprintf(" ETA: %.1fm", etaSecs/60)
					}
				}
				
				// Create progress bar
				barWidth := 30
				filled := int(pct * float64(barWidth) / 100.0)
				bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
				
				if duration > 0 {
					fmt.Fprintf(pc.errWriter,
						"Stage 3: Computing C4 IDs... [%s] %d/%d files (%.0f%%, %.0f/s avg)%s",
						bar, status.C4Computed, filesToProcess, pct, avgRate, etaStr)
				} else {
					fmt.Fprintf(pc.errWriter,
						"Stage 3: Computing C4 IDs... [%s] %d/%d files (%.0f%%)%s",
						bar, status.C4Computed, filesToProcess, pct, etaStr)
				}
					
			case StageComplete:
				// Only show complete when everything is truly done
				if status.C4Computed >= status.RegularFiles {
					// Make sure all stage summaries were printed
					if lastStage == StageMetadata {
						// Stage 2 summary wasn't printed yet
						regularFileCount = status.RegularFiles
						duration := time.Since(stageStartTime).Seconds()
						if duration > 0 {
							avgRate := float64(status.MetadataScanned - stageStartMetadataCount) / duration
							fmt.Fprintf(pc.errWriter, "\n✓ Stage 2 complete: Scanned metadata for %d items, found %d files in %.1fs (%.0f items/s)\n",
								status.MetadataScanned, regularFileCount, duration, avgRate)
						}
					} else if lastStage == StageC4ID {
						// Stage 3 summary wasn't printed yet
						duration := time.Since(stageStartTime).Seconds()
						if duration > 0 {
							avgRate := float64(status.C4Computed - stageStartC4Count) / duration
							fmt.Fprintf(pc.errWriter, "\n✓ Stage 3 complete: Computed %d C4 IDs in %.1fs (%.0f files/s)\n",
								status.C4Computed, duration, avgRate)
						}
					}
					// Print total time
					fmt.Fprintf(pc.errWriter, "✓ Total scan time: %.1fs\n", elapsed)
					return
				}
				// Not really complete yet, continue to show progress
			}
			
		case <-pc.scanner.ctx.Done():
			// Clear progress line
			fmt.Fprintf(pc.errWriter, "\r\033[K")
			return
		}
	}
}

// RunWithTimeout runs the scanner with a timeout
func (pc *ProgressiveCLI) RunWithTimeout(timeout time.Duration) error {
	// Start scanner
	if err := pc.scanner.Start(); err != nil {
		return err
	}
	
	// Set up timeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	
	done := make(chan struct{})
	var scanErr error
	
	// Run scan in goroutine
	go func() {
		pc.scanner.Wait()
		close(done)
	}()
	
	// Start progress reporter if enabled
	if pc.showProgress {
		go pc.progressReporter()
	}
	
	// Wait for completion or timeout
	select {
	case <-done:
		if pc.verbose {
			fmt.Fprintln(pc.errWriter, "\n# Scan complete")
		}
		
	case <-timer.C:
		if pc.verbose {
			fmt.Fprintln(pc.errWriter, "\n# Timeout reached, outputting partial results")
		}
		pc.scanner.Stop()
	}
	
	// Output results
	err := pc.scanner.OutputCurrentState(pc.writer)
	if err != nil {
		return err
	}
	
	return scanErr
}

// GetStatus returns the current scan status
func (pc *ProgressiveCLI) GetStatus() *ScanStatus {
	return pc.scanner.RequestStatus()
}

// OutputSnapshot outputs the current state without stopping
func (pc *ProgressiveCLI) OutputSnapshot(w io.Writer) error {
	return pc.scanner.OutputCurrentState(w)
}

// Stop gracefully stops the scanner
func (pc *ProgressiveCLI) Stop() {
	pc.scanner.Stop()
}