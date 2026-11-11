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
			content: `-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  false,
			wantErr: false,
		},
		{
			name: "directory entry with size",
			content: `drwxr-xr-x 2025-09-19T12:00:00Z 0 dir/
`,
			strict:  false,
			wantErr: false,
		},
		{
			name: "directive rejected",
			content: `@base c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  false,
			wantErr: true,
		},
		{
			name: "c4m header rejected",
			content: `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  false,
			wantErr: true,
		},
		{
			name: "duplicate filenames in strict mode",
			content: `-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 200 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`,
			strict:  true,
			wantErr: true,
		},
		{
			name: "unsorted entries in strict mode",
			content: `-rw-r--r-- 2025-09-19T12:00:00Z 100 z.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
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

func TestValidatorSortCheck(t *testing.T) {
	// Test that strict validator catches unsorted entries
	strictValidator := NewValidator(true)

	// Properly sorted manifest
	sortedContent := `-rw-r--r-- 2025-09-19T12:00:00Z 100 a.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 b.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	err := strictValidator.ValidateManifest(strings.NewReader(sortedContent))
	if err != nil {
		t.Errorf("Sorted manifest should pass validation: %v", err)
	}

	// Unsorted manifest
	unsortedContent := `-rw-r--r-- 2025-09-19T12:00:00Z 100 z.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
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
	withDupsContent := `-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 200 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	err := strictValidator.ValidateManifest(strings.NewReader(withDupsContent))
	if err == nil && strictValidator.Strict {
		t.Error("Strict validation should fail for duplicate entries")
	}

	// The duplicate is reported at its line, naming the first one; the
	// decoder alone would keep only the last and say nothing.
	found := false
	for _, e := range strictValidator.GetErrors() {
		if e.Field == "duplicate" && e.Line == 2 && strings.Contains(e.Message, `"test.txt" (first at line 1)`) {
			found = true
		}
	}
	if !found {
		t.Errorf("duplicate not reported at line 2: %v", strictValidator.GetErrors())
	}
	// Lax mode reports it too: the spec says parsers MUST reject duplicates.
	if err := NewValidator(false).ValidateManifest(strings.NewReader(withDupsContent)); err == nil {
		t.Error("lax validation must also fail for a duplicate path")
	}
}

func TestValidationReport(t *testing.T) {
	// Create a manifest with various issues
	content := `-rw-r--r-- 2025-09-19T12:00:00Z 100 z.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
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
	// A valid manifest should produce no errors
	content := `-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	validator := NewValidator(false)
	validator.ValidateManifest(strings.NewReader(content))

	errors := validator.GetErrors()
	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}

	// Test manifest with actual errors
	invalidContent := `not a valid manifest`
	validator2 := NewValidator(false)
	validator2.ValidateManifest(strings.NewReader(invalidContent))
	errors2 := validator2.GetErrors()
	if len(errors2) == 0 {
		t.Error("Expected errors for invalid manifest")
	}

	// Directives produce errors, not warnings
	directiveContent := `@c4m 1.0
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`
	validator3 := NewValidator(false)
	validator3.ValidateManifest(strings.NewReader(directiveContent))
	errors3 := validator3.GetErrors()
	if len(errors3) == 0 {
		t.Error("Expected errors for directive line")
	}
}

func TestValidateFile(t *testing.T) {
	// Create a temporary manifest file
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "test.c4m")

	content := `-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
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
