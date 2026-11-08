package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Incomplete ingests.
//
// A batch-mode ingest (SyncBatch) writes each object to a temp file in
// the store root and publishes it at its hash name only after the Sync
// barrier, so a name in the store always means durable bytes. A process
// killed before its barrier leaves its temp files behind. Nobody can
// tell a complete one from a torn one (the name carries no ID), so they
// cannot be replayed; Joshua's rule (2026-11-08): "On boot a system
// should garbage collect incomplete transactions that cannot, or should
// not, be replayed to completion." Opening a store is its boot.
//
// A temp file is named .ingest.<host>.<pid>.<random> so a later open can
// tell whose it was. NewTreeStore sweeps the ones whose writer is dead:
// same host (the store may sit on shared storage, where another host's
// pids mean nothing here) and a pid that is no longer running. Everything
// else is kept: a live writer's files, another host's files, and the
// pre-1.1 form .ingest.<random>, which records no writer and is swept
// only once it is older than legacyIngestAge. Store objects, the files
// at hash names, are never touched.

const ingestPrefix = ".ingest."

// legacyIngestAge is how old a pre-1.1 temp file (.ingest.<random>, no
// writer recorded) must be before the sweep treats its writer as dead.
const legacyIngestAge = 24 * time.Hour

// ingestHost names this host in temp-file names. Hostnames may contain
// dots; the name is parsed from the right, so that is fine.
var ingestHost = func() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, h)
}()

// ingestPattern is the os.CreateTemp pattern for this process's ingests.
func ingestPattern() string {
	return fmt.Sprintf("%s%s.%d.*", ingestPrefix, ingestHost, os.Getpid())
}

// parseIngestName splits .ingest.<host>.<pid>.<random>. ok is false for
// any other name, including the pre-1.1 form .ingest.<random>.
func parseIngestName(name string) (host string, pid int, ok bool) {
	if !strings.HasPrefix(name, ingestPrefix) {
		return "", 0, false
	}
	rest := name[len(ingestPrefix):]
	i := strings.LastIndexByte(rest, '.')
	if i < 0 {
		return "", 0, false
	}
	rest = rest[:i] // drop <random>
	j := strings.LastIndexByte(rest, '.')
	if j < 0 {
		return "", 0, false
	}
	pid, err := strconv.Atoi(rest[j+1:])
	if err != nil || pid <= 0 || rest[:j] == "" {
		return "", 0, false
	}
	return rest[:j], pid, true
}

// SweepIngest removes the incomplete ingests that dead writers left in
// the store root and returns how many it removed. NewTreeStore calls it;
// a long-running process may call it again. It never removes a store
// object, a live writer's temp file, or another host's.
func (s *TreeStore) SweepIngest() (int, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return 0, err
	}
	removed := 0
	now := time.Now()
	for _, d := range entries {
		name := d.Name()
		if d.IsDir() || !strings.HasPrefix(name, ingestPrefix) {
			continue
		}
		if host, pid, ok := parseIngestName(name); ok {
			if host != ingestHost || processAlive(pid) {
				continue
			}
		} else {
			info, err := d.Info()
			if err != nil || now.Sub(info.ModTime()) < legacyIngestAge {
				continue
			}
		}
		if err := os.Remove(filepath.Join(s.root, name)); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
