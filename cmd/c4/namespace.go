package main

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
)

// namespacePath maps a local c4m file path to a c4d namespace path.
// Files under $HOME map to /home/{identity}/relative/path.
// Files outside $HOME map to /mnt/local/absolute/path.
func namespacePath(c4mPath, identity string) (string, error) {
	abs, err := filepath.Abs(c4mPath)
	if err != nil {
		return "", err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Normalize home to ensure clean prefix matching
	home = filepath.Clean(home)
	if !strings.HasSuffix(home, "/") {
		home += "/"
	}

	if strings.HasPrefix(abs, home) {
		rel := strings.TrimPrefix(abs, home)
		return "/home/" + identity + "/" + rel, nil
	}

	return "/mnt/local" + abs, nil
}

// identityFromConfig extracts the caller identity from the TLS client cert
// configured in ~/.c4d/config.yaml.
func identityFromConfig() string {
	client, _ := c4dVersionClient()
	if client.Transport == nil {
		return ""
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil {
		return ""
	}
	certs := transport.TLSClientConfig.Certificates
	if len(certs) == 0 {
		return ""
	}
	leaf, err := x509.ParseCertificate(certs[0].Certificate[0])
	if err != nil {
		return ""
	}
	if len(leaf.EmailAddresses) > 0 {
		return leaf.EmailAddresses[0]
	}
	return leaf.Subject.CommonName
}

// registerNamespacePath registers a c4m file in the c4d namespace.
// If the c4m file exists, its C4 ID is computed and sent.
// Returns nil on success or if c4d is not reachable (best-effort).
func registerNamespacePath(c4mPath string) error {
	identity := identityFromConfig()
	if identity == "" {
		return nil // no identity configured, skip
	}

	nsPath, err := namespacePath(c4mPath, identity)
	if err != nil {
		return nil // can't map path, skip
	}

	// Only register if the c4m file exists (we need a C4 ID)
	f, err := os.Open(c4mPath)
	if err != nil {
		return nil // file doesn't exist yet, skip
	}
	defer f.Close()

	id := c4.Identify(f)

	client, addr := c4dVersionClient()
	req, err := http.NewRequest("PUT", addr+nsPath, strings.NewReader(id.String()))
	if err != nil {
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil // c4d not reachable, skip
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("c4d namespace PUT: %s", resp.Status)
	}
	return nil
}

// unregisterNamespacePath removes a c4m file from the c4d namespace.
// Best-effort: returns nil if c4d is not reachable.
func unregisterNamespacePath(c4mPath string) error {
	identity := identityFromConfig()
	if identity == "" {
		return nil
	}

	nsPath, err := namespacePath(c4mPath, identity)
	if err != nil {
		return nil
	}

	client, addr := c4dVersionClient()
	req, err := http.NewRequest("DELETE", addr+nsPath, nil)
	if err != nil {
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	return nil
}
