package main

import (
	"time"

	c4 "github.com/Avalanche-io/c4/id"
)

type FsItem struct {
	Id       *c4.ID
	Path     *string
	Folder   bool
	Link     bool
	LinkPath *string
	Socket   bool
	Bytes    int64
	Modified time.Time
}

// func newItem(path string) (item map[string]interface{}) {
// 	item = make(map[string]interface{})
// 	if item == nil {
// 		fmt.Fprintf(os.Stderr, "Unable to allocate space for file information for \"%s\".", path)
// 		os.Exit(1)
// 	}
// 	f, err := os.Lstat(path)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Unable to get status for \"%s\": %s\n", path, err)
// 		os.Exit(1)
// 	}

// 	item["folder"] = f.IsDir()
// 	item["link"] = f.Mode()&os.ModeSymlink == os.ModeSymlink
// 	item["socket"] = f.Mode()&os.ModeSocket == os.ModeSocket
// 	item["bytes"] = f.Size()
// 	item["modified"] = f.ModTime().UTC()

// 	return item
// }

// func newItem(path string) (*FsItem, error) {

// 	f, err := os.Lstat(path)
// 	if err != nil {
// 		return nil, err
// 	}

// 	item := FsItem{
// 		Path:     &path,
// 		Folder:   f.IsDir(),
// 		Link:     f.Mode()&os.ModeSymlink == os.ModeSymlink,
// 		Socket:   f.Mode()&os.ModeSocket == os.ModeSocket,
// 		Bytes:    f.Size(),
// 		Modified: f.ModTime().UTC(),
// 	}
// 	return &item, nil
// }

// func (f *FsItem) Stat(path string) error {
// 	stat, err := os.Lstat(path)
// 	if err != nil {
// 		return err
// 	}

// 	f.Path = &path
// 	f.Folder = stat.IsDir()
// 	f.Link = stat.Mode()&os.ModeSymlink == os.ModeSymlink
// 	f.Socket = stat.Mode()&os.ModeSocket == os.ModeSocket
// 	f.Bytes = stat.Size()
// 	f.Modified = stat.ModTime().UTC()
// 	if f.Socket || f.Link {
// 		f.Id = nullId()
// 	}
// 	return nil
// }

// func (f *FsItem) IsFile() bool {
// 	return !f.Folder && !f.Link && !f.Socket
// }

// func (f *FsItem) Identify() (*c4.ID, error) {
// 	return fileID(f.Path)
// }

// func walkFilesystem(depth int, filename string, relative_path string) (*c4.ID, error) {
// 	path, err := filepath.Abs(filename)
// 	if err != nil {
// 		return nil, err
// 	}

// 	item := &FsItem{}
// 	err = item.Stat(path)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if item.IsFile() {
// 		id, err := item.Identify()
// 		if err != nil {
// 			return nil, err
// 		}
// 		item.Id = id
// 	} else if item.Link {
// 		newFilepath, err := filepath.EvalSymlinks(filename)
// 		if err != nil {
// 			return nil, err
// 		}
// 		item.LinkPath = &newFilepath
// 		if links_flag { // Then follow the link
// 			id, err := walkFilesystem(depth, newFilepath, relative_path)
// 			if err != nil {
// 				return item.Id, err
// 			}
// 			item.Id = id
// 		}
// 	} else if item.Folder {
// 		files, err := ioutil.ReadDir(path)
// 		if err != nil {
// 			return item.Id, err
// 		}
// 		var childIDs c4.IDSlice
// 		for _, file := range files {
// 			path := filename + string(filepath.Separator) + file.Name()
// 			id, err := walkFilesystem(depth-1, path, relative_path)
// 			if err != nil {
// 				return item.Id, err
// 			}
// 			childIDs.Push(id)
// 		}
// 		id, err := childIDs.ID()
// 		if err != nil {
// 			return item.Id, err
// 		}
// 		item.Id = id
// 	}
// 	if depth >= 0 || recursive_flag {
// 		output(item)
// 	}
// 	return item.Id, nil
// }
