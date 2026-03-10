package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Avalanche-io/c4/progscan"
	flag "github.com/spf13/pflag"
)

func runScan(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)

	var (
		outFile    string
		level      int
		compact    bool
		background bool
		status     bool
		stop       string
	)

	fs.StringVarP(&outFile, "output", "o", "", "Write c4m to file (default: stdout)")
	fs.IntVar(&level, "level", 2, "Stop after phase: 0=structure, 1=metadata, 2=identity")
	fs.BoolVar(&compact, "compact", false, "Output canonical c4m without padding")
	fs.BoolVarP(&background, "background", "b", false, "Run scan in c4d background")
	fs.BoolVar(&status, "status", false, "Show active scan jobs from c4d")
	fs.StringVar(&stop, "stop", "", "Cancel a running scan job by ID")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `c4 scan - Progressive filesystem scan

Usage:
  c4 scan [options] <path>        # Scan a directory
  c4 scan --status                # List active scans from c4d
  c4 scan --stop <job-id>         # Cancel a running scan

Scans a directory tree and produces a c4m file in three phases:
  Phase 0: Structure discovery (readdir, no stat)
  Phase 1: Metadata resolution (stat for mode, size, timestamps)
  Phase 2: Content identification (C4 hash computation)

The working file is always a valid c4m — null fields indicate
unresolved data. Use --level to stop early for faster results.

With -b, the scan runs in c4d's background and the CLI returns
immediately. Query progress with --status.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  c4 scan ~/projects                    # Full scan to stdout
  c4 scan ~/projects -o project.c4m     # Write to file
  c4 scan ~/projects --level 0          # Structure only (fastest)
  c4 scan ~/projects --level 1          # Structure + metadata
  c4 scan ~/projects --compact          # Canonical c4m output
  c4 scan ~/projects -o p.c4m -b        # Background via c4d
  c4 scan --status                      # List active scans
  c4 scan --stop abc123                 # Cancel a scan
`)
	}

	fs.Parse(args)

	// Status mode: query c4d for active scans.
	if status {
		runScanStatus()
		return
	}

	// Stop mode: cancel a running scan.
	if stop != "" {
		runScanStop(stop)
		return
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	root := fs.Arg(0)

	// Resolve the root path.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: %v\n", err)
		os.Exit(1)
	}
	fi, err := os.Stat(absRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: %v\n", err)
		os.Exit(1)
	}
	if !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "c4 scan: %s is not a directory\n", root)
		os.Exit(1)
	}

	// Background mode: send to c4d.
	if background {
		if outFile == "" {
			fmt.Fprintf(os.Stderr, "c4 scan: -o required for background mode\n")
			os.Exit(1)
		}
		absOut, _ := filepath.Abs(outFile)
		runScanBackground(absRoot, absOut, level)
		return
	}

	// Foreground mode: scan locally.
	runScanForeground(absRoot, outFile, level, compact)
}

func runScanForeground(absRoot, outFile string, level int, compact bool) {
	// Determine output: file or temp file (for stdout mode).
	workFile := outFile
	toStdout := outFile == ""
	if toStdout {
		tmp, err := os.CreateTemp("", "c4scan-*.c4m")
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4 scan: create temp: %v\n", err)
			os.Exit(1)
		}
		workFile = tmp.Name()
		tmp.Close()
		defer os.Remove(workFile)
	}

	sc, err := progscan.New(absRoot, workFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: %v\n", err)
		os.Exit(1)
	}
	defer sc.Close()

	// Phase 0: Structure.
	start := time.Now()
	if err := sc.Phase0(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 0: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Phase 0: %d files, %d dirs (%s)\n",
		sc.Files, sc.Dirs, time.Since(start).Round(time.Millisecond))

	if level < 1 {
		outputResult(sc, workFile, toStdout, compact)
		return
	}

	// Phase 1: Metadata.
	start = time.Now()
	if err := sc.Phase1(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 1: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Phase 1: metadata (%s)\n",
		time.Since(start).Round(time.Millisecond))

	if level < 2 {
		outputResult(sc, workFile, toStdout, compact)
		return
	}

	// Phase 2: Identity.
	start = time.Now()
	if err := sc.Phase2(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 2: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Phase 2: C4 IDs (%s)\n",
		time.Since(start).Round(time.Millisecond))

	outputResult(sc, workFile, toStdout, compact)
}

func runScanBackground(root, outPath string, level int) {
	if !c4dConfigured() {
		fmt.Fprintf(os.Stderr, "c4 scan: c4d not configured — background scan requires c4d\n")
		os.Exit(1)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"root":     root,
		"out_path": outPath,
		"level":    level,
	})

	initC4dConnection()
	resp, err := c4dClient.Post(c4dAddr()+"/etc/scans", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: c4d not reachable: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "c4 scan: c4d: %s\n", resp.Status)
		os.Exit(1)
	}

	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	fmt.Printf("Scan started: %s\n", result.ID)
	fmt.Printf("Output: %s\n", outPath)
	fmt.Fprintf(os.Stderr, "Query progress: c4 scan --status\n")
}

func runScanStatus() {
	if !c4dConfigured() {
		fmt.Fprintf(os.Stderr, "c4 scan: c4d not configured\n")
		os.Exit(1)
	}

	initC4dConnection()
	resp, err := c4dClient.Get(c4dAddr() + "/etc/scans")
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: c4d not reachable: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var jobs []struct {
		ID      string `json:"id"`
		Root    string `json:"root"`
		OutPath string `json:"out_path"`
		Status  string `json:"status"`
		Files   int    `json:"files"`
		Dirs    int    `json:"dirs"`
		Error   string `json:"error,omitempty"`
	}

	json.NewDecoder(resp.Body).Decode(&jobs)

	if len(jobs) == 0 {
		fmt.Println("No active scans")
		return
	}

	for _, j := range jobs {
		fmt.Printf("%-18s %-10s %d files, %d dirs  %s\n", j.ID, j.Status, j.Files, j.Dirs, j.Root)
		if j.Error != "" {
			fmt.Printf("  error: %s\n", j.Error)
		}
	}
}

func runScanStop(id string) {
	if !c4dConfigured() {
		fmt.Fprintf(os.Stderr, "c4 scan: c4d not configured\n")
		os.Exit(1)
	}

	initC4dConnection()
	req, _ := http.NewRequest("DELETE", c4dAddr()+"/etc/scans/"+id, nil)
	resp, err := c4dClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: c4d not reachable: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "c4 scan: job %s not found\n", id)
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusNoContent {
		fmt.Fprintf(os.Stderr, "c4 scan: %s\n", resp.Status)
		os.Exit(1)
	}
	fmt.Printf("Canceled: %s\n", id)
}

func outputResult(sc *progscan.Scanner, workFile string, toStdout, compact bool) {
	if toStdout {
		if compact {
			if err := sc.Compact(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "c4 scan: compact: %v\n", err)
				os.Exit(1)
			}
		} else {
			data, err := os.ReadFile(workFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "c4 scan: read: %v\n", err)
				os.Exit(1)
			}
			os.Stdout.Write(data)
		}
		return
	}

	// Writing to file — compact if requested.
	if compact {
		compactPath := workFile + ".compact"
		f, err := os.Create(compactPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4 scan: %v\n", err)
			os.Exit(1)
		}
		if err := sc.Compact(f); err != nil {
			f.Close()
			os.Remove(compactPath)
			fmt.Fprintf(os.Stderr, "c4 scan: compact: %v\n", err)
			os.Exit(1)
		}
		f.Close()
		sc.Close()
		if err := os.Rename(compactPath, workFile); err != nil {
			fmt.Fprintf(os.Stderr, "c4 scan: rename: %v\n", err)
			os.Exit(1)
		}
	}

	fi, _ := os.Stat(workFile)
	fmt.Fprintf(os.Stderr, "Wrote %s (%s)\n", workFile, formatBytes(fi.Size()))
}
