package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
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

	resp, err := c4dClient.Get(c4dAddr() + "/versions")
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

func expandHome(path, home string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		return filepath.Join(home, path[2:])
	}
	return path
}
