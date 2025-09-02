package c4m

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BundleCLI provides command-line interface for bundle scanning
type BundleCLI struct {
	scanner      *BundleScanner
	config       *BundleConfig
	verbose      bool
	showProgress bool
	errWriter    io.Writer
}

// NewBundleCLI creates a new bundle CLI
func NewBundleCLI(options ...BundleCLIOption) *BundleCLI {
	cli := &BundleCLI{
		config:       DefaultBundleConfig(),
		errWriter:    os.Stderr,
		showProgress: true,
	}
	
	for _, opt := range options {
		opt(cli)
	}
	
	return cli
}

// BundleCLIOption configures the bundle CLI
type BundleCLIOption func(*BundleCLI)

// WithBundleConfig sets the bundle configuration
func WithBundleConfig(config *BundleConfig) BundleCLIOption {
	return func(c *BundleCLI) {
		c.config = config
	}
}

// WithBundleVerbose enables verbose output
func WithBundleVerbose(v bool) BundleCLIOption {
	return func(c *BundleCLI) {
		c.verbose = v
	}
}

// WithBundleProgress enables progress reporting
func WithBundleProgress(p bool) BundleCLIOption {
	return func(c *BundleCLI) {
		c.showProgress = p
	}
}

// CreateBundle creates a new bundle for the given path
func (bc *BundleCLI) CreateBundle(scanPath string) error {
	// Resolve absolute path
	absPath, err := filepath.Abs(scanPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	
	if bc.verbose {
		fmt.Fprintf(bc.errWriter, "# Creating new bundle for: %s\n", absPath)
	}
	
	// Create bundle scanner
	scanner, err := NewBundleScanner(absPath, bc.config)
	if err != nil {
		return fmt.Errorf("failed to create bundle: %w", err)
	}
	bc.scanner = scanner
	
	fmt.Fprintf(bc.errWriter, "# Bundle created: %s\n", scanner.bundle.Path)
	fmt.Fprintf(bc.errWriter, "# Starting scan #%d\n", scanner.scan.Number)
	
	return bc.runScan()
}

// ResumeBundle resumes scanning from an existing bundle
func (bc *BundleCLI) ResumeBundle(bundlePath string) error {
	if bc.verbose {
		fmt.Fprintf(bc.errWriter, "# Resuming bundle: %s\n", bundlePath)
	}
	
	// Open existing bundle
	scanner, err := ResumeBundleScanner(bundlePath, bc.config)
	if err != nil {
		return fmt.Errorf("failed to resume bundle: %w", err)
	}
	bc.scanner = scanner
	
	fmt.Fprintf(bc.errWriter, "# Resumed scan #%d in bundle: %s\n", 
		scanner.scan.Number, scanner.bundle.Path)
	
	return bc.runScan()
}

// runScan executes the scanning process
func (bc *BundleCLI) runScan() error {
	// Start scanner
	if err := bc.scanner.Start(); err != nil {
		return fmt.Errorf("failed to start scanner: %w", err)
	}
	
	// Start progress reporter if enabled
	if bc.showProgress {
		go bc.progressReporter()
	}
	
	// Wait for completion
	if err := bc.scanner.Wait(); err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	
	// Report completion
	status := bc.scanner.GetStatus()
	fmt.Fprintf(bc.errWriter, "\n✓ Scan complete: %d items, %d chunks written\n",
		status.TotalFound, status.ChunksWritten)
	fmt.Fprintf(bc.errWriter, "✓ Bundle saved to: %s\n", bc.scanner.bundle.Path)
	
	return nil
}

// progressReporter reports scan progress
func (bc *BundleCLI) progressReporter() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	lastChunks := int64(0)
	
	for {
		select {
		case <-ticker.C:
			status := bc.scanner.GetStatus()
			if status == nil {
				continue
			}
			
			// Clear line and show progress
			fmt.Fprintf(bc.errWriter, "\r\033[K")
			
			// Show chunk progress when chunks are written
			if status.ChunksWritten > lastChunks {
				fmt.Fprintf(bc.errWriter, "✓ Wrote chunk %d (%d entries)\n", 
					status.ChunksWritten, status.TotalFound)
				lastChunks = status.ChunksWritten
			}
			
			// Show current stage progress
			switch status.Stage {
			case StageStructure:
				fmt.Fprintf(bc.errWriter, "Scanning: %d items found", status.TotalFound)
			case StageMetadata:
				pct := float64(status.MetadataScanned) * 100.0 / float64(status.TotalFound)
				fmt.Fprintf(bc.errWriter, "Metadata: %d/%d (%.0f%%)", 
					status.MetadataScanned, status.TotalFound, pct)
			case StageC4ID:
				if status.RegularFiles > 0 {
					pct := float64(status.C4Computed) * 100.0 / float64(status.RegularFiles)
					fmt.Fprintf(bc.errWriter, "C4 IDs: %d/%d files (%.0f%%)", 
						status.C4Computed, status.RegularFiles, pct)
				}
			case StageComplete:
				return
			}
			
			// Add chunks written
			if status.ChunksWritten > 0 {
				fmt.Fprintf(bc.errWriter, " | %d chunks", status.ChunksWritten)
			}
		}
	}
}

// HandleBundleCommand processes bundle-related commands
func HandleBundleCommand(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: c4 --bundle <path> or c4 --bundle --resume <bundle>")
	}
	
	// Parse options
	var (
		resume   bool
		dev      bool
		verbose  bool
		path     string
	)
	
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--resume", "-r":
			resume = true
		case "--dev":
			dev = true
		case "--verbose", "-v":
			verbose = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				path = args[i]
			}
		}
	}
	
	if path == "" {
		return fmt.Errorf("path required")
	}
	
	// Create CLI with options
	var config *BundleConfig
	if dev {
		config = DevBundleConfig()
		fmt.Fprintln(os.Stderr, "# Using development configuration (small chunks)")
	} else {
		config = DefaultBundleConfig()
	}
	
	cli := NewBundleCLI(
		WithBundleConfig(config),
		WithBundleVerbose(verbose),
	)
	
	// Execute command
	if resume {
		return cli.ResumeBundle(path)
	}
	return cli.CreateBundle(path)
}