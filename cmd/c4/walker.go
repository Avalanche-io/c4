package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/etcenter/c4/asset"
)

func newItem(path string) (item map[string]interface{}) {
	item = make(map[string]interface{})
	if item == nil {
		fmt.Fprintf(os.Stderr, "Unable to allocate space for file information for \"%s\".", path)
		os.Exit(1)
	}
	f, err := os.Lstat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get status for \"%s\": %s\n", path, err)
		os.Exit(1)
	}

	item["folder"] = f.IsDir()
	item["link"] = f.Mode()&os.ModeSymlink == os.ModeSymlink
	item["socket"] = f.Mode()&os.ModeSocket == os.ModeSocket
	item["bytes"] = f.Size()
	item["modified"] = f.ModTime().UTC()

	return item
}

func walkFilesystem(depth int, filename string, relative_path string) (id *asset.ID) {
	path, err := filepath.Abs(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", filename, err)
		os.Exit(1)
	}

	item := newItem(path)
	if item["socket"] == true {
		id = nullId()
	} else if item["link"] == true && !links_flag {
		newFilepath, _ := filepath.EvalSymlinks(filename)
		item["link"] = newFilepath
		id = nullId()
	} else if item["link"] == true {
		newFilepath, err := filepath.EvalSymlinks(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to follow link %s. %s\n", newFilepath, err)
			item["link"] = newFilepath
			id = nullId()
		} else {
			item["link"] = newFilepath
			var linkId asset.IDSlice
			linkId.Push(walkFilesystem(depth-1, newFilepath, relative_path))
			id = linkId.ID()
		}
	} else {
		if item["folder"] == true {
			files, err := ioutil.ReadDir(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to read directory: %v\n", err)
				os.Exit(1)
			}
			var childIDs asset.IDSlice
			for _, file := range files {
				path := filename + string(filepath.Separator) + file.Name()
				childIDs.Push(walkFilesystem(depth-1, path, relative_path))
			}
			id = childIDs.ID()
		} else {
			id = fileID(path)
		}
	}
	item["c4id"] = id.String()
	if depth >= 0 || recursive_flag {
		output(path, item)
	}
	return
}
