// Package establish manages the write-safety gate for colon syntax.
//
// Read through colon syntax is always allowed. Write requires prior
// establishment via "c4 mk". This prevents accidental writes from
// colon typos — a trailing colon shouldn't silently change "copy file"
// to "write into namespace."
//
// Capsule establishment uses a centralized registry (~/.c4/capsules/).
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

// capsulesDir returns the path to the capsules registry.
func capsulesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".c4", "capsules"), nil
}

// capsuleKey returns a deterministic filename for a c4m path.
func capsuleKey(c4mPath string) (string, error) {
	abs, err := filepath.Abs(c4mPath)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(h[:16]), nil
}

// IsCapsuleEstablished checks if a capsule file has been established for writing.
func IsCapsuleEstablished(c4mPath string) bool {
	dir, err := capsulesDir()
	if err != nil {
		return false
	}
	key, err := capsuleKey(c4mPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, key))
	return err == nil
}

// EstablishCapsule marks a capsule file as established for writing.
// The capsule file itself need not exist yet (create-on-write).
func EstablishCapsule(c4mPath string) error {
	dir, err := capsulesDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create capsules dir: %w", err)
	}
	key, err := capsuleKey(c4mPath)
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(c4mPath)
	return os.WriteFile(filepath.Join(dir, key), []byte(abs+"\n"), 0644)
}

// RemoveCapsuleEstablishment removes the establishment marker.
func RemoveCapsuleEstablishment(c4mPath string) error {
	dir, err := capsulesDir()
	if err != nil {
		return err
	}
	key, err := capsuleKey(c4mPath)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, key))
}

// LocationEntry holds the connection info for a named location.
type LocationEntry struct {
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

// locationsDir returns the path to the locations registry.
func locationsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".c4", "locations"), nil
}

// IsLocationEstablished checks if a location name is registered.
func IsLocationEstablished(name string) bool {
	dir, err := locationsDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, name))
	return err == nil
}

// EstablishLocation registers a named location with its address.
func EstablishLocation(name, address string) error {
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
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0644)
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
