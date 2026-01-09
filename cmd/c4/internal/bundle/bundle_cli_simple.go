package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	scanner := NewBundleScannerImpl(bundle, scan, sbc.config)
	
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

// findResumePath determines the last processed path from the manifest
func (sbc *SimpleBundleCLI) findResumePath(manifest *Manifest) string {
	if len(manifest.Entries) == 0 {
		return ""
	}

	// Find the deepest directory that was being processed
	deepestDir := ""
	maxDepth := -1

	for _, entry := range manifest.Entries {
		if entry.Depth > maxDepth {
			maxDepth = entry.Depth
			// Build the path from the entry
			if entry.Mode.IsDir() || strings.HasSuffix(entry.Name, "/") {
				deepestDir = strings.TrimSuffix(entry.Name, "/")
			}
		}
	}

	return deepestDir
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
	
	// Load the last chunk to determine resume point
	var lastManifest *Manifest
	var resumePath string

	if len(scan.ProgressChunks) > 0 {
		// Load the last chunk to find where we left off
		lastChunkName := scan.ProgressChunks[len(scan.ProgressChunks)-1]
		lastChunkPath := filepath.Join(bundle.Path, "c4", lastChunkName)

		data, err := os.ReadFile(lastChunkPath)
		if err != nil {
			return fmt.Errorf("failed to read last chunk: %w", err)
		}

		parser := NewParser(strings.NewReader(string(data)))
		lastManifest, err = parser.ParseAll()
		if err != nil {
			return fmt.Errorf("failed to parse last chunk: %w", err)
		}

		// Find the deepest path processed
		resumePath = sbc.findResumePath(lastManifest)
		fmt.Fprintf(os.Stderr, "# Last processed path: %s\n", resumePath)
	} else {
		// No chunks yet, start from beginning
		resumePath = bundle.ScanPath
	}

	// Create scanner with resume support
	config := sbc.config
	if config == nil {
		config = DefaultBundleConfig()
	}

	// Use Scanner for actual scanning
	scanner := NewBundleScannerImpl(bundle, scan, config)
	scanner.SkipC4IDs = false // Default to computing C4 IDs

	// Resume scanning from the last position
	fmt.Fprintf(os.Stderr, "# Resuming scan from: %s\n", resumePath)
	if err := scanner.ScanPath(bundle.ScanPath); err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Mark scan as complete
	now := time.Now()
	scan.CompletedAt = &now
	bundle.writeHeader()

	fmt.Fprintf(os.Stderr, "# Scan resumed and completed successfully\n")
	return nil
}