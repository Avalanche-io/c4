# Progress Feedback and ID Caching

## Overview

This document describes two complementary features that improve the performance and user experience of c4 filesystem scanning:

1. **Progress Feedback**: Real-time visual progress indicators during long-running scan operations
2. **ID Caching**: Persistent cache of C4 IDs to avoid redundant computation on repeated scans

## Motivation

### Progress Feedback

Large filesystem scans (thousands to millions of files) can take minutes to hours. Currently, c4 provides no feedback during scanning, leaving users uncertain whether:
- The scan is progressing or has stalled
- How much work remains
- Which files are being processed
- Whether to wait or cancel the operation

This creates a poor user experience for production workflows where c4 scans are part of regular operations.

### ID Caching

C4 ID computation involves reading entire files and computing SHA-512 hashes. For large files or directories with many files, this is CPU and I/O intensive. When scanning the same filesystem repeatedly (common in backup, versioning, and integrity checking workflows), most files haven't changed between scans. Re-computing C4 IDs for unchanged files wastes resources.

A persistent cache allows c4 to:
- Skip C4 computation for files that haven't changed
- Dramatically speed up repeated scans
- Reduce I/O load on storage systems
- Enable faster integrity checking workflows

## Feature 1: Progress Feedback

### User Experience

#### Terminal Progress Display

During scanning, c4 displays a live progress indicator on stderr:

```
Stage 3: Computing C4 IDs... [██████████████░░░░░░░░░░░░░░░░] 42,156/87,234 files (48%, 1,234/s avg) ETA: 36s
```

The display updates in real-time and shows:
- Current scanning stage (Structure, Metadata, or C4 IDs)
- Progress bar with visual fill
- Files processed / total files
- Percentage complete
- Average processing rate
- Estimated time to completion

#### Platform-Specific Status Check

On macOS/BSD:
- Press **Ctrl+T** to display detailed status without interrupting the scan
- Continues scanning after showing status

On Linux/Unix:
- Send **SIGUSR1** signal: `kill -USR1 <pid>`
- Same behavior as Ctrl+T

On all platforms:
- **Ctrl+C** gracefully stops scanning and outputs partial results

#### Detailed Status Output

When user requests status (Ctrl+T or SIGUSR1):

```
═══════════════════════════════════════════════════════════════════
C4 Progressive Scan Status - 2025-11-18 10:35:22
───────────────────────────────────────────────────────────────────
Scanning: /Users/joshua/projects
Duration: 2m 18s | Files Found: 87,234
───────────────────────────────────────────────────────────────────
Stage 1 - Structure Discovery: COMPLETE
  Items found: 87,234 (4,512 directories, 82,722 files)
  Completed in: 8.2s (10,638 items/s average)

Stage 2 - Metadata Collection: COMPLETE
  Items processed: 87,234
  Total size: 342GB
  Completed in: 3.1s (28,140 items/s average)

Stage 3 - C4 ID Computation: IN PROGRESS
  Files processed: 42,156 / 82,722 (50.9%)
  Bytes processed: 174GB / 342GB (50.9%)
  Current rate: 652 files/s
  Average rate: 1,234 files/s
  Estimated remaining: 36s
  Cache hits: 38,442 (91.2%)
  Cache misses: 3,714 (8.8%)
───────────────────────────────────────────────────────────────────
Recent: Processing node_modules/... (1,234 files)
═══════════════════════════════════════════════════════════════════
```

### Technical Approach

#### Multi-Stage Scanning Architecture

The progressive scanner operates in three overlapping stages:

1. **Stage 1 - Structure Discovery**
   - Fast directory traversal using platform-specific optimizations
   - Identifies files vs directories vs symlinks
   - Builds initial tree structure
   - No I/O beyond directory reads

2. **Stage 2 - Metadata Collection**
   - Reads file stats (size, permissions, timestamps)
   - Resolves symlinks
   - Identifies regular files needing C4 computation
   - Minimal I/O (stat calls only)

3. **Stage 3 - C4 ID Computation**
   - Computes SHA-512 hashes for files
   - Checks cache before computing
   - Generates C4 IDs
   - Heaviest I/O and CPU operation

Stages run concurrently with worker pools:
- Structure scanning: 2x CPU count workers
- Metadata collection: 2x CPU count workers
- C4 computation: CPU count workers (CPU-bound)

#### Progress Tracking

Progress is tracked using atomic counters:

```go
type ProgressTracker struct {
    // Global counters
    totalFound      int64  // Total items discovered
    metadataScanned int64  // Items with metadata
    c4Computed      int64  // Files with C4 IDs
    regularFiles    int64  // Regular files needing C4 computation

    // Cache statistics
    cacheHits       int64  // Cache lookups that succeeded
    cacheMisses     int64  // Cache lookups that failed

    // Timing
    startTime       time.Time
    stageStartTimes [3]time.Time
}
```

#### Display Update Loop

A dedicated goroutine updates the progress display:

```go
func (ps *ProgressiveScanner) progressReporter() {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            status := ps.getStatus()
            ps.renderProgress(status)
        case <-ps.done:
            return
        }
    }
}
```

Updates are written to stderr using ANSI escape codes:
- `\r` - Carriage return to overwrite current line
- `\033[K` - Clear to end of line
- Progress bar rendered with Unicode blocks: `█` (filled) and `░` (empty)

#### Signal Handling

Platform-specific signal handling:

```go
// On macOS/BSD: SIGINFO (Ctrl+T)
if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" {
    signals = append(signals, syscall.SIGINFO)
}

// On Linux: SIGUSR1
if runtime.GOOS == "linux" {
    signals = append(signals, syscall.SIGUSR1)
}

// All platforms: SIGINT (Ctrl+C), SIGTERM
signals = append(signals, syscall.SIGINT, syscall.SIGTERM)
```

Signal handlers:
- **SIGINFO/SIGUSR1**: Output detailed status, continue scanning
- **SIGINT/SIGTERM**: Output partial results, graceful shutdown

### Integration Points

#### Command-Line Interface

New flags for c4 command:

```bash
c4 --progressive /path/to/scan          # Enable progress display
c4 --progressive --verbose /path        # Verbose mode with progress
c4 --progressive --quiet /path          # Suppress progress, show on signals only
```

Progress is automatically enabled for:
- Terminal output (isatty check)
- Scans expected to take more than a few seconds
- Can be explicitly disabled with `--no-progress`

#### Existing Code Integration

The progressive scanner is already partially implemented in:
- `/Users/joshua/ws/active/c4/c4/c4m/progressive_scanner.go`
- `/Users/joshua/ws/active/c4/c4/c4m/progressive_cli.go`

Enhancements needed:
- Integration with ID cache lookups
- Cache hit/miss statistics in progress display
- Improved ETA calculation using cache hit rate

## Feature 2: ID Caching

### User Experience

#### Transparent Operation

ID caching is completely transparent to users. When enabled:
- First scan of files computes C4 IDs normally
- Subsequent scans check the cache first
- If file hasn't changed (same size + modtime), use cached ID
- If file has changed, recompute ID and update cache
- No user action required

#### Cache Statistics

Users can view cache statistics:

```bash
c4 cache stats
```

Output:
```
C4 ID Cache Statistics
───────────────────────────────────────────
Location: /Users/joshua/.c4/idcache.db
Size: 24.3 MB
Entries: 142,567

Recent scans:
  Hits: 128,442 (90.1%)
  Misses: 14,125 (9.9%)
  Total lookups: 142,567
  Time saved: ~3m 42s
```

#### Cache Management

```bash
c4 cache clear              # Clear entire cache
c4 cache clear /path        # Clear cache entries for specific path
c4 cache stats              # Show cache statistics
c4 cache compact            # Compact database (reclaim space)
```

### Technical Approach

#### Cache Storage

SQLite database at `~/.c4/idcache.db`:

**Schema:**

```sql
CREATE TABLE cache (
    path TEXT PRIMARY KEY,
    size INTEGER NOT NULL,
    modtime INTEGER NOT NULL,  -- Unix timestamp in nanoseconds
    c4id TEXT NOT NULL,
    last_verified INTEGER NOT NULL,  -- When we last verified this entry
    UNIQUE(path, size, modtime)
);

CREATE INDEX idx_path ON cache(path);
CREATE INDEX idx_last_verified ON cache(last_verified);
```

**Rationale:**
- SQLite provides reliable, embedded storage without external dependencies
- Single database file simplifies deployment and backup
- Built-in ACID guarantees ensure cache consistency
- Efficient indexing for path lookups
- Cross-platform support

#### Cache Key Design

Cache entries are keyed by: `(path, size, modtime)`

This triple provides strong confidence that file content hasn't changed:
- **Path**: Unique file location
- **Size**: Content length in bytes
- **Modtime**: Last modification timestamp

If any component changes, cache is invalidated and C4 ID is recomputed.

**Trade-offs:**
- **False negatives**: If a file is modified and restored to original content with same modtime and size, we'll miss the cache (rare, acceptable)
- **False positives**: Impossible - if size or modtime changes, we always recompute
- **Race conditions**: If file is modified during scan, cache may be stale. This is inherent to any filesystem scanning and not specific to caching

#### Cache Lookup Flow

```go
func (c *IDCache) Lookup(path string, size int64, modtime time.Time) (*id.ID, bool) {
    // Query cache
    var c4id string
    err := c.db.QueryRow(`
        SELECT c4id FROM cache
        WHERE path = ? AND size = ? AND modtime = ?
    `, path, size, modtime.UnixNano()).Scan(&c4id)

    if err == sql.ErrNoRows {
        return nil, false  // Cache miss
    }
    if err != nil {
        return nil, false  // Error treated as miss
    }

    // Parse and return cached ID
    cached, err := id.Parse(c4id)
    if err != nil {
        return nil, false
    }

    return cached, true  // Cache hit
}
```

#### Cache Update Flow

```go
func (c *IDCache) Store(path string, size int64, modtime time.Time, c4id *id.ID) error {
    _, err := c.db.Exec(`
        INSERT OR REPLACE INTO cache (path, size, modtime, c4id, last_verified)
        VALUES (?, ?, ?, ?, ?)
    `, path, size, modtime.UnixNano(), c4id.String(), time.Now().UnixNano())

    return err
}
```

#### Integration with Progressive Scanner

Modified C4 computation worker:

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

#### Cache Invalidation Strategy

**Automatic Invalidation:**
- Cache entry is invalid if size or modtime doesn't match
- No explicit invalidation needed - lookup simply fails
- Stale entries naturally replaced when files are rescanned

**Manual Invalidation:**
- `c4 cache clear` removes all entries
- `c4 cache clear /path` removes entries under path prefix
- Cache cleanup removes entries not verified in last 90 days

**Cleanup Strategy:**

```go
func (c *IDCache) Cleanup(maxAge time.Duration) error {
    cutoff := time.Now().Add(-maxAge).UnixNano()
    _, err := c.db.Exec(`
        DELETE FROM cache
        WHERE last_verified < ?
    `, cutoff)
    return err
}
```

Default cleanup: Run on startup, remove entries older than 90 days.

#### Thread Safety

SQLite in WAL mode provides concurrent read/write access:

```go
func NewIDCache(path string) (*IDCache, error) {
    db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
    if err != nil {
        return nil, err
    }

    // Set pragmas for performance
    db.Exec("PRAGMA synchronous = NORMAL")
    db.Exec("PRAGMA cache_size = -64000")  // 64MB cache
    db.Exec("PRAGMA temp_store = MEMORY")

    return &IDCache{db: db}, nil
}
```

Multiple scanner workers can:
- **Read**: Concurrent cache lookups (no locks)
- **Write**: Serialized cache updates (SQLite handles locking)

### Configuration

#### Cache Location

Default: `~/.c4/idcache.db`

Override with environment variable:
```bash
export C4_CACHE_PATH=/custom/path/cache.db
```

Or command-line flag:
```bash
c4 --cache-path=/custom/path/cache.db /path/to/scan
```

#### Cache Control Flags

```bash
c4 --no-cache /path              # Disable cache for this scan
c4 --cache-readonly /path        # Use cache but don't update
c4 --cache-writeonly /path       # Populate cache but don't use for lookups
c4 --cache-stats /path           # Show cache statistics after scan
```

#### Performance Tuning

Environment variables:

```bash
C4_CACHE_ENABLED=1              # Enable/disable caching (default: 1)
C4_CACHE_PATH=$HOME/.c4/idcache.db
C4_CACHE_SIZE_MB=64             # SQLite cache size (default: 64)
C4_CACHE_CLEANUP_DAYS=90        # Cleanup age threshold (default: 90)
```

## Implementation Considerations

### Performance Impact

#### Progress Display Overhead
- Progress updates run in separate goroutine
- Updates throttled to 100ms intervals (10 FPS)
- Atomic counter operations have minimal overhead (~10ns)
- Terminal I/O to stderr doesn't block main scan
- **Expected overhead: < 0.5% of total scan time**

#### Cache Overhead
- Cache lookup: Single indexed SQL query (~100-500μs)
- Cache store: Single indexed SQL insert (~200-800μs)
- C4 computation saved: ~1-100ms per file (depending on size)
- **Break-even: File > ~100KB, or cache hit rate > 10%**
- **Typical speedup: 5-50x for repeated scans with high hit rates**

### Error Handling

#### Cache Errors
- Cache initialization failure: Log warning, continue without cache
- Cache lookup failure: Treat as cache miss, compute C4 ID
- Cache store failure: Log warning, continue scanning
- Corrupt cache database: Delete and recreate on next run

**Philosophy**: Cache is a performance optimization, not critical infrastructure. Any cache error should degrade gracefully to uncached operation.

#### Progress Display Errors
- Terminal not available: Disable progress display
- Signal setup failure: Log warning, continue without signal handling
- Display render error: Log warning, continue scanning

### Platform Compatibility

#### Progress Display
- **macOS/BSD**: Ctrl+T (SIGINFO) native support
- **Linux**: SIGUSR1 signal for status
- **Windows**: No signal support, progress bar only
- **All platforms**: Ctrl+C graceful shutdown

#### Cache Storage
- **All platforms**: SQLite with WAL mode
- **Path handling**: Use filepath.Clean() for consistent cache keys
- **File timestamps**: Store as Unix nanoseconds for consistency

### Testing Strategy

#### Progress Display Tests
1. **Unit tests**:
   - Rate calculation accuracy
   - ETA calculation with varying rates
   - Progress bar rendering at different widths
   - Counter atomicity

2. **Integration tests**:
   - Full scan with progress enabled
   - Signal handling (SIGINFO, SIGUSR1, SIGINT)
   - Non-terminal output detection
   - Concurrent stage progress

3. **Visual tests** (manual):
   - Different terminal sizes
   - Various file counts and sizes
   - Cache hit rate impact on display

#### Cache Tests
1. **Unit tests**:
   - Cache lookup hit/miss scenarios
   - Cache invalidation logic
   - Concurrent access safety
   - Database schema validation

2. **Integration tests**:
   - First scan (populate cache)
   - Second scan (use cache)
   - Modified file detection
   - Cache cleanup operation
   - Cache corruption recovery

3. **Performance tests**:
   - Cache overhead measurement
   - Speedup with various hit rates
   - Large cache performance (millions of entries)
   - Concurrent worker scaling

### Migration Path

Since c4 is pre-release (1.0), no backward compatibility required:
- Add cache in 1.0 release
- Cache schema can evolve with version migrations if needed
- Progress display is runtime-only, no persistence concerns

## Success Criteria

### Progress Feedback
1. Display updates remain smooth at 10 FPS minimum
2. Progress overhead < 1% of total scan time
3. Accurate ETA within 20% for steady workloads
4. Clean separation of progress (stderr) and output (stdout)
5. Signal-based status works on macOS, Linux, and BSD
6. Graceful degradation when terminal unavailable

### ID Caching
1. Cache hit rate > 90% for repeated scans of mostly-unchanged filesystems
2. Cache lookup overhead < 1ms per file
3. Speedup of 10x or more for rescans with 90%+ hit rate
4. Cache size remains reasonable (< 1GB for 1M files)
5. Correct invalidation - no false positives
6. Graceful operation when cache unavailable

### Combined
1. Progress display shows cache hit/miss statistics
2. ETA calculation accounts for cache hit rate
3. Cache operations don't interfere with progress updates
4. Both features work together seamlessly

## Future Enhancements

### Progress Feedback
- Web dashboard for remote monitoring
- JSON progress stream for tool integration
- Historical performance tracking
- Per-directory progress breakdown

### ID Caching
- Shared network cache for team workflows
- Cache warm-up from previous manifest files
- Content-addressable cache (deduplicate by C4 ID)
- Cache export/import for deployment scenarios

### Advanced Features
- Incremental scanning (only check changed paths)
- Watch mode (continuous monitoring)
- Distributed scanning with progress aggregation
- Machine learning for better ETA prediction
