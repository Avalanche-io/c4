package c4m

import (
	"fmt"
	"os"
	"path/filepath"
)

// CompartmentBundleCLI provides CLI for compartmentalized bundle operations
type CompartmentBundleCLI struct {
	config  *BundleConfig
	verbose bool
}

// NewCompartmentBundleCLI creates a new compartmentalized bundle CLI
func NewCompartmentBundleCLI(config *BundleConfig, verbose bool) *CompartmentBundleCLI {
	if config == nil {
		config = DefaultBundleConfig()
	}
	return &CompartmentBundleCLI{
		config:  config,
		verbose: verbose,
	}
}

// CreateBundle creates a new bundle with compartmentalized chunking
func (cbc *CompartmentBundleCLI) CreateBundle(scanPath string) error {
	// Resolve absolute path
	absPath, err := filepath.Abs(scanPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	
	// Verify path exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}
	
	if cbc.verbose {
		fmt.Fprintf(os.Stderr, "# Creating compartmentalized bundle for: %s\n", absPath)
		fmt.Fprintf(os.Stderr, "# Configuration:\n")
		fmt.Fprintf(os.Stderr, "#   Max entries per chunk: %d\n", cbc.config.MaxEntriesPerChunk)
		fmt.Fprintf(os.Stderr, "#   Max bytes per chunk: %d\n", cbc.config.MaxBytesPerChunk)
		fmt.Fprintf(os.Stderr, "#   Compartment threshold: 50%% of limits\n")
	}
	
	// Create scanner
	scanner, err := NewCompartmentBundleScanner(absPath, cbc.config)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}
	
	fmt.Fprintf(os.Stderr, "# Bundle created: %s\n", scanner.GetBundlePath())
	fmt.Fprintf(os.Stderr, "# Starting compartmentalized scan...\n")
	
	// Run scan
	if err := scanner.Scan(); err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	
	// Report results
	fmt.Fprintf(os.Stderr, "\n✓ Scan complete\n")
	fmt.Fprintf(os.Stderr, "✓ Chunks written: %d\n", scanner.GetChunksWritten())
	fmt.Fprintf(os.Stderr, "✓ Bundle saved to: %s\n", scanner.GetBundlePath())
	
	return nil
}

// ResumeBundle resumes scanning from an existing bundle
func (cbc *CompartmentBundleCLI) ResumeBundle(bundlePath string) error {
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
	
	// TODO: Implement actual resume logic with compartmentalized scanning
	fmt.Fprintf(os.Stderr, "# Resume functionality not yet fully implemented\n")
	fmt.Fprintf(os.Stderr, "# Would resume from last chunk and continue scanning\n")
	
	return nil
}