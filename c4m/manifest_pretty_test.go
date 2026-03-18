package c4m

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestWritePretty(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	testID, _ := c4.Parse("c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")

	tests := []struct {
		name     string
		manifest *Manifest
		want     []string // Lines to check for in output
	}{
		{
			name: "pretty print with aligned C4 IDs",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      100,
						Name:      "a.txt",
						C4ID:      testID,
					},
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      1234567,
						Name:      "very_long_filename.txt",
						C4ID:      testID,
					},
					{
						Mode:      os.ModeDir | 0755,
						Timestamp: testTime,
						Size:      4096,
						Name:      "directory/",
						C4ID:      testID,
					},
				},
			},
			want: []string{
				"-rw-r--r-- Jan 15",                                                 // Timestamp starts with month
				"       100 a.txt",                                                  // Size padded
				" 1,234,567 very_long_filename.txt",                                 // Size with commas
				"drwxr-xr-x",                                                        // Directory mode
				"     4,096 directory/",                                             // Directory size
				"c41j3C6Jqga95PL",                                                   // C4 IDs aligned
			},
		},
		{
			name: "pretty print with very long lines",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      100,
						Name:      "this_is_a_very_very_very_long_filename_that_exceeds_normal_column_width.txt",
						C4ID:      testID,
					},
				},
			},
			want: []string{
				"this_is_a_very_very_very_long_filename_that_exceeds_normal_column_width.txt",
				"c41j3C6Jqga95PL", // C4 ID should be pushed to next column boundary
			},
		},
		{
			name: "pretty print with symlinks",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{
						Mode:      os.ModeSymlink | 0777,
						Timestamp: testTime,
						Size:      0,
						Name:      "link",
						Target:    "target.txt",
						C4ID:      testID,
					},
				},
			},
			want: []string{
				"lrwxrwxrwx Jan 15",  // Timestamp starts with month
				"0 link -> target.txt",  // Size and link
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := NewEncoder(&buf).SetPretty(true).Encode(tt.manifest)
			if err != nil {
				t.Fatalf("Encode (pretty) error = %v", err)
			}

			output := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("Output missing expected content %q\nGot:\n%s", want, output)
				}
			}
		})
	}
}

func TestFormatSizeWithCommas(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{0, "0"},
		{123, "123"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{12345678, "12,345,678"},
		{123456789, "123,456,789"},
		{1234567890, "1,234,567,890"},
		{-1234, "-1,234"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatSizeWithCommas(tt.size)
			if got != tt.want {
				t.Errorf("formatSizeWithCommas(%d) = %q, want %q", tt.size, got, tt.want)
			}
		})
	}
}

func TestPrettyPrintComparison(t *testing.T) {
	// Create a manifest with varied content
	manifest := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      42,
				Name:      "small.txt",
				C4ID:      c4.ID{},
			},
			{
				Mode:      0755,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      1234567,
				Name:      "large.sh",
				C4ID:      c4.ID{},
			},
		},
	}

	// Generate both canonical and pretty output
	var canonical bytes.Buffer
	err := NewEncoder(&canonical).Encode(manifest)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var pretty bytes.Buffer
	err = NewEncoder(&pretty).SetPretty(true).Encode(manifest)
	if err != nil {
		t.Fatalf("Encode (pretty) error = %v", err)
	}

	// Canonical should have no commas
	if strings.Contains(canonical.String(), ",") {
		t.Error("Canonical form should not contain commas")
	}

	// Pretty should have commas for large numbers
	if !strings.Contains(pretty.String(), "1,234,567") {
		t.Error("Pretty form should contain comma-formatted numbers")
	}

	// Pretty should have padded sizes
	if !strings.Contains(pretty.String(), "      42") {
		t.Error("Pretty form should have padded sizes")
	}
}