package scan

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

// These tests exist for the race detector: each exercises one place where
// the scanner or the CLI was touched from two goroutines without
// synchronisation. Run with `go test -race`. Without -race they still
// check that the lifecycle ends cleanly.

func lifecycleTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	createTestFiles(t, dir, []string{
		"a.txt:alpha",
		"b.txt:beta",
		"d/c.txt:gamma",
	})
	return dir
}

// A snapshot taken while Start is still setting the root entry.
func TestRaceSnapshotConcurrentWithStart(t *testing.T) {
	ps := NewProgressiveScanner(lifecycleTree(t))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := ps.Start(); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			var buf bytes.Buffer
			_ = ps.OutputCurrentState(&buf)
		}
	}()
	wg.Wait()
	ps.Stop()
}

// Stop called before, during and after Start. Before: Start then does
// nothing, and Wait returns at once. During: the two race for the
// lifecycle. After: the ordinary order.
func TestRaceStopAroundStart(t *testing.T) {
	t.Run("before", func(t *testing.T) {
		ps := NewProgressiveScanner(lifecycleTree(t))
		ps.Stop()
		if err := ps.Start(); err != nil {
			t.Fatal(err)
		}
		ps.Wait()
	})
	t.Run("during", func(t *testing.T) {
		ps := NewProgressiveScanner(lifecycleTree(t))
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = ps.Start()
		}()
		go func() {
			defer wg.Done()
			ps.Stop()
		}()
		wg.Wait()
		ps.Stop() // whichever ran first, everything Start launched is stopped
	})
	t.Run("after", func(t *testing.T) {
		ps := NewProgressiveScanner(lifecycleTree(t))
		if err := ps.Start(); err != nil {
			t.Fatal(err)
		}
		ps.Stop()
	})
}

// The CLI stopped while Run is starting its progress reporter, with
// progress on (the snapshot test runs with progress off, which is why
// -race never saw this one).
func TestRaceCLIStopWhileReporterStarts(t *testing.T) {
	cli := NewProgressiveCLI(lifecycleTree(t), WithProgress(true), WithOutput(io.Discard, io.Discard))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = cli.Run()
	}()
	go func() {
		defer wg.Done()
		cli.Stop()
	}()
	wg.Wait()
	cli.Stop()
}
