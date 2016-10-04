package fs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shirou/gopsutil/mem"

	"github.com/cheekybits/is"

	"github.com/etcenter/c4/asset"
	"github.com/etcenter/c4/fs"
	"github.com/etcenter/c4/test"
)

func TestBufferedMultiTasks(t *testing.T) {
	is := is.New(t)
	_ = is
	dir := "/source/foo/bar"

	start := time.Now()
	names := []string{"Id", "/target1/bar", "/target2/bat", "/target3/baz"}
	engine := fs.NewTaskEngine(names)
	engine.Start()
	defer func() {
		engine.Close()
		end := time.Now()
		t.Log("Time: ", end.Sub(start))
		for _, q := range engine.Queues {
			t.Log("Average Task Time: ", q.Key, q.AvgTime())
		}
	}()

	mtb := fs.NewMTB(20000)
	engine.StartTask(func(src string) *fs.Buffer {
		return mtb.Get(20)
	})

	engine.TaskHandler("Id", func(src string, b *fs.Buffer) {
		time.Sleep(time.Duration(1) * time.Millisecond)
		// fmt.Printf("Id task for: %s\n", src)
	})
	engine.TaskHandler("/target1/bar", func(src string, b *fs.Buffer) {
		time.Sleep(time.Duration(10) * time.Millisecond)
		// fmt.Printf("Copy to '/target1/bar' task: %s\n", src)
	})
	engine.TaskHandler("/target2/bat", func(src string, b *fs.Buffer) {
		time.Sleep(time.Duration(3) * time.Millisecond)
		// fmt.Printf("Copy to '/target2/bat' task: %s\n", src)
	})
	engine.TaskHandler("/target3/baz", func(src string, b *fs.Buffer) {
		time.Sleep(time.Duration(6) * time.Millisecond)
		// fmt.Printf("Copy to '/target3/baz' task: %s\n", src)
	})

	go func() {
		for i := 0; i < 100; i++ {
			s := fmt.Sprintf("%s/file_%04d.dat", dir, i)
			engine.Add(s)
		}
		engine.InputDone()
	}()
}

func TestMultiTargetFileCopy(t *testing.T) {
	is := is.New(t)
	_ = is
	tmp := make([]string, 3)
	tmp[0] = test.TempDir(is)
	tmp[1] = test.TempDir(is)
	tmp[2] = test.TempDir(is)
	defer func() {
		for i := range tmp {
			test.DeleteDir(&tmp[i])
		}
	}()

	build_test_fs(is, tmp[0], 4, 10, 0)

	start := time.Now()
	names := []string{"Id", tmp[1], tmp[2]}
	engine := fs.NewTaskEngine(names)
	// engine.Threads = 1
	engine.Start()
	v, _ := mem.VirtualMemory()
	mtb := fs.NewMTB(v.Available)
	defer func() {
		engine.Close()
		end := time.Now()

		t.Log("Peak Ram: ", mtb.Peak/(1024*1024), "MB")
		t.Log("Time: ", end.Sub(start))
		for _, q := range engine.Queues {
			t.Log("Average Task Time: ", q.Key, q.AvgTime())
		}
	}()

	engine.StartTask(func(src string) *fs.Buffer {
		info, err := os.Stat(src)
		is.NoErr(err)
		f, err := os.OpenFile(src, os.O_RDONLY, 0600)
		is.NoErr(err)
		defer f.Close()
		b := mtb.Get(uint64(info.Size()))
		f.Read(b.Bytes())
		return b
	})

	engine.TaskHandler("Id", func(src string, b *fs.Buffer) {
		id, err := asset.Identify(b.Reader())
		_ = id
		is.NoErr(err)
	})

	for i := 1; i < 3; i++ {
		temppath := tmp[i]
		engine.TaskHandler(temppath, func(src string, b *fs.Buffer) {
			// time.Sleep(time.Duration(10) * time.Millisecond)
			filename := temppath + src[len(tmp[0]):]
			path := filepath.Dir(filename)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				err := os.MkdirAll(path, 0777)
				is.NoErr(err)
			}
			f, err := os.Create(filename)
			is.NoErr(err)
			defer f.Close()
			// fmt.Printf("Writing: %s\n", filename)
			n, err := f.Write(b.Bytes())
			is.NoErr(err)
			if n != len(b.Bytes()) {
				t.Error("Bad write size ", n, "vs", len(b.Bytes()))
			}
		})
	}

	filepath.Walk(tmp[0], func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			engine.Add(path)
		}
		return nil
	})
	engine.InputDone()

}
