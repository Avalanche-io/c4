package c4m

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidator(t *testing.T) {
	// Test NewValidator
	validator := NewValidator(false)
	if validator == nil {
		t.Fatal("NewValidator returned nil")
	}
	if validator.Strict {
		t.Error("Expected non-strict validator")
	}

	strictValidator := NewValidator(true)
	if !strictValidator.Strict {
		t.Error("Expected strict validator")
	}
}

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name    string
		content string
		strict  bool
		wantErr bool
	}{
		{
			name: "valid manifest",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  false,
			wantErr: false,
		},
		{
			name: "missing header",
			content: `-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  false,
			wantErr: true,
		},
		{
			name: "invalid C4 ID",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt invalid-c4-id
`,
			strict:  true,
			wantErr: true,
		},
		{
			name: "directory entry with size",
			content: `@c4m 1.0
drwxr-xr-x 2025-09-19T12:00:00Z 0 dir/
`,
			strict:  false,
			wantErr: false,
		},
		{
			name: "with @base",
			content: `@c4m 1.0
@base c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  false,
			wantErr: false,
		},
		{
			name: "duplicate filenames in strict mode",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 200 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  true,
			wantErr: true,
		},
		{
			name: "unsorted entries in strict mode",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 z.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 a.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(tt.strict)
			err := validator.ValidateManifest(strings.NewReader(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEntryLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{
			name:    "valid file entry",
			line:    "-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp",
			wantErr: false,
		},
		{
			name:    "valid directory entry",
			line:    "drwxr-xr-x 2025-09-19T12:00:00Z 0 dir/",
			wantErr: false,
		},
		{
			name:    "invalid format",
			line:    "not a valid entry",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh validator for each test
			validator := NewValidator(false)
			validator.lineNum = 1
			validator.validateEntry(tt.line)
			hasError := len(validator.errors) > 0
			if hasError != tt.wantErr {
				t.Errorf("validateEntry() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidatorSortCheck(t *testing.T) {
	// Test that strict validator catches unsorted entries
	strictValidator := NewValidator(true)

	// Properly sorted manifest
	sortedContent := `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 a.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 b.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	err := strictValidator.ValidateManifest(strings.NewReader(sortedContent))
	if err != nil {
		t.Errorf("Sorted manifest should pass validation: %v", err)
	}

	// Unsorted manifest
	unsortedContent := `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 z.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 a.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	err = strictValidator.ValidateManifest(strings.NewReader(unsortedContent))
	// Strict validation should catch sorting issues
	if err == nil && strictValidator.Strict {
		t.Error("Strict validation should fail for unsorted entries")
	}
}

func TestValidatorDuplicates(t *testing.T) {
	strictValidator := NewValidator(true)

	// Test manifest with duplicates
	withDupsContent := `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 200 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	err := strictValidator.ValidateManifest(strings.NewReader(withDupsContent))
	if err == nil && strictValidator.Strict {
		t.Error("Strict validation should fail for duplicate entries")
	}

	// Check that duplicates are tracked
	if strictValidator.seenPaths["test.txt"] != 2 {
		t.Errorf("Expected 2 occurrences of test.txt, got %d", strictValidator.seenPaths["test.txt"])
	}
}

func TestValidationReport(t *testing.T) {
	// Create a manifest with various issues
	content := `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 z.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 a.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 a.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	// Test non-strict mode (should fail due to duplicates)
	validator := NewValidator(false)
	err := validator.ValidateManifest(strings.NewReader(content))
	if err == nil {
		t.Error("Non-strict validation should fail due to duplicate entries")
	}

	// Test strict mode (should fail)
	strictValidator := NewValidator(true)
	err = strictValidator.ValidateManifest(strings.NewReader(content))
	if err == nil {
		t.Error("Strict validation should fail for unsorted duplicates")
	}

	// Check that the validator tracked errors
	if len(strictValidator.errors) == 0 {
		t.Error("Expected validation errors to be tracked")
	}
}

func TestGetErrorsAndWarnings(t *testing.T) {
	// Create a manifest that generates a warning (unknown version)
	content := `@c4m 2.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	validator := NewValidator(false)
	validator.ValidateManifest(strings.NewReader(content))

	// GetErrors returns the errors slice (may be nil if no errors)
	errors := validator.GetErrors()
	// This manifest is valid but has a version warning, so no errors
	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}

	// GetWarnings should return warnings slice (version 2.0 generates a warning)
	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warning for unknown version 2.0")
	}

	// Test manifest with actual errors
	invalidContent := `not a valid manifest`
	validator2 := NewValidator(false)
	validator2.ValidateManifest(strings.NewReader(invalidContent))
	errors2 := validator2.GetErrors()
	if len(errors2) == 0 {
		t.Error("Expected errors for invalid manifest")
	}
}

func TestBuildPath(t *testing.T) {
	validator := NewValidator(false)

	// Test depth 0 (root level)
	path := validator.buildPath("file.txt", 0)
	if path != "file.txt" {
		t.Errorf("buildPath at depth 0: got %q, want %q", path, "file.txt")
	}

	// Set up depth stack for nested paths
	validator.depthStack = []string{"dir1/", "dir2/"}

	// Test depth 1
	path = validator.buildPath("file.txt", 1)
	if path != "dir1/file.txt" {
		t.Errorf("buildPath at depth 1: got %q, want %q", path, "dir1/file.txt")
	}

	// Test depth 2
	path = validator.buildPath("file.txt", 2)
	if path != "dir1/dir2/file.txt" {
		t.Errorf("buildPath at depth 2: got %q, want %q", path, "dir1/dir2/file.txt")
	}

	// Test depth beyond stack
	path = validator.buildPath("file.txt", 5)
	if path != "dir1/dir2/file.txt" {
		t.Errorf("buildPath beyond stack: got %q, want %q", path, "dir1/dir2/file.txt")
	}
}

func TestBuildExpectedPath(t *testing.T) {
	validator := NewValidator(false)

	// Test depth 0 (root level)
	path := validator.buildExpectedPath("file.txt", 0)
	if path != "file.txt" {
		t.Errorf("buildExpectedPath at depth 0: got %q, want %q", path, "file.txt")
	}

	// Set up depth stack
	validator.depthStack = []string{"parent/", "child/"}

	// Test depth 1
	path = validator.buildExpectedPath("file.txt", 1)
	if path != "parent/file.txt" {
		t.Errorf("buildExpectedPath at depth 1: got %q, want %q", path, "parent/file.txt")
	}

	// Test depth 2
	path = validator.buildExpectedPath("file.txt", 2)
	if path != "parent/child/file.txt" {
		t.Errorf("buildExpectedPath at depth 2: got %q, want %q", path, "parent/child/file.txt")
	}
}

func TestValidateFile(t *testing.T) {
	// Create a temporary manifest file
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "test.c4m")

	content := `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test manifest: %v", err)
	}

	// Test valid manifest
	err := ValidateFile(manifestPath, false)
	if err != nil {
		t.Errorf("ValidateFile failed for valid manifest: %v", err)
	}

	// Test strict mode
	err = ValidateFile(manifestPath, true)
	if err != nil {
		t.Errorf("ValidateFile strict mode failed for valid manifest: %v", err)
	}

	// Test non-existent file
	err = ValidateFile(filepath.Join(tmpDir, "nonexistent.c4m"), false)
	if err == nil {
		t.Error("ValidateFile should fail for non-existent file")
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErgo    bool
	}{
		{
			name: "canonical format",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			wantErgo: false,
		},
		{
			name: "ergonomic format with month",
			content: `@c4m 1.0
-rw-r--r-- Jan 15 2025 100 test.txt
`,
			wantErgo: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(false)
			validator.ValidateManifest(strings.NewReader(tt.content))
			if validator.isErgonomic != tt.wantErgo {
				t.Errorf("isErgonomic = %v, want %v", validator.isErgonomic, tt.wantErgo)
			}
		})
	}
}

func TestValidatorHandleDirective(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantLayers int64
	}{
		{
			name: "with layer directive",
			content: `@c4m 1.0
@layer test
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
@end
`,
			wantLayers: 1,
		},
		{
			name: "multiple layers",
			content: `@c4m 1.0
@layer first
-rw-r--r-- 2025-09-19T12:00:00Z 100 a.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
@end
@layer second
-rw-r--r-- 2025-09-19T12:00:00Z 100 b.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
@end
`,
			wantLayers: 2,
		},
		{
			name: "no layers",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			wantLayers: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(false)
			validator.ValidateManifest(strings.NewReader(tt.content))
			stats := validator.GetStats()
			if stats.Layers != tt.wantLayers {
				t.Errorf("Layers = %d, want %d", stats.Layers, tt.wantLayers)
			}
		})
	}
}

func TestValidateC4ID(t *testing.T) {
	tests := []struct {
		name    string
		content string
		strict  bool
		wantErr bool
	}{
		{
			name: "valid C4 ID",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  true,
			wantErr: false,
		},
		{
			name: "missing C4 ID is allowed",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt
`,
			strict:  true,
			wantErr: false,
		},
		{
			name: "invalid C4 ID format",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c4invalid
`,
			strict:  true,
			wantErr: true,
		},
		{
			name: "C4 ID too short",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c4abc
`,
			strict:  true,
			wantErr: true,
		},
		{
			name: "directory without C4 ID is ok",
			content: `@c4m 1.0
drwxr-xr-x 2025-09-19T12:00:00Z 0 dir/
`,
			strict:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(tt.strict)
			err := validator.ValidateManifest(strings.NewReader(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationErrorString(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name:     "with line and column",
			err:      ValidationError{Line: 5, Column: 10, Message: "test error"},
			expected: "line 5, col 10: test error",
		},
		{
			name:     "with line only",
			err:      ValidationError{Line: 3, Column: 0, Message: "test error"},
			expected: "line 3: test error",
		},
		{
			name:     "no line info",
			err:      ValidationError{Line: 0, Column: 0, Message: "test error"},
			expected: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseNameAndRest(t *testing.T) {
	tests := []struct {
		name          string
		fields        []string
		expectedName  string
		expectedC4ID  string
		expectWarning bool
	}{
		{
			name:         "empty fields",
			fields:       []string{},
			expectedName: "",
			expectedC4ID: "",
		},
		{
			name:         "simple file",
			fields:       []string{"file.txt", "c4abc123"},
			expectedName: "file.txt",
			expectedC4ID: "c4abc123",
		},
		{
			name:         "simple directory",
			fields:       []string{"dir/"},
			expectedName: "dir/",
			expectedC4ID: "",
		},
		{
			name:         "directory with c4id",
			fields:       []string{"dir/", "c4abc123"},
			expectedName: "dir/",
			expectedC4ID: "c4abc123",
		},
		{
			name:          "quoted directory with slash inside quotes",
			fields:        []string{`"my`, `dir/"`, "c4abc123"},
			expectedName:  "my dir/",
			expectedC4ID:  "c4abc123",
			expectWarning: true,
		},
		{
			name:          "quoted directory with slash outside quotes",
			fields:        []string{`"mydir"/`, "c4abc123"},
			expectedName:  "mydir/",
			expectedC4ID:  "c4abc123",
			expectWarning: true,
		},
		{
			name:         "quoted file name with spaces",
			fields:       []string{`"my`, `file.txt"`, "c4abc123"},
			expectedName: `my file.txt`, // Quotes are stripped, content joined
			expectedC4ID: "c4abc123",
		},
		{
			name:         "file name without c4id",
			fields:       []string{"file.txt"},
			expectedName: "file.txt",
			expectedC4ID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(false)
			name, _, c4id := v.parseNameAndRest(tt.fields)

			if name != tt.expectedName {
				t.Errorf("parseNameAndRest() name = %q, want %q", name, tt.expectedName)
			}
			if c4id != tt.expectedC4ID {
				t.Errorf("parseNameAndRest() c4id = %q, want %q", c4id, tt.expectedC4ID)
			}
			if tt.expectWarning && len(v.warnings) == 0 {
				t.Error("parseNameAndRest() expected warning, got none")
			}
		})
	}
}