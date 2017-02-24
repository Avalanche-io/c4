package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/blang/vfs"

	c4os "github.com/Avalanche-io/c4/os"
	flag "github.com/ogier/pflag"
)

func id_main(f *flag.FlagSet) {
	file_list := f.Args()

	if len(file_list) == 0 {
		identify_pipe()
	} else if len(file_list) == 1 && !(recursive_flag || include_meta) && depth == 0 {
		identify_file(file_list[0])
	} else {
		identify_files(file_list)
	}

}

func identify_pipe() {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(os.Stdin)
		printID(encode(reader))
	} else {
		flag.Usage()
	}
}

func identify_file(filename string) {
	path, err := filepath.Abs(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", filename, err)
		os.Exit(1)
	}
	fs := c4os.NewFileSystem(vfs.OS(), []byte(path))
	err = fs.Walk(nil, func(key []byte, attrs c4os.Attributes) error {
		fmt.Printf("%s: %s\n", string(key), attrs.ID())
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in walkFilesystem %s. %s\n", filename, err)
		os.Exit(1)
	}
}

func identify_files(file_list []string) {
	for _, file := range file_list {
		path, err := filepath.Abs(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", file, err)
			os.Exit(1)
		}
		if depth < 0 {
			depth = 0
		}

		fs := c4os.NewFileSystem(vfs.OS(), []byte(path))
		err = fs.Walk(nil, func(key []byte, attrs c4os.Attributes) error {
			fmt.Printf("%s: %s\n", string(key), attrs.ID())
			return nil
		})
		if err != nil {
			panic(err)
		}

		// walkFilesystem(depth, path, "")

	}
}
