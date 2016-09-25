package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	flag "github.com/ogier/pflag"

	"github.com/etcenter/c4/asset"
	"github.com/etcenter/c4/attributes"
)

func id_main(f *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, "id_main\n")
	file_list := f.Args()
	if len(file_list) == 0 {
		fmt.Fprintf(os.Stderr, "pipe\n")
		identify_pipe()
	} else if len(file_list) == 1 && !(recursive_flag || include_meta) && depth == 0 {
		fmt.Fprintf(os.Stderr, "one\n")
		walk_one(file_list[0])
	} else {
		fmt.Fprintf(os.Stderr, "all\n")
		walk_all(file_list)
	}

}

func identify_pipe() {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(os.Stdin)
		id, err := asset.Identify(reader)
		if err != nil {
			panic(err)
		}
		printID(id)
	} else {
		flag.Usage()
	}
}

func walk_one(filename string) {
	fmt.Fprintf(os.Stderr, "walk_one\n")
	path, err := filepath.Abs(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", filename, err)
		os.Exit(1)
	}
	// ch := make(chan attributes.FsInfo)
	info, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	item := attributes.NewFsInfo(info)

	ch := item.EncodedNestedJsonChan(os.Stdout)
	defer close(ch)

	id, err := walkFilesystem(-1, path, "", ch)
	if err != nil {
		panic(err)
	}
	printID(id)
	// attributes.JsonEncodeKVChan(os.Stdout, ch)
	// if err != nil {
	// 	panic(err)
	// }
}

func walk_all(file_list []string) {
	// fmt.Fprintf(os.Stderr, "walk_all\n")
	for _, file := range file_list {
		path, err := filepath.Abs(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", file, err)
			os.Exit(1)
		}
		if depth < 0 {
			depth = 0
		}

		// info, err := os.Stat(path)
		// if err != nil {
		// 	panic(err)
		// }
		// item := attributes.NewFsInfo(info)

		// ch := make(chan attributes.FsInfo)
		// ch := item.EncodedNestedJsonChan(os.Stdout)

		fmt.Fprintf(os.Stdout, "{")

		id, err := walkFilesystem(depth, path, "", nil)
		if err != nil {
			panic(err)
		}
		_ = id
		fmt.Fprintf(os.Stdout, "}")
		// printID(id)

		// err = attributes.JsonEncodeKVChan(os.Stdout, ch)
		// if err != nil {
		// 	panic(err)
		// }
	}
}
