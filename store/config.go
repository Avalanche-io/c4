package store

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OpenConfigured returns a TreeStore based on configuration.
// Resolution order:
//  1. C4_STORE environment variable
//  2. ~/.c4/config file (store setting)
//  3. Returns nil, nil if no store configured.
func OpenConfigured() (*TreeStore, error) {
	path := configuredPath()
	if path == "" {
		return nil, nil
	}
	return NewTreeStore(path)
}

// IsConfigured reports whether a content store is configured.
func IsConfigured() bool {
	return configuredPath() != ""
}

// AutoStoreEnabled reports whether auto-store is enabled.
// Checks C4_STORE_AUTO env var and ~/.c4/config store_auto setting.
func AutoStoreEnabled() bool {
	if v := os.Getenv("C4_STORE_AUTO"); v != "" {
		return v == "true" || v == "1" || v == "yes"
	}
	return configValue("store_auto") == "true"
}

// DefaultStorePath returns the default store location (~/.c4/store).
func DefaultStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".c4", "store")
}

// SetupDefaultStore creates the default store at ~/.c4/store and writes
// the config entry. Returns the new TreeStore.
func SetupDefaultStore() (*TreeStore, error) {
	storePath := DefaultStorePath()
	if storePath == "" {
		return nil, fmt.Errorf("cannot determine home directory")
	}

	s, err := NewTreeStore(storePath)
	if err != nil {
		return nil, err
	}

	// Write store path to config.
	configPath := configFilePath()
	if configPath == "" {
		return s, nil
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return s, nil // store created, config write failed — not fatal
	}

	// Read existing config, update or append store line.
	lines := readConfigLines(configPath)
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "store ") || strings.HasPrefix(strings.TrimSpace(line), "store=") {
			lines[i] = "store = " + storePath
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, "store = "+storePath)
	}

	return s, os.WriteFile(configPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func configuredPath() string {
	if v := os.Getenv("C4_STORE"); v != "" {
		// S3 URIs are not handled by TreeStore.
		if strings.HasPrefix(v, "s3://") {
			return "" // S3 backend not yet implemented
		}
		return v
	}
	if v := configValue("store"); v != "" {
		if strings.HasPrefix(v, "s3://") {
			return ""
		}
		return v
	}
	return ""
}

func configFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".c4", "config")
}

func configValue(key string) string {
	path := configFilePath()
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		// Support "key = value" and "key=value"
		var k, v string
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			k = strings.TrimSpace(line[:idx])
			v = strings.TrimSpace(line[idx+1:])
		} else if idx := strings.IndexByte(line, ' '); idx >= 0 {
			k = strings.TrimSpace(line[:idx])
			v = strings.TrimSpace(line[idx+1:])
		} else {
			continue
		}
		if k == key {
			return v
		}
	}
	return ""
}

func readConfigLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}
