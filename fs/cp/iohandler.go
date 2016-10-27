package cp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type IoHandler struct {
	IoCh      chan string
	ErrCh     chan error
	TargetArg string
	Target    string
	files     []string
}

func NewIo(args []string, stdioch chan string, stderrch chan error) *IoHandler {
	io := &IoHandler{stdioch, stderrch, "", "", nil}
	switch {
	case io.ifUsage(len(args) == 0):
		return nil
	case io.ifUsage(len(args) == 1 && args[0] == ""):
		return nil
	case io.setTarget(args[len(args)-1]):
		return nil
	}
	io.files = args[:len(args)-1]
	return io
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

func (io *IoHandler) Walk(file string, verbose bool) {
	filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
		if verbose {
			io.Out(fmt.Sprintf("%s -> %s\n", path, io.TargetPathTo(path)))
		}
		io.Copy(path, info)
		return nil
	})
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
		io.IfError(Error("Failed to copy files identical " + src_path))
		return
	} else if !os.IsNotExist(err) {
		io.IfError(err)
		return
	}

	if src_info.IsDir() {
		os.MkdirAll(target_path, src_info.Mode().Perm())
	} else if !src_info.Mode().IsRegular() {
		io.IfError(Error("Failed to copy non regular file " + src_path))
	} else {
		io.copyFileContents(src_path, target_path)
	}

	return
}

func (i *IoHandler) copyFileContents(src, dst string) {
	in, err := os.Open(src)
	if i.IfError(err) {
		return
	}
	defer in.Close()
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

func (io *IoHandler) ifUsage(test bool) bool {
	if test {
		io.ErrCh <- Error(usage)
	}
	return test
}

func (io *IoHandler) TargetPathTo(path string) string {
	return io.TargetArg + string(os.PathSeparator) + path
}

func (io *IoHandler) setTarget(target_arg string) bool {
	io.TargetArg = target_arg
	var err error
	io.Target, err = filepath.EvalSymlinks(target_arg)
	if err != nil {
		io.ErrCh <- err
		return true
	}
	return false
}
