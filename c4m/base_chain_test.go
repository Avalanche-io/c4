package c4m

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestBaseChainResolution(t *testing.T) {
	t.Run("NoBase", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:      "file1.txt",
			Mode:      0644,
			Size:      100,
			Timestamp: time.Now(),
			Depth:     0,
		})

		resolver := NewBaseChainResolver("")
		result, err := resolver.ResolveChain(manifest)
		if err != nil {
			t.Fatalf("Failed to resolve chain: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(result.Entries))
		}
	})

	t.Run("SimpleChain", func(t *testing.T) {
		// Create base manifest
		base := NewManifest()
		base.AddEntry(&Entry{
			Name:      "file1.txt",
			Mode:      0644,
			Size:      100,
			Timestamp: time.Now(),
			Depth:     0,
		})

		// Create derived manifest with @base
		derived := NewManifest()
		derived.Base = c4.Identify(strings.NewReader("fake-base-id"))
		derived.AddEntry(&Entry{
			Name:      "file2.txt",
			Mode:      0644,
			Size:      200,
			Timestamp: time.Now(),
			Depth:     0,
		})

		// Mock the loading by putting base in cache
		resolver := NewBaseChainResolver("")
		resolver.cache[derived.Base] = base

		result, err := resolver.ResolveChain(derived)
		if err != nil {
			t.Fatalf("Failed to resolve chain: %v", err)
		}

		if len(result.Entries) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(result.Entries))
		}

		// Check order (should be sorted)
		if result.Entries[0].Name != "file1.txt" {
			t.Errorf("First entry should be file1.txt, got %s", result.Entries[0].Name)
		}
		if result.Entries[1].Name != "file2.txt" {
			t.Errorf("Second entry should be file2.txt, got %s", result.Entries[1].Name)
		}
	})

	t.Run("UpdateEntry", func(t *testing.T) {
		// Create base manifest with a file
		base := NewManifest()
		base.AddEntry(&Entry{
			Name:      "file1.txt",
			Mode:      0644,
			Size:      100,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Depth:     0,
		})

		// Create derived manifest that updates the same file
		derived := NewManifest()
		derived.Base = c4.Identify(strings.NewReader("fake-base-id"))
		derived.AddEntry(&Entry{
			Name:      "file1.txt",
			Mode:      0644,
			Size:      200, // Different size
			Timestamp: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Depth:     0,
			C4ID:      c4.Identify(strings.NewReader("new-content")),
		})

		// Mock the loading
		resolver := NewBaseChainResolver("")
		resolver.cache[derived.Base] = base

		result, err := resolver.ResolveChain(derived)
		if err != nil {
			t.Fatalf("Failed to resolve chain: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(result.Entries))
		}

		// Check that the entry was updated
		if result.Entries[0].Size != 200 {
			t.Errorf("Entry should have updated size 200, got %d", result.Entries[0].Size)
		}
	})

	t.Run("CycleDetection", func(t *testing.T) {
		// Create a manifest that references itself
		manifest := NewManifest()
		selfID := c4.Identify(strings.NewReader("self-reference"))
		manifest.Base = selfID

		resolver := NewBaseChainResolver("")
		resolver.cache[selfID] = manifest

		_, err := resolver.ResolveChain(manifest)
		if err == nil {
			t.Error("Expected error for cycle, got nil")
		}
		if !strings.Contains(err.Error(), "cycle") {
			t.Errorf("Expected cycle error, got: %v", err)
		}
	})

	t.Run("DeepChain", func(t *testing.T) {
		resolver := NewBaseChainResolver("")

		// Create a chain of manifests
		var previousID c4.ID
		for i := 0; i < 10; i++ {
			m := NewManifest()
			if i > 0 {
				m.Base = previousID
			}
			m.AddEntry(&Entry{
				Name:      fmt.Sprintf("file%d.txt", i),
				Mode:      0644,
				Size:      int64(i * 100),
				Timestamp: time.Now(),
				Depth:     0,
			})

			// Generate ID for this manifest
			var buf strings.Builder
			m.WriteTo(&buf)
			id := c4.Identify(strings.NewReader(buf.String()))

			resolver.cache[id] = m
			previousID = id
		}

		// Resolve from the last manifest
		lastManifest := NewManifest()
		lastManifest.Base = previousID
		lastManifest.AddEntry(&Entry{
			Name:      "final.txt",
			Mode:      0644,
			Size:      1000,
			Timestamp: time.Now(),
			Depth:     0,
		})

		result, err := resolver.ResolveChain(lastManifest)
		if err != nil {
			t.Fatalf("Failed to resolve deep chain: %v", err)
		}

		// Should have all 11 files
		if len(result.Entries) != 11 {
			t.Errorf("Expected 11 entries, got %d", len(result.Entries))
		}
	})
}