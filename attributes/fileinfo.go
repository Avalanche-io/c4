package attributes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"reflect"
	"time"

	"github.com/etcenter/c4/asset"
)

type FsInfo interface {
	ID() *asset.ID
	Name() string
	Regular() bool
	Folder() bool
	Link() bool
	Pipe() bool
	Socket() bool
	Device() bool
	EncodedNestedJsonChan(io.Writer) chan<- FsInfo
}

type FileInfo struct {
	Id   *asset.ID
	Info os.FileInfo
}

type FolderInfo struct {
	Ids  asset.IDSlice
	Info os.FileInfo
	Size *big.Int
}

type KeyFsInfo map[string]FsInfo

func NewFsInfo(info os.FileInfo) FsInfo {
	if info.IsDir() {
		size := big.NewInt(0)
		f := FolderInfo{
			Info: info,
			Size: size}
		return &f
	} else {
		f := FileInfo{nil, info}
		return &f
	}
}

// FileInfo

func (f *FileInfo) ID() *asset.ID {
	return f.Id
}

func (f *FileInfo) Name() string {
	return f.Info.Name()
}

func (f *FileInfo) Bytes() *big.Int {
	size := big.NewInt(f.Info.Size())
	return size
}

func (f *FileInfo) Regular() bool {
	return (f.Info.Mode() & os.ModeType) == 0
}

func (f *FileInfo) Folder() bool {
	return (f.Info.Mode() & os.ModeDir) != 0
}

func (f *FileInfo) Link() bool {
	return (f.Info.Mode() & os.ModeSymlink) != 0
}

func (f *FileInfo) Pipe() bool {
	return (f.Info.Mode() & os.ModeNamedPipe) != 0
}

func (f *FileInfo) Socket() bool {
	return (f.Info.Mode() & os.ModeSocket) != 0
}

func (f *FileInfo) Device() bool {
	return (f.Info.Mode() & os.ModeDevice) != 0
}

type osfileinfo struct {
	Name    string      `json:"name"`
	Size    int64       `json:"size"`
	Mode    uint32      `json:"mode"`
	ModTime time.Time   `json:"modtime"`
	IsDir   bool        `json:"isdir"`
	Sys     interface{} `json:"sys,omitempty"`
}

// MarshalJSON adds output support for package encoding/json.
func (f *FileInfo) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(f.Id)
	if err != nil {
		return nil, err
	}
	output := []byte(`{"id":`)
	output = append(output, data...)
	output = append(output, []byte(`,`)...)
	info := osfileinfo{
		f.Info.Name(),
		f.Info.Size(),
		uint32(f.Info.Mode()),
		f.Info.ModTime().UTC(),
		f.Info.IsDir(),
		nil,
		// f.Info.Sys(),
	}
	data, err = json.Marshal(&info)
	if err != nil {
		return nil, err
	}
	output = append(output, []byte(`"info":`)...)
	output = append(output, data...)
	output = append(output, byte('}'))
	// fmt.Printf("data: %s\n", string(output))
	return output, nil
}

func (f *FileInfo) EncodedNestedJsonChan(w io.Writer) chan<- FsInfo {
	ch := make(chan FsInfo)
	go func() {
		JsonEncodeArrayChan(w, ch)
	}()
	return ch
}

// FolderInfo

func (f *FolderInfo) ID() *asset.ID {
	f.Ids.Sort()
	id, err := f.Ids.ID()
	if err != nil {
		panic(err)
	}

	return id
}

func (f *FolderInfo) Name() string {
	return f.Info.Name()
}

func (f *FolderInfo) Bytes() *big.Int {
	size := big.NewInt(f.Info.Size())
	return size
}

func (f *FolderInfo) Regular() bool {
	return (f.Info.Mode() & os.ModeType) == 0
}

func (f *FolderInfo) Folder() bool {
	return (f.Info.Mode() & os.ModeDir) != 0
}

func (f *FolderInfo) Link() bool {
	return (f.Info.Mode() & os.ModeSymlink) != 0
}

func (f *FolderInfo) Pipe() bool {
	return (f.Info.Mode() & os.ModeNamedPipe) != 0
}

func (f *FolderInfo) Socket() bool {
	return (f.Info.Mode() & os.ModeSocket) != 0
}

func (f *FolderInfo) Device() bool {
	return (f.Info.Mode() & os.ModeDevice) != 0
}

func (f *FolderInfo) AttributesJSON() ([]byte, error) {
	id, err := f.Ids.ID()
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	output := []byte(`"id":`)
	output = append(output, data...)
	output = append(output, []byte(`,`)...)
	info := osfileinfo{
		f.Info.Name(),
		f.Info.Size(),
		uint32(f.Info.Mode()),
		f.Info.ModTime().UTC(),
		f.Info.IsDir(),
		nil,
		// f.Info.Sys(),
	}
	data, err = json.Marshal(&info)
	if err != nil {
		return nil, err
	}
	output = append(output, []byte(`"info":`)...)
	output = append(output, data...)
	return output, nil
}

func (f *FolderInfo) MarshalJSON() ([]byte, error) {

	output := []byte(`{`)
	b, err := f.AttributesJSON()
	if err != nil {
		return nil, err
	}
	output = append(output, b...)
	output = append(output, byte('}'))
	// fmt.Printf("data: %s\n", string(output))
	return output, nil
}

// func (k KeyFsInfo) EncodeJSON() ([]byte, error) {
// 	items := []string{}
// 	for k, v := range k {
// 		data, err := json.Marshal(v)
// 		if err != nil {
// 			return nil, err
// 		}
// 		item := `"` + k + `":` + string(data)
// 		items = append(items, item)
// 	}
// 	return []byte(strings.Join(items, ",")), nil
// }

// From issue #11940 at github.com/golang/go code by lukescott.
// This function encodes arbitrarily long streams of json objects
// as arrays.
func JsonEncodeArrayChan(w io.Writer, vc interface{}) (err error) {
	cval := reflect.ValueOf(vc)
	_, err = w.Write([]byte{'['})
	if err != nil {
		return
	}
	var buf *bytes.Buffer
	var enc *json.Encoder
	v, ok := cval.Recv()
	if !ok {
		goto End
	}
	// create buffer & encoder only if we have a value
	buf = new(bytes.Buffer)
	enc = json.NewEncoder(buf)
	goto Encode
Loop:
	v, ok = cval.Recv()
	if !ok {
		goto End
	}
	if _, err = w.Write([]byte{','}); err != nil {
		return
	}
Encode:
	err = enc.Encode(v.Interface())
	if err == nil {
		_, err = w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
		buf.Reset()
	}
	if err != nil {
		return
	}
	goto Loop
End:
	_, err = w.Write([]byte{']'})
	return
}

func (f *FolderInfo) EncodedNestedJsonChan(w io.Writer) chan<- FsInfo {
	// fmt.Fprintf(os.Stderr, "\nEncodedNestedJsonChan %s\n", f.Name())

	_, err := w.Write([]byte(`"` + f.Name() + `":{`))
	if err != nil {
		return nil
	}
	ch := make(chan FsInfo)

	go func() {
		defer func() {
			_, err := w.Write([]byte{'}'})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}
		}()

		buf := new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		// comma := false
		for info := range ch {
			// fmt.Fprintf(os.Stderr, "\nrange %s\n", info.Name())
			// if comma {

			// }
			// comma = true
			_, err = buf.Write([]byte(`"` + info.Name() + `":`))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}

			switch info := info.(type) {
			// case *FolderInfo:
			// 	_, err = w.Write([]byte("FolderInfo"))
			// 	if err != nil {
			// 		fmt.Fprintf(os.Stderr, "%s\n", err)
			// 		break
			// 	}
			// 	continue
			case *FileInfo:
				err := enc.Encode(info)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err)
					break
				}
				_, err = w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
				buf.Reset()
			}
			_, err = w.Write([]byte{','})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				break
			}

		}

		// fmt.Fprintf(os.Stderr, "\nDone\n")

		b, err := f.AttributesJSON()
		if err != nil {
			return
		}
		// fmt.Fprintf(os.Stderr, "\nAttributesJson\n")
		_, err = w.Write(b)
		if err != nil {
			return
		}
		// fmt.Fprintf(os.Stderr, "\nWrote Attributes\n")

		_, err = w.Write([]byte{'}'})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
	}()
	return ch
}

// func JsonEncodeKVChan(w io.Writer, vc chan KeyFsInfo) (err error) {
// 	_, err = w.Write([]byte{'{'})
// 	if err != nil {
// 		return
// 	}
// 	defer func() {
// 		_, err = w.Write([]byte{'}'})
// 	}()
// 	var buf *bytes.Buffer
// 	var b []byte
// 	v, ok := <-vc
// 	if !ok {
// 		return
// 	}
// 	buf = new(bytes.Buffer)
// 	for {
// 		b, err = v.EncodeJSON()
// 		if err == nil {
// 			_, err = buf.Write(b)
// 			if err == nil {
// 				_, err = w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
// 				buf.Reset()
// 			}
// 		}
// 		if err != nil {
// 			return
// 		}
// 		v, ok = <-vc
// 		if !ok {
// 			return
// 		}
// 		if _, err = w.Write([]byte{','}); err != nil {
// 			return
// 		}
// 	}
// }
