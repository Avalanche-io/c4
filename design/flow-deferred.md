# Flow Links: Deferred Work

**Status**: Parked — revisit after v1 flow ships with production usage
**Date**: 2026-03-08

Work identified during design and audit that is intentionally excluded
from the v1 implementation. Each item has a rationale for deferral and
a trigger condition for when it should be revisited.

---

## 1. Bidirectional Reconciliation

**What**: `<>` operator fulfillment — three-way merge with ancestor
tracking, conflict detection, conflict resolution policies
(last-writer-wins, flag-conflict, manual resolution).

**Why deferred**: Highest-risk phase. Requires ancestor tracking,
dual-watch goroutines (local Subscribe + remote long-poll), three-way
c4m merge, and a conflict data model. The CAP theorem guarantees that
bidirectional sync across partitions will produce conflicts. Building
the conflict resolution machinery before anyone has used unidirectional
flow in production is premature.

**What ships instead**: The `<>` operator parses and round-trips in
c4m files. c4d recognizes bidirectional channels in the registry. But
the reconciliation engine does not fulfill them — they appear as
"direction not yet supported" in channel status. Users who need
bidirectional can achieve it with paired outbound+inbound channels on
non-overlapping subtrees.

**Revisit when**: Users report needing true bidirectional sync on
overlapping paths, OR operational experience with outbound+inbound
reveals patterns that inform the conflict resolution design.

**Design docs**: `flow-c4d-design.md` §F (bidirectional section),
`flow-paper-v4.md` (CAP analysis, DPI bounds).

---

## 2. Visibility & Monitoring Platform

**What**: Staleness estimation (Σ(t) = H(S_t | D_t)), source change
rate tracking, mesh topology API (`/etc/mesh/topology`,
`/etc/mesh/chains`), chain detection with DPI-implied bounds,
Prometheus metrics endpoint (`/metrics`), Graphviz export
(`c4 mesh dot`), recommendations engine ("direct link would reduce
staleness by 60%").

**Why deferred**: Observability for a system with zero production
users. The information-theoretic metrics (staleness estimation,
chain bounds) are academically sound but operationally premature.
No user will interpret Σ=2.4 vs Σ=8.7 without experience using the
system first. Building a monitoring platform before the thing being
monitored exists in production violates the minimalist principle.

**What ships instead**: Basic channel status in the channel API:
synced/pending/unbound, last sync timestamp, pending operation count.
Enough to answer "is my flow working?" without a monitoring stack.

**Revisit when**: Operators report they cannot diagnose flow problems
with basic channel status, OR deployment scales beyond single-digit
nodes where topology visualization becomes necessary.

**Design doc**: `flow-visibility-design.md` (full design ready to
implement when the time comes).

---

## 3. `mk` to `init` Rename

**What**: Rename `c4 mk` to `c4 init`. Remove `--sync` flag (replaced
by `c4 ln <>`). Remove location registration from `mk` (replaced by
auto-resolution via c4d).

**Why deferred**: Separable breaking change. The rename is a good idea
but should not be smuggled in as a side effect of flow support. It
deserves its own discussion, its own deprecation period, and its own
release note. Bundling it with flow links conflates two concerns.

**What ships instead**: `c4 mk` continues to work as-is. The `--sync`
flag remains but is unnecessary — users can use `c4 ln <>` instead.
Location registration via `c4 mk studio: host:port` remains available
as a manual fallback alongside c4d auto-resolution.

**Revisit when**: Flow links are stable and the reduced scope of `mk`
makes the rename obviously right.

---

## 4. Advanced Flow Configuration

**What**: Per-location flow overrides (check_interval, conflict_strategy,
max_concurrent_transfers, max_bytes_per_hour, delivery mode),
allowed_locations allowlist, per-location direction restrictions,
auto-approval for trusted paths.

**Why deferred**: Configuration surface area that nobody needs yet.
The c4 philosophy says "defaults over configuration." v1 should have
sensible defaults. If someone needs rate limiting, they can configure
it at the network level. If someone needs direction restrictions, they
can reject channels manually.

**What ships instead**: `locations:` config section maps names to
addresses. That's it. No flow-specific config knobs. Sensible defaults:
30s check interval, explicit approval required for all channels.

**Revisit when**: Users report needing per-location policy that cannot
be achieved through the approval workflow or network-level controls.

---

## 5. Information-Theoretic Operations

**What**: Staleness metric (SourceChangeRate * TimeSinceLastSync as
proxy for Σ(t)), state-sufficient vs history-dependent delivery mode,
DPI chain bound calculations, reconciliation priority ordering by
estimated information loss.

**Why deferred**: Research concepts from the academic paper. Beautiful
theory, but no user will configure `delivery: "history"` vs
`delivery: "state"` in v1. The distinction between state-sufficient
and history-dependent content is real but can be addressed later when
snapshot chain flow becomes a concrete use case.

**What ships instead**: Reconciliation runs all channels with equal
priority. Content delivery is always state-sufficient (push current
c4m, not history). This is correct for the filesystem-state use case
that covers all v1 scenarios.

**Design doc**: `flow-c4d-design.md` §K, `flow-paper-v4.md`
(information theory section).

---

## 6. mDNS Discovery

**What**: c4d nodes advertise `_c4d._tcp` with cert CN as instance
name. Location names auto-resolve by matching discovered peer CNs.
Zero-config LAN peer discovery.

**Why deferred**: Nice-to-have for LAN deployments but not required
for flow to work. Explicit `locations:` config is sufficient and more
predictable. mDNS adds a dependency (likely `github.com/hashicorp/mdns`
or similar) and platform-specific behavior (firewall rules, multicast
support).

**What ships instead**: Config-based location resolution only. Users
add `locations:` entries to their c4d config.

**Revisit when**: Users report that manual location configuration is
a significant friction point for LAN deployments.

---

## 7. CLI Commands: `c4 status`, `c4 mesh`, `c4 dig`

**What**: New CLI verbs for flow observability. `c4 status` shows node
overview with channel summary. `c4 mesh` shows topology graph.
`c4 dig` traces content provenance through flow chains.

**Why deferred**: These consume the visibility APIs (item 2 above).
Without the monitoring platform, there is nothing for these commands
to display beyond what `c4 ls` and the channel API already provide.

**What ships instead**: `c4 ls :` shows flow entries inline.
Direct channel API queries via curl or future `c4d channel list`.

**Revisit when**: Visibility platform (item 2) ships.

---

## 8. Auto-Approval for Trusted Paths

**What**: c4d configuration to auto-approve flow declarations from
specific namespace paths or targeting specific locations, without
manual admin approval.

**Why deferred**: Re-introduces implicit behavior. The explicit
approval model is safer for v1. Auto-approval is a convenience that
should be added after users have experience with the approval workflow
and understand its security implications.

**What ships instead**: All flow declarations require explicit
approval via `POST /etc/channels/{id}/approve`.

**Revisit when**: Users report that manual approval is burdensome
for trusted internal workflows.
