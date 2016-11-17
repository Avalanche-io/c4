package cp_test

import (
	"sync"
	"testing"

	"github.com/cheekybits/is"

	"github.com/etcenter/c4/client"
	c4 "github.com/etcenter/c4/fs/cp"
)

func TestID(t *testing.T) {
	is := is.New(t)
	tempdirs := setup(is, 2)
	defer teardown(is, tempdirs)
	is.Equal(len(tempdirs), 2)
	srcdir := tempdirs[0]
	targetdir := tempdirs[1]
	_ = srcdir

	args := []string{"-R"}
	args = append(args, build_file_list(is, []string{"*"})...)
	client.CpFlags = client.CpFlagsInit()
	err := client.CpFlags.Parse(args)
	is.NoErr(err)

	c4_stdoutch := make(chan string, 1)
	c4_stderrch := make(chan error, 1)
	var c4_stderr, c4_stdout []string

	io, ok := c4.NewController(client.CpFlags.Args(), uint64(1), c4_stdoutch, c4_stderrch)
	go func() {
		defer close(c4_stdoutch)
		defer close(c4_stderrch)
		if !ok {
			return
		}
		c4.CpMain(io, client.RecursiveFlag, client.VerboseFlag)
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		c4_stderr = normalize_errch(c4_stderrch, targetdir)
		wg.Done()
	}()
	go func() {
		c4_stdout = normalize_strch(c4_stdoutch, targetdir)
		wg.Done()
	}()
	wg.Wait()

	is.OK(true)
}
