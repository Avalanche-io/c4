//go:build c4testhooks
// +build c4testhooks

package main

// Test-only hooks, compiled in only with -tags c4testhooks (buildC4Hooked in
// the tests). A release build has none of this.
//
// C4_TEST_STORE_PASS names actions to run between the scan and the store
// pass of `c4 id -s`, separated by ";". Paths are relative to the scanned
// directory:
//
//	rm PATH          remove a file
//	unreadable PATH  make a file unreadable (chmod 000)
//	write PATH TEXT  replace a file's bytes with TEXT
//	breakstore       replace the store's root with a regular file, so every
//	                 later Put fails
//
// This makes the window a scan cannot see (a file that changes or goes
// between the two passes) deterministic in a test.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4/store"
)

func init() {
	storePassHook = func(baseDir string, s store.Store) {
		spec := os.Getenv("C4_TEST_STORE_PASS")
		if spec == "" {
			return
		}
		for _, action := range strings.Split(spec, ";") {
			f := strings.SplitN(strings.TrimSpace(action), " ", 3)
			if len(f) == 0 || f[0] == "" {
				continue
			}
			var err error
			switch {
			case f[0] == "rm" && len(f) == 2:
				err = os.Remove(filepath.Join(baseDir, f[1]))
			case f[0] == "unreadable" && len(f) == 2:
				err = os.Chmod(filepath.Join(baseDir, f[1]), 0)
			case f[0] == "write" && len(f) == 3:
				err = os.WriteFile(filepath.Join(baseDir, f[1]), []byte(f[2]), 0o644)
			case f[0] == "breakstore" && len(f) == 1:
				r, ok := s.(interface{ Root() string })
				if !ok {
					err = fmt.Errorf("store has no root")
					break
				}
				if err = os.RemoveAll(r.Root()); err == nil {
					err = os.WriteFile(r.Root(), []byte("not a store\n"), 0o644)
				}
			default:
				err = fmt.Errorf("unknown action")
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "c4testhooks: %q: %v\n", action, err)
				os.Exit(99)
			}
		}
	}
}
