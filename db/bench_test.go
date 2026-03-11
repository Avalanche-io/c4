package db

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Avalanche-io/c4"
)

// --- Helpers ---

func benchID(i int) c4.ID {
	return c4.Identify(strings.NewReader(fmt.Sprintf("bench-%d", i)))
}

func openBenchDB(b *testing.B) *DB {
	b.Helper()
	db, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { db.Close() })
	return db
}

func populatedTree(n int) *node {
	root := defaultRoot()
	for i := 0; i < n; i++ {
		dir := fmt.Sprintf("mnt/dir-%d", i%10)
		sub := fmt.Sprintf("sub-%d", (i/10)%10)
		leaf := fmt.Sprintf("leaf-%d", i)
		root = root.put([]string{"mnt", fmt.Sprintf("dir-%d", i%10), sub, leaf}, benchID(i))
		_ = dir
	}
	return root
}

// --- Snapshot (atomic pointer load) ---

func BenchmarkSnapshot(b *testing.B) {
	db := openBenchDB(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := db.Snapshot()
		s.Release()
	}
}

// --- Resolve at various tree depths ---

func BenchmarkResolve(b *testing.B) {
	depths := []struct {
		name string
		path []string
	}{
		{"depth1", []string{"mnt"}},
		{"depth2", []string{"mnt", "dir-0"}},
		{"depth3", []string{"mnt", "dir-0", "sub-0", "leaf-0"}},
	}

	root := populatedTree(1000)

	for _, tc := range depths {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				root.resolve(tc.path)
			}
		})
	}
}

// --- Resolve miss ---

func BenchmarkResolveMiss(b *testing.B) {
	root := populatedTree(1000)
	path := []string{"mnt", "dir-0", "sub-0", "nonexistent"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root.resolve(path)
	}
}

// --- COW Put ---

func BenchmarkPut(b *testing.B) {
	for _, size := range []int{10, 100, 1000, 10000} {
		b.Run(fmt.Sprintf("tree_%d", size), func(b *testing.B) {
			root := populatedTree(size)
			id := benchID(999999)
			path := []string{"mnt", "newdir", "newfile"}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				root.put(path, id)
			}
		})
	}
}

// --- COW Del ---

func BenchmarkDel(b *testing.B) {
	root := populatedTree(1000)
	id := benchID(0)
	root = root.put([]string{"mnt", "dir-0", "sub-0", "leaf-0"}, id)
	path := []string{"mnt", "dir-0", "sub-0", "leaf-0"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root.del(path)
	}
}

// --- Full transaction ---

func BenchmarkUpdateTx(b *testing.B) {
	db := openBenchDB(b)
	db.Update(func(tx *Tx) error {
		for i := 0; i < 100; i++ {
			tx.Put(fmt.Sprintf("mnt/dir/file-%d", i), benchID(i))
		}
		return nil
	})
	id := benchID(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.Update(func(tx *Tx) error {
			return tx.Put(fmt.Sprintf("mnt/bench/%d", i), id)
		})
	}
}

// --- Concurrent reads ---

func BenchmarkConcurrentReads(b *testing.B) {
	db := openBenchDB(b)
	db.Update(func(tx *Tx) error {
		return tx.Put("mnt/target", benchID(0))
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			snap := db.Snapshot()
			snap.Resolve("mnt/target")
			snap.Release()
		}
	})
}

// --- Concurrent readers + single writer ---

func BenchmarkConcurrentReadWrite(b *testing.B) {
	db := openBenchDB(b)
	db.Update(func(tx *Tx) error {
		return tx.Put("mnt/file", benchID(0))
	})

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				db.Update(func(tx *Tx) error {
					return tx.Put(fmt.Sprintf("mnt/w/%d", i), benchID(i+100000))
				})
				i++
			}
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			snap := db.Snapshot()
			snap.Resolve("mnt/file")
			snap.Release()
		}
	})
	b.StopTimer()
	close(done)
	wg.Wait()
}

// --- Structural sharing ---

func BenchmarkStructuralSharing(b *testing.B) {
	root := defaultRoot()
	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			root = root.put(
				[]string{"mnt", fmt.Sprintf("dir-%d", i), fmt.Sprintf("file-%d", j)},
				benchID(i*10+j),
			)
		}
	}
	id := benchID(999999)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		root.put([]string{"mnt", "dir-50", "newfile"}, id)
	}
}

// --- LeafCount and WalkLeaves ---

func BenchmarkLeafCount(b *testing.B) {
	root := populatedTree(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root.leafCount()
	}
}

func BenchmarkWalkLeaves(b *testing.B) {
	root := populatedTree(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root.walkLeaves(func(_ string, _ c4.ID) {})
	}
}

// --- Writer contention ---

func BenchmarkWriterContention(b *testing.B) {
	db := openBenchDB(b)
	id := benchID(0)

	b.ResetTimer()
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := atomic.AddInt64(&counter, 1)
			db.Update(func(tx *Tx) error {
				return tx.Put(fmt.Sprintf("mnt/c/%d", n), id)
			})
		}
	})
}

// --- Snapshot hold/release throughput ---

func BenchmarkSnapshotHoldRelease(b *testing.B) {
	db := openBenchDB(b)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s := db.Snapshot()
			s.Release()
		}
	})
}

// --- GC ---

func BenchmarkGC(b *testing.B) {
	db := openBenchDB(b)
	db.Update(func(tx *Tx) error {
		for i := 0; i < 100; i++ {
			tx.Put(fmt.Sprintf("mnt/dir/file-%d", i), benchID(i))
		}
		return nil
	})
	// Store some blobs to give GC something to scan
	for i := 0; i < 100; i++ {
		db.StoreBytes([]byte(fmt.Sprintf("blob-%d", i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.GC()
	}
}
