package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

type nodeVersion struct {
	Address string       `json:"address"`
	Online  bool         `json:"online"`
	Info    *versionInfo `json:"info,omitempty"`
}

type versionInfo struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Go       string `json:"go"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Identity string `json:"identity,omitempty"`
}

// c4dConfig is the subset of ~/.c4d/config.yaml we need.
type c4dConfig struct {
	Listen string `yaml:"listen"`
	TLS    struct {
		Cert string `yaml:"cert"`
		Key  string `yaml:"key"`
		CA   string `yaml:"ca"`
	} `yaml:"tls"`
}

func runVersion(args []string) {
	fmt.Printf("c4 %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)

	client, addr := c4dVersionClient()
	resp, err := client.Get(addr + "/versions")
	if err != nil {
		fmt.Printf("\nc4d: not reachable\n")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("\nc4d: error (%s)\n", resp.Status)
		return
	}

	var nodes []nodeVersion
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		fmt.Printf("\nc4d: bad response\n")
		return
	}

	fmt.Println()
	for _, n := range nodes {
		if !n.Online || n.Info == nil {
			fmt.Printf("  %-30s offline\n", n.Address)
			continue
		}
		label := n.Address
		if n.Info.Identity != "" {
			if n.Address == "local" {
				label = n.Info.Identity
			} else {
				label = n.Info.Identity + " (" + n.Address + ")"
			}
		}
		fmt.Printf("  %-30s c4d %s (%s) %s/%s\n", label, n.Info.Version, n.Info.Commit, n.Info.OS, n.Info.Arch)
	}
}

// c4dVersionClient returns an HTTP client and base URL for reaching c4d.
// It reads ~/.c4d/config.yaml for address and TLS config, falling back
// to C4D_ADDR env or http://localhost:17433.
func c4dVersionClient() (*http.Client, string) {
	client := &http.Client{Timeout: 3 * time.Second}

	// Try reading the c4d config
	home, err := os.UserHomeDir()
	if err != nil {
		return client, c4dAddr()
	}
	data, err := os.ReadFile(filepath.Join(home, ".c4d", "config.yaml"))
	if err != nil {
		return client, c4dAddr()
	}
	var cfg c4dConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return client, c4dAddr()
	}

	// Determine listen address
	listen := cfg.Listen
	if listen == "" {
		return client, c4dAddr()
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return client, c4dAddr()
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, port)

	// Check for TLS
	certFile := expandHome(cfg.TLS.Cert, home)
	keyFile := expandHome(cfg.TLS.Key, home)
	caFile := expandHome(cfg.TLS.CA, home)

	if certFile == "" || keyFile == "" || caFile == "" {
		return client, "http://" + addr
	}

	// Load mTLS client config
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return client, "http://" + addr
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return client, "http://" + addr
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)

	client.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates:       []tls.Certificate{cert},
			RootCAs:            caPool,
			InsecureSkipVerify: true, // c4d uses identity certs, not hostname certs
		},
	}
	return client, "https://" + addr
}

func expandHome(path, home string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		return filepath.Join(home, path[2:])
	}
	return path
}
