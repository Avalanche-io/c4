# Retention Implementation Status

Checkpoint against [retention.md](retention.md). Last updated 2026-03-07.

## Complete (Implemented, Tested, Committed)

| Feature | Location |
|---------|----------|
| Reference graph (forward index, backward index, event-driven purgatory, resurrection) | `c4d internal/refgraph/refgraph.go` |
| Pressure curve reclamation | `c4d internal/refgraph/reclaim.go` |
| TTL-bearing paths (trash-can policy model) | `c4d internal/namespace/ttl.go` |
| Expiration goroutine (auto-remove expired entries) | `c4d internal/namespace/expire.go` |
| Server wiring (refgraph + TTL on namespace PUT/DELETE) | `c4d internal/server/namespace.go`, `server.go` |
| Purge endpoint (`POST /admin/purge`) | `c4d internal/server/purge.go` |
| `c4 du` (reachability breakdown: active/purgeable/total/limit) | `c4 cmd/c4/du.go` + `c4d internal/server/du.go` |
| `c4 rm --purge` (flush purgatory) | `c4 cmd/c4/namespace.go` (`runPurge`) |
| `--retain` flag (sends `X-TTL-Policy` header on `c4 mk`) | `c4 cmd/c4/mk.go`, `namespace.go` |
| `--snapshot-retain N` (raw history pruning + orphan cleanup) | `c4 cmd/c4/internal/managed/managed.go` |
| Raw snapshots on every mutation | `c4 cmd/c4/internal/managed/managed.go` (`Snapshot`) |
| Undo/redo | `c4 cmd/c4/internal/managed/managed.go` |
| Manual tags | `c4 cmd/c4/internal/managed/managed.go` |
| Startup rebuild (full reachability scan before reclamation) | `c4d serve.go` |

## Deferred (Designed, Not Yet Implemented)

| Feature | What's Needed |
|---------|---------------|
| `--snapshot-retain` applying to tags | Currently prunes raw history only. Design says it controls tag count, not raw snapshots. Wire tag pruning into the retention logic. |
| Auto-tagging by cadence (`--snapshot-cadence`) | Evaluate time-since-last-tag at CLI mutation points. Tag automatically when cadence threshold met. |
| Auto-tagging by threshold (`--snapshot-threshold`) | Evaluate diff size from last tagged snapshot at CLI mutation points. Tag when entry-change count exceeds threshold. |
| Periodic consistency audit | Hourly full-scan comparing maintained refgraph against derived refgraph. Log and fix discrepancies. Currently only startup rebuild exists. |
| `c4 config store.limit` CLI command | Config field exists in c4d. No CLI command to get/set it. |
| `c4 status` (observability) | Show node health, storage summary, per-c4m reachability breakdown. |

## Future Scope (Explicitly Deferred)

| Feature | Notes |
|---------|-------|
| Shred / tombstones | Forced deletion with `410 Gone` response and cross-node propagation. Enterprise/compliance scope (GDPR, legal, departing employees). |
| Legal hold / `--auth-required` | Access-controlled un-establishment. Enterprise scope. Uses existing path model. |
| Diff-based snapshot storage | Store snapshots as diffs from previous snapshot instead of full c4m copies. Pruning becomes diff-collapse. Future optimization. |
| Cross-node shared archive c4m | Shared c4m established on multiple nodes, synced via relay. Requires relay sync (mesh phase). |

## Testing Gaps

Retention needs integration testing and benchmarking before it is considered production-ready:

- **Leak testing:** Verify no unbounded storage consumption under sustained load.
- **Fuzzing:** Random namespace PUT/DELETE sequences, verify refgraph consistency.
- **Simulated load:** Many c4m files with overlapping blob references, verify correct purgatory behavior.
- **Race conditions:** Concurrent namespace mutations with refgraph updates.
- **c4m linking correctness:** Nested c4m references traced correctly through forward index.
- **Crash recovery:** Kill c4d mid-operation, verify consistency audit repairs state.
- **Pressure curve:** Verify reclamation actually frees space and stays within limits.
