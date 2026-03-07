// Package establish manages the write-safety gate for colon syntax.
//
// Read through colon syntax is always allowed. Write requires prior
// establishment via "c4 mk". This prevents accidental writes from
// colon typos — a trailing colon shouldn't silently change "copy file"
// to "write into namespace."
//
// C4m file establishment uses a centralized registry (~/.c4/c4m/).
// Location establishment uses a registry directory (~/.c4/locations/).
package establish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp.*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// c4mDir returns the path to the c4m file registry.
func c4mDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".c4", "c4m"), nil
}

// c4mKey returns a deterministic filename for a c4m path.
func c4mKey(c4mPath string) (string, error) {
	abs, err := filepath.Abs(c4mPath)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(h[:16]), nil
}

// C4mEntry holds establishment metadata for a c4m file.
type C4mEntry struct {
	Path      string     `json:"path"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IsExpired returns true if the entry has a TTL and it has passed.
func (e *C4mEntry) IsExpired() bool {
	return e.ExpiresAt != nil && time.Now().After(*e.ExpiresAt)
}

// IsC4mEstablished checks if a c4m file has been established for writing.
func IsC4mEstablished(c4mPath string) bool {
	entry := GetC4mEntry(c4mPath)
	if entry == nil {
		return false
	}
	return !entry.IsExpired()
}

// GetC4mEntry returns the establishment entry for a c4m file, or nil.
func GetC4mEntry(c4mPath string) *C4mEntry {
	dir, err := c4mDir()
	if err != nil {
		return nil
	}
	key, err := c4mKey(c4mPath)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, key))
	if err != nil {
		return nil
	}
	var entry C4mEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Legacy format: plain text path
		abs, _ := filepath.Abs(c4mPath)
		return &C4mEntry{Path: abs}
	}
	return &entry
}

// EstablishC4m marks a c4m file as established for writing.
// The c4m file itself need not exist yet (create-on-write).
func EstablishC4m(c4mPath string) error {
	return EstablishC4mWithTTL(c4mPath, nil)
}

// EstablishC4mWithTTL marks a c4m file as established with an optional TTL.
func EstablishC4mWithTTL(c4mPath string, expiresAt *time.Time) error {
	dir, err := c4mDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create c4m dir: %w", err)
	}
	key, err := c4mKey(c4mPath)
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(c4mPath)
	entry := C4mEntry{
		Path:      abs,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, key), data, 0644)
}

// RemoveC4mEstablishment removes the establishment marker.
func RemoveC4mEstablishment(c4mPath string) error {
	dir, err := c4mDir()
	if err != nil {
		return err
	}
	key, err := c4mKey(c4mPath)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, key))
}

// LocationEntry holds the connection info for a named location.
type LocationEntry struct {
	Address   string     `json:"address"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IsExpired returns true if the entry has a TTL and it has passed.
func (e *LocationEntry) IsExpired() bool {
	return e.ExpiresAt != nil && time.Now().After(*e.ExpiresAt)
}

// locationsDir returns the path to the locations registry.
func locationsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".c4", "locations"), nil
}

// IsLocationEstablished checks if a location name is registered and not expired.
func IsLocationEstablished(name string) bool {
	entry := GetLocation(name)
	if entry == nil {
		return false
	}
	return !entry.IsExpired()
}

// EstablishLocation registers a named location with its address.
func EstablishLocation(name, address string) error {
	return EstablishLocationWithTTL(name, address, nil)
}

// EstablishLocationWithTTL registers a named location with an optional TTL.
func EstablishLocationWithTTL(name, address string, expiresAt *time.Time) error {
	dir, err := locationsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create locations dir: %w", err)
	}

	entry := LocationEntry{
		Address:   address,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, name), data, 0644)
}

// RemoveLocation removes a location registration.
func RemoveLocation(name string) error {
	dir, err := locationsDir()
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, name))
}

// GetLocation returns the entry for a named location, or nil if not found.
func GetLocation(name string) *LocationEntry {
	dir, err := locationsDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil
	}
	var entry LocationEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}
	return &entry
}

// ListLocations returns all registered location names and their entries.
func ListLocations() (map[string]LocationEntry, error) {
	dir, err := locationsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	result := make(map[string]LocationEntry)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var entry LocationEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		result[e.Name()] = entry
	}
	return result, nil
}
