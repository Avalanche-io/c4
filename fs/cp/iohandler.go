package cp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Avalanche-io/pathstack"
	"github.com/Avalanche-io/sled"

	"github.com/etcenter/c4/asset"
	"github.com/etcenter/c4/db"
	"github.com/etcenter/c4/fs"
)

type IoHandler struct {
	IoCh      chan string
	ErrCh     chan error
	TargetArg string
	Target    string
	Buffers   *db.MultiTaskBuffer
	files     []string
	fwchan    chan Filewriter
	wg        sync.WaitGroup
	pathstack *pathstack.PathStack
	kv        *sled.Sled
}

type Filewriter struct {
	Reader io.Reader
	Path   string
}

func NewIo(args []string, buffer_count uint64, stdioch chan string, stderrch chan error) (*IoHandler, bool) {
	io := &IoHandler{stdioch, stderrch, "", "", db.NewMTB(buffer_count), nil, nil, sync.WaitGroup{}, pathstack.New(), sled.New()}
	switch {
	case io.ifUsage(len(args) == 0):
		return io, false
	case io.ifUsage(len(args) == 1 && args[0] == ""):
		return io, false
	case io.setTarget(args[len(args)-1]):
		return io, false
	}
	io.files = args[:len(args)-1]
	return io, true
}

func (io *IoHandler) IfError(err error) bool {
	if err != nil {
		io.ErrCh <- err
		return true
	}
	return false
}

func (io *IoHandler) Out(message string) {
	io.IoCh <- message
}

func (io *IoHandler) Files() []string {
	return io.files
}

func (io *IoHandler) LogCopy(path string) {
	io.Out(path + " -> " + io.TargetPathTo(path) + "\n")
}

func (io *IoHandler) Walk(file string, verbose bool) {
	filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
		if verbose {
			io.LogCopy(path)
		}
		io.Copy(path, info)
		return nil
	})
}

type CopyParams struct {
	Soruce  string
	Target  string
	SrcInfo os.FileInfo
}

func (io *IoHandler) addDependancy(src_path string, src_info os.FileInfo, target_path string) {
	dir := filepath.Dir(target_path)
	fmt.Printf("%s\n", target_path)
	last := filepath.Dir(io.pathstack.Peek())
	if last != dir {
		if io.pathstack.IsParent(target_path) {
			// fmt.Printf("\tPush: %s\n", fs.Green(target_path))
		} else {
			// fmt.Printf("\tPopDif: %s\n", fs.Red(target_path))
			root := io.pathstack.CommonRoot(target_path)
			// fmt.Printf("\t  Root: %s\n", fs.Bold(root))
			difstack := io.pathstack.PopDiff(target_path)
			cancel := make(chan struct{})
			for p := range difstack.Popper(cancel) {
				if p == root {
					close(cancel)

					break
				}
				fmt.Printf("%s\n", fs.Blue(p))
				c := io.kv.Get(p).(*CopyParams)

				if c.SrcInfo.IsDir() {
					os.MkdirAll(c.Target, c.SrcInfo.Mode().Perm())
				} else if !c.SrcInfo.Mode().IsRegular() {
					io.IfError(cpError("Failed to copy non regular file " + c.Soruce))
				} else {
					io.copyFileContents(c.Soruce, c.Target)
				}
				// fmt.Println(v)
				// io.pathstack.Pop()
			}

		}
	} else {
		// fmt.Printf("\tPush: %s\n", fs.Yellow(dir))
	}

	io.kv.Set(target_path, &CopyParams{src_path, target_path, src_info})
	io.pathstack.Push(target_path)

}

func (io *IoHandler) Copy(path string, src_info os.FileInfo) {
	cwd, err := os.Getwd()
	if io.IfError(err) {
		return
	}

	src_path := cwd + string(os.PathSeparator) + path
	target_path := io.TargetArg + string(os.PathSeparator) + path

	target_info, err := os.Stat(target_path)
	if err == nil && os.SameFile(src_info, target_info) {
		io.IfError(cpError("Failed to copy files identical " + src_path))
		return
	} else if !os.IsNotExist(err) {
		io.IfError(err)
		return
	}

	// dir := target_path
	// if !src_info.IsDir() {

	// }

	io.addDependancy(src_path, src_info, target_path)

	// if src_info.IsDir() {
	// 	os.MkdirAll(target_path, src_info.Mode().Perm())
	// } else if !src_info.Mode().IsRegular() {
	// 	io.IfError(cpError("Failed to copy non regular file " + src_path))
	// } else {
	// 	io.copyFileContents(src_path, target_path)
	// }

	return
}

func (i *IoHandler) read(src string) io.Reader {
	reader, out := io.Pipe()

	go func() {
		in, err := os.Open(src)
		if i.IfError(err) {
			return
		}
		defer func() {
			in.Close()
			i.IfError(out.Close())
		}()
		_, err = io.Copy(out, in)
		if i.IfError(err) {
			return
		}
	}()
	return reader
}

func (i *IoHandler) write(in io.Reader, dst string) {
	out, err := os.Create(dst)
	if i.IfError(err) {
		return
	}
	defer func() {
		i.IfError(out.Close())
	}()
	_, err = io.Copy(out, in)
	if i.IfError(err) {
		return
	}
	i.IfError(out.Sync())
}

func (i *IoHandler) copyFileContents(src string, dst string) {
	reader := i.read(src)

	idr, idw := io.Pipe()
	fr, fw := io.Pipe()

	i.wg.Add(1)
	go func() {
		defer i.wg.Done()
		defer fw.Close()
		defer idw.Close()
		mw := io.MultiWriter(idw, fw)
		_, err := io.Copy(mw, reader)
		if i.IfError(err) {
			return
		}

	}()
	i.wg.Add(1)
	go func() {
		defer i.wg.Done()
		e := asset.NewIDEncoder()
		_, err := io.Copy(e, idr)
		if i.IfError(err) {
			return
		}
		id := e.ID()
		_ = id
		// dir := filepath.Dir(dst)
		// ids := i.kv.Get(dir).(string)
		// ids = ids + id.String()
		// i.kv.Set(dir, ids)
		fmt.Printf("%s: %s\n", fs.Red(id), dst)
	}()
	i.fwchan <- Filewriter{fr, dst}
}

func (io *IoHandler) ifUsage(test bool) bool {
	if test {
		io.ErrCh <- cpError(usage)
	}
	return test
}

func (io *IoHandler) TargetPathTo(path string) string {
	return io.TargetArg + string(os.PathSeparator) + path
}

func (i *IoHandler) Wait() {
	i.wg.Wait()
}

func (io *IoHandler) setTarget(target_arg string) bool {
	io.TargetArg = target_arg
	var err error
	io.Target, err = filepath.EvalSymlinks(target_arg)
	if err != nil {
		io.ErrCh <- err
		return true
	}
	fwchan := make(chan Filewriter)
	io.fwchan = fwchan

	io.wg.Add(1)
	go func() {
		defer io.wg.Done()
		for fw := range fwchan {
			io.write(fw.Reader, fw.Path)
		}
	}()
	return false
}
