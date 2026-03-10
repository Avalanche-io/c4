package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/progscan"
	flag "github.com/spf13/pflag"
)

// stderrIsTerminal is true when stderr is a terminal (supports color).
var stderrIsTerminal bool

func init() {
	fi, err := os.Stderr.Stat()
	if err == nil {
		stderrIsTerminal = fi.Mode()&os.ModeCharDevice != 0
	}
}

// phaseLog writes a dim-colored message to stderr when it's a terminal.
func phaseLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if stderrIsTerminal {
		fmt.Fprintf(os.Stderr, "\033[2m%s\033[0m\n", msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", msg)
	}
}

func runScan(args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "ls":
			runScanLs(args[1:])
			return
		case "rm":
			runScanRm(args[1:])
			return
		case "status":
			runScanStatusCmd(args[1:])
			return
		}
	}

	fs := flag.NewFlagSet("scan", flag.ExitOnError)

	var (
		outFile string
		level   int
		compact bool
	)

	fs.StringVarP(&outFile, "output", "o", "", "Write c4m to file (default: stdout)")
	fs.IntVar(&level, "level", 2, "Stop after phase: 0=structure, 1=metadata, 2=identity")
	fs.BoolVar(&compact, "compact", false, "Output canonical c4m without padding")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `c4 scan - Progressive filesystem scan

Usage:
  c4 scan [options] <path>        # Scan a directory
  c4 scan ls                      # List active scans from c4d
  c4 scan rm <job-id>             # Cancel a running scan
  c4 scan status <file.c4m>       # Show resolution progress

Scans a directory tree and produces a c4m file in three phases:
  Phase 0: Structure discovery (readdir, no stat)
  Phase 1: Metadata resolution (stat for mode, size, timestamps)
  Phase 2: Content identification (C4 hash computation)

The working file is always a valid c4m — null fields indicate
unresolved data. Use --level to stop early for faster results.

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
  c4 scan ls                            # List active scans
  c4 scan rm abc123                     # Cancel a scan
  c4 scan status project.c4m            # Check scan progress
`)
	}

	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	root := fs.Arg(0)

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

	runScanForeground(absRoot, outFile, level, compact)
}

func runScanForeground(absRoot, outFile string, level int, compact bool) {
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

	// For level 0 to stdout (non-compact), stream display lines during
	// the walk so output begins immediately with no buffering delay.
	streamedPhase0 := false
	if toStdout && level == 0 && !compact {
		sc.Display = os.Stdout
		streamedPhase0 = true
	}

	start := time.Now()
	if err := sc.Phase0(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 0: %v\n", err)
		os.Exit(1)
	}
	phaseLog("Phase 0: %d files, %d dirs (%s)",
		sc.Files, sc.Dirs, time.Since(start).Round(time.Millisecond))

	if level < 1 {
		if !streamedPhase0 {
			outputResult(sc, workFile, toStdout, compact)
		}
		return
	}

	start = time.Now()
	if err := sc.Phase1(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 1: %v\n", err)
		os.Exit(1)
	}
	phaseLog("Phase 1: metadata (%s)",
		time.Since(start).Round(time.Millisecond))

	if level < 2 {
		outputResult(sc, workFile, toStdout, compact)
		return
	}

	start = time.Now()
	if err := sc.Phase2(); err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan: phase 2: %v\n", err)
		os.Exit(1)
	}
	phaseLog("Phase 2: C4 IDs (%s)",
		time.Since(start).Round(time.Millisecond))

	outputResult(sc, workFile, toStdout, compact)
}

// runScanLs lists active scan jobs from c4d in c4m format.
func runScanLs(args []string) {
	if !c4dConfigured() {
		fmt.Fprintf(os.Stderr, "c4 scan ls: c4d not configured\n")
		os.Exit(1)
	}

	initC4dConnection()
	resp, err := c4dClient.Get(c4dAddr() + "/etc/scans")
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan ls: c4d not reachable: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var jobs []struct {
		ID        string    `json:"id"`
		Root      string    `json:"root"`
		OutPath   string    `json:"out_path"`
		Status    string    `json:"status"`
		Files     int       `json:"files"`
		Dirs      int       `json:"dirs"`
		StartedAt time.Time `json:"started_at"`
		Error     string    `json:"error,omitempty"`
	}

	json.NewDecoder(resp.Body).Decode(&jobs)

	if len(jobs) == 0 {
		fmt.Println("No active scans")
		return
	}

	// Output as c4m format: each scan is a directory entry.
	m := c4m.NewManifest()
	for _, j := range jobs {
		name := j.Root
		if name != "" && name[len(name)-1] != '/' {
			name += "/"
		}
		ts := j.StartedAt
		if ts.IsZero() {
			ts = c4m.NullTimestamp()
		}
		m.AddEntry(&c4m.Entry{
			Mode:      os.ModeDir,
			Timestamp: ts,
			Size:      -1,
			Name:      name,
		})
	}

	enc := c4m.NewEncoder(os.Stdout)
	enc.Encode(m)
}

// runScanRm cancels a running scan job.
func runScanRm(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 scan rm <job-id>\n")
		os.Exit(1)
	}
	id := args[0]

	if !c4dConfigured() {
		fmt.Fprintf(os.Stderr, "c4 scan rm: c4d not configured\n")
		os.Exit(1)
	}

	initC4dConnection()
	req, _ := http.NewRequest("DELETE", c4dAddr()+"/etc/scans/"+id, nil)
	resp, err := c4dClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan rm: c4d not reachable: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "c4 scan rm: job %s not found\n", id)
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusNoContent {
		fmt.Fprintf(os.Stderr, "c4 scan rm: %s\n", resp.Status)
		os.Exit(1)
	}
	fmt.Printf("Canceled: %s\n", id)
}

// runScanStatusCmd reads a c4m file and reports resolution progress.
func runScanStatusCmd(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 scan status <file.c4m>\n")
		os.Exit(1)
	}

	prog, err := progscan.ReadProgress(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4 scan status: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Entries:  %d (%d files, %d dirs)\n", prog.Total, prog.Files, prog.Dirs)
	fmt.Printf("Metadata: %d/%d", prog.HasMeta, prog.Total)
	if prog.HasMeta == prog.Total {
		fmt.Print(" (complete)")
	}
	fmt.Println()

	fmt.Printf("C4 IDs:   %d/%d", prog.HasC4ID, prog.Files)
	if prog.HasC4ID == prog.Files {
		fmt.Print(" (complete)")
	}
	fmt.Println()

	if prog.TotalBytes > 0 {
		fmt.Printf("Size:     %s\n", formatBytes(prog.TotalBytes))
	}

	fmt.Println(prog.Bar(40))
}

// runScanStart sends a scan request to c4d for background execution.
func runScanStart(root, outPath string, level int) {
	if !c4dConfigured() {
		fmt.Fprintf(os.Stderr, "c4 scan: c4d not configured\n")
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
	phaseLog("Query progress: c4 scan status %s", outPath)
}

func outputResult(sc *progscan.Scanner, workFile string, toStdout, compact bool) {
	if toStdout {
		if compact {
			if err := sc.Compact(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "c4 scan: compact: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Decode and write with aligned C4 ID column.
			m, err := sc.DecodeWorking()
			if err != nil {
				fmt.Fprintf(os.Stderr, "c4 scan: decode: %v\n", err)
				os.Exit(1)
			}
			if err := progscan.DisplayWrite(os.Stdout, m); err != nil {
				fmt.Fprintf(os.Stderr, "c4 scan: write: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

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
	phaseLog("Wrote %s (%s)", workFile, formatBytes(fi.Size()))
}
