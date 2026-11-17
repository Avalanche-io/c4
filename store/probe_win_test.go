package store

// PROBE ONLY, never to land: measures, on master's TreeStore, whether
// concurrent Puts of the same bytes from separate TreeStore instances (as
// separate processes would be) fail, and whether a rename onto an existing
// file fails while that file is held open. Split is kept out of it.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Avalanche-io/c4"
)

func probeSameIDPuts(t *testing.T, withReader bool) (puts, fails int64, first string) {
	const rounds, writers = 60, 8
	shared := make([][]byte, 12)
	for i := range shared {
		shared[i] = []byte(fmt.Sprintf("probe shared content %d", i))
	}
	var nPut, nFail int64
	var mu sync.Mutex
	for r := 0; r < rounds; r++ {
		dir := t.TempDir()
		var wg sync.WaitGroup
		stop, done := make(chan struct{}), make(chan struct{})
		if withReader {
			rs, err := NewTreeStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				defer close(done)
				for {
					select {
					case <-stop:
						return
					default:
					}
					for _, b := range shared {
						f, err := rs.Open(c4.Identify(bytes.NewReader(b)))
						if err == nil {
							io.Copy(io.Discard, f)
							f.Close()
						}
					}
				}
			}()
		}
		for k := 0; k < writers; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				s, err := NewTreeStore(dir)
				if err != nil {
					t.Error(err)
					return
				}
				s.SetSplitThreshold(1 << 20)
				s.SetSyncMode(SyncNone)
				for i := 0; i < len(shared); i++ {
					atomic.AddInt64(&nPut, 1)
					if _, err := s.Put(bytes.NewReader(shared[(i+k)%len(shared)])); err != nil {
						atomic.AddInt64(&nFail, 1)
						mu.Lock()
						if first == "" {
							first = err.Error()
						}
						mu.Unlock()
					}
				}
			}(k)
		}
		wg.Wait()
		close(stop)
		if withReader {
			<-done
		}
	}
	return nPut, nFail, first
}

func TestProbeSameIDPutsNoReader(t *testing.T) {
	p, f, e := probeSameIDPuts(t, false)
	t.Logf("PROBE %s/%s same-ID Puts, no reader: %d of %d failed; first: %s", runtime.GOOS, runtime.Version(), f, p, e)
	if f > 0 {
		t.Errorf("PROBE: %d same-ID Puts failed", f)
	}
}

func TestProbeSameIDPutsWithReader(t *testing.T) {
	p, f, e := probeSameIDPuts(t, true)
	t.Logf("PROBE %s/%s same-ID Puts, a reader opening them: %d of %d failed; first: %s", runtime.GOOS, runtime.Version(), f, p, e)
	if f > 0 {
		t.Errorf("PROBE: %d same-ID Puts failed", f)
	}
}

// The OS fact underneath: os.Rename onto an existing file, held open by
// os.Open, and not held (control).
func TestProbeRenameOntoOpenFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "dst")
	for _, held := range []bool{false, true} {
		os.WriteFile(dst, []byte("same"), 0600)
		src := filepath.Join(dir, "src")
		os.WriteFile(src, []byte("same"), 0600)
		var f *os.File
		if held {
			f, _ = os.Open(dst)
		}
		err := os.Rename(src, dst)
		t.Logf("PROBE %s/%s rename onto existing file, held open=%v: err=%v", runtime.GOOS, runtime.Version(), held, err)
		if f != nil {
			f.Close()
		}
		os.Remove(src)
	}
}
