package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type duResponse struct {
	ReferencedCount   int64 `json:"referenced_count"`
	ReferencedBytes   int64 `json:"referenced_bytes"`
	UnreferencedCount int64 `json:"unreferenced_count"`
	UnreferencedBytes int64 `json:"unreferenced_bytes"`
	TotalCount        int64 `json:"total_count"`
	TotalBytes        int64 `json:"total_bytes"`
	MaxStoreBytes     int64 `json:"max_store_bytes,omitempty"`
}

func runDU(args []string) {
	if !c4dConfigured() {
		fmt.Fprintf(os.Stderr, "c4d not configured — du requires a backing store\n")
		fmt.Fprintf(os.Stderr, "Run: c4d init\n")
		os.Exit(1)
	}
	client, addr := c4dVersionClient()
	resp, err := client.Get(addr + "/du")
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4d not reachable: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotImplemented {
		fmt.Fprintf(os.Stderr, "c4d: du not supported for this store backend\n")
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "c4d: %s\n", resp.Status)
		os.Exit(1)
	}

	var du duResponse
	if err := json.NewDecoder(resp.Body).Decode(&du); err != nil {
		fmt.Fprintf(os.Stderr, "c4d: bad response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%-14s %s  (%d objects)\n", "Active:", formatBytes(du.ReferencedBytes), du.ReferencedCount)
	fmt.Printf("%-14s %s  (%d objects)\n", "Purgeable:", formatBytes(du.UnreferencedBytes), du.UnreferencedCount)
	fmt.Printf("%-14s %s  (%d objects)\n", "Total:", formatBytes(du.TotalBytes), du.TotalCount)

	if du.MaxStoreBytes > 0 {
		pct := float64(du.TotalBytes) / float64(du.MaxStoreBytes) * 100
		fmt.Printf("%-14s %s  (%.1f%% used)\n", "Limit:", formatBytes(du.MaxStoreBytes), pct)
	}
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<40:
		return fmt.Sprintf("%.1f TiB", float64(b)/float64(1<<40))
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
