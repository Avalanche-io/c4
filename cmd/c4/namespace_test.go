package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNamespacePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}

	tests := []struct {
		name     string
		c4mPath  string
		identity string
		want     string
	}{
		{
			name:     "file under home",
			c4mPath:  home + "/projects/movie/project.c4m",
			identity: "josh@example.com",
			want:     "/home/josh@example.com/projects/movie/project.c4m",
		},
		{
			name:     "file at home root",
			c4mPath:  home + "/test.c4m",
			identity: "alice@example.com",
			want:     "/home/alice@example.com/test.c4m",
		},
		{
			name:     "file outside home",
			c4mPath:  "/tmp/shared/project.c4m",
			identity: "josh@example.com",
			want:     "/mnt/local/tmp/shared/project.c4m",
		},
		{
			name:     "file at filesystem root",
			c4mPath:  "/data/renders.c4m",
			identity: "josh@example.com",
			want:     "/mnt/local/data/renders.c4m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := namespacePath(tt.c4mPath, tt.identity)
			if err != nil {
				t.Fatalf("namespacePath: %v", err)
			}
			if got != tt.want {
				t.Errorf("namespacePath(%q, %q) = %q, want %q", tt.c4mPath, tt.identity, got, tt.want)
			}
		})
	}
}

func TestC4dConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}

	// No config → not configured
	if c4dConfigured() {
		t.Error("should not be configured without config.yaml")
	}

	// Create config → configured
	os.MkdirAll(filepath.Join(home, ".c4d"), 0755)
	os.WriteFile(filepath.Join(home, ".c4d", "config.yaml"), []byte("listen: :7433\n"), 0644)

	if !c4dConfigured() {
		t.Error("should be configured with config.yaml present")
	}
}

func TestRegisterLocalOnlyMode(t *testing.T) {
	// No c4d config → local-only mode → registration is a no-op
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpHome)
	}

	err := registerNamespacePath("/tmp/nonexistent.c4m")
	if err != nil {
		t.Errorf("local-only mode should return nil, got: %v", err)
	}
}
