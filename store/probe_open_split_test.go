package store

// PROBE ONLY, never to land: on master, while one TreeStore splits its
// root, another opens objects present throughout. Counts the Open errors
// by kind, so a Windows sharing violation (the split's rename holding the
// file) is told from a not-exist (the lookup race #171 fixes).

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestProbeOpenDuringSplit(t *testing.T) {
	kinds := map[string]int{}
	var mu sync.Mutex
	opens := 0
	for round := 0; round < 6; round++ {
		dir := t.TempDir()
		w, _ := NewTreeStore(dir)
		w.SetSyncMode(SyncNone)
		var ids []c4.ID
		for i := 0; i < DefaultSplitThreshold; i++ {
			b := make([]byte, 24)
			rand.Read(b)
			id, err := w.Put(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		o, _ := NewTreeStore(dir)
		var stop int32
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pass := 0; atomic.LoadInt32(&stop) == 0 || pass < 1; pass++ {
				for _, id := range ids {
					f, err := o.Open(id)
					mu.Lock()
					opens++
					if err != nil {
						k := "other: " + err.Error()
						if os.IsNotExist(err) {
							k = "not-exist"
						}
						if len(k) > 160 {
							k = k[:60] + "…" + k[len(k)-80:]
						}
						kinds[k]++
					}
					mu.Unlock()
					if f != nil {
						f.Close()
					}
				}
			}
		}()
		b := make([]byte, 24)
		rand.Read(b)
		w.Put(bytes.NewReader(b))
		atomic.StoreInt32(&stop, 1)
		wg.Wait()
	}
	t.Logf("PROBE %s/%s: %d opens during 6 root splits", runtime.GOOS, runtime.Version(), opens)
	failed := 0
	for k, n := range kinds {
		t.Logf("PROBE %s/%s: %d × %s", runtime.GOOS, runtime.Version(), n, k)
		failed += n
	}
	if failed > 0 {
		t.Errorf("PROBE: %d opens failed", failed)
	}
	_ = fmt.Sprint
}
