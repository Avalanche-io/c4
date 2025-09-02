# Progressive UI Status Requirements

## Overview

The Progressive UI Status system provides real-time feedback during filesystem scanning operations, displaying concurrent progress across multiple scanning stages with detailed metrics and progress bars.

## Motivation

Large filesystem scans can take hours or days. Users need:
- Continuous feedback that the scan is progressing
- Understanding of which stage is active
- Estimates of completion time
- Ability to check status without interrupting
- Clear separation of progress info from actual output

## Display Architecture

### Two-Line Status Display

```
Line 1: Summary and per-stage progress
Line 2: Overall progress bar with completion estimate

Example:
Total: 784,429 items (51,478 dirs, 732,951 files, 3.2TB) | S1: 784,429 ✓ | S2: 650,123 (82%) | S3: 425,891/732,951 (58%)
[████████████████████░░░░░░░░░░░░░░░] Overall: 54% complete | 3.2M/s | ETA: 4m 32s | Elapsed: 5m 18s
```

### Output Streams

- **stderr**: All progress updates, status lines, informational messages
- **stdout**: Clean C4M output only (or bundle operations)
- Never mix progress updates with actual output

## Scanning Stages

### Stage 1: Structure Discovery
- Traverse directories
- Identify files vs directories vs symlinks
- Count total items
- Build initial tree structure

### Stage 2: Metadata Collection  
- Read file stats (size, permissions, timestamps)
- Resolve symlinks
- Identify regular files needing C4 computation
- Calculate total bytes to process

### Stage 3: C4 ID Computation
- Compute SHA-512 hashes
- Generate C4 IDs
- Heaviest CPU operation
- Progress based on bytes processed

## Progress Tracking

### Per-Stage Metrics

```go
type StageProgress struct {
    Name            string
    ItemsTotal      int64
    ItemsCompleted  int64
    BytesTotal      int64
    BytesCompleted  int64
    StartTime       time.Time
    CompletionTime  *time.Time
    CurrentRate     float64  // Items or bytes per second
    AverageRate     float64  // Since stage start
}
```

### Global Metrics

```go
type GlobalProgress struct {
    TotalItems      int64
    TotalDirs       int64
    TotalFiles      int64
    TotalBytes      int64
    TotalSymlinks   int64
    StartTime       time.Time
    ElapsedTime     time.Duration
    EstimatedTotal  time.Duration
    CurrentPhase    string  // "Discovering", "Analyzing", "Computing", "Complete"
}
```

## Update Mechanism

### Refresh Rate
- Update every 100ms for smooth animation
- Batch metric calculations to minimize overhead
- Use atomic operations for counter updates

### Rate Calculations
- **Instantaneous**: Last 1 second of activity
- **Average**: Since stage/scan start
- **Smoothed**: Exponential moving average over 5 seconds

### Progress Bar

```go
type ProgressBar struct {
    Width       int     // Terminal width - margins
    Completed   float64 // 0.0 to 1.0
    Style       string  // Unicode blocks or ASCII
    ShowPercent bool
    ShowETA     bool
    ShowRate    bool
}
```

Adaptive sizing:
- Detect terminal width
- Minimum 20 characters
- Scale to available space

## Stage Concurrency Display

Since stages run concurrently, show overlapping progress:

```
Structure: ████████████████████░ 95% | Metadata: ████████████░░░░░░░░ 60% | C4 IDs: ████░░░░░░░░░░░░░░░░ 20%
↓ 8,234/s                            ↓ 5,123/s                           ↓ 1,234/s
```

Or compressed single-line:
```
S1: 450K/500K | S2: 300K/450K | S3: 150K/300K | Rates: 8234/5123/1234 per sec
```

## Signal Handling

### Ctrl+T (SIGINFO on macOS/BSD)
- Output current status without stopping
- Show detailed breakdown by stage
- Include memory usage and CPU stats
- Return to progress display

### Ctrl+C (SIGINT)
- Graceful shutdown
- Save current progress (if using bundles)
- Output partial results
- Clean status line before exit

### SIGUSR1 (All platforms)
- Alternative to Ctrl+T for Linux
- Same status output behavior
- Useful for automation/scripting

## Status Command Output

When user requests status (Ctrl+T):

```
═══════════════════════════════════════════════════════════════════
C4 Progressive Scan Status - 2025-09-01 14:35:22
───────────────────────────────────────────────────────────────────
Scanning: /Users/joshua/projects
Duration: 5m 32s | Memory: 234MB | CPU: 85%
───────────────────────────────────────────────────────────────────
Stage 1 - Structure Discovery: COMPLETE
  Items found: 784,429 (51,478 directories, 732,951 files)
  Completed in: 43.2s (18,151 items/sec average)
  
Stage 2 - Metadata Collection: COMPLETE  
  Items processed: 784,429
  Total size: 3.2TB
  Completed in: 12.8s (61,283 items/sec average)
  
Stage 3 - C4 ID Computation: IN PROGRESS
  Files processed: 425,891 / 732,951 (58.1%)
  Bytes processed: 1.8TB / 3.2TB (56.3%)
  Current rate: 45.2 MB/s
  Average rate: 52.8 MB/s
  Estimated remaining: 4m 32s
───────────────────────────────────────────────────────────────────
Recent: Processing node_modules/... (45,234 files)
═══════════════════════════════════════════════════════════════════
```

## Configuration

### Display Options

```go
type UIConfig struct {
    // Display settings
    UpdateInterval   time.Duration // Default: 100ms
    ProgressStyle    string        // "unicode", "ascii", "simple"
    ColorOutput      bool          // ANSI colors if terminal supports
    CompactMode      bool          // Single line vs multi-line
    
    // Rate calculation
    RateWindow       time.Duration // Period for rate averaging
    ShowInstantRate  bool          // Show current vs average
    
    // Progress bar
    BarWidth         int           // 0 = auto-detect
    ShowETA          bool          // Show time estimates
    ShowBytes        bool          // Show byte counts vs just files
    
    // Status output
    VerboseStatus    bool          // Detailed vs summary status
    ShowMemoryStats  bool          // Include memory usage
    ShowCPUStats     bool          // Include CPU usage
}
```

### Development Mode

```go
const (
    DEV_MODE = true
    
    // Artificial delays for testing UI
    DEV_STRUCTURE_DELAY = 10 * time.Millisecond
    DEV_METADATA_DELAY  = 15 * time.Millisecond  
    DEV_C4_DELAY        = 50 * time.Millisecond
)
```

## Terminal Handling

### Capabilities Detection
- Check terminal width/height
- Detect color support
- Identify platform (for signal differences)
- Handle non-terminal output (pipes)

### Line Management
- Use ANSI escape codes for line clearing
- Carriage return for in-place updates
- Save/restore cursor position for multi-line

### Responsive Design
- Adapt to terminal resize events
- Truncate long paths intelligently
- Collapse details in narrow terminals

## Error Display

Errors appear above progress display:

```
⚠ Permission denied: /Users/joshua/private/secrets.txt
⚠ Symlink cycle detected: /Users/joshua/links/recursive
Total: 784,429 items | S1: ✓ | S2: 650,123 | S3: 425,891
[████████████████░░░░] 74% | 2 warnings | ETA: 2m 15s
```

## Performance Considerations

### Minimal Overhead
- Update calculations off main path
- Atomic counters for lock-free updates
- Batch terminal writes

### Memory Efficiency
- Fixed-size status structures
- No per-file tracking in UI
- Circular buffer for rate history

### CPU Usage
- Separate goroutine for UI updates
- Sleep between updates
- Skip updates if terminal can't keep up

## Testing Strategy

### Unit Tests
- Rate calculations with various inputs
- Progress bar rendering at different widths
- ETA calculations with varying rates
- Signal handler responses

### Integration Tests
- Full scan with progress display
- Terminal resize during scan
- Signal handling (SIGINFO/SIGUSR1)
- Non-terminal output (pipe to file)

### Visual Tests
- Different terminal emulators
- Various terminal sizes
- Color vs monochrome
- Unicode vs ASCII fallback

## Success Criteria

1. Updates remain smooth at 10fps minimum
2. Progress overhead < 1% of scan time
3. Accurate ETA within 20% for steady workloads
4. Clean separation of progress and output
5. Graceful degradation in limited terminals
6. Status available via signal without interruption
7. Memory usage for UI < 10MB
8. Works over SSH and in Docker containers

## Future Enhancements

### Phase 1 (Current)
- Basic two-line progress
- Three-stage tracking
- Simple progress bars
- Signal-based status

### Phase 2
- Colored output with severity
- Sparkline graphs for rates
- Per-directory progress tracking
- Pause/resume indicators

### Phase 3
- Web dashboard option
- JSON progress stream for tools
- Historical rate analysis
- Predictive completion using ML

### Phase 4
- Distributed scan coordination
- Multi-scanner aggregated view
- Real-time collaboration features
- Progress persistence across sessions