package main

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/etcenter/c4/asset"
	"github.com/etcenter/c4/attributes"
)

// func fileID(path *string) (*asset.ID, error) {
// 	f, err := os.Open(*path)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer f.Close()
// 	id, err := asset.Identify(f)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return id, nil
// }

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
	// fmt.Fprintf(os.Stderr, "\nwalkFilesystem %d, %s\n", depth, filename)
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
			// // jout.Encode(item)
			// kv := make(attributes.KeyFsInfo)
			// kv[path] = item
			// // KeyFsInfo
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
		// go func() {
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
		// }()
	case item.Link():
		newFilepath, err := filepath.EvalSymlinks(filename)
		if err != nil {
			panic(err)
		}
		if links_flag { // Then follow the link
			return walkFilesystem(depth, newFilepath, relative_path, achan)
		}
	}
	// if item.IsFile() {
	// 	id, err := item.Identify()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	item.Id = id
	// } else if item.Link {
	// 	newFilepath, err := filepath.EvalSymlinks(filename)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	item.LinkPath = &newFilepath
	// 	if links_flag { // Then follow the link
	// 		id, err := walkFilesystem(depth, newFilepath, relative_path)
	// 		if err != nil {
	// 			return item.Id, err
	// 		}
	// 		item.Id = id
	// 	}
	// } else if item.Folder {
	// 	files, err := ioutil.ReadDir(path)
	// 	if err != nil {
	// 		return item.Id, err
	// 	}
	// 	var childIDs asset.IDSlice
	// 	for _, file := range files {
	// 		path := filename + string(filepath.Separator) + file.Name()
	// 		id, err := walkFilesystem(depth-1, path, relative_path)
	// 		if err != nil {
	// 			return item.Id, err
	// 		}
	// 		childIDs.Push(id)
	// 	}
	// 	id, err := childIDs.ID()
	// 	if err != nil {
	// 		return item.Id, err
	// 	}
	// 	item.Id = id
	// }

	return item.ID(), nil
}
