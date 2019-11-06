package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestLoggerStore(t *testing.T) {
	path := os.TempDir()
	path = filepath.Join(path, "logger_test")
	err := os.Mkdir(path, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(path)
	buff := new(bytes.Buffer)
	logger := NewLogger(Folder(path), buff, 0)
	var st Store
	st = logger
	// Create arbitrary test data
	testdata := "foo"
	id := c4.Identify(strings.NewReader(testdata))

	// Test Logger Create
	w, err := st.Create(id)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 512)

	n, _ := buff.Read(data)
	if string(data[:n]) != fmt.Sprintf("%s Create\n", id) {
		t.Errorf("log output for Create does not match expected")
	}
	// Test Logger io.WriteCloser Write
	_, err = w.Write([]byte(testdata))
	if err != nil {
		t.Fatal(err)
	}

	n, _ = buff.Read(data)
	if string(data[:n]) != fmt.Sprintf("%s Write %d\n", id, len(testdata)) {
		t.Errorf("log output for Write does not match expected")
	}
	// Test Logger io.WriteCloser Close
	err = w.Close()
	if err != nil {
		t.Fatal(err)
	}

	n, _ = buff.Read(data)
	if string(data[:n]) != fmt.Sprintf("%s Close\n", id) {
		t.Errorf("log output for Close does not match expected")
	}

	// Test Logger Open
	f, err := st.Open(id)
	if err != nil {
		t.Error(err)
	}

	n, _ = buff.Read(data)
	if string(data[:n]) != fmt.Sprintf("%s Open\n", id) {
		t.Errorf("log output for Open does not match expected")
	}

	data2 := make([]byte, 512)
	n, err = f.Read(data2)
	if err != nil {
		t.Error(err)
	}
	data2 = data2[:n]
	if string(data2) != testdata {
		t.Errorf("wrong data read from file, expted %q, go %q", testdata, string(data2))
	}

	n, _ = buff.Read(data)
	if string(data[:n]) != fmt.Sprintf("%s Read %d\n", id, len(testdata)) {
		t.Errorf("log output for Read does not match expected")
	}

	_, err = f.Read(data2)
	if err != io.EOF {
		t.Errorf("expected io.EOF, but got %v", err)
	}
	n, _ = buff.Read(data)
	if string(data[:n]) != fmt.Sprintf("%s Read %d\n%s Read error EOF\n", id, 0, id) {
		t.Errorf("log output for Read does not match expected")
	}

	f.Close()
	n, _ = buff.Read(data)
	if string(data[:n]) != fmt.Sprintf("%s Close\n", id) {
		t.Errorf("log output for Close does not match expected")
	}
}
