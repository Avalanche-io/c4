package fs_test

import (
	// "errors"
	// "fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	// "github.com/shirou/gopsutil/mem"

	"github.com/cheekybits/is"

	"github.com/etcenter/c4/fs"
	"github.com/etcenter/c4/test"
)

func TestBufferedMultiTasks(t *testing.T) {
	is := is.New(t)
	_ = is
	// dir := "/source/foo/bar"

	start := time.Now()
	names := []string{"Id", "/target1/bar", "/target2/bat", "/target3/baz"}
	engine := fs.NewTaskEngine(names, 20)
	engine.Start()
	defer func() {
		engine.Close()
		end := time.Now()
		t.Log("Time: ", end.Sub(start))
		for _, q := range engine.Queues {
			t.Log("Average Task Time: ", q.Key, q.AvgTime())
		}
	}()

	// mtb := fs.NewMTB(20000)
	engine.StartTask(func(item *fs.Item, mtb *fs.MultiTaskBuffer) (*fs.Buffer, error) {
		return mtb.Get(20), nil
	})

	// type TaskFunc func(i Item, b *Buffer) error
	engine.TaskHandler("Id", fs.IdTask)
	//                    func(src string, b *fs.Buffer) {
	// 	time.Sleep(time.Duration(1) * time.Millisecond)
	// 	// fmt.Printf("Id task for: %s\n", src)
	// })
	engine.TaskHandler("/target1/bar", func(i *fs.Item, b *fs.Buffer) error {
		time.Sleep(time.Duration(10) * time.Millisecond)
		// fmt.Printf("Copy to '/target1/bar' task: %s\n", src)
		return nil
	})

	engine.TaskHandler("/target2/bat", func(i *fs.Item, b *fs.Buffer) error {
		time.Sleep(time.Duration(3) * time.Millisecond)
		// fmt.Printf("Copy to '/target2/bat' task: %s\n", src)
		return nil
	})

	engine.TaskHandler("/target3/baz", func(i *fs.Item, b *fs.Buffer) error {
		time.Sleep(time.Duration(6) * time.Millisecond)
		// fmt.Printf("Copy to '/target3/baz' task: %s\n", src)
		return nil
	})

	tmp := test.TempDir(is)
	defer test.DeleteDir(&tmp)
	// threads := 8
	build_test_fs(is, tmp, 8, 20, 0)

	f := fs.New(tmp)
	f.Add(tmp)
	engine.EnqueueFS(f)

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

	build_test_fs(is, tmp[0], 20, 20, 0)

	start := time.Now()
	names := []string{"Id", tmp[1], tmp[2]}
	engine := fs.NewTaskEngine(names, 20)
	ech := engine.Start()
	go func() {
		for e := range ech {
			t.Error("Task engine error: ", fs.Red(e.Error()))
		}
	}()
	defer func() {
		engine.Close()
		end := time.Now()

		t.Log("Time: ", end.Sub(start))
		for _, q := range engine.Queues {
			t.Log("Average Task Time: ", q.Key, q.AvgTime())
		}
	}()

	engine.TaskHandler("Id", fs.IdTask)
	source_prefix := tmp[0]

	for i := 1; i < 3; i++ {
		temppath := tmp[i]
		engine.TaskHandler(temppath, func(item *fs.Item, b *fs.Buffer) error {
			target := temppath + item.Path()[len(source_prefix):]

			if item.IsDir() {
				os.MkdirAll(target, 0777)
			} else {
				os.MkdirAll(filepath.Dir(target), 0777)
				f, err := os.Create(target)
				if err != nil {
					return err
				}
				defer f.Close()
				n, err := f.Write(b.Bytes())
				if err != nil {
					return err
				}
				is.Equal(n, item.Size())
			}
			return nil
		})
	}

	f := fs.New(tmp[0])
	f.Add(tmp[0])
	engine.EnqueueFS(f)
}
