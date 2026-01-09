package c4m

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestIDList(t *testing.T) {
	t.Run("NewIDList creates empty list", func(t *testing.T) {
		list := NewIDList()
		if list.Count() != 0 {
			t.Errorf("expected 0 items, got %d", list.Count())
		}
	})

	t.Run("Add and Get", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))

		list.Add(id1)
		list.Add(id2)

		if list.Count() != 2 {
			t.Errorf("expected 2 items, got %d", list.Count())
		}

		if list.Get(0) != id1 {
			t.Errorf("expected id1 at index 0")
		}
		if list.Get(1) != id2 {
			t.Errorf("expected id2 at index 1")
		}

		// Out of bounds returns nil ID
		if !list.Get(-1).IsNil() {
			t.Errorf("expected nil ID for negative index")
		}
		if !list.Get(100).IsNil() {
			t.Errorf("expected nil ID for out of bounds index")
		}
	})

	t.Run("Canonical format", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))

		list.Add(id1)
		list.Add(id2)

		canonical := list.Canonical()

		// Should have trailing newline on each line
		lines := strings.Split(strings.TrimSuffix(canonical, "\n"), "\n")
		if len(lines) != 2 {
			t.Errorf("expected 2 lines, got %d", len(lines))
		}

		if lines[0] != id1.String() {
			t.Errorf("expected %s, got %s", id1.String(), lines[0])
		}
		if lines[1] != id2.String() {
			t.Errorf("expected %s, got %s", id2.String(), lines[1])
		}
	})

	t.Run("ComputeC4ID", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		c4id := list.ComputeC4ID()
		if c4id.IsNil() {
			t.Errorf("expected non-nil C4 ID")
		}

		// Same list should produce same C4 ID
		list2 := NewIDList()
		list2.Add(id1)
		c4id2 := list2.ComputeC4ID()
		if c4id != c4id2 {
			t.Errorf("expected same C4 ID for same list")
		}
	})
}

func TestParseIDList(t *testing.T) {
	t.Run("parse valid ID list", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))

		input := id1.String() + "\n" + id2.String() + "\n"
		list, err := ParseIDListFromString(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if list.Count() != 2 {
			t.Errorf("expected 2 items, got %d", list.Count())
		}
	})

	t.Run("tolerant of whitespace", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))

		// Extra whitespace, blank lines
		input := "\n  " + id1.String() + "  \n\n"
		list, err := ParseIDListFromString(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if list.Count() != 1 {
			t.Errorf("expected 1 item, got %d", list.Count())
		}
	})

	t.Run("invalid ID format", func(t *testing.T) {
		input := "not-a-valid-c4-id\n"
		_, err := ParseIDListFromString(input)
		if err == nil {
			t.Errorf("expected error for invalid C4 ID")
		}
	})
}

func TestIsIDListContent(t *testing.T) {
	t.Run("valid ID list content", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))
		content := []byte(id1.String() + "\n")
		if !IsIDListContent(content) {
			t.Errorf("expected true for valid ID list content")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		if IsIDListContent([]byte("")) {
			t.Errorf("expected false for empty content")
		}
	})

	t.Run("non-ID content", func(t *testing.T) {
		content := []byte("hello world\n")
		if IsIDListContent(content) {
			t.Errorf("expected false for non-ID content")
		}
	})

	t.Run("mixed content", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))
		content := []byte(id1.String() + "\nhello\n")
		if IsIDListContent(content) {
			t.Errorf("expected false for mixed content")
		}
	})
}

func TestDataBlock(t *testing.T) {
	t.Run("parse ID list data block", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		content := list.Canonical()
		listID := list.ComputeC4ID()

		block, err := ParseDataBlock(listID, content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !block.IsIDList {
			t.Errorf("expected IsIDList to be true")
		}

		if block.ID != listID {
			t.Errorf("expected ID %s, got %s", listID, block.ID)
		}
	})

	t.Run("parse base64 data block", func(t *testing.T) {
		// Some arbitrary non-ID content
		content := []byte("hello world")
		contentID := c4.Identify(bytes.NewReader(content))

		// Base64 encode
		encoded := "aGVsbG8gd29ybGQ=" // base64("hello world")

		block, err := ParseDataBlock(contentID, encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.IsIDList {
			t.Errorf("expected IsIDList to be false")
		}

		if string(block.Content) != "hello world" {
			t.Errorf("expected 'hello world', got '%s'", string(block.Content))
		}
	})

	t.Run("content hash mismatch", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		content := list.Canonical()
		wrongID := c4.Identify(strings.NewReader("wrong"))

		_, err := ParseDataBlock(wrongID, content)
		if err == nil {
			t.Errorf("expected error for hash mismatch")
		}
	})

	t.Run("GetIDList from block", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		content := list.Canonical()
		listID := list.ComputeC4ID()

		block, err := ParseDataBlock(listID, content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		retrieved, err := block.GetIDList()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if retrieved.Count() != 1 {
			t.Errorf("expected 1 item, got %d", retrieved.Count())
		}
	})
}

func TestFormatDataBlock(t *testing.T) {
	t.Run("format ID list block", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		block := CreateDataBlockFromIDList(list)
		formatted := FormatDataBlock(block)

		if !strings.HasPrefix(formatted, "@data ") {
			t.Errorf("expected to start with '@data '")
		}

		if !strings.Contains(formatted, id1.String()) {
			t.Errorf("expected to contain ID")
		}
	})
}

func TestCreateDataBlockFromIDList(t *testing.T) {
	list := NewIDList()
	id1 := c4.Identify(strings.NewReader("test1"))
	id2 := c4.Identify(strings.NewReader("test2"))
	list.Add(id1)
	list.Add(id2)

	block := CreateDataBlockFromIDList(list)

	if !block.IsIDList {
		t.Errorf("expected IsIDList to be true")
	}

	// Verify content matches canonical form
	if string(block.Content) != list.Canonical() {
		t.Errorf("content mismatch")
	}

	// Verify ID is correct
	expectedID := list.ComputeC4ID()
	if block.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, block.ID)
	}
}

func TestFormatDataBlockIDList(t *testing.T) {
	list := NewIDList()
	id1 := c4.Identify(strings.NewReader("test1"))
	id2 := c4.Identify(strings.NewReader("test2"))
	list.Add(id1)
	list.Add(id2)

	block := CreateDataBlockFromIDList(list)
	formatted := FormatDataBlock(block)

	// Should start with @data directive
	if !strings.HasPrefix(formatted, "@data ") {
		t.Errorf("expected to start with @data, got %q", formatted[:20])
	}

	// Should contain the block ID
	if !strings.Contains(formatted, block.ID.String()) {
		t.Errorf("expected to contain block ID")
	}

	// Should contain the C4 IDs in plain text (not base64)
	if !strings.Contains(formatted, id1.String()) {
		t.Errorf("expected to contain id1 in plain text")
	}
	if !strings.Contains(formatted, id2.String()) {
		t.Errorf("expected to contain id2 in plain text")
	}
}

func TestFormatDataBlockBinary(t *testing.T) {
	// Create a non-ID list data block with binary content
	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}
	id := c4.Identify(bytes.NewReader(binaryContent))

	block := &DataBlock{
		ID:       id,
		Content:  binaryContent,
		IsIDList: false,
	}

	formatted := FormatDataBlock(block)

	// Should start with @data directive
	if !strings.HasPrefix(formatted, "@data ") {
		t.Errorf("expected to start with @data")
	}

	// Should contain base64 encoded content (AAECA//+)
	// {0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE} encodes to "AAECA//+"
	if !strings.Contains(formatted, "AAECA//+") {
		t.Errorf("expected base64 encoded content 'AAECA//+', got %q", formatted)
	}

	// Should NOT contain raw binary
	if strings.Contains(formatted, string(binaryContent)) {
		t.Errorf("should not contain raw binary content")
	}
}

func TestFormatDataBlockLongContent(t *testing.T) {
	// Create a data block with content longer than 76 chars to test line wrapping
	longContent := bytes.Repeat([]byte("ABCDEFGHIJ"), 20) // 200 bytes
	id := c4.Identify(bytes.NewReader(longContent))

	block := &DataBlock{
		ID:       id,
		Content:  longContent,
		IsIDList: false,
	}

	formatted := FormatDataBlock(block)

	// Split into lines and check line lengths
	lines := strings.Split(formatted, "\n")
	for i, line := range lines {
		if i == 0 {
			continue // Skip @data directive line
		}
		if len(line) > 76 && line != "" {
			t.Errorf("line %d exceeds 76 chars: %d", i, len(line))
		}
	}
}

func TestDataBlockGetIDList(t *testing.T) {
	t.Run("returns error for non-ID list", func(t *testing.T) {
		block := &DataBlock{
			ID:       c4.Identify(strings.NewReader("test")),
			Content:  []byte("not an ID list"),
			IsIDList: false,
		}

		_, err := block.GetIDList()
		if err == nil {
			t.Error("expected error for non-ID list block")
		}
		if !strings.Contains(err.Error(), "not an ID list") {
			t.Errorf("expected 'not an ID list' error, got: %v", err)
		}
	})

	t.Run("returns ID list for valid block", func(t *testing.T) {
		// Create a valid ID list block
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))
		list.Add(id1)
		list.Add(id2)

		block := CreateDataBlockFromIDList(list)

		retrieved, err := block.GetIDList()
		if err != nil {
			t.Fatalf("GetIDList() error = %v", err)
		}

		if retrieved.Count() != 2 {
			t.Errorf("expected 2 IDs, got %d", retrieved.Count())
		}
		if retrieved.Get(0) != id1 {
			t.Errorf("expected first ID to be %s", id1)
		}
		if retrieved.Get(1) != id2 {
			t.Errorf("expected second ID to be %s", id2)
		}
	})
}
