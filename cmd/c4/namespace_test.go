package main

import (
	"testing"
)

func TestNamespacePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

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

func TestRegisterNamespacePathNoC4d(t *testing.T) {
	// With no c4d running and no TLS config, registration should be a no-op
	t.Setenv("HOME", t.TempDir())
	t.Setenv("C4D_ADDR", "http://localhost:1") // unreachable

	err := registerNamespacePath("/tmp/nonexistent.c4m")
	if err != nil {
		t.Errorf("registerNamespacePath should be nil when c4d unreachable, got: %v", err)
	}
}
