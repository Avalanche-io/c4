package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

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
		printID(encode(bufio.NewReader(os.Stdin)))
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
	id, err := walkFilesystem(-1, path, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in walkFilesystem %s. %s\n", filename, err)
		os.Exit(1)
	}
	printID(id)
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
		walkFilesystem(depth, path, "")
	}
}
