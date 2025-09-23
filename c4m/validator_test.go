package c4m

import (
	"os"
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

func TestValidateBundle(t *testing.T) {
	// Create a test bundle
	tmpDir := t.TempDir()
	scanPath := tmpDir + "/data"
	os.MkdirAll(scanPath, 0755)

	config := DevBundleConfig()
	config.BundleDir = tmpDir
	bundle, err := CreateBundle(scanPath, config)
	if err != nil {
		t.Fatalf("Failed to create test bundle: %v", err)
	}

	// Add some test data
	manifest := NewManifest()
	manifest.AddEntry(&Entry{
		Name: "test.txt",
		Size: 100,
		Mode: 0644,
	})

	scan, err := bundle.NewScan(scanPath)
	if err != nil {
		t.Fatalf("Failed to create scan: %v", err)
	}
	bundle.AddProgressChunk(scan, manifest)
	bundle.CompleteScan(scan)

	// Test validation
	validator := NewValidator(false)
	err = validator.ValidateBundle(bundle.Path)
	if err != nil {
		t.Errorf("ValidateBundle() failed: %v", err)
	}

	// Test with non-existent bundle
	err = validator.ValidateBundle("/non/existent/path")
	if err == nil {
		t.Error("Expected error for non-existent bundle")
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