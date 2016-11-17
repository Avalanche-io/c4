package cp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	flow "github.com/Avalanche-io/flowengine"

	"github.com/etcenter/c4/asset"
	"github.com/etcenter/c4/db"
)

type CpController struct {
	IoCh      chan string
	ErrCh     chan error
	TargetArg string
	Target    string
	Buffers   *db.MultiTaskBuffer
	files     []string
	fwchan    chan Filewriter
	wg        sync.WaitGroup
	Fe        *flow.FlowEngine
}

type Filewriter struct {
	Reader io.Reader
	Path   string
}

func NewController(args []string, buffer_count uint64, stdioch chan string, stderrch chan error) (*CpController, bool) {

	fe := flow.Begin()
	c := &CpController{stdioch, stderrch, "", "", db.NewMTB(buffer_count), nil, nil, sync.WaitGroup{}, fe}
	switch {
	case c.ifUsage(len(args) == 0):
		return c, false
	case c.ifUsage(len(args) == 1 && args[0] == ""):
		return c, false
	case c.setTarget(args[len(args)-1]):
		return c, false
	}
	cwd, err := os.Getwd()
	if c.IfError(err) {
		return c, false
	}
	c.Fe.SetRoot(cwd)
	c.files = args[:len(args)-1]
	c.Fe.Run()
	return c, true
}

func (c *CpController) IfError(err error) bool {
	if err != nil {
		c.ErrCh <- err
		return true
	}
	return false
}

func (c *CpController) Out(message string) {
	c.IoCh <- message
}

func (c *CpController) Files() []string {
	return c.files
}

func (c *CpController) LogCopy(path string) {
	c.Out(path + " -> " + c.TargetPathTo(path) + "\n")
}

func (c *CpController) Walk(file string, verbose bool) {
	filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
		if verbose {
			c.LogCopy(path)
		}
		c.Copy(path, info)
		return nil
	})
}

func (c *CpController) Copy(path string, src_info os.FileInfo) {
	cwd, err := os.Getwd()
	if c.IfError(err) {
		return
	}

	src_path := cwd + string(os.PathSeparator) + path
	target_path := c.TargetArg + string(os.PathSeparator) + path

	target_info, err := os.Stat(target_path)
	if err == nil && os.SameFile(src_info, target_info) {
		c.IfError(cpError("Failed to copy files identical " + src_path))
		return
	} else if !os.IsNotExist(err) {
		c.IfError(err)
		return
	}

	if src_info.IsDir() {
		c.Fe.Begin(src_path)
		c.Fe.Proc(func(node *flow.Node) *asset.ID {
			fmt.Printf("Mkdir(%s)\n", target_path)
			os.MkdirAll(target_path, src_info.Mode().Perm())
			id, err := asset.Identify(strings.NewReader(target_path))
			if c.IfError(err) {
				return nil
			}
			return id
		})

	} else if !src_info.Mode().IsRegular() {
		c.IfError(cpError("Failed to copy non regular file " + src_path))
	} else {
		c.Fe.Begin(rc_path)
		c.Fe.Proc(func(node *flow.Node) *asset.ID {
			fmt.Printf("Copy(%s, %s)\n", src_path, target_path)
			c.copyFileContents(src_path, target_path)
			id, err := asset.Identify(strings.NewReader(target_path))
			if c.IfError(err) {
				return nil
			}
			return id
		})
	}
	return
}

func (c *CpController) read(src string) io.Reader {
	reader, out := io.Pipe()

	go func() {
		in, err := os.Open(src)
		if c.IfError(err) {
			return
		}
		defer func() {
			in.Close()
			c.IfError(out.Close())
		}()
		_, err = io.Copy(out, in)
		if c.IfError(err) {
			return
		}
	}()
	return reader
}

func (c *CpController) write(in io.Reader, dst string) {
	out, err := os.Create(dst)
	if c.IfError(err) {
		return
	}
	defer func() {
		c.IfError(out.Close())
	}()
	_, err = io.Copy(out, in)
	if c.IfError(err) {
		return
	}
	c.IfError(out.Sync())
}

func (c *CpController) copyFileContents(src string, dst string) {
	reader := c.read(src)

	idr, idw := io.Pipe()
	fr, fw := io.Pipe()

	go func() {
		defer fw.Close()
		defer idw.Close()

		mw := io.MultiWriter(idw, fw)
		_, err := io.Copy(mw, reader)
		if c.IfError(err) {
			return
		}

	}()
	go func() {
		e := asset.NewIDEncoder()
		_, err := io.Copy(e, idr)
		if c.IfError(err) {
			return
		}
		id := e.ID()
		_ = id
		// fmt.Printf("%s: %s\n", id, src)
	}()
	c.fwchan <- Filewriter{fr, dst}
}

func (c *CpController) ifUsage(test bool) bool {
	if test {
		c.ErrCh <- cpError(usage)
	}
	return test
}

func (c *CpController) TargetPathTo(path string) string {
	return c.TargetArg + string(os.PathSeparator) + path
}

func (i *CpController) Wait() {
	i.wg.Wait()
}

func (c *CpController) setTarget(target_arg string) bool {
	c.TargetArg = target_arg
	var err error
	c.Target, err = filepath.EvalSymlinks(target_arg)
	if err != nil {
		c.ErrCh <- err
		return true
	}
	fwchan := make(chan Filewriter)
	c.fwchan = fwchan

	go func() {
		for fw := range fwchan {
			c.write(fw.Reader, fw.Path)
		}
	}()
	return false
}
