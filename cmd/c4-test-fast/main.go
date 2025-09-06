package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Avalanche-io/c4/c4m"
)

func main() {
	var bundlePath string
	var scanPath string
	
	flag.StringVar(&bundlePath, "bundle", "", "Path to bundle output directory")
	flag.StringVar(&scanPath, "scan", "", "Path to scan")
	flag.Parse()
	
	if scanPath == "" {
		scanPath = flag.Arg(0)
	}
	
	if scanPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s [--bundle <output>] <scan-path>\n", os.Args[0])
		os.Exit(1)
	}
	
	// Use default bundle path if not specified
	if bundlePath == "" {
		bundlePath = filepath.Base(scanPath) + ".c4m_bundle_fast"
	}
	
	fmt.Printf("Creating bundle at: %s\n", bundlePath)
	fmt.Printf("Scanning: %s\n", scanPath)
	fmt.Printf("FAST MODE: Skipping C4 ID computation for structural testing\n")
	
	// Create bundle with path as base name
	bundle, err := c4m.CreateBundle(scanPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create bundle: %v\n", err)
		os.Exit(1)
	}
	
	// Move to desired location if specified
	if bundlePath != bundle.Path {
		if err := os.Rename(bundle.Path, bundlePath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to move bundle: %v\n", err)
			os.Exit(1)
		}
		bundle.Path = bundlePath
	}
	
	// Start scan
	scan, err := bundle.NewScan(scanPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start scan: %v\n", err)
		os.Exit(1)
	}
	
	// Use the new scanner v2 with fast mode enabled
	scanner := c4m.NewScannerV2(bundle, scan, nil)
	scanner.SkipC4IDs = true  // Enable fast mode
	
	// Perform the scan
	if err := scanner.ScanPath(scanPath); err != nil {
		fmt.Fprintf(os.Stderr, "Scan failed: %v\n", err)
		os.Exit(1)
	}
	
	// Complete the scan
	if err := scanner.Complete(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to complete scan: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println("Fast scan completed successfully!")
	
	// Print statistics
	stats := scanner.GetStatistics()
	fmt.Printf("Total entries: %v\n", stats["totalEntries"])
	fmt.Printf("Chunks written: %v\n", stats["chunksWritten"])
}