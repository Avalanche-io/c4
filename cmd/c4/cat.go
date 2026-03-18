package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

func runCat(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 cat <c4id>\n")
		fmt.Fprintf(os.Stderr, "\nRetrieve content by C4 ID from the configured store.\n")
		os.Exit(1)
	}

	target := args[0]

	if !looksLikeC4ID(target) {
		fatalf("Error: %q does not look like a C4 ID (90 base58 chars starting with c4)", target)
	}

	id, err := c4.Parse(target)
	if err != nil {
		fatalf("Error: invalid C4 ID: %v", err)
	}

	s, err := store.OpenConfigured()
	if err != nil {
		fatalf("Error opening store: %v", err)
	}
	if s == nil {
		fatalf("Error: no content store configured.\nSet C4_STORE=/path/to/store or s3://bucket/prefix")
	}

	rc, err := s.Open(id)
	if err != nil {
		fatalf("Error: content not found for %s", target)
	}
	defer rc.Close()

	io.Copy(os.Stdout, rc)
}

func looksLikeC4ID(s string) bool {
	if len(s) != 90 || !strings.HasPrefix(s, "c4") {
		return false
	}
	for _, ch := range s[2:] {
		if !isBase58(byte(ch)) {
			return false
		}
	}
	return true
}

func isBase58(b byte) bool {
	return (b >= '1' && b <= '9') ||
		(b >= 'A' && b <= 'H') ||
		(b >= 'J' && b <= 'N') ||
		(b >= 'P' && b <= 'Z') ||
		(b >= 'a' && b <= 'k') ||
		(b >= 'm' && b <= 'z')
}
