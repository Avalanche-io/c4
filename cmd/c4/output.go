package main

import (
	"fmt"
	"path/filepath"
)

func metadata_output(item map[string]interface{}, path string, baseName string, rootPath string) {
	if formatting_string == "path" {
		fmt.Printf("\"%s\":\n", path)
		fmt.Printf("  c4id: %s\n", item["c4id"])
	} else {
		fmt.Printf("%s:\n", item["c4id"])
		fmt.Printf("  path: \"%s\"\n", path)
	}
	fmt.Printf("  name:  \"%s\"\n", baseName)
	if item["folder"] == false {
		fmt.Printf("  folder:  false\n")
	} else {
		fmt.Printf("  folder:  true\n")
	}
	if item["link"] == false {
		fmt.Printf("  link:  false\n")
	} else {
		linkPath := item["link"].(string)
		if !absolute_flag {
			linkPath, _ = filepath.Rel(rootPath, linkPath)
		}
		fmt.Printf("  link:  \"%s\"\n", linkPath)
	}
	fmt.Printf("  bytes:  %d\n", item["bytes"])
}

func output(path string, item map[string]interface{}) {
	rootPath, _ := filepath.Abs(".")
	baseName := filepath.Base(path)

	newPath := path
	if !absolute_flag {
		newPath, _ = filepath.Rel(rootPath, path)
	}
	if include_meta {
		metadata_output(item, newPath, baseName, rootPath)
	} else {
		if formatting_string == "path" {
			fmt.Printf("%s:  %s\n", newPath, item["c4id"])
		} else {
			fmt.Printf("%s:  %s\n", item["c4id"], newPath)
		}
	}
}
