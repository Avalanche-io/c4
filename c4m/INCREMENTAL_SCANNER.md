# Incremental Filesystem Scanner with C4M

This document describes patterns for building high-performance filesystem scanners that use c4m as a presentation layer. The goal is responsive UI feedback during scans of large, deep filesystems while progressively resolving metadata and content identifiers.

---

## The Challenge

Scanning a large filesystem involves multiple expensive operations:

| Operation | Cost | Notes |
|-----------|------|-------|
| `readdir` | Low | Returns names only (fast) |
| `stat` | Medium | File metadata (mode, size, mtime) |
| `readlink` | Medium | Symlink target resolution |
| `c4.Identify` | High | Full file read + SHA-512 hash |

A naive approach that does all operations for each file before returning results would be unusable for:
- Deep hierarchies (thousands of nested directories)
- Wide directories (millions of files in one folder)
- Large files (gigabytes to terabytes)
- Remote filesystems (network latency per operation)

---

## Progressive Resolution Strategy

C4M's null value support enables **progressive resolution** - return what you have immediately, refine later:

```
Level 0: Names + Structure (readdir only)
         drwxr-xr-x - - docs/
           ---------- - - readme.md
           ---------- - - api.md

Level 1: + Metadata (stat)
         drwxr-xr-x 2024-01-15T10:00:00Z 4096 docs/
           -rw-r--r-- 2024-01-15T09:30:00Z 1234 readme.md
           -rw-r--r-- 2024-01-14T15:00:00Z 5678 api.md

Level 2: + Symlink targets (readlink)
         lrwxrwxrwx 2024-01-15T10:00:00Z 12 latest -> ./v2.0/

Level 3: + Content IDs (c4.Identify)
         -rw-r--r-- 2024-01-15T09:30:00Z 1234 readme.md c4abc123...
```

Each level can be returned as a **changeset** layered on top of the previous state.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      Client Application                      │
│  (file browser, sync tool, backup system, etc.)             │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Request: "list /project/src"
                              │ Response: c4m manifest (partial)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Scanner Service                          │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │ Priority     │  │ Walk         │  │ Resolution       │  │
│  │ Queue        │◄─┤ Coordinator  │◄─┤ Workers          │  │
│  │              │  │              │  │ (stat, readlink, │  │
│  │ User focus   │  │ BreadthFirst │  │  c4.Identify)    │  │
│  │ drives order │  │ traversal    │  │                  │  │
│  └──────────────┘  └──────────────┘  └──────────────────┘  │
│                              │                               │
│                              ▼                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              Manifest Cache                           │  │
│  │  path → Entry (with resolution level tracking)        │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    absfs.FileSystem                          │
│  (osfs, memfs, s3fs, sftpfs, etc.)                          │
└─────────────────────────────────────────────────────────────┘
```

---

## Using absfs/fstools for Efficient Walking

The [absfs/fstools](https://github.com/absfs/fstools) package provides optimized filesystem traversal with multiple strategies:

### BreadthFirst for Interactive Navigation

For UI-driven scanning, **BreadthFirst** traversal is ideal - it visits all entries at each depth level before descending:

```go
import (
    "github.com/absfs/absfs"
    "github.com/absfs/fstools"
    "github.com/absfs/osfs"
    "github.com/Avalanche-io/c4/c4m"
)

// Scanner provides incremental filesystem scanning
type Scanner struct {
    fs       absfs.FileSystem
    cache    map[string]*ScanEntry
    priority *PriorityQueue
    mu       sync.RWMutex
}

// ScanEntry tracks resolution state for each path
type ScanEntry struct {
    Entry    *c4m.Entry
    Level    ResolutionLevel
    Priority int
}

type ResolutionLevel int

const (
    LevelName     ResolutionLevel = iota // readdir only
    LevelMetadata                        // + stat
    LevelSymlink                         // + readlink
    LevelContent                         // + c4.Identify
)

// StartScan begins scanning from root using BreadthFirst traversal
func (s *Scanner) StartScan(root string) error {
    opts := fstools.Options{
        Traversal: fstools.BreadthOrder,
        Less: func(a, b string) bool {
            // Directories first, then alphabetical
            return a < b
        },
    }

    return fstools.WalkWithOptions(s.fs, root, opts, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return nil // Skip errors, continue scanning
        }

        // Create entry with Level 0 (name + type only)
        entry := &c4m.Entry{
            Name: filepath.Base(path),
            Size: -1, // Null - not yet resolved
        }

        if info.IsDir() {
            entry.Name += "/"
            entry.Mode = os.ModeDir | 0755
        }

        s.mu.Lock()
        s.cache[path] = &ScanEntry{
            Entry: entry,
            Level: LevelName,
        }
        s.mu.Unlock()

        // Queue for metadata resolution
        s.priority.Push(path, defaultPriority)

        return nil
    })
}
```

### PreOrder for Complete Scans

For batch operations (backup, sync), **PreOrder** ensures parent directories are processed before children:

```go
func (s *Scanner) FullScan(root string) (*c4m.Manifest, error) {
    manifest := c4m.NewManifest()
    depthStack := []int{0}

    opts := fstools.Options{
        Traversal: fstools.PreOrder,
    }

    err := fstools.WalkWithOptions(s.fs, root, opts, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return nil
        }

        // Calculate depth from path
        depth := strings.Count(path, string(os.PathSeparator))

        entry := &c4m.Entry{
            Name:      filepath.Base(path),
            Mode:      info.Mode(),
            Size:      info.Size(),
            Timestamp: info.ModTime().UTC(),
            Depth:     depth,
        }

        if info.IsDir() {
            entry.Name += "/"
        }

        manifest.AddEntry(entry)
        return nil
    })

    return manifest, err
}
```

---

## Priority-Based Resolution

User navigation should drive what gets resolved first:

```go
// PriorityQueue manages resolution ordering
type PriorityQueue struct {
    items    []priorityItem
    mu       sync.Mutex
    cond     *sync.Cond
}

type priorityItem struct {
    path     string
    priority int // Higher = more urgent
    level    ResolutionLevel
}

const (
    PriorityBackground = 0   // Default scanning
    PriorityVisible    = 50  // Currently displayed directory
    PriorityFocused    = 100 // User actively navigating here
)

// Boost increases priority for a path and its immediate children
func (s *Scanner) Boost(path string, priority int) {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Boost the path itself
    if entry, ok := s.cache[path]; ok {
        s.priority.UpdatePriority(path, priority)
    }

    // Boost immediate children
    prefix := path
    if !strings.HasSuffix(prefix, "/") {
        prefix += "/"
    }

    for p := range s.cache {
        if strings.HasPrefix(p, prefix) {
            // Only boost direct children (one level down)
            remainder := strings.TrimPrefix(p, prefix)
            if !strings.Contains(remainder, "/") || strings.HasSuffix(remainder, "/") {
                s.priority.UpdatePriority(p, priority)
            }
        }
    }
}
```

---

## Returning Partial Manifests

When a client requests a directory listing, return what's available immediately:

```go
// GetManifest returns current state for a path (may be partial)
func (s *Scanner) GetManifest(path string, depth int) *c4m.Manifest {
    s.mu.RLock()
    defer s.mu.RUnlock()

    manifest := c4m.NewManifest()
    prefix := path
    if !strings.HasSuffix(prefix, "/") {
        prefix += "/"
    }

    for p, scanEntry := range s.cache {
        if !strings.HasPrefix(p, prefix) {
            continue
        }

        // Calculate relative depth
        remainder := strings.TrimPrefix(p, prefix)
        entryDepth := strings.Count(remainder, "/")
        if strings.HasSuffix(remainder, "/") {
            entryDepth-- // Directory trailing slash doesn't count
        }

        if entryDepth > depth {
            continue
        }

        // Copy entry with current resolution state
        entry := *scanEntry.Entry
        entry.Depth = entryDepth
        manifest.AddEntry(&entry)
    }

    manifest.SortEntries()
    return manifest
}
```

---

## Incremental Updates via Changesets

When resolution completes, send changesets rather than full manifests:

```go
// Changeset represents updates since a previous manifest state
type Changeset struct {
    BaseID   c4.ID      // ID of the manifest this patches
    Updates  []*c4m.Entry // Entries with new/updated values
    Complete []string    // Paths now fully resolved
}

// GetChangeset returns updates since the given manifest ID
func (s *Scanner) GetChangeset(baseID c4.ID, path string) *Changeset {
    // Track what changed since baseID was computed
    // This requires versioning in the cache
    // ...
}

// As a c4m layer:
func (cs *Changeset) ToManifest() *c4m.Manifest {
    m := c4m.NewManifest()
    m.Base = cs.BaseID

    for _, entry := range cs.Updates {
        m.AddEntry(entry)
    }

    return m
}
```

---

## Resolution Workers

Background workers progressively resolve entries:

```go
// ResolutionWorker processes the priority queue
func (s *Scanner) ResolutionWorker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        // Get highest priority item needing resolution
        item := s.priority.Pop()
        if item == nil {
            time.Sleep(10 * time.Millisecond)
            continue
        }

        s.mu.RLock()
        scanEntry := s.cache[item.path]
        s.mu.RUnlock()

        if scanEntry == nil {
            continue
        }

        // Resolve to next level
        switch scanEntry.Level {
        case LevelName:
            s.resolveStat(item.path, scanEntry)
        case LevelMetadata:
            if scanEntry.Entry.IsSymlink() {
                s.resolveSymlink(item.path, scanEntry)
            } else {
                s.resolveContent(item.path, scanEntry)
            }
        case LevelSymlink:
            s.resolveContent(item.path, scanEntry)
        }
    }
}

func (s *Scanner) resolveStat(path string, scanEntry *ScanEntry) {
    info, err := s.fs.Stat(path)
    if err != nil {
        return
    }

    s.mu.Lock()
    scanEntry.Entry.Mode = info.Mode()
    scanEntry.Entry.Size = info.Size()
    scanEntry.Entry.Timestamp = info.ModTime().UTC()
    scanEntry.Level = LevelMetadata
    s.mu.Unlock()

    // Re-queue for content resolution (if file)
    if !info.IsDir() {
        s.priority.Push(path, scanEntry.Priority)
    }
}

func (s *Scanner) resolveContent(path string, scanEntry *ScanEntry) {
    f, err := s.fs.Open(path)
    if err != nil {
        return
    }
    defer f.Close()

    id := c4.Identify(f)

    s.mu.Lock()
    scanEntry.Entry.C4ID = id
    scanEntry.Level = LevelContent
    s.mu.Unlock()
}
```

---

## Handling Very Large Directories with Paged Manifests

For directories with millions of files, use **paged manifests** where each page is a changeset layered on the previous via `@base`. This approach:

1. Each page/block gets its own C4 ID (verifiable, cacheable)
2. Pages chain together: `page1 ← page2 ← page3 ← ...`
3. Final merge produces a canonical C4 ID for the entire directory
4. Parent manifest can reference the directory with a single C4 ID

```
Page 1 (first 10,000 files):
@c4m 1.0
-rw-r--r-- ... file_00001.dat
-rw-r--r-- ... file_00002.dat
...
-rw-r--r-- ... file_10000.dat

Page 1 ID: c4PageOneID...

Page 2 (next 10,000 files):
@c4m 1.0
@base c4PageOneID...
-rw-r--r-- ... file_10001.dat
-rw-r--r-- ... file_10002.dat
...
-rw-r--r-- ... file_20000.dat

Page 2 ID: c4PageTwoID...

... continue until directory fully scanned ...

Final merged manifest has canonical ID: c4FinalDirID...
Parent can now reference: drwxr-xr-x ... huge_dir/ c4FinalDirID...
```

### Implementation

```go
// PagedDirectoryScanner scans large directories in blocks
type PagedDirectoryScanner struct {
    fs        absfs.FileSystem
    pageSize  int
    store     store.Store // Where to persist page manifests
}

// ScanDirectory scans a directory, returning manifest pages via channel
// Each page references the previous via @base
func (p *PagedDirectoryScanner) ScanDirectory(ctx context.Context, path string) (<-chan *c4m.Manifest, error) {
    dir, err := p.fs.Open(path)
    if err != nil {
        return nil, err
    }

    pages := make(chan *c4m.Manifest, 2) // Small buffer

    go func() {
        defer close(pages)
        defer dir.Close()

        var prevPageID c4.ID
        pageNum := 0

        for {
            // Read a page worth of entries
            names, err := dir.Readdirnames(p.pageSize)
            if len(names) == 0 {
                break
            }

            // Build manifest for this page
            builder := c4m.NewBuilder()
            if !prevPageID.IsNil() {
                builder.WithBaseID(prevPageID)
            }

            for _, name := range names {
                info, statErr := p.fs.Stat(filepath.Join(path, name))
                if statErr != nil {
                    continue
                }

                if info.IsDir() {
                    builder.AddDir(name, c4m.WithMode(info.Mode()),
                        c4m.WithTimestamp(info.ModTime()))
                } else {
                    builder.AddFile(name, c4m.WithMode(info.Mode()),
                        c4m.WithSize(info.Size()),
                        c4m.WithTimestamp(info.ModTime()))
                }
            }

            page := builder.MustBuild()
            pageID := page.ComputeC4ID()

            // Persist the page (so it can be resolved later)
            p.persistPage(pageID, page)

            // Send to consumer
            select {
            case pages <- page:
            case <-ctx.Done():
                return
            }

            prevPageID = pageID
            pageNum++

            if err != nil { // EOF or error
                break
            }
        }
    }()

    return pages, nil
}

// persistPage stores a manifest page for later resolution
func (p *PagedDirectoryScanner) persistPage(id c4.ID, manifest *c4m.Manifest) error {
    data, err := c4m.Marshal(manifest)
    if err != nil {
        return err
    }

    w, err := p.store.Create(id)
    if err != nil {
        return err
    }
    defer w.Close()

    _, err = w.Write(data)
    return err
}

// FinalizeDirectory merges all pages into a single canonical manifest
func (p *PagedDirectoryScanner) FinalizeDirectory(lastPageID c4.ID) (*c4m.Manifest, error) {
    // Use the Merge helper to flatten the @base chain
    getter := c4m.FromStore(p.store)

    lastPage, err := getter.Get(lastPageID)
    if err != nil {
        return nil, err
    }

    return lastPage.Merge(getter)
}
```

### Client-Side Progressive Loading

The client can request pages incrementally and merge locally:

```go
// Client progressively loads directory contents
type DirectoryLoader struct {
    cache    c4m.MapGetter
    current  *c4m.Manifest
}

// LoadPage incorporates a new page into the view
func (d *DirectoryLoader) LoadPage(page *c4m.Manifest) error {
    pageID := page.ComputeC4ID()
    d.cache[pageID] = page

    // Merge all pages so far
    merged, err := page.Merge(d.cache)
    if err != nil {
        return err
    }

    d.current = merged
    return nil
}

// Entries returns current merged view
func (d *DirectoryLoader) Entries() []*c4m.Entry {
    if d.current == nil {
        return nil
    }
    return d.current.Entries
}

// FinalID returns the canonical ID once all pages are loaded
func (d *DirectoryLoader) FinalID() c4.ID {
    if d.current == nil {
        return c4.ID{}
    }
    return d.current.ComputeC4ID()
}
```

### Why Paged > Streaming

| Aspect | Streaming | Paged with @base |
|--------|-----------|------------------|
| Verifiability | None - entries ephemeral | Each page has C4 ID |
| Resumability | Start over on disconnect | Resume from last page ID |
| Cacheability | No | Each page cacheable |
| Final identity | None | Merged manifest has canonical ID |
| Parent reference | Must inline all entries | Single C4 ID for directory |
| Network efficiency | Re-transmit on error | Only retransmit failed page |

---

## Handling Very Deep Hierarchies

For deeply nested structures, limit concurrent descent and use path-based prioritization:

```go
// DepthLimiter prevents unbounded descent
type DepthLimiter struct {
    maxConcurrentDepths int
    activeDepths        map[int]int // depth -> count of active scans
    mu                  sync.Mutex
}

func (d *DepthLimiter) CanDescend(depth int) bool {
    d.mu.Lock()
    defer d.mu.Unlock()

    // Allow unlimited scanning at shallow depths
    if depth < 3 {
        return true
    }

    // Limit concurrent deep scans
    total := 0
    for _, count := range d.activeDepths {
        total += count
    }

    return total < d.maxConcurrentDepths
}
```

---

## Example: Interactive File Browser

Putting it all together for a file browser that shows immediate feedback:

```go
func main() {
    // Use osfs for local filesystem (could be s3fs, sftpfs, etc.)
    fs := osfs.NewFileSystem()

    scanner := NewScanner(fs)

    // Start background scanning
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go scanner.StartScan("/home/user/projects")

    // Start resolution workers
    for i := 0; i < runtime.NumCPU(); i++ {
        go scanner.ResolutionWorker(ctx)
    }

    // Simulate user navigation
    http.HandleFunc("/api/list", func(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Query().Get("path")

        // Boost priority for this path
        scanner.Boost(path, PriorityFocused)

        // Return current state immediately
        manifest := scanner.GetManifest(path, 1) // depth=1 for immediate children

        data, _ := c4m.Marshal(manifest)
        w.Header().Set("Content-Type", "text/plain")
        w.Write(data)
    })

    http.HandleFunc("/api/updates", func(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Query().Get("path")
        baseID := r.URL.Query().Get("base") // Previous manifest ID

        // Return changeset since last request
        // Client can merge this with their cached state
        id, _ := c4.Parse(baseID)
        changeset := scanner.GetChangeset(id, path)

        // Return as c4m layer
        manifest := changeset.ToManifest()
        data, _ := c4m.Marshal(manifest)
        w.Write(data)
    })

    http.ListenAndServe(":8080", nil)
}
```

---

## C4M Format for Partial Results

A partial manifest with null values clearly indicates what's not yet resolved:

```
@c4m 1.0
drwxr-xr-x 2024-01-15T10:00:00Z 4096 project/
  ---------- - - readme.md
  ---------- - - src/
    ---------- - - main.go
    ---------- - - util.go
  -rw-r--r-- 2024-01-15T09:30:00Z 1234 config.json c4abc123...
```

In this example:
- `project/` has full metadata (stat completed)
- `readme.md`, `src/`, `main.go`, `util.go` have names only (readdir completed)
- `config.json` is fully resolved (including C4 ID)

---

## Performance Considerations

| Technique | Benefit |
|-----------|---------|
| BreadthFirst traversal | Show structure quickly, details later |
| Priority queue | User focus drives resolution order |
| Batched readdir | Reduce syscall overhead for large dirs |
| Worker pool | Parallelize stat/hash operations |
| Paged manifests | Verifiable chunks with canonical IDs |
| @base chaining | Resume from any page, cache intermediates |
| Depth limiting | Prevent resource exhaustion on deep trees |
| Null values in c4m | Clear indication of resolution state |
| Final merge | Canonical ID for entire directory |

---

## Integration with C4M Merge

When combining scan results from multiple sources or sessions:

```go
// Resume a previous scan from a known page
func (s *Scanner) ResumeFromPage(pageID c4.ID, getter c4m.Getter) error {
    // Load the page and merge to get current state
    page, err := getter.Get(pageID)
    if err != nil {
        return err
    }

    merged, err := page.Merge(getter)
    if err != nil {
        return err
    }

    // Populate cache with merged state
    for _, entry := range merged.Entries {
        level := LevelContent
        if entry.C4ID.IsNil() {
            level = LevelMetadata
        }
        if entry.Size < 0 {
            level = LevelName
        }

        s.cache[entry.Name] = &ScanEntry{
            Entry: entry,
            Level: level,
        }

        // Queue incomplete entries for resolution
        if level < LevelContent {
            s.priority.Push(entry.Name, PriorityBackground)
        }
    }

    return nil
}
```

---

## Summary

Building an incremental filesystem scanner with c4m involves:

1. **Progressive resolution**: Return names first, then metadata, then content IDs
2. **Priority-driven**: User navigation boosts resolution priority
3. **BreadthFirst walking**: Show directory structure quickly
4. **Null values**: c4m format clearly indicates unresolved fields
5. **Paged manifests**: Each page has a C4 ID, chains via @base
6. **Worker pools**: Parallelize expensive operations (stat, hash)
7. **Depth limiting**: Prevent resource exhaustion
8. **Final merge**: Produce canonical C4 ID for directories of any size

The key insight is that **paged @base chains** give you the benefits of streaming (handle any size) while preserving content-addressability:
- Each page is verifiable and cacheable
- Interrupted scans resume from the last page ID
- The final merged manifest has a canonical C4 ID
- Parent directories can reference child directories with a single ID

The [absfs](https://github.com/absfs) ecosystem provides the filesystem abstraction, and [fstools](https://github.com/absfs/fstools) provides optimized walking strategies. C4M provides the presentation layer with built-in support for partial/incremental data through null values and layered @base chains.
