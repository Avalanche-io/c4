package c4m

import (
	"sort"
	"testing"
)

func TestNaturalSort(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "basic numeric",
			input: []string{"file10.txt", "file2.txt", "file1.txt"},
			want:  []string{"file1.txt", "file2.txt", "file10.txt"},
		},
		{
			name:  "multiple numeric sequences",
			input: []string{"a10b20", "a2b3", "a10b3", "a2b20"},
			want:  []string{"a2b3", "a2b20", "a10b3", "a10b20"},
		},
		{
			name:  "leading zeros",
			input: []string{"file001.txt", "file010.txt", "file100.txt", "file002.txt"},
			want:  []string{"file001.txt", "file002.txt", "file010.txt", "file100.txt"},
		},
		{
			name:  "mixed with directories",
			input: []string{"file10.txt", "dir2/", "file2.txt", "dir10/", "file1.txt", "dir1/"},
			want:  []string{"dir1/", "dir2/", "dir10/", "file1.txt", "file2.txt", "file10.txt"},
		},
		{
			name:  "no numbers",
			input: []string{"zebra", "apple", "banana"},
			want:  []string{"apple", "banana", "zebra"},
		},
		{
			name:  "same prefix different extensions",
			input: []string{"file.zip", "file.txt", "file.doc"},
			want:  []string{"file.doc", "file.txt", "file.zip"},
		},
		{
			name:  "version numbers",
			input: []string{"v1.10.2", "v1.2.1", "v1.10.10", "v1.2.10"},
			want:  []string{"v1.2.1", "v1.2.10", "v1.10.2", "v1.10.10"},
		},
		{
			name:  "frame sequences",
			input: []string{"frame.0100.exr", "frame.0010.exr", "frame.0001.exr", "frame.1000.exr"},
			want:  []string{"frame.0001.exr", "frame.0010.exr", "frame.0100.exr", "frame.1000.exr"},
		},
		{
			name:  "empty strings",
			input: []string{"", "a", "", "b"},
			want:  []string{"", "", "a", "b"},
		},
		{
			name:  "special characters",
			input: []string{"file@10.txt", "file#2.txt", "file$1.txt"},
			want:  []string{"file#2.txt", "file$1.txt", "file@10.txt"},
		},
		{
			name:  "hidden files",
			input: []string{".file10", ".file2", "file1", ".file1"},
			want:  []string{".file1", ".file2", ".file10", "file1"},
		},
		{
			name:  "case sensitivity",
			input: []string{"File10.txt", "file2.txt", "FILE1.txt"},
			want:  []string{"FILE1.txt", "File10.txt", "file2.txt"},
		},
		{
			name:  "unicode",
			input: []string{"文件10.txt", "文件2.txt", "文件1.txt"},
			want:  []string{"文件1.txt", "文件2.txt", "文件10.txt"},
		},
		{
			name:  "very large numbers",
			input: []string{"file999999999999.txt", "file1000000000000.txt", "file1.txt"},
			want:  []string{"file1.txt", "file999999999999.txt", "file1000000000000.txt"},
		},
		{
			name:  "negative numbers",
			input: []string{"temp-10.txt", "temp-2.txt", "temp-1.txt", "temp0.txt", "temp1.txt"},
			want:  []string{"temp0.txt", "temp1.txt", "temp-1.txt", "temp-2.txt", "temp-10.txt"},
		},
		{
			name:  "decimal numbers",
			input: []string{"file1.5.txt", "file1.10.txt", "file1.2.txt"},
			want:  []string{"file1.2.txt", "file1.5.txt", "file1.10.txt"},
		},
		{
			name:  "paths with natural sort",
			input: []string{"dir10/file2.txt", "dir2/file10.txt", "dir2/file2.txt", "dir10/file10.txt"},
			want:  []string{"dir2/file2.txt", "dir2/file10.txt", "dir10/file2.txt", "dir10/file10.txt"},
		},
		{
			name:  "identical strings",
			input: []string{"file.txt", "file.txt", "file.txt"},
			want:  []string{"file.txt", "file.txt", "file.txt"},
		},
		{
			name:  "numbers at start",
			input: []string{"10file.txt", "2file.txt", "1file.txt"},
			want:  []string{"1file.txt", "2file.txt", "10file.txt"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make([]string, len(tt.input))
			copy(got, tt.input)
			sort.Slice(got, func(i, j int) bool {
				return NaturalLess(got[i], got[j])
			})
			
			if len(got) != len(tt.want) {
				t.Errorf("Length mismatch: got %d, want %d", len(got), len(tt.want))
				return
			}
			
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNaturalLessDirect(t *testing.T) {
	// Test NaturalLess function directly
	tests := []struct {
		a    string
		b    string
		want bool // true if a < b
	}{
		{"file1.txt", "file10.txt", true},
		{"file10.txt", "file1.txt", false},
		{"file2.txt", "file2.txt", false},
		{"dir/", "file", true}, // directories first
	}
	
	for _, tt := range tests {
		got := NaturalLess(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("NaturalLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNaturalCompare(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int // -1 if a < b, 0 if a == b, 1 if a > b
	}{
		{
			name: "simple numeric",
			a:    "file2.txt",
			b:    "file10.txt",
			want: -1,
		},
		{
			name: "equal strings",
			a:    "file.txt",
			b:    "file.txt",
			want: 0,
		},
		{
			name: "alphabetic only",
			a:    "apple",
			b:    "banana",
			want: -1,
		},
		{
			name: "leading zeros same value",
			a:    "file001.txt",
			b:    "file1.txt",
			want: 1, // file1.txt comes before file001.txt
		},
		{
			name: "directory vs file",
			a:    "dir/",
			b:    "file",
			want: -1, // Directories first
		},
		{
			name: "empty vs non-empty",
			a:    "",
			b:    "file",
			want: -1,
		},
		{
			name: "case sensitive",
			a:    "File",
			b:    "file",
			want: -1, // Capital letters come first in ASCII
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got int
			if NaturalLess(tt.a, tt.b) {
				got = -1
			} else if NaturalLess(tt.b, tt.a) {
				got = 1
			} else {
				got = 0
			}
			
			if got != tt.want {
				t.Errorf("NaturalLess comparison(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNaturalSortEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		check func(t *testing.T, sorted []string)
	}{
		{
			name:  "nil slice",
			input: nil,
			check: func(t *testing.T, sorted []string) {
				if sorted != nil {
					t.Error("Expected nil slice to remain nil")
				}
			},
		},
		{
			name:  "empty slice",
			input: []string{},
			check: func(t *testing.T, sorted []string) {
				if len(sorted) != 0 {
					t.Error("Expected empty slice to remain empty")
				}
			},
		},
		{
			name:  "single element",
			input: []string{"file.txt"},
			check: func(t *testing.T, sorted []string) {
				if len(sorted) != 1 || sorted[0] != "file.txt" {
					t.Error("Single element slice should remain unchanged")
				}
			},
		},
		{
			name:  "already sorted",
			input: []string{"file1.txt", "file2.txt", "file10.txt"},
			check: func(t *testing.T, sorted []string) {
				expected := []string{"file1.txt", "file2.txt", "file10.txt"}
				for i, v := range sorted {
					if v != expected[i] {
						t.Errorf("Already sorted slice changed at index %d", i)
					}
				}
			},
		},
		{
			name:  "reverse sorted",
			input: []string{"file10.txt", "file2.txt", "file1.txt"},
			check: func(t *testing.T, sorted []string) {
				expected := []string{"file1.txt", "file2.txt", "file10.txt"}
				for i, v := range sorted {
					if v != expected[i] {
						t.Errorf("Index %d: got %q, want %q", i, v, expected[i])
					}
				}
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input != nil {
				sorted := make([]string, len(tt.input))
				copy(sorted, tt.input)
				sort.Slice(sorted, func(i, j int) bool {
					return NaturalLess(sorted[i], sorted[j])
				})
				tt.check(t, sorted)
			} else {
				tt.check(t, nil)
			}
		})
	}
}

func BenchmarkNaturalSort(b *testing.B) {
	// Create test data with various patterns
	data := []string{
		"file100.txt", "file20.txt", "file3.txt",
		"dir10/", "dir2/", "dir1/",
		"version1.10.2", "version1.2.1", "version1.10.10",
		"frame.0100.exr", "frame.0010.exr", "frame.0001.exr",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testData := make([]string, len(data))
		copy(testData, data)
		sort.Slice(testData, func(i, j int) bool {
			return NaturalLess(testData[i], testData[j])
		})
	}
}

func BenchmarkStandardSort(b *testing.B) {
	// Same test data for comparison
	data := []string{
		"file100.txt", "file20.txt", "file3.txt",
		"dir10/", "dir2/", "dir1/",
		"version1.10.2", "version1.2.1", "version1.10.10",
		"frame.0100.exr", "frame.0010.exr", "frame.0001.exr",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testData := make([]string, len(data))
		copy(testData, data)
		sort.Strings(testData)
	}
}