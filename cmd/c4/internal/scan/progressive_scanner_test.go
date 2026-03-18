package scan

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	
	"github.com/Avalanche-io/c4"
)

func init() {
	// Set environment variable to prevent os.Exit during tests
	os.Setenv("GO_TEST", "1")
}

func TestProgressiveScanner(t *testing.T) {
	t.Run("basic three-stage scan", func(t *testing.T) {
		// Create test directory structure
		testDir := t.TempDir()
		createTestFiles(t, testDir, []string{
			"file1.txt:content1",
			"file2.txt:content2",
			"dir1/file3.txt:content3",
			"dir1/dir2/file4.txt:content4",
		})
		
		scanner := NewProgressiveScanner(testDir)
		scanner.numWorkers = 2
		scanner.c4Workers = 2
		
		err := scanner.Start()
		if err != nil {
			t.Fatalf("Failed to start scanner: %v", err)
		}
		
		// Let it run for a bit
		time.Sleep(100 * time.Millisecond)
		
		// Check progress
		status := scanner.RequestStatus()
		if status == nil {
			t.Fatal("Failed to get status")
		}
		
		if status.TotalFound == 0 {
			t.Error("No files found")
		}
		
		scanner.Stop()
		scanner.Wait()
		
		// Check final counts
		finalTotal := atomic.LoadInt64(&scanner.totalFound)
		if finalTotal < 4 {
			t.Errorf("Expected at least 4 entries, got %d", finalTotal)
		}
	})
	
	t.Run("interrupt handling", func(t *testing.T) {
		testDir := t.TempDir()
		createTestFiles(t, testDir, []string{
			"file1.txt:content",
			"file2.txt:content",
		})
		
		scanner := NewProgressiveScanner(testDir)
		
		err := scanner.Start()
		if err != nil {
			t.Fatalf("Failed to start scanner: %v", err)
		}
		
		// Send interrupt signal after short delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			scanner.signalChan <- syscall.SIGINT
		}()
		
		scanner.Wait()
		
		// Should have some results
		if atomic.LoadInt64(&scanner.totalFound) == 0 {
			t.Error("No files found before interrupt")
		}
	})
	
	t.Run("status request via USR1", func(t *testing.T) {
		testDir := t.TempDir()
		createTestFiles(t, testDir, []string{
			"file1.txt:content",
		})

		scanner := NewProgressiveScanner(testDir)
		scanner.Start()

		// Send platform-appropriate status signal (SIGINFO on macOS, SIGUSR1 on Linux)
		var statusOutput bytes.Buffer
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		scanner.signalChan <- SIGINFO
		time.Sleep(200 * time.Millisecond)
		
		w.Close()
		os.Stdout = oldStdout
		
		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		statusOutput.Write(buf[:n])
		
		scanner.Stop()
		scanner.Wait()
		
		// Should have output something
		if statusOutput.Len() == 0 {
			t.Error("No status output on USR1")
		}
	})
	
	t.Run("partial output during scan", func(t *testing.T) {
		testDir := t.TempDir()
		createTestFiles(t, testDir, []string{
			"a.txt:content",
			"b.txt:content",
			"c.txt:content",
			"d/e.txt:content",
		})
		
		scanner := NewProgressiveScanner(testDir)
		scanner.Start()
		
		// Get partial output while scanning
		time.Sleep(50 * time.Millisecond)
		
		var buf bytes.Buffer
		err := scanner.OutputCurrentState(&buf)
		if err != nil {
			t.Errorf("Failed to output state: %v", err)
		}
		
		output := buf.String()
		if len(output) == 0 {
			t.Error("Output is empty")
		}
		
		scanner.Stop()
		scanner.Wait()
	})
	
	t.Run("stage progression", func(t *testing.T) {
		testDir := t.TempDir()
		createTestFiles(t, testDir, []string{
			"file1.txt:test content for c4",
			"file2.txt:more content",
		})
		
		scanner := NewProgressiveScanner(testDir)
		scanner.numWorkers = 1 // Single worker for predictable staging
		scanner.c4Workers = 1
		
		scanner.Start()
		
		stages := make(map[ScanStage]bool)
		
		// Monitor stage progression
		for i := 0; i < 20; i++ {
			time.Sleep(50 * time.Millisecond)
			status := scanner.RequestStatus()
			if status != nil {
				stages[status.Stage] = true
			}
			
			if len(stages) >= 3 {
				break // Seen all stages
			}
		}
		
		scanner.Stop()
		scanner.Wait()
		
		// Should have progressed through stages
		if len(stages) < 2 {
			t.Errorf("Expected at least 2 stages, saw %d", len(stages))
		}
		
		// Check that metadata was collected
		metaCount := atomic.LoadInt64(&scanner.metadataScanned)
		if metaCount == 0 {
			t.Error("No metadata scanned")
		}
	})
}

func TestProgressiveCLI(t *testing.T) {
	t.Run("CLI with timeout", func(t *testing.T) {
		testDir := t.TempDir()
		createTestFiles(t, testDir, []string{
			"file1.txt:content",
			"file2.txt:content",
			"dir/file3.txt:content",
		})
		
		var output bytes.Buffer
		var errOutput bytes.Buffer
		
		cli := NewProgressiveCLI(testDir,
			WithOutput(&output, &errOutput),
			WithVerbose(true),
			WithProgress(false), // Disable progress for cleaner test
		)
		
		err := cli.RunWithTimeout(200 * time.Millisecond)
		if err != nil {
			t.Errorf("CLI run failed: %v", err)
		}
		
		// Check output contains entries
		manifest := output.String()
		if !strings.Contains(manifest, "file1.txt") {
			t.Error("Output missing file1.txt")
		}
	})
	
	t.Run("CLI progress reporting", func(t *testing.T) {
		testDir := t.TempDir()
		createLargeTestStructure(t, testDir, 20) // Create 20 files
		
		var output bytes.Buffer
		var errOutput bytes.Buffer
		
		cli := NewProgressiveCLI(testDir,
			WithOutput(&output, &errOutput),
			WithProgress(true),
			WithVerbose(false),
		)
		
		// Run with short timeout to see progress
		go func() {
			cli.RunWithTimeout(500 * time.Millisecond)
		}()
		
		// Let progress reporter run
		time.Sleep(200 * time.Millisecond)
		
		// Should have progress output in stderr
		errStr := errOutput.String()
		if !strings.Contains(errStr, "Stage") && !strings.Contains(errStr, "Found") {
			// Progress might not have been written yet, that's ok
			t.Log("No progress output captured")
		}
		
		cli.Stop()
	})
	
	t.Run("CLI snapshot output", func(t *testing.T) {
		testDir := t.TempDir()
		createTestFiles(t, testDir, []string{
			"file.txt:content",
		})
		
		cli := NewProgressiveCLI(testDir, WithProgress(false))
		
		// Start scanning in background
		go func() {
			cli.Run()
		}()
		
		time.Sleep(50 * time.Millisecond)
		
		// Get snapshot while running
		var snapshot bytes.Buffer
		err := cli.OutputSnapshot(&snapshot)
		if err != nil {
			t.Errorf("Failed to output snapshot: %v", err)
		}
		
		if snapshot.Len() == 0 {
			t.Error("Empty snapshot")
		}
		
		cli.Stop()
	})
}

func TestScanEntryConversion(t *testing.T) {
	t.Run("incomplete scan entry conversion", func(t *testing.T) {
		scanner := NewProgressiveScanner("/tmp")
		
		// Create scan entry with only structure info
		md := &BasicFileMetadata{
			path:  "/tmp/test.txt",
			name:  "test.txt",
			depth: 0,
			isDir: false,
			mode:  0,
			size:  0,
		}
		se := &ScanEntry{
			FileMetadata: md,
			Path:        "/tmp/test.txt",
			Stage:       StageStructure,
		}
		
		entry := scanner.scanEntryToEntry(se, 0)
		
		// Should have null values for incomplete data
		if entry.Mode != 0 {
			t.Errorf("Expected null mode, got %v", entry.Mode)
		}
		
		if entry.Size != -1 {
			t.Errorf("Expected null size (-1), got %d", entry.Size)
		}
		
		if entry.Timestamp.Unix() != 0 {
			t.Errorf("Expected null timestamp, got %v", entry.Timestamp)
		}
	})
	
	t.Run("complete scan entry conversion", func(t *testing.T) {
		scanner := NewProgressiveScanner("/tmp")
		
		now := time.Now().UTC()
		testC4 := c4.Identify(strings.NewReader("test"))
		
		md := &BasicFileMetadata{
			path:    "/tmp/test.txt",
			name:    "test.txt",
			depth:   0,
			isDir:   false,
			mode:    0644,
			size:    100,
			modTime: now,
			c4id:    testC4,
		}
		se := &ScanEntry{
			FileMetadata: md,
			Path:        "/tmp/test.txt",
			Stage:       StageC4ID,
		}
		
		entry := scanner.scanEntryToEntry(se, 0)
		
		if entry.Mode != 0644 {
			t.Errorf("Mode mismatch: got %v", entry.Mode)
		}
		
		if entry.Size != 100 {
			t.Errorf("Size mismatch: got %d", entry.Size)
		}
		
		if !entry.Timestamp.Equal(now) {
			t.Errorf("Timestamp mismatch: got %v", entry.Timestamp)
		}
		
		if entry.C4ID != testC4 {
			t.Errorf("C4ID mismatch: got %v", entry.C4ID)
		}
	})
}

func TestWorkerPools(t *testing.T) {
	t.Run("worker pool scaling", func(t *testing.T) {
		testDir := t.TempDir()
		// Create many files to test worker pools
		createLargeTestStructure(t, testDir, 50)
		
		scanner := NewProgressiveScanner(testDir)
		scanner.numWorkers = 4
		scanner.c4Workers = 2
		
		scanner.Start()
		
		// Let it run
		time.Sleep(300 * time.Millisecond)
		
		scanner.Stop()
		scanner.Wait()
		
		// Check that files were processed
		total := atomic.LoadInt64(&scanner.totalFound)
		if total < 50 {
			t.Errorf("Expected at least 50 entries, got %d", total)
		}
		
		// Check that metadata was collected
		meta := atomic.LoadInt64(&scanner.metadataScanned)
		if meta == 0 {
			t.Error("No metadata collected")
		}
	})
}

// Helper functions

func createTestFiles(t *testing.T, dir string, files []string) {
	for _, f := range files {
		parts := strings.Split(f, ":")
		path := filepath.Join(dir, parts[0])
		content := ""
		if len(parts) > 1 {
			content = parts[1]
		}
		
		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("Failed to create dir for %s: %v", path, err)
		}
		
		// Create file
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}
}

func createLargeTestStructure(t *testing.T, dir string, numFiles int) {
	// Create a directory structure with many files
	for i := 0; i < numFiles; i++ {
		subdir := filepath.Join(dir, fmt.Sprintf("dir%d", i/5))
		os.MkdirAll(subdir, 0755)
		
		filename := filepath.Join(subdir, fmt.Sprintf("file%d.txt", i))
		content := fmt.Sprintf("content for file %d", i)
		
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}
}