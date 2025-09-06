package c4m

import (
	"fmt"
	"os"
	"path/filepath"
)

// SimpleBundleCLI provides a simple CLI for bundle operations
type SimpleBundleCLI struct {
	config  *BundleConfig
	verbose bool
}

// NewSimpleBundleCLI creates a new simple bundle CLI
func NewSimpleBundleCLI(config *BundleConfig, verbose bool) *SimpleBundleCLI {
	if config == nil {
		config = DefaultBundleConfig()
	}
	return &SimpleBundleCLI{
		config:  config,
		verbose: verbose,
	}
}

// CreateBundle creates a new bundle for the given path
func (sbc *SimpleBundleCLI) CreateBundle(scanPath string) error {
	// Resolve absolute path
	absPath, err := filepath.Abs(scanPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	
	// Verify path exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}
	
	if sbc.verbose {
		fmt.Fprintf(os.Stderr, "# Creating bundle for: %s\n", absPath)
		fmt.Fprintf(os.Stderr, "# Configuration:\n")
		fmt.Fprintf(os.Stderr, "#   Max entries per chunk: %d\n", sbc.config.MaxEntriesPerChunk)
		fmt.Fprintf(os.Stderr, "#   Max bytes per chunk: %d\n", sbc.config.MaxBytesPerChunk)
		fmt.Fprintf(os.Stderr, "#   Max chunk interval: %v\n", sbc.config.MaxChunkInterval)
	}
	
	// Create bundle
	bundle, err := CreateBundle(absPath, sbc.config)
	if err != nil {
		return fmt.Errorf("failed to create bundle: %w", err)
	}
	
	// Start scan
	scan, err := bundle.NewScan(absPath)
	if err != nil {
		return fmt.Errorf("failed to create scan: %w", err)
	}
	
	// Create scanner - use V2 for correct ordering
	scanner := NewScannerV2(bundle, scan, sbc.config)
	
	TimedPrintf("Bundle created: %s\n", bundle.Path)
	TimedPrintln("Starting scan...")
	
	// Run scan
	if err := scanner.ScanPath(absPath); err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	
	// Complete scan
	if err := scanner.Complete(); err != nil {
		return fmt.Errorf("failed to complete scan: %w", err)
	}
	
	// Report results
	stats := scanner.GetStatistics()
	TimedPrintln("")
	TimedPrintln("✓ Scan complete")
	TimedPrintf("✓ Chunks written: %d\n", stats["chunks_written"])
	TimedPrintf("✓ Total entries: %d\n", stats["total_entries"])
	TimedPrintf("✓ Avg entries per chunk: %d\n", stats["avg_entries"])
	TimedPrintf("✓ Bundle saved to: %s\n", bundle.Path)
	
	return nil
}

// ResumeBundle resumes scanning from an existing bundle
func (sbc *SimpleBundleCLI) ResumeBundle(bundlePath string) error {
	// Open existing bundle
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open bundle: %w", err)
	}
	
	// Find or create scan
	scan, err := bundle.ResumeScan()
	if err != nil {
		// No incomplete scan, start new one
		scan, err = bundle.NewScan(bundle.ScanPath)
		if err != nil {
			return fmt.Errorf("failed to create new scan: %w", err)
		}
		fmt.Fprintf(os.Stderr, "# Starting new scan #%d in existing bundle\n", scan.Number)
	} else {
		fmt.Fprintf(os.Stderr, "# Resuming scan #%d\n", scan.Number)
		fmt.Fprintf(os.Stderr, "# Progress chunks found: %d\n", len(scan.ProgressChunks))
	}
	
	// TODO: Implement actual resume logic
	fmt.Fprintf(os.Stderr, "# Resume functionality not yet fully implemented\n")
	fmt.Fprintf(os.Stderr, "# Would resume from last chunk and continue scanning\n")
	
	return nil
}