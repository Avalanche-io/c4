package establish

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapsuleEstablishment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := t.TempDir()
	c4mPath := filepath.Join(dir, "project.c4m")

	// Not established initially
	if IsCapsuleEstablished(c4mPath) {
		t.Error("should not be established before mk")
	}

	// Establish
	if err := EstablishCapsule(c4mPath); err != nil {
		t.Fatalf("EstablishCapsule: %v", err)
	}

	// Now established
	if !IsCapsuleEstablished(c4mPath) {
		t.Error("should be established after mk")
	}

	// No marker file next to the c4m (the whole point of the fix)
	matches, _ := filepath.Glob(filepath.Join(dir, "*.established"))
	if len(matches) > 0 {
		t.Errorf("should NOT create sibling marker files, found: %v", matches)
	}

	// Establishment is stored centrally
	capDir, _ := capsulesDir()
	entries, err := os.ReadDir(capDir)
	if err != nil {
		t.Fatalf("capsules dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry in capsules dir, got %d", len(entries))
	}

	// Remove establishment
	if err := RemoveCapsuleEstablishment(c4mPath); err != nil {
		t.Fatalf("RemoveCapsuleEstablishment: %v", err)
	}

	if IsCapsuleEstablished(c4mPath) {
		t.Error("should not be established after removal")
	}
}

func TestCapsuleEstablishmentWithoutFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := t.TempDir()
	c4mPath := filepath.Join(dir, "new.c4m")

	// Can establish even if .c4m doesn't exist yet (create-on-write)
	if err := EstablishCapsule(c4mPath); err != nil {
		t.Fatalf("EstablishCapsule on nonexistent file: %v", err)
	}

	if !IsCapsuleEstablished(c4mPath) {
		t.Error("should be established for nonexistent capsule")
	}
}

func TestLocationEstablishment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if IsLocationEstablished("studio") {
		t.Error("should not be established before mk")
	}

	if err := EstablishLocation("studio", "cloud.example.com:7433"); err != nil {
		t.Fatalf("EstablishLocation: %v", err)
	}

	if !IsLocationEstablished("studio") {
		t.Error("should be established after mk")
	}

	entry := GetLocation("studio")
	if entry == nil {
		t.Fatal("GetLocation returned nil")
	}
	if entry.Address != "cloud.example.com:7433" {
		t.Errorf("Address = %q, want cloud.example.com:7433", entry.Address)
	}

	locs, err := ListLocations()
	if err != nil {
		t.Fatalf("ListLocations: %v", err)
	}
	if len(locs) != 1 {
		t.Errorf("ListLocations returned %d entries, want 1", len(locs))
	}

	if err := RemoveLocation("studio"); err != nil {
		t.Fatalf("RemoveLocation: %v", err)
	}

	if IsLocationEstablished("studio") {
		t.Error("should not be established after removal")
	}
}

func TestGetLocationNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if entry := GetLocation("nonexistent"); entry != nil {
		t.Error("GetLocation should return nil for nonexistent location")
	}
}

func TestListLocationsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	locs, err := ListLocations()
	if err != nil {
		t.Fatalf("ListLocations: %v", err)
	}
	if len(locs) != 0 {
		t.Errorf("ListLocations returned %d entries, want 0", len(locs))
	}
}
