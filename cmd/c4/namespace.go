package main

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// c4dConfigured reports whether c4d is set up on this machine.
// The presence of ~/.c4d/config.yaml is the intent signal:
// if you configured c4d, you intend to use it as a backing store.
func c4dConfigured() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".c4d", "config.yaml"))
	return err == nil
}

// namespacePath maps a local c4m file path to a c4d namespace path.
// Files under $HOME map to /home/{identity}/relative/path.
// Files outside $HOME map to /mnt/local/absolute/path.
// Namespace paths always use forward slashes regardless of OS.
func namespacePath(c4mPath, identity string) (string, error) {
	abs, err := filepath.Abs(c4mPath)
	if err != nil {
		return "", err
	}
	// Convert to forward slashes for namespace (virtual path)
	abs = filepath.ToSlash(abs)

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Normalize home to ensure clean prefix matching
	home = filepath.ToSlash(filepath.Clean(home))
	if !strings.HasSuffix(home, "/") {
		home += "/"
	}

	if strings.HasPrefix(abs, home) {
		rel := strings.TrimPrefix(abs, home)
		return "/home/" + identity + "/" + rel, nil
	}

	// Strip drive letter on Windows (e.g. "C:/tmp" -> "/tmp")
	if len(abs) >= 2 && abs[1] == ':' {
		abs = abs[2:]
	}

	return "/mnt/local" + abs, nil
}

// identityFromConfig extracts the caller identity from the TLS client cert
// configured in ~/.c4d/config.yaml.
func identityFromConfig() (string, error) {
	initC4dConnection()
	client := c4dClient
	if client == nil || client.Transport == nil {
		return "", fmt.Errorf("no TLS configured in c4d config")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil {
		return "", fmt.Errorf("no TLS configured in c4d config")
	}
	certs := transport.TLSClientConfig.Certificates
	if len(certs) == 0 {
		return "", fmt.Errorf("no client certificate in c4d config")
	}
	leaf, err := x509.ParseCertificate(certs[0].Certificate[0])
	if err != nil {
		return "", fmt.Errorf("parse client cert: %w", err)
	}
	if len(leaf.EmailAddresses) > 0 {
		return leaf.EmailAddresses[0], nil
	}
	if leaf.Subject.CommonName != "" {
		return leaf.Subject.CommonName, nil
	}
	return "", fmt.Errorf("client cert has no identity (no email SAN or CN)")
}

// registerNamespacePath registers a c4m file in the c4d namespace.
// In backing-store mode (c4d configured): errors are real errors.
// In local-only mode (no c4d): returns nil immediately, no-op.
// If retain is non-nil, sets a TTL policy on the namespace path.
func registerNamespacePath(c4mPath string, retain ...*time.Duration) error {
	if !c4dConfigured() {
		return nil
	}

	identity, err := identityFromConfig()
	if err != nil {
		return fmt.Errorf("namespace registration: %w", err)
	}

	nsPath, err := namespacePath(c4mPath, identity)
	if err != nil {
		return fmt.Errorf("namespace path: %w", err)
	}

	// Only register if the c4m file exists (we need a C4 ID to send)
	f, err := os.Open(c4mPath)
	if err != nil {
		// File doesn't exist yet — that's OK at c4 mk time.
		// It will be registered when writeManifest creates it.
		return nil
	}
	defer f.Close()

	id := c4.Identify(f)

	// Push the c4m blob to c4d's content store so the flow engine
	// (and any other consumer) can retrieve it via store.Get().
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seek c4m file for push: %w", err)
	}
	client, addr := c4dClient, c4dAddr()
	putReq, err := http.NewRequest(http.MethodPut, addr+"/", f)
	if err != nil {
		return fmt.Errorf("c4m blob push: %w", err)
	}
	putReq.Header.Set("Content-Type", "application/octet-stream")
	putResp, err := client.Do(putReq)
	if err != nil {
		return fmt.Errorf("c4m blob push: %w", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		return fmt.Errorf("c4d PUT c4m blob: %s", putResp.Status)
	}

	req, err := http.NewRequest("PUT", addr+nsPath, strings.NewReader(id.String()))
	if err != nil {
		return fmt.Errorf("namespace registration: %w", err)
	}

	if len(retain) > 0 && retain[0] != nil {
		req.Header.Set("X-TTL-Policy", retain[0].String())
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("c4d not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("c4d namespace PUT %s: %s", nsPath, resp.Status)
	}
	return nil
}

// runPurge flushes all purgatory content from c4d.
func runPurge() {
	if !c4dConfigured() {
		fmt.Fprintf(os.Stderr, "Error: c4d not configured (no backing store to purge)\n")
		os.Exit(1)
	}

	client, addr := c4dClient, c4dAddr()
	req, err := http.NewRequest("POST", addr+"/admin/purge", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: c4d not reachable: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: c4d purge: %s\n", resp.Status)
		os.Exit(1)
	}

	// Parse response for summary
	var result struct {
		Reclaimed  int   `json:"reclaimed"`
		FreedBytes int64 `json:"freed_bytes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Println("purgatory flushed")
		return
	}

	if result.Reclaimed == 0 {
		fmt.Println("purgatory empty — nothing to flush")
	} else {
		fmt.Printf("flushed %d blobs (%.1f MB freed)\n", result.Reclaimed,
			float64(result.FreedBytes)/(1024*1024))
	}
}

// unregisterNamespacePath removes a c4m file from the c4d namespace.
// In backing-store mode: errors are real errors.
// In local-only mode: returns nil immediately, no-op.
func unregisterNamespacePath(c4mPath string) error {
	if !c4dConfigured() {
		return nil
	}

	identity, err := identityFromConfig()
	if err != nil {
		return fmt.Errorf("namespace unregistration: %w", err)
	}

	nsPath, err := namespacePath(c4mPath, identity)
	if err != nil {
		return fmt.Errorf("namespace path: %w", err)
	}

	client, addr := c4dClient, c4dAddr()
	req, err := http.NewRequest("DELETE", addr+nsPath, nil)
	if err != nil {
		return fmt.Errorf("namespace unregistration: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("c4d not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("c4d namespace DELETE %s: %s", nsPath, resp.Status)
	}
	return nil
}
