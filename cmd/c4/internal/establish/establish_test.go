package establish

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestC4mEstablishment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := t.TempDir()
	c4mPath := filepath.Join(dir, "project.c4m")

	// Not established initially
	if IsC4mEstablished(c4mPath) {
		t.Error("should not be established before mk")
	}

	// Establish
	if err := EstablishC4m(c4mPath); err != nil {
		t.Fatalf("EstablishC4m: %v", err)
	}

	// Now established
	if !IsC4mEstablished(c4mPath) {
		t.Error("should be established after mk")
	}

	// No marker file next to the c4m (the whole point of the fix)
	matches, _ := filepath.Glob(filepath.Join(dir, "*.established"))
	if len(matches) > 0 {
		t.Errorf("should NOT create sibling marker files, found: %v", matches)
	}

	// Establishment is stored centrally
	regDir, _ := c4mDir()
	entries, err := os.ReadDir(regDir)
	if err != nil {
		t.Fatalf("c4m dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry in c4m dir, got %d", len(entries))
	}

	// Remove establishment
	if err := RemoveC4mEstablishment(c4mPath); err != nil {
		t.Fatalf("RemoveC4mEstablishment: %v", err)
	}

	if IsC4mEstablished(c4mPath) {
		t.Error("should not be established after removal")
	}
}

func TestC4mEstablishmentWithoutFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := t.TempDir()
	c4mPath := filepath.Join(dir, "new.c4m")

	// Can establish even if .c4m doesn't exist yet (create-on-write)
	if err := EstablishC4m(c4mPath); err != nil {
		t.Fatalf("EstablishC4m on nonexistent file: %v", err)
	}

	if !IsC4mEstablished(c4mPath) {
		t.Error("should be established for nonexistent c4m file")
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

func TestC4mEstablishmentWithTTL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c4mPath := filepath.Join(t.TempDir(), "ttl.c4m")

	// Establish with future TTL
	future := time.Now().Add(time.Hour)
	if err := EstablishC4mWithTTL(c4mPath, &future); err != nil {
		t.Fatalf("EstablishC4mWithTTL: %v", err)
	}
	if !IsC4mEstablished(c4mPath) {
		t.Error("should be established with future TTL")
	}

	entry := GetC4mEntry(c4mPath)
	if entry == nil {
		t.Fatal("GetC4mEntry returned nil")
	}
	if entry.ExpiresAt == nil {
		t.Fatal("ExpiresAt should be set")
	}
	if entry.IsExpired() {
		t.Error("should not be expired yet")
	}
}

func TestC4mEstablishmentExpired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c4mPath := filepath.Join(t.TempDir(), "expired.c4m")

	// Establish with past TTL
	past := time.Now().Add(-time.Hour)
	if err := EstablishC4mWithTTL(c4mPath, &past); err != nil {
		t.Fatalf("EstablishC4mWithTTL: %v", err)
	}

	// File exists but IsC4mEstablished should return false
	if IsC4mEstablished(c4mPath) {
		t.Error("should not be established with expired TTL")
	}

	entry := GetC4mEntry(c4mPath)
	if entry == nil {
		t.Fatal("GetC4mEntry should still return entry")
	}
	if !entry.IsExpired() {
		t.Error("should be expired")
	}
}

func TestC4mLegacyFormat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	c4mPath := filepath.Join(t.TempDir(), "legacy.c4m")
	abs, _ := filepath.Abs(c4mPath)

	// Write legacy format (plain text)
	dir, _ := c4mDir()
	os.MkdirAll(dir, 0755)
	key, _ := c4mKey(c4mPath)
	os.WriteFile(filepath.Join(dir, key), []byte(abs+"\n"), 0644)

	// Should still be recognized
	if !IsC4mEstablished(c4mPath) {
		t.Error("legacy format should be established")
	}

	entry := GetC4mEntry(c4mPath)
	if entry == nil {
		t.Fatal("GetC4mEntry returned nil for legacy")
	}
	if entry.Path != abs {
		t.Errorf("Path = %q, want %q", entry.Path, abs)
	}
	if entry.ExpiresAt != nil {
		t.Error("legacy should have nil ExpiresAt")
	}
}

func TestLocationWithTTL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	future := time.Now().Add(time.Hour)
	if err := EstablishLocationWithTTL("builds", "localhost:7433", &future); err != nil {
		t.Fatalf("EstablishLocationWithTTL: %v", err)
	}
	if !IsLocationEstablished("builds") {
		t.Error("should be established with future TTL")
	}

	entry := GetLocation("builds")
	if entry.ExpiresAt == nil {
		t.Fatal("ExpiresAt should be set")
	}
}

func TestLocationExpired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	past := time.Now().Add(-time.Hour)
	if err := EstablishLocationWithTTL("old", "localhost:7433", &past); err != nil {
		t.Fatalf("EstablishLocationWithTTL: %v", err)
	}

	if IsLocationEstablished("old") {
		t.Error("should not be established with expired TTL")
	}
}
