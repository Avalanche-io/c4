package store

// PROBE ONLY, never to land: on #171 (store-split), while one TreeStore
// splits its root, another Puts bytes already stored (the skip-if-present
// path) and Removes objects. Counts each call's errors by kind, so a
// Windows sharing violation during the split's rename shows up.

import (
	"bytes"
	"crypto/rand"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestProbePutRemoveDuringSplit(t *testing.T) {
	kinds := map[string]int{}
	var mu sync.Mutex
	note := func(op string, err error) {
		mu.Lock()
		defer mu.Unlock()
		kinds[op+" calls"]++
		if err == nil {
			return
		}
		k := op + " other: " + err.Error()
		if os.IsNotExist(err) {
			k = op + " not-exist"
		}
		if len(k) > 160 {
			k = k[:50] + "…" + k[len(k)-90:]
		}
		kinds[k]++
	}
	for round := 0; round < 6; round++ {
		dir := t.TempDir()
		w, _ := NewTreeStore(dir)
		w.SetSyncMode(SyncNone)
		var ids []c4.ID
		var data [][]byte
		for i := 0; i < DefaultSplitThreshold; i++ {
			b := make([]byte, 24)
			rand.Read(b)
			id, err := w.Put(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
			data = append(data, b)
		}
		o, _ := NewTreeStore(dir)
		o.SetSyncMode(SyncNone)
		var stop int32
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pass := 0; atomic.LoadInt32(&stop) == 0 || pass < 1; pass++ {
				for i := range ids {
					if i%16 == 0 && pass == 0 { // remove each sixteenth object once
						note("Remove", o.Remove(ids[i]))
						continue
					}
					if i%16 == 0 {
						continue
					}
					_, err := o.Put(bytes.NewReader(data[i])) // already stored: the skip
					note("Put", err)
				}
			}
		}()
		b := make([]byte, 24)
		rand.Read(b)
		w.Put(bytes.NewReader(b))
		atomic.StoreInt32(&stop, 1)
		wg.Wait()
	}
	failed := 0
	for k, n := range kinds {
		t.Logf("PROBE %s/%s: %d × %s", runtime.GOOS, runtime.Version(), n, k)
		if k != "Put calls" && k != "Remove calls" {
			failed += n
		}
	}
	if failed > 0 {
		t.Errorf("PROBE: %d calls failed", failed)
	}
}
