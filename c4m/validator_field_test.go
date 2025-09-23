package c4m

import (
	"strings"
	"testing"
)

func TestMissingFields(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing size in file entry",
			line:    "-rw-r--r-- 2025-09-19T12:00:00Z file.txt",
			wantErr: true,
			errMsg:  "insufficient fields",
		},
		{
			name:    "missing size in directory entry",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z dirname/",
			wantErr: true,
			errMsg:  "insufficient fields",
		},
		{
			name:    "missing timestamp",
			line:    "-rw-r--r-- 100 file.txt",
			wantErr: true,
			errMsg:  "insufficient fields",
		},
		{
			name:    "missing mode",
			line:    "2025-09-19T12:00:00Z 100 file.txt",
			wantErr: true,
			errMsg:  "insufficient fields",
		},
		{
			name:    "only mode provided",
			line:    "-rw-r--r--",
			wantErr: true,
			errMsg:  "insufficient fields",
		},
		{
			name:    "valid minimum file entry",
			line:    "-rw-r--r-- 2025-09-19T12:00:00Z 100 file.txt",
			wantErr: false,
		},
		{
			name:    "valid minimum directory entry",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 0 dirname/",
			wantErr: false,
		},
		{
			name:    "directory with non-zero size",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 4096 dirname/",
			wantErr: false,
		},
		{
			name:    "directory with null size",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z - dirname/",
			wantErr: false,
		},
		{
			name:    "file with null size",
			line:    "-rw-r--r-- 2025-09-19T12:00:00Z - emptyfile.txt",
			wantErr: false,
		},
		{
			name:    "symlink with target",
			line:    "lrwxrwxrwx 2025-09-19T12:00:00Z 7 link -> target",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(false)
			validator.errors = nil
			validator.lineNum = 1
			validator.seenPaths = make(map[string]int)
			validator.depthStack = []string{}
			validator.seenDirAtDepth = make(map[int]bool)
			validator.validateEntry(tt.line)

			hasError := len(validator.errors) > 0
			if hasError != tt.wantErr {
				t.Errorf("validateEntry() hasError = %v, wantErr %v", hasError, tt.wantErr)
				if hasError {
					t.Logf("Errors: %v", validator.errors)
				}
			}

			if tt.wantErr && hasError && tt.errMsg != "" {
				found := false
				for _, err := range validator.errors {
					if strings.Contains(err.Message, tt.errMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, validator.errors)
				}
			}
		})
	}
}

func TestDirectoryNameParsing(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantName string
		wantErr  bool
	}{
		{
			name:     "simple directory name",
			line:     "drwxr-xr-x 2025-09-19T12:00:00Z 0 dirname/",
			wantName: "dirname/",
			wantErr:  false,
		},
		{
			name:     "directory with spaces",
			line:     "drwxr-xr-x 2025-09-19T12:00:00Z 0 my directory name/",
			wantName: "my directory name/",
			wantErr:  false,
		},
		{
			name:     "directory starting with number",
			line:     "drwxr-xr-x 2025-09-19T12:00:00Z 100 0 dirname/",
			wantName: "0 dirname/",
			wantErr:  false,
		},
		{
			name:     "quoted directory (non-canonical)",
			line:     "drwxr-xr-x 2025-09-19T12:00:00Z 0 \"dirname\"/",
			wantName: "dirname/",
			wantErr:  false, // Should parse with warning
		},
		{
			name:     "quoted directory including slash",
			line:     "drwxr-xr-x 2025-09-19T12:00:00Z 0 \"dirname/\"",
			wantName: "dirname/",
			wantErr:  false, // Should parse with warning
		},
		{
			name:     "invalid name: current directory",
			line:     "drwxr-xr-x 2025-09-19T12:00:00Z 0 ./",
			wantName: "./",
			wantErr:  true, // Should be flagged as invalid
		},
		{
			name:     "invalid name: parent directory",
			line:     "drwxr-xr-x 2025-09-19T12:00:00Z 0 ../",
			wantName: "../",
			wantErr:  true, // Should be flagged as invalid
		},
		{
			name:     "directory with complex name",
			line:     "drwxr-xr-x 2025-09-19T12:00:00Z 256 my-project_v2.0 (beta)/",
			wantName: "my-project_v2.0 (beta)/",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(false)
			validator.errors = nil
			validator.lineNum = 1
			validator.seenPaths = make(map[string]int)
			validator.depthStack = []string{}
			validator.seenDirAtDepth = make(map[int]bool)

			validator.validateEntry(tt.line)

			hasError := len(validator.errors) > 0
			if hasError != tt.wantErr {
				t.Errorf("validateEntry() hasError = %v, wantErr %v", hasError, tt.wantErr)
				if hasError {
					t.Logf("Errors: %v", validator.errors)
				}
			}

			// Check that the correct path was parsed
			if !tt.wantErr && validator.currentPath != tt.wantName {
				t.Errorf("Expected parsed name %q, got %q", tt.wantName, validator.currentPath)
			}
		})
	}
}

func TestSizeFieldValidation(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "zero size directory",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 0 empty/",
			wantErr: false,
		},
		{
			name:    "non-zero size directory",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 1024 data/",
			wantErr: false,
		},
		{
			name:    "null size directory",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z - pending/",
			wantErr: false,
		},
		{
			name:    "invalid size format",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z abc dirname/",
			wantErr: true,
			errMsg:  "invalid size",
		},
		{
			name:    "negative size",
			line:    "-rw-r--r-- 2025-09-19T12:00:00Z -100 file.txt",
			wantErr: true,
			errMsg:  "size cannot be less than -1",
		},
		{
			name:    "file with zero size",
			line:    "-rw-r--r-- 2025-09-19T12:00:00Z 0 empty.txt",
			wantErr: false,
		},
		{
			name:    "file with null size",
			line:    "-rw-r--r-- 2025-09-19T12:00:00Z - unknown.txt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(false)
			validator.errors = nil
			validator.lineNum = 1
			validator.seenPaths = make(map[string]int)
			validator.depthStack = []string{}
			validator.seenDirAtDepth = make(map[int]bool)

			validator.validateEntry(tt.line)

			hasError := len(validator.errors) > 0
			if hasError != tt.wantErr {
				t.Errorf("validateEntry() hasError = %v, wantErr %v", hasError, tt.wantErr)
				if hasError {
					t.Logf("Errors: %v", validator.errors)
				}
			}

			if tt.wantErr && hasError && tt.errMsg != "" {
				found := false
				for _, err := range validator.errors {
					if strings.Contains(err.Message, tt.errMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, validator.errors)
				}
			}
		})
	}
}

func TestInvalidDirectoryNames(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "current directory",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 0 ./",
			wantErr: true,
			errMsg:  "path traversal not allowed",
		},
		{
			name:    "parent directory",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 0 ../",
			wantErr: true,
			errMsg:  "path traversal not allowed",
		},
		{
			name:    "root directory",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 0 /",
			wantErr: true,
			errMsg:  "directory name cannot be just '/'",
		},
		{
			name:    "empty directory name",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 0 /",
			wantErr: true,
			errMsg:  "directory name cannot be just '/'",
		},
		{
			name:    "valid directory",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 0 valid/",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(true) // Strict mode
			validator.errors = nil
			validator.lineNum = 1
			validator.seenPaths = make(map[string]int)
			validator.depthStack = []string{}
			validator.seenDirAtDepth = make(map[int]bool)

			validator.validateEntry(tt.line)

			hasError := len(validator.errors) > 0
			if hasError != tt.wantErr {
				t.Errorf("validateEntry() hasError = %v, wantErr %v", hasError, tt.wantErr)
				if hasError {
					t.Logf("Errors: %v", validator.errors)
				}
			}

			if tt.wantErr && hasError && tt.errMsg != "" {
				found := false
				for _, err := range validator.errors {
					if strings.Contains(err.Message, tt.errMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, validator.errors)
				}
			}
		})
	}
}