package main

import (
	"fmt"
	"path/filepath"
)

func metadata_output(item *FsItem, path string, baseName string, rootPath string) {
	if formatting_string == "path" {
		fmt.Printf("\"%s\":\n", path)
		fmt.Printf("  c4id: %s\n", item.Id.String())
	} else {
		fmt.Printf("%s:\n", item.Id.String())
		fmt.Printf("  path: \"%s\"\n", path)
	}
	fmt.Printf("  name:  \"%s\"\n", baseName)
	if item.Folder {
		fmt.Printf("  folder:  true\n")
	} else {
		fmt.Printf("  folder:  false\n")
	}
	if item.Link {
		fmt.Printf("  link:  false\n")
	} else {
		linkPath := *item.LinkPath
		if !absolute_flag {
			linkPath, _ = filepath.Rel(rootPath, linkPath)
		}
		fmt.Printf("  link:  \"%s\"\n", linkPath)
	}
	fmt.Printf("  bytes:  %d\n", item.Bytes)
}

func output(item *FsItem) {
	rootPath, _ := filepath.Abs(".")
	baseName := filepath.Base(*item.Path)

	newPath := *item.Path
	if !absolute_flag {
		newPath, _ = filepath.Rel(rootPath, *item.Path)
	}
	if include_meta {
		metadata_output(item, newPath, baseName, rootPath)
	} else {
		if formatting_string == "path" {
			fmt.Printf("%s:  %s\n", newPath, item.Id.String())
		} else {
			fmt.Printf("%s:  %s\n", item.Id.String(), newPath)
		}
	}
}
