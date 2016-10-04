package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/etcenter/c4/asset"
	"github.com/etcenter/c4/attributes"
)

func identify_file(path string, item attributes.FsInfo) error {
	switch item := item.(type) {
	case *attributes.FileInfo:
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		id, err := asset.Identify(f)
		if err != nil {
			return err
		}
		item.Id = id
	case *attributes.FolderInfo:
		return errors.New("Wrong type (*attributes.FolderInfo)")
	default:
		return errors.New("Wrong type (unknown)")
	}
	return nil
}

func update_folder_ids(item attributes.FsInfo, id *asset.ID) error {
	switch item := item.(type) {
	case *attributes.FileInfo:
		return errors.New("Wrong type (*attributes.FileInfo)")
	case *attributes.FolderInfo:
		item.Ids.Push(id)
	default:
		return errors.New("Wrong type (unknown)")
	}
	return nil
}

func walkFilesystem(depth int, filename string, relative_path string, achan chan<- attributes.FsInfo) (*asset.ID, error) {
	str := fmt.Sprintf("\nwalkFilesystem %d, %s\n", depth, cyan(filename))
	fmt.Fprintf(os.Stderr, bold(str))
	path, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	item := attributes.NewFsInfo(info)

	switch {
	case item.Regular():
		err := identify_file(path, item)
		if err != nil {
			panic(err)
		}
		if depth >= 0 || recursive_flag {
			if achan != nil {
				achan <- item
			}
		}
	case item.Folder():
		files, err := ioutil.ReadDir(path)
		if err != nil {
			return nil, err
		}
		ch := item.EncodedNestedJsonChan(os.Stdout)

		for _, file := range files {
			path := filename + string(filepath.Separator) + file.Name()
			id, err := walkFilesystem(depth-1, path, relative_path, ch)
			if err != nil {
				return nil, err
			}
			err = update_folder_ids(item, id)
			if err != nil {
				panic(err)
			}
		}
		if achan != nil {
			close(achan)
		}
	case item.Link():
		newFilepath, err := filepath.EvalSymlinks(filename)
		if err != nil {
			panic(err)
		}
		if links_flag { // Then follow the link
			return walkFilesystem(depth, newFilepath, relative_path, achan)
		}
	}
	return item.ID(), nil
}
