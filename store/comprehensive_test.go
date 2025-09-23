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

func TestRAMStoreComprehensive(t *testing.T) {
	store := NewRAM()

	// Test Create and Write
	id := c4.Identify(strings.NewReader("ram test"))
	writer, err := store.Create(id)
	if err != nil {
		t.Fatal(err)
	}

	testData := []byte("ram test content")
	n, err := writer.Write(testData)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}

	err = writer.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Test Open and Read
	reader, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(content, testData) {
		t.Errorf("Expected %q, got %q", testData, content)
	}

	err = reader.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Test Remove
	err = store.Remove(id)
	if err != nil {
		t.Fatal(err)
	}

	// Verify removed
	_, err = store.Open(id)
	if err == nil {
		t.Error("Expected error opening removed item")
	}
}

func TestRAMStoreMultipleWrites(t *testing.T) {
	store := NewRAM()

	id := c4.Identify(strings.NewReader("multi"))
	writer, err := store.Create(id)
	if err != nil {
		t.Fatal(err)
	}

	// Write in multiple chunks
	chunks := [][]byte{
		[]byte("chunk1"),
		[]byte("chunk2"),
		[]byte("chunk3"),
	}

	for _, chunk := range chunks {
		_, err = writer.Write(chunk)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = writer.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Read back
	reader, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	expected := "chunk1chunk2chunk3"
	if string(content) != expected {
		t.Errorf("Expected %q, got %q", expected, string(content))
	}
}

func TestValidatingStoreComprehensive(t *testing.T) {
	backing := NewRAM()
	store := NewValidating(backing)

	// Test valid write
	validContent := "valid content"
	validID := c4.Identify(strings.NewReader(validContent))

	writer, err := store.Create(validID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = writer.Write([]byte(validContent))
	if err != nil {
		t.Fatal(err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Test valid read
	reader, err := store.Open(validID)
	if err != nil {
		t.Fatal(err)
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != validContent {
		t.Errorf("Expected %q, got %q", validContent, string(content))
	}

	err = reader.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Test Remove
	err = store.Remove(validID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidatingStoreInvalidContent(t *testing.T) {
	backing := NewRAM()
	store := NewValidating(backing)

	// Create with one ID but write different content
	wrongContent := "wrong content"
	correctContent := "correct content"
	id := c4.Identify(strings.NewReader(correctContent))

	writer, err := store.Create(id)
	if err != nil {
		t.Fatal(err)
	}

	_, err = writer.Write([]byte(wrongContent))
	if err != nil {
		t.Fatal(err)
	}

	// Close should detect validation error
	err = writer.Close()
	if err == nil {
		t.Error("Expected validation error on close")
	}
}

func TestLoggerStoreComprehensive(t *testing.T) {
	// Create temp log file
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "store.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}

	backing := NewRAM()
	store := NewLogger(backing, logFile, 0) // 0 means all flags enabled

	// Test operations that should be logged
	id := c4.Identify(strings.NewReader("logged content"))

	// Create
	writer, err := store.Create(id)
	if err != nil {
		t.Fatal(err)
	}

	// Write
	testData := []byte("logged content")
	_, err = writer.Write(testData)
	if err != nil {
		t.Fatal(err)
	}

	// Close writer
	err = writer.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Open
	reader, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}

	// Read
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(content, testData) {
		t.Errorf("Expected %q, got %q", testData, content)
	}

	// Close reader
	err = reader.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Remove
	err = store.Remove(id)
	if err != nil {
		t.Fatal(err)
	}

	// Check that log file has content
	logFile.Close()
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(logContent) == 0 {
		t.Error("Expected log file to have content")
	}
}

func TestFolderStoreComprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	store := Folder(tmpDir)

	// Test Create and Write
	id := c4.Identify(strings.NewReader("folder test"))
	writer, err := store.Create(id)
	if err != nil {
		t.Fatal(err)
	}

	testData := []byte("folder test content")
	_, err = writer.Write(testData)
	if err != nil {
		t.Fatal(err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Test Open and Read
	reader, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(content, testData) {
		t.Errorf("Expected %q, got %q", testData, content)
	}

	err = reader.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Test Remove
	err = store.Remove(id)
	if err != nil {
		t.Fatal(err)
	}

	// Verify removed
	_, err = store.Open(id)
	if err == nil {
		t.Error("Expected error opening removed item")
	}
}

func TestMAPStoreComprehensive(t *testing.T) {
	// MAP store uses file paths as values, so we need actual files
	tmpDir := t.TempDir()
	m := make(map[c4.ID]string)
	store := NewMap(m)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.dat")
	testData := []byte("test content")
	err := os.WriteFile(testFile, testData, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Map the ID to the file path
	id := c4.Identify(bytes.NewReader(testData))
	m[id] = testFile

	// Test Open
	reader, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(content, testData) {
		t.Errorf("Expected %q, got %q", testData, content)
	}

	err = reader.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Test Create (creates a new file at the path)
	newFile := filepath.Join(tmpDir, "new.dat")
	newID := c4.Identify(strings.NewReader("new"))
	m[newID] = newFile

	writer, err := store.Create(newID)
	if err != nil {
		t.Fatal(err)
	}

	newData := []byte("new content")
	_, err = writer.Write(newData)
	if err != nil {
		t.Fatal(err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Verify file was created
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("Expected file to be created")
	}

	// Test Remove
	err = store.Remove(id)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file was removed
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Expected file to be removed")
	}
}

func TestMAPStoreOperations(t *testing.T) {
	tmpDir := t.TempDir()
	m := make(map[c4.ID]string)
	store := NewMap(m)

	// Test Load on empty store
	id := c4.Identify(strings.NewReader("nonexistent"))
	value := store.Load(id)
	if value != "" {
		t.Error("Expected empty value for nonexistent key")
	}

	// Test LoadOrStore
	testFile := filepath.Join(tmpDir, "test.dat")
	actual, _ := store.LoadOrStore(id, testFile)
	if actual != testFile {
		t.Error("Expected stored value to match")
	}

	// Test LoadOrStore with existing key
	newFile := filepath.Join(tmpDir, "new.dat")
	actual, _ = store.LoadOrStore(id, newFile)
	if actual != testFile {
		t.Error("Expected original value for existing key")
	}

	// Test Delete
	store.Delete(id)
	value = store.Load(id)
	if value != "" {
		t.Error("Expected empty value after Delete")
	}
}

func TestMAPStoreRange(t *testing.T) {
	m := make(map[c4.ID]string)
	store := NewMap(m)

	// Add some items
	for i := 0; i < 5; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("test%d", i)))
		store.LoadOrStore(id, fmt.Sprintf("/path/to/file%d", i))
	}

	// Test Range
	count := 0
	store.Range(func(key c4.ID, value string) bool {
		count++
		return true // Continue iteration
	})

	if count != 5 {
		t.Errorf("Expected Range to iterate over 5 items, got %d", count)
	}

	// Test Range with early termination
	count = 0
	store.Range(func(key c4.ID, value string) bool {
		count++
		return count < 3 // Stop after 3 items
	})

	if count != 3 {
		t.Errorf("Expected Range to stop after 3 items, got %d", count)
	}
}