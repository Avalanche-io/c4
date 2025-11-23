# Implementation Plan: Progress Feedback and ID Caching

This document outlines the phased implementation of progress feedback and ID caching features for c4.

## Phase Overview

| Phase | Focus | Complexity | Dependencies |
|-------|-------|------------|--------------|
| Phase 1 | Progress Feedback Enhancement | Medium | None |
| Phase 2 | SQLite Cache Implementation | Medium | None |
| Phase 3 | Cache Integration with Scanner | Low | Phases 1 & 2 |
| Phase 4 | Cache Management Commands | Low | Phase 2 |

## Phase 1: Progress Feedback Enhancement

**Goal**: Complete and enhance the existing progressive scanner with improved progress display and cache-aware metrics.

### Tasks

#### 1.1: Enhance Progress Display
- Add cache hit/miss counters to ProgressTracker
- Update progress display to show cache statistics
- Improve ETA calculation to account for cache hit rate
- Add bytes-per-second rate in addition to files-per-second

**Files to modify**:
- `c4m/progressive_scanner.go`
- `c4m/progressive_cli.go`

**Implementation details**:
```go
// Add to ScanStatus struct
type ScanStatus struct {
    // ... existing fields ...
    CacheHits     int64
    CacheMisses   int64
    BytesScanned  int64
    TotalBytes    int64
}
```

**Acceptance criteria**:
- Progress display shows cache hit/miss statistics
- ETA calculation uses weighted average based on cache hit rate
- Bytes processed shown in addition to file count
- Tests verify cache statistics display correctly

#### 1.2: Improve Rate Calculations
- Implement exponential moving average for smoother rate display
- Track both instantaneous and average rates
- Use cache hit rate to predict future speed

**Files to modify**:
- `c4m/progressive_cli.go` (progressReporter function)

**Implementation details**:
```go
// Add smoothed rate calculation
type RateTracker struct {
    alpha         float64  // Smoothing factor (0.2 = 20% weight to new values)
    smoothedRate  float64
    lastCount     int64
    lastTime      time.Time
}

func (rt *RateTracker) Update(count int64) float64 {
    now := time.Now()
    if !rt.lastTime.IsZero() {
        elapsed := now.Sub(rt.lastTime).Seconds()
        instantRate := float64(count - rt.lastCount) / elapsed
        rt.smoothedRate = rt.alpha*instantRate + (1-rt.alpha)*rt.smoothedRate
    }
    rt.lastCount = count
    rt.lastTime = now
    return rt.smoothedRate
}
```

**Acceptance criteria**:
- Rate display doesn't fluctuate wildly
- ETA stabilizes after initial scan phase
- Tests verify rate smoothing algorithm

#### 1.3: Add Bytes Tracking
- Track total bytes found during metadata stage
- Track bytes processed during C4 computation
- Display bytes-based progress bar option

**Files to modify**:
- `c4m/progressive_scanner.go`

**Implementation details**:
```go
// Add to ProgressiveScanner
totalBytes      int64  // Total bytes in regular files
bytesProcessed  int64  // Bytes for which C4 ID computed

// Update in metadataWorker
if entry.FileMetadata.Mode().IsRegular() {
    atomic.AddInt64(&ps.totalBytes, entry.FileMetadata.Size())
}

// Update in c4Worker
atomic.AddInt64(&ps.bytesProcessed, entry.FileMetadata.Size())
```

**Acceptance criteria**:
- Total bytes accurately reflects sum of file sizes
- Bytes processed tracks progress through C4 computation
- Display shows both file and byte progress

#### 1.4: Testing
- Unit tests for progress calculations
- Integration tests for display output
- Signal handling tests

**Test files to create/modify**:
- `c4m/progressive_cli_test.go`
- `c4m/progressive_scanner_test.go`

**Acceptance criteria**:
- All tests pass
- Coverage > 80% for new code
- Manual testing confirms smooth progress display

### Phase 1 Completion Criteria
- [ ] Progress display shows cache statistics (placeholders OK for now)
- [ ] Bytes tracking integrated into progress display
- [ ] Rate calculations smoothed and stable
- [ ] ETA calculation enhanced
- [ ] All tests passing
- [ ] Manual verification on large directory scan

### Estimated Scope
- Files modified: 4
- Tests added/modified: 3
- New code: ~300 lines
- Modified code: ~200 lines

---

## Phase 2: SQLite Cache Implementation

**Goal**: Implement the persistent SQLite cache for C4 IDs with thread-safe operations.

### Tasks

#### 2.1: Create Cache Package Structure
- Create `cache` package under c4 root
- Define cache interface and implementation
- Set up SQLite with WAL mode

**Files to create**:
- `cache/cache.go`
- `cache/cache_test.go`
- `cache/schema.go`

**Implementation details**:
```go
package cache

import (
    "database/sql"
    "time"
    "github.com/Avalanche-io/c4/id"
    _ "github.com/mattn/go-sqlite3"
)

type IDCache struct {
    db   *sql.DB
    path string
}

type CacheEntry struct {
    Path         string
    Size         int64
    ModTime      time.Time
    C4ID         *id.ID
    LastVerified time.Time
}

func New(path string) (*IDCache, error)
func (c *IDCache) Lookup(path string, size int64, modtime time.Time) (*id.ID, bool)
func (c *IDCache) Store(path string, size int64, modtime time.Time, c4id *id.ID) error
func (c *IDCache) Delete(path string) error
func (c *IDCache) DeletePrefix(prefix string) error
func (c *IDCache) Stats() (*CacheStats, error)
func (c *IDCache) Cleanup(maxAge time.Duration) error
func (c *IDCache) Close() error
```

**Acceptance criteria**:
- Cache package compiles and imports successfully
- Interface is clean and minimal
- SQLite database initializes correctly

#### 2.2: Implement Database Schema
- Create schema initialization
- Set up indices for efficient lookups
- Configure SQLite pragmas for performance

**Implementation in `cache/schema.go`**:
```go
const schema = `
CREATE TABLE IF NOT EXISTS cache (
    path TEXT PRIMARY KEY,
    size INTEGER NOT NULL,
    modtime INTEGER NOT NULL,
    c4id TEXT NOT NULL,
    last_verified INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_path ON cache(path);
CREATE INDEX IF NOT EXISTS idx_last_verified ON cache(last_verified);
CREATE INDEX IF NOT EXISTS idx_size_modtime ON cache(size, modtime);
`

func initDB(db *sql.DB) error {
    // Execute schema
    if _, err := db.Exec(schema); err != nil {
        return err
    }

    // Set pragmas
    pragmas := []string{
        "PRAGMA journal_mode=WAL",
        "PRAGMA synchronous=NORMAL",
        "PRAGMA cache_size=-64000",  // 64MB
        "PRAGMA temp_store=MEMORY",
        "PRAGMA busy_timeout=5000",  // 5s timeout
    }

    for _, pragma := range pragmas {
        if _, err := db.Exec(pragma); err != nil {
            return err
        }
    }

    return nil
}
```

**Acceptance criteria**:
- Schema creates successfully
- Indices improve query performance
- WAL mode enables concurrent access

#### 2.3: Implement Core Cache Operations
- Implement Lookup with proper error handling
- Implement Store with INSERT OR REPLACE
- Implement Delete and DeletePrefix
- Ensure thread-safe operations

**Implementation in `cache/cache.go`**:
```go
func (c *IDCache) Lookup(path string, size int64, modtime time.Time) (*id.ID, bool) {
    var c4id string
    err := c.db.QueryRow(`
        SELECT c4id FROM cache
        WHERE path = ? AND size = ? AND modtime = ?
    `, path, size, modtime.UnixNano()).Scan(&c4id)

    if err == sql.ErrNoRows {
        return nil, false
    }
    if err != nil {
        // Log error but return false (treat as miss)
        return nil, false
    }

    cached, err := id.Parse(c4id)
    if err != nil {
        return nil, false
    }

    return cached, true
}

func (c *IDCache) Store(path string, size int64, modtime time.Time, c4id *id.ID) error {
    _, err := c.db.Exec(`
        INSERT OR REPLACE INTO cache (path, size, modtime, c4id, last_verified)
        VALUES (?, ?, ?, ?, ?)
    `, path, size, modtime.UnixNano(), c4id.String(), time.Now().UnixNano())

    return err
}

func (c *IDCache) DeletePrefix(prefix string) error {
    _, err := c.db.Exec(`
        DELETE FROM cache WHERE path LIKE ?
    `, prefix+"%")
    return err
}
```

**Acceptance criteria**:
- Lookup returns correct results for hits and misses
- Store persists entries correctly
- Delete operations work as expected
- Concurrent operations don't corrupt database

#### 2.4: Implement Cache Statistics
- Add Stats method to return cache metrics
- Track hit/miss rates
- Calculate cache size and entry count

**Implementation**:
```go
type CacheStats struct {
    Path         string
    Entries      int64
    SizeBytes    int64
    OldestEntry  time.Time
    NewestEntry  time.Time
}

func (c *IDCache) Stats() (*CacheStats, error) {
    stats := &CacheStats{Path: c.path}

    // Count entries
    err := c.db.QueryRow(`SELECT COUNT(*) FROM cache`).Scan(&stats.Entries)
    if err != nil {
        return nil, err
    }

    // Get database file size
    fi, err := os.Stat(c.path)
    if err == nil {
        stats.SizeBytes = fi.Size()
    }

    // Get date range
    var oldest, newest int64
    c.db.QueryRow(`SELECT MIN(last_verified), MAX(last_verified) FROM cache`).Scan(&oldest, &newest)
    if oldest > 0 {
        stats.OldestEntry = time.Unix(0, oldest)
        stats.NewestEntry = time.Unix(0, newest)
    }

    return stats, nil
}
```

**Acceptance criteria**:
- Stats returns accurate metrics
- Performance acceptable even with large caches
- Tests verify statistics calculations

#### 2.5: Implement Cache Cleanup
- Add cleanup method to remove old entries
- Support configurable age threshold
- Compact database after cleanup

**Implementation**:
```go
func (c *IDCache) Cleanup(maxAge time.Duration) error {
    cutoff := time.Now().Add(-maxAge).UnixNano()

    result, err := c.db.Exec(`
        DELETE FROM cache WHERE last_verified < ?
    `, cutoff)
    if err != nil {
        return err
    }

    deleted, _ := result.RowsAffected()

    // Compact database if significant deletions
    if deleted > 1000 {
        _, err = c.db.Exec("VACUUM")
    }

    return err
}
```

**Acceptance criteria**:
- Cleanup removes entries older than threshold
- Database compaction reclaims space
- Tests verify cleanup behavior

#### 2.6: Testing
- Unit tests for all cache operations
- Concurrency tests
- Corruption recovery tests
- Performance benchmarks

**Test coverage**:
- Cache initialization and schema creation
- Lookup hits and misses
- Store operations
- Delete and DeletePrefix
- Cleanup with various age thresholds
- Concurrent access from multiple goroutines
- Database corruption handling
- Performance with 10K, 100K, 1M entries

**Acceptance criteria**:
- All tests pass
- Coverage > 85%
- Benchmarks show acceptable performance
- Concurrency tests pass with -race flag

### Phase 2 Completion Criteria
- [ ] Cache package compiles and passes all tests
- [ ] Core operations (Lookup, Store, Delete) working
- [ ] Statistics and cleanup implemented
- [ ] Thread-safe concurrent access verified
- [ ] Performance benchmarks acceptable
- [ ] Documentation complete

### Estimated Scope
- Files created: 3-4
- Test files: 2-3
- New code: ~500 lines
- Test code: ~600 lines

---

## Phase 3: Cache Integration with Scanner

**Goal**: Integrate the cache with the progressive scanner for transparent caching during scans.

### Tasks

#### 3.1: Add Cache to ProgressiveScanner
- Add cache field to ProgressiveScanner
- Add cache initialization option
- Add cache hit/miss counters

**Files to modify**:
- `c4m/progressive_scanner.go`

**Implementation**:
```go
type ProgressiveScanner struct {
    // ... existing fields ...

    // Cache
    cache       *cache.IDCache
    cacheHits   int64
    cacheMisses int64
}

func (ps *ProgressiveScanner) SetCache(c *cache.IDCache) {
    ps.cache = c
}

func (ps *ProgressiveScanner) GetCacheStats() (hits, misses int64) {
    return atomic.LoadInt64(&ps.cacheHits), atomic.LoadInt64(&ps.cacheMisses)
}
```

**Acceptance criteria**:
- Scanner accepts cache instance
- Cache can be nil (caching disabled)
- Cache statistics accessible

#### 3.2: Integrate Cache Lookups in C4 Worker
- Check cache before computing C4 ID
- Update cache on misses
- Track hit/miss statistics

**Files to modify**:
- `c4m/progressive_scanner.go` (computeC4ID function)

**Implementation** (already shown in design doc):
```go
func (ps *ProgressiveScanner) computeC4ID(entry *ScanEntry) {
    // Check cache first
    if ps.cache != nil {
        cachedID, hit := ps.cache.Lookup(
            entry.Path,
            entry.FileMetadata.Size(),
            entry.FileMetadata.ModTime(),
        )
        if hit {
            entry.FileMetadata.SetID(cachedID)
            atomic.AddInt64(&ps.cacheHits, 1)
            atomic.AddInt64(&ps.c4Computed, 1)
            return
        }
        atomic.AddInt64(&ps.cacheMisses, 1)
    }

    // Cache miss - compute C4 ID
    file, err := os.Open(entry.Path)
    if err != nil {
        return
    }
    defer file.Close()

    c4id := c4.Identify(file)
    entry.FileMetadata.SetID(c4id)

    // Store in cache
    if ps.cache != nil {
        ps.cache.Store(
            entry.Path,
            entry.FileMetadata.Size(),
            entry.FileMetadata.ModTime(),
            c4id,
        )
    }

    atomic.AddInt64(&ps.c4Computed, 1)
}
```

**Acceptance criteria**:
- Cache lookups happen before C4 computation
- Cache hits skip file I/O and hashing
- Cache misses update cache after computation
- Statistics tracked accurately

#### 3.3: Update CLI to Support Cache
- Add cache initialization to CLI
- Add cache configuration flags
- Update progress display with cache stats

**Files to modify**:
- `c4m/progressive_cli.go`
- `cmd/c4/main.go`

**Implementation**:
```go
// In progressive_cli.go
type ProgressiveCLI struct {
    // ... existing fields ...
    cachePath string
    useCache  bool
}

func (pc *ProgressiveCLI) Run() error {
    // Initialize cache if enabled
    if pc.useCache {
        cachePath := pc.cachePath
        if cachePath == "" {
            home, _ := os.UserHomeDir()
            cachePath = filepath.Join(home, ".c4", "idcache.db")
        }

        cache, err := cache.New(cachePath)
        if err != nil {
            fmt.Fprintf(pc.errWriter, "Warning: Cache initialization failed: %v\n", err)
        } else {
            defer cache.Close()
            pc.scanner.SetCache(cache)

            // Run cleanup on startup
            cache.Cleanup(90 * 24 * time.Hour)  // 90 days
        }
    }

    // ... rest of Run implementation ...
}

// In main.go - add flags
var (
    cacheFlag     bool
    cachePathFlag string
    noCacheFlag   bool
)

flag.BoolVar(&cacheFlag, "cache", true, "Enable C4 ID caching")
flag.StringVar(&cachePathFlag, "cache-path", "", "Cache database path")
flag.BoolVar(&noCacheFlag, "no-cache", false, "Disable caching")
```

**Acceptance criteria**:
- Cache initializes automatically with default path
- Custom cache path works
- Cache can be disabled
- Errors are handled gracefully

#### 3.4: Testing
- Integration tests with cache enabled
- Test cache hit/miss scenarios
- Test cache persistence across scans
- Performance comparison with/without cache

**Test scenarios**:
1. First scan populates cache
2. Second scan uses cache (high hit rate)
3. Modified files detected and recomputed
4. Cache disabled works correctly
5. Cache errors handled gracefully

**Acceptance criteria**:
- All integration tests pass
- Cache improves performance measurably
- Tests verify correct invalidation behavior

### Phase 3 Completion Criteria
- [ ] Cache integrated with progressive scanner
- [ ] CLI supports cache configuration
- [ ] Cache statistics displayed during scan
- [ ] Tests verify cache integration
- [ ] Performance improvement measurable
- [ ] Error handling robust

### Estimated Scope
- Files modified: 3
- Test files modified: 2-3
- New code: ~200 lines
- Modified code: ~150 lines

---

## Phase 4: Cache Management Commands

**Goal**: Provide user-facing commands for cache management and inspection.

### Tasks

#### 4.1: Implement Cache Stats Command
- Add `c4 cache stats` subcommand
- Display cache statistics in readable format
- Show recent hit/miss rates if available

**Files to modify**:
- `cmd/c4/main.go`

**Files to create**:
- `cmd/c4/cache_commands.go`

**Implementation**:
```go
func runCacheStats(args []string) {
    cachePath := getCachePath()

    cache, err := cache.New(cachePath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    defer cache.Close()

    stats, err := cache.Stats()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("C4 ID Cache Statistics\n")
    fmt.Printf("─────────────────────────────────\n")
    fmt.Printf("Location: %s\n", stats.Path)
    fmt.Printf("Size: %s\n", formatBytes(stats.SizeBytes))
    fmt.Printf("Entries: %s\n", formatNumber(stats.Entries))
    fmt.Printf("\n")

    if !stats.OldestEntry.IsZero() {
        fmt.Printf("Oldest entry: %s\n", stats.OldestEntry.Format("2006-01-02 15:04:05"))
        fmt.Printf("Newest entry: %s\n", stats.NewestEntry.Format("2006-01-02 15:04:05"))
    }
}
```

**Acceptance criteria**:
- Stats display cache information clearly
- Formatting is human-readable
- Works with empty cache
- Error handling for missing/corrupt cache

#### 4.2: Implement Cache Clear Command
- Add `c4 cache clear` subcommand
- Support clearing entire cache
- Support clearing by path prefix
- Add confirmation prompt for safety

**Implementation**:
```go
func runCacheClear(args []string) {
    cachePath := getCachePath()
    prefix := ""

    if len(args) > 0 {
        prefix = args[0]
    }

    cache, err := cache.New(cachePath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    defer cache.Close()

    if prefix == "" {
        // Clear entire cache - ask for confirmation
        fmt.Printf("Clear entire cache at %s? (y/N): ", cachePath)
        var response string
        fmt.Scanln(&response)
        if response != "y" && response != "Y" {
            fmt.Println("Cancelled")
            return
        }

        // Delete cache file
        cache.Close()
        os.Remove(cachePath)
        fmt.Println("Cache cleared")
    } else {
        // Clear by prefix
        err := cache.DeletePrefix(prefix)
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error: %v\n", err)
            os.Exit(1)
        }
        fmt.Printf("Cleared cache entries for: %s\n", prefix)
    }
}
```

**Acceptance criteria**:
- Full cache clear works
- Prefix-based clear works
- Confirmation prevents accidents
- Non-interactive mode supported with --force flag

#### 4.3: Implement Cache Compact Command
- Add `c4 cache compact` subcommand
- Run VACUUM to reclaim space
- Show size before/after

**Implementation**:
```go
func runCacheCompact(args []string) {
    cachePath := getCachePath()

    cache, err := cache.New(cachePath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    defer cache.Close()

    // Get size before
    statsBefore, _ := cache.Stats()

    // Run vacuum
    _, err = cache.db.Exec("VACUUM")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }

    // Get size after
    statsAfter, _ := cache.Stats()

    fmt.Printf("Cache compacted\n")
    fmt.Printf("Before: %s\n", formatBytes(statsBefore.SizeBytes))
    fmt.Printf("After: %s\n", formatBytes(statsAfter.SizeBytes))
    fmt.Printf("Saved: %s\n", formatBytes(statsBefore.SizeBytes - statsAfter.SizeBytes))
}
```

**Acceptance criteria**:
- Compact reduces database size
- Size comparison shown
- Works even with no reclaimable space

#### 4.4: Update Main Command Dispatcher
- Add cache subcommand routing
- Update help text
- Add cache command documentation

**Files to modify**:
- `cmd/c4/main.go`

**Implementation**:
```go
func main() {
    if len(os.Args) > 1 {
        switch os.Args[1] {
        case "cache":
            runCacheCommand(os.Args[2:])
            return
        // ... existing cases ...
        }
    }
    // ... existing main logic ...
}

func runCacheCommand(args []string) {
    if len(args) == 0 {
        fmt.Fprintf(os.Stderr, "Usage: c4 cache <stats|clear|compact>\n")
        os.Exit(1)
    }

    switch args[0] {
    case "stats":
        runCacheStats(args[1:])
    case "clear":
        runCacheClear(args[1:])
    case "compact":
        runCacheCompact(args[1:])
    default:
        fmt.Fprintf(os.Stderr, "Unknown cache command: %s\n", args[0])
        os.Exit(1)
    }
}
```

**Acceptance criteria**:
- All cache subcommands accessible
- Help text updated
- Command structure clean and consistent

#### 4.5: Testing
- Test all cache management commands
- Test error conditions
- Test with missing/corrupt cache
- Integration testing with CLI

**Test coverage**:
- Cache stats with populated cache
- Cache stats with empty cache
- Cache clear (full and prefix)
- Cache compact
- Error handling for all commands
- Help text accuracy

**Acceptance criteria**:
- All commands work correctly
- Error messages helpful
- Edge cases handled
- Manual testing confirms usability

### Phase 4 Completion Criteria
- [ ] Cache stats command working
- [ ] Cache clear command working
- [ ] Cache compact command working
- [ ] Help text updated
- [ ] All commands tested
- [ ] User documentation complete

### Estimated Scope
- Files created: 1-2
- Files modified: 1-2
- New code: ~300 lines
- Test code: ~200 lines

---

## Implementation Schedule

### Recommended Order

1. **Phase 1** (Progress Feedback Enhancement)
   - Can be implemented independently
   - Provides immediate UX improvement
   - Prepares infrastructure for cache statistics

2. **Phase 2** (SQLite Cache Implementation)
   - Can be developed in parallel with Phase 1
   - Independent package, minimal dependencies
   - Can be tested thoroughly in isolation

3. **Phase 3** (Cache Integration)
   - Requires completion of Phases 1 and 2
   - Combines both features
   - Integration testing critical

4. **Phase 4** (Cache Management)
   - Requires Phase 2 complete
   - Can be done in parallel with Phase 3
   - User-facing polish

### Parallelization Opportunities

- Phase 1 and Phase 2 can be developed simultaneously by different developers
- Phase 4 can start once Phase 2 core is stable
- Testing can proceed incrementally for each phase

## Testing Strategy

### Unit Testing
- Each phase has dedicated unit tests
- Test coverage target: > 80%
- Use table-driven tests for comprehensive scenarios
- Mock filesystem for testing scanner integration

### Integration Testing
- Test complete scan workflows with cache
- Test cache persistence across multiple scans
- Test error recovery scenarios
- Test concurrent access patterns

### Performance Testing
- Benchmark cache lookup overhead
- Measure speedup with various hit rates
- Test with realistic directory structures
- Profile for bottlenecks

### Manual Testing
- Large directory scans (100K+ files)
- Repeated scans to verify caching
- Signal handling (Ctrl+T, Ctrl+C)
- Different terminal types and sizes

## Success Metrics

### Phase 1 Success
- Progress display updates smoothly (10+ FPS)
- Overhead < 1% of scan time
- ETA accuracy within 20%
- Signal handling works on all platforms

### Phase 2 Success
- Cache operations complete in < 1ms
- Database size reasonable (< 100 bytes per entry)
- Concurrent access safe and tested
- Cleanup effectively reclaims space

### Phase 3 Success
- Cache integration transparent
- 10x+ speedup on rescans with 90% hit rate
- No false positives in cache validation
- Graceful degradation when cache fails

### Phase 4 Success
- Cache commands intuitive and helpful
- Statistics provide useful insights
- Clear command prevents accidents
- Compact reclaims space effectively

## Documentation Requirements

### Code Documentation
- Package-level documentation for cache package
- Function comments for all public APIs
- Examples in godoc format
- Architecture decision records for key choices

### User Documentation
- README update with cache feature description
- Command-line help text for all cache commands
- Examples of common workflows
- Troubleshooting guide for cache issues

### Developer Documentation
- Implementation notes for each phase
- Testing strategy documentation
- Performance characteristics
- Future enhancement ideas

## Risk Mitigation

### Technical Risks
1. **SQLite performance**: Mitigated by WAL mode, pragmas, and benchmarking
2. **Cache invalidation bugs**: Mitigated by conservative invalidation strategy
3. **Concurrent access issues**: Mitigated by SQLite locking and testing
4. **Progress display overhead**: Mitigated by separate goroutine and throttling

### Process Risks
1. **Scope creep**: Mitigated by clear phase boundaries
2. **Testing gaps**: Mitigated by comprehensive test plan
3. **Platform incompatibility**: Mitigated by cross-platform testing
4. **Performance regression**: Mitigated by benchmarking

## Rollback Plan

Each phase is independently useful:
- Phase 1 alone improves UX
- Phase 2 alone provides cache infrastructure
- Phases 3 & 4 can be disabled via flags

If issues arise:
- Cache can be disabled with `--no-cache` flag
- Progress display can be disabled with `--no-progress` flag
- Corrupt cache automatically recreated
- No data loss possible (cache is optimization only)

## Future Enhancements (Post-Implementation)

### Near Term
- Cache warm-up from manifest files
- Cache export/import for distribution
- Cache statistics in machine-readable format (JSON)

### Medium Term
- Shared network cache for team workflows
- Content-addressable cache (deduplicate by C4 ID)
- Incremental scanning (only changed paths)

### Long Term
- Distributed cache architecture
- Cache replication and synchronization
- Machine learning for ETA prediction
- Web-based cache dashboard
