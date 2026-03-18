package store

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// OpenConfigured returns a TreeStore based on configuration.
// For S3-backed stores, use OpenStore instead.
// Resolution order:
//  1. C4_STORE environment variable (local paths only)
//  2. ~/.c4/config file (store setting, local paths only)
//  3. Returns nil, nil if no store configured.
func OpenConfigured() (*TreeStore, error) {
	path := configuredPath()
	if path == "" {
		return nil, nil
	}
	return NewTreeStore(path)
}

// OpenStore returns a Store based on configuration, supporting local
// filesystem and S3 backends. Multiple stores can be configured —
// writes go to the first, reads check all in order.
//
// C4_STORE can be a single path/URI or comma-separated list:
//
//	C4_STORE=/fast/ssd,s3://bucket/c4?region=us-west-2,/mnt/archive
//
// Alternatively, ~/.c4/config can have multiple store lines:
//
//	store = /fast/ssd
//	store = s3://bucket/c4?region=us-west-2
//
// Returns nil, nil if no store configured.
func OpenStore() (Store, error) {
	endpoints := configuredEndpoints()
	if len(endpoints) == 0 {
		return nil, nil
	}

	var stores []Store
	for _, ep := range endpoints {
		var s Store
		var err error
		if strings.HasPrefix(ep, "s3://") {
			s, err = openS3Configured(ep)
		} else {
			s, err = NewTreeStore(ep)
		}
		if err != nil {
			return nil, fmt.Errorf("store %q: %w", ep, err)
		}
		stores = append(stores, s)
	}

	if len(stores) == 1 {
		return stores[0], nil
	}
	return NewMultiStore(stores...), nil
}

// openS3Configured parses an s3:// URI and returns an S3Store.
// Format: s3://bucket/prefix?region=X&endpoint=Y
// Credentials come from AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY env vars.
func openS3Configured(raw string) (*S3Store, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse S3 URI: %w", err)
	}
	bucket := u.Host
	if bucket == "" {
		return nil, fmt.Errorf("S3 URI missing bucket: %s", raw)
	}
	prefix := strings.TrimPrefix(u.Path, "/")

	region := u.Query().Get("region")
	if region == "" {
		region = "us-east-1"
	}

	endpoint := u.Query().Get("endpoint")
	if endpoint == "" {
		endpoint = os.Getenv("C4_S3_ENDPOINT")
	}

	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set for S3 store")
	}

	return NewS3Store(bucket, prefix, region, endpoint, accessKey, secretKey), nil
}

// IsConfigured reports whether a content store is configured.
func IsConfigured() bool {
	return configuredRaw() != ""
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

// configuredEndpoints returns all configured store endpoints.
// C4_STORE can be comma-separated. Config file can have multiple store lines.
func configuredEndpoints() []string {
	if v := os.Getenv("C4_STORE"); v != "" {
		var endpoints []string
		for _, ep := range strings.Split(v, ",") {
			ep = strings.TrimSpace(ep)
			if ep != "" {
				endpoints = append(endpoints, ep)
			}
		}
		return endpoints
	}
	return configValues("store")
}

// configuredRaw returns the first configured store value.
// Returns "" if no store is configured.
func configuredRaw() string {
	eps := configuredEndpoints()
	if len(eps) == 0 {
		return ""
	}
	return eps[0]
}

// configuredPath returns the local filesystem path for the store, or ""
// if no local store is configured (e.g., S3 URIs return "").
func configuredPath() string {
	raw := configuredRaw()
	if strings.HasPrefix(raw, "s3://") {
		return ""
	}
	return raw
}

func configFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".c4", "config")
}

// configValues returns all values for a key from the config file.
func configValues(key string) []string {
	path := configFilePath()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var values []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
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
		if k == key && v != "" {
			values = append(values, v)
		}
	}
	return values
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
