package manifest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	filepath "path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/xtgo/set"
)

type FileInfo struct {
	mode  os.FileMode
	size  int64
	mtime time.Time
	name  string

	id c4.ID

	metadata c4.ID
}

func (i *FileInfo) Name() string {
	return filepath.Base(i.name)
}

func (i *FileInfo) Size() int64 {
	return i.size
}

func (i *FileInfo) Mode() os.FileMode {
	return i.mode
}

func (i *FileInfo) ModTime() time.Time {
	return i.mtime
}

func (i *FileInfo) IsDir() bool {
	return i.mode.IsDir()
}

func (i *FileInfo) Sys() interface{} {
	return nil
}

func (i *FileInfo) ID() c4.ID {
	return i.id
}

func (i *FileInfo) Metadata() c4.ID {
	return i.metadata
}

func (i *FileInfo) MkString(sizepadding, namepadding int) infoStringer {
	return infoStringer{sizepadding + 1, namepadding, i}
}

func (i *FileInfo) MarshalJson() ([]byte, error) {

	// sizestr := strconv.Itoa(int(i.size))
	info := struct {
		Mode     string `json:"mode"`
		ModTime  string `json:"mod_time"`
		Size     int    `json:"size"`
		Name     string `json:"name"`
		ID       string `json:"id,omitempty"`
		Metadata string `json:"metadata,omitempty"`
	}{
		Mode:    i.mode.String(),
		ModTime: i.mtime.Format(time.RFC3339),
		Size:    int(i.size),
		Name:    i.name,
	}
	if !i.id.IsNil() {
		info.ID = i.id.String()
	}
	if !i.metadata.IsNil() {
		info.Metadata = i.metadata.String()
	}
	return json.Marshal(info)
}

func (i *FileInfo) UnmarshalJson(data []byte) error {
	info := struct {
		Mode     string `json:"mode"`
		ModTime  string `json:"mod_time"`
		Size     int    `json:"size"`
		Name     string `json:"name"`
		ID       string `json:"id,omitempty"`
		Metadata string `json:"metadata,omitempty"`
	}{}
	err := json.Unmarshal(data, &info)
	if err != nil {
		return err
	}

	mode, err := ParseFileMode(info.Mode)
	if err != nil {
		return err
	}
	mtime, err := time.Parse(time.RFC3339, info.ModTime)
	if err != nil {
		return err
	}
	i.mode = mode
	i.mtime = mtime
	i.name = info.Name
	i.size = int64(info.Size)
	if len(info.ID) == 90 {
		id, err := c4.Parse(info.ID)
		if err != nil {
			return err
		}
		i.id = id
	}

	if len(info.Metadata) == 90 {
		id, err := c4.Parse(info.Metadata)
		if err != nil {
			return err
		}
		i.metadata = id
	}

	return nil
}

func ParseFileInfo(line string) (*FileInfo, error) {
	line = strings.TrimSpace(line)
	x := strings.Index(line, " ")

	mode, err := ParseFileMode(line[:x])
	if err != nil {
		return nil, err
	}
	// fmt.Printf("line[:x] = %q\n", line[:x])

	line = strings.TrimSpace(line[x:])
	x = strings.Index(line, " ")
	size, err := strconv.Atoi(line[:x])
	if err != nil {
		return nil, err
	}
	// fmt.Printf("line[:x] = %q\n", line[:x])

	line = strings.TrimSpace(line[x:])
	x = strings.Index(line, " ")
	mtime, err := time.Parse(time.RFC3339, line[:x])
	if err != nil {
		return nil, err
	}
	// fmt.Printf("line[:x] = %q\n", line[:x])
	line = strings.TrimSpace(line[x:])
	x = strings.Index(line, " ")
	if x == -1 {
		x = len(line)
	}
	name := strings.TrimRight(line[:x], "/")
	info := &FileInfo{
		mode:  mode,
		size:  int64(size),
		mtime: mtime,
		name:  name,
	}

	// fmt.Printf("line[:x] = %q\n", line[:x])
	line = strings.TrimSpace(line[x:])
	if len(line) < 90 {
		return info, nil
	}
	// fmt.Printf("line[:x] = %q\n", line[:90])
	id, err := c4.Parse(line[:90])
	if err != nil {
		return nil, err
	}
	info.id = id
	line = strings.TrimSpace(line[x:])
	if len(line) < 90 {
		return info, nil
	}
	// fmt.Printf("line[:x] = %q\n", line[:90])
	id, err = c4.Parse(line[:90])
	if err != nil {
		return nil, err
	}
	info.metadata = id

	return info, nil
}

type infoStringer struct {
	sizepadding int
	namepadding int
	i           *FileInfo
}

func (is infoStringer) String() string {
	output := is.i.mode.String()
	sizestr := strconv.Itoa(int(is.i.Size()))
	output += fmt.Sprintf("%*s%s ", is.sizepadding-len(sizestr), " ", sizestr)
	output += is.i.mtime.Format(time.RFC3339) + " "
	output += is.i.Name()
	if is.i.mode.IsDir() {
		output += "/"
	}
	if !is.i.id.IsNil() {
		output += fmt.Sprintf("%*s%s ", is.namepadding-len(is.i.Name()), " ", is.i.id.String())
		if !is.i.metadata.IsNil() {
			output += " " + is.i.metadata.String()
		}
	}

	return output
}

func MakeFileInfo(mode os.FileMode, size int64, mtime time.Time, name string, id, metadata c4.ID) *FileInfo {
	return &FileInfo{
		mode:     mode,
		size:     size,
		mtime:    mtime.UTC(),
		name:     name,
		id:       id,
		metadata: metadata,
	}
}

func NewFileInfo(info os.FileInfo, ids ...c4.ID) *FileInfo {
	out := &FileInfo{
		mode:  info.Mode(),
		size:  info.Size(),
		mtime: info.ModTime().UTC(),
		name:  info.Name(),
	}

	if fileinfo, ok := info.(*FileInfo); ok {
		out.id = fileinfo.id
		out.metadata = fileinfo.metadata
	}

	if len(ids) > 0 {
		out.id = ids[0]
		if len(ids) > 1 {
			out.metadata = ids[1]
		}
	}

	return out
}

// type Manifest []string

// func (m Manifest) Len() int      { return len(m) }
// func (m Manifest) Swap(i, j int) { m[i], m[j] = m[j], m[i] }
// func (m Manifest) Less(i, j int) bool {
// 	return less(m[i], m[j])
// }

// func depth(path string) int {
// 	var i int
// 	for _, c := range []byte(path) {
// 		if c != '/' {
// 			continue
// 		}
// 		i++
// 	}
// 	return i
// }

func less(a, b string) bool {
	ax := strings.LastIndex(a, "/")
	if ax < 0 {
		panic(fmt.Sprintf("last index x: %d %q", ax, a))
	}

	bx := strings.LastIndex(b, "/")
	if bx < 0 {
		panic(fmt.Sprintf("last index x: %d %q", bx, a))
	}

	if a[:ax] != b[:bx] {
		return a[:ax] < b[:bx]
	}

	return a[ax:] < b[bx:]
}

// 	if len(b) > len(a) {
// 	}

// 	items := []struct {
// 		path string
// 		x    int
// 	}{
// 		{path: "/foo/", x: 4},
// 		{path: "/foo/bar", x: 4},
// 		{path: "/foo/baz", x: 4},
// 		{path: "/foo/ba/", x: 7},
// 		{path: "/foo/ba/some", x: 7},
// 		{path: "/zoo/", x: 4},
// 		{path: "/zoo/foo", x: 4},
// 		{path: "/zoo/fes/", x: 7},
// 		{path: "/zoo/fes/flop", x: 7},
// 	}

// 	shufledItems := []struct {
// 		path string
// 		x    int
// 	}{
// 		{path: "/zoo/fes/flop", x: 8}, // /zoo/fes
// 		{path: "/zoo/", x: 4},         // /zoo
// 		{path: "/zoo/fes/", x: 8},     // /zoo/fes
// 		{path: "/foo/bar", x: 4},      // /foo
// 		{path: "/foo/ba/some", x: 7},  // /foo/ba
// 		{path: "/foo/", x: 4},         // /foo
// 		{path: "/zoo/foo", x: 4},      // /zoo
// 		{path: "/foo/baz", x: 4},      // /foo
// 		{path: "/foo/ba/", x: 7},      // /foo/ba
// 	}

// 	sortedItems := []struct {
// 		path string
// 		x    int
// 	}{
// 		{path: "/foo/", x: 4},         // /foo :: "/"
// 		{path: "/foo/bar", x: 4},      // /foo :: "/bar"
// 		{path: "/foo/baz", x: 4},      // /foo :: "/baz"
// 		{path: "/foo/ba/", x: 7},      // /foo/ba ::  "/"
// 		{path: "/foo/ba/some", x: 7},  // /foo/ba :: "/some"
// 		{path: "/zoo/", x: 4},         // /zoo :: "/"
// 		{path: "/zoo/foo", x: 4},      // /zoo :: "/foo"
// 		{path: "/zoo/fes/", x: 8},     // /zoo/fes ::  "/"
// 		{path: "/zoo/fes/flop", x: 8}, // /zoo/fes ::  "/flop"

// 	}

// 	a = strings.Replace(a, "/", string(0), -1)
// 	b = strings.Replace(b, "/", string(0), -1)

// 	return a < b
// }

// type ManifestFile struct {
// 	i int
// 	m Manifest
// }

// Manifest type
type M map[string]*FileInfo

func NewManifest() *M {
	m := make(map[string]*FileInfo)
	return (*M)(&m)
}
func (mm *M) Len() int {
	m := (map[string]*FileInfo)(*mm)
	return len(m)
}

func (mm *M) SetFileInfo(path string, info os.FileInfo) {

	m := (map[string]*FileInfo)(*mm)
	if fileinfo, ok := info.(*FileInfo); ok {
		m[path] = fileinfo
		return
	}

	m[path] = NewFileInfo(info)
	// mm = (*M)(&m)
}

func (mm *M) SetId(path string, id c4.ID) {
	m := (map[string]*FileInfo)(*mm)
	info, ok := m[path]
	if !ok {
		panic("cannot set id no such path in manifest " + path)
	}
	info.id = id
	m[path] = info
	// mm = (*M)(&m)
}

func (mm *M) SetMetadata(path string, id c4.ID) {
	m := (map[string]*FileInfo)(*mm)
	info, ok := m[path]
	if !ok {
		panic("cannot set metadata id no such path in manifest " + path)
	}
	info.metadata = id
	m[path] = info
	// mm = (*M)(&m)
}

// type paths [][]byte

// func (p paths) Len() int           { return len(p) }
// func (p paths) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
// func (p paths) Less(i, j int) bool { return bytes.Compare(p[i], p[j]) < 0 }
func (mm *M) Get(path string) *FileInfo {
	m := (map[string]*FileInfo)(*mm)
	return m[path]
}

func (mm *M) Paths() []string {
	m := (map[string]*FileInfo)(*mm)
	paths := make([]string, mm.Len())
	i := 0
	for k := range m {
		paths[i] = k
		i++
	}
	nlist := newNilList(paths)
	return nlist.StringSlice()
}

func (mm *M) Marshal() ([]byte, error) {
	// fmt.Printf("Marshal\n")
	m := (map[string]*FileInfo)(*mm)
	paths := make([]string, mm.Len())
	i := 0
	var maxsize int
	var maxname int
	for k, info := range m {
		sizestr := strconv.Itoa(int(info.Size()))
		if len(sizestr) > maxsize {
			maxsize = len(sizestr)
		}
		if len(info.Name()) > maxname {
			maxname = len(info.Name())
		}

		paths[i] = k
		i++
	}

	nlist := newNilList(paths)
	// fmt.Printf("len(nlist): %d\n", nlist.Len())
	var buff bytes.Buffer

	copy(paths, nlist.StringSlice())

	var ids c4.IDs
	for _, path := range paths {
		info := m[path]
		if !info.ID().IsNil() {
			ids = append(ids, info.ID())
			if !info.Metadata().IsNil() {
				ids = append(ids, info.Metadata())
			}
		}

		depth := len(strings.Split(path, "/")) - 1
		fmt.Fprintf(&buff, strings.Repeat("\t", depth)+"%s\n", info.MkString(maxsize, maxname))
	}
	sort.Sort(ids)
	ids = ids[:set.Uniq(ids)]
	for _, id := range ids {
		fmt.Fprintf(&buff, id.String()+"\n")
	}

	return buff.Bytes(), nil
}

func (mm *M) Unmarshal(r io.Reader) error {
	m := (map[string]*FileInfo)(*mm)
	buff := bufio.NewReader(r)
	var done bool
	var currentpath []string
	for !done {

		line, err := buff.ReadString('\n')
		if err == io.EOF {
			done = true
		}
		if len(line) == 0 {
			continue
		}
		var depth int
		for i, c := range line {
			if c != '\t' {
				break
			}
			depth = i + 1
		}
		currentpath = currentpath[:depth]

		if len(line) == 91 {
			_, err := c4.Parse(line[:len(line)-1])
			if err == nil {
				done = true
				continue
			}
		}

		info, err := ParseFileInfo(line)
		if err != nil {
			return err
		}
		path := filepath.Join(strings.Join(currentpath, "/"), info.Name())
		m[path] = info
		if info.IsDir() {
			currentpath = append(currentpath, info.Name())
		}
	}
	return nil
}
