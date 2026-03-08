# The Missing Link: Flow as a Filesystem Primitive for Partition-Tolerant Environments

**Joshua Kolden**
Avalanche, Inc. / Cinema Content Creation Cloud (C4) Project

---

## Abstract

Filesystems provide two link primitives: hard links for intra-volume naming with immediate consistency, and symbolic links for intra-network naming with synchronous consistency. Neither addresses the partition-normal regime — systems separated by organizational firewalls, air gaps, intermittent connectivity, and geographic distance — that dominates modern computing. We show that this gap is not an engineering oversight but a structural consequence of the CAP theorem: three partition regimes require three consistency models, which require three link primitives. Two exist. One is missing. We define the *flow link*, a third primitive that binds a local path to a remote path across partition boundaries with eventual consistency. Using sheaf-theoretic analysis, we prove the correspondence is structurally forced: the three consistency models arise as the unique sheaf conditions at each scope, and no alternative model exists. Information-theoretic bounds from the Data Processing Inequality constrain flow link chain behavior. Flow links compose with Unix permission bits to produce useful emergent behaviors — including giving practical semantics to previously degenerate permission modes — without special-case logic, evidence that the primitive occupies the correct level of abstraction.

---

## 1. Introduction

Three partition regimes exist in computing. Within a single volume, partitions are impossible. Across a reachable network, partitions are exceptional. Across organizational, geographic, and connectivity boundaries, partitions are normal. The CAP theorem [11] requires a different consistency model for each regime. Filesystems have a link primitive for the first two — hard links (immediate consistency) and symbolic links (synchronous consistency) — but not for the third.

This paper identifies the missing primitive. The argument is not that a third link type would be useful, but that it is *required*. The CAP theorem dictates that a primitive operating in the partition-normal regime must sacrifice consistency for availability — it cannot provide the synchronous semantics of symbolic links, because those semantics require failing on partition, and in the partition-normal regime, that means failing continuously. The sheaf-theoretic analysis of Section 5 proves the consistency model is unique: the separated presheaf condition is the strongest achievable for the coarsest covering topology, and no alternative exists between eventual and synchronous consistency. This is not a design proposal. It is the identification of a primitive that the theory demands.

We call this primitive a *flow link*. A flow link declares that a local path is related to a remote path, in a specified direction, with eventual consistency semantics. It does not require the remote path to be reachable. Like a symbolic link, it is a declaration of relationship, not a mechanism of fulfillment — just as NFS and the VFS layer make symlinks work across machines, flow links rely on infrastructure daemons to propagate content. The flow link itself is the declaration.

Flow links are not aliasing primitives. Hard links and symbolic links are both forms of aliasing — they give two names to the same resource. Flow links are *communication* primitives. They declare that two distinct resources, on two distinct systems, should converge over time through an asynchronous protocol. This shift from aliasing to communication is not a design choice — it is forced by the partition regime. Aliasing requires shared access to a common referent. When access is partitioned, aliasing is impossible. Communication — message passing — is the only mechanism that works across partitions. This is the same shift that distributed systems theory predicts for any system moving from shared memory to message passing, and it explains why flow links naturally have properties that would be anomalous in an aliasing primitive: directionality (channels have direction; aliases do not), asynchrony (channels operate over time; aliases resolve at a point in time), and policy composition (channel endpoints have access control; aliases inherit it from the referent).

The historical pattern confirms the structural argument. Hard links were introduced in the early 1970s when computing happened on a single machine [4]. Symbolic links were introduced in 4.2BSD in 1983 [1] when machines gained multiple volumes and network mounts. Each extended the filesystem's naming reach to match the scope of contemporary computing. The third scope — partition-normal operation — arrived decades ago. The corresponding primitive has not.

The remainder of this paper is organized as follows. Section 2 characterizes the gap at partition boundaries. Section 3 defines flow links. Section 4 establishes the formal CAP correspondence and proves it is structurally forced. Section 5 establishes information-theoretic bounds. Section 6 examines permission composition. Section 7 surveys related work. Section 8 discusses implications. Section 9 concludes.

---

## 2. The Gap

### 2.1 Partition Boundaries

The visible network — the set of filesystems a kernel can mount and resolve pathnames through — covers a shrinking fraction of the systems with which any given machine must exchange data. Organizational boundaries partition networks between collaborating companies. Air gaps intentionally prohibit connectivity in security-sensitive environments. Intermittent connectivity characterizes mobile devices, field equipment, remote offices, and spacecraft. Geographic distance makes synchronous protocols impractical — Abadi's PACELC framework [6] formalizes the latency-consistency tradeoff that renders cross-continent NFS mounts operationally unusable.

In all of these scenarios, the filesystem cannot express the relationship between a local path and a path on the unreachable system. A symbolic link to an NFS path on a disconnected server is not degraded — it is broken. The symlink model provides exactly two states: resolvable or error. There is no third state corresponding to "related but not currently synchronized."

### 2.2 Current Practice

In the absence of a filesystem primitive, cross-partition data relationships are managed externally: synchronization tools (rsync [3], Unison [7], Syncthing) with configuration invisible to the filesystem; replication configurations (S3, database replication) in the infrastructure layer; scripts and cron jobs encoding flow relationships as procedural code; documentation and institutional knowledge. The filesystem itself contains no indication these relationships exist.

This is the same situation that existed before symbolic links: cross-boundary references exist, are important, and are managed entirely outside the filesystem's knowledge.

### 2.3 The Cost

The absence imposes four concrete costs. *Opacity*: a directory listing reveals nothing about cross-partition relationships. *Fragility*: when a synchronization relationship breaks, the filesystem is unaware. *Non-composability*: filesystem permissions cannot interact with flow relationships the filesystem does not know about — a write-only directory with an outbound flow could serve as an ingest point, but this composition is inexpressible when one component lives in the filesystem and the other in external configuration. *Non-portability*: moving a synchronized directory to a new system requires reconfiguring external tools; the filesystem preserves contents and permissions but not synchronization relationships.

---

## 3. Flow Links

### 3.1 Definition

A *flow link* is a filesystem primitive that binds a local path to a remote path across a partition boundary. It is an ordered tuple (*local_path*, *remote_path*, *direction*), where *direction* is one of {outbound, inbound, bidirectional}. The flow link is stored as filesystem metadata associated with *local_path*, analogous to how a symbolic link stores its target path.

### 3.2 Directional Forms

**Outbound.** Content at the local path propagates to the remote path. Notation: `local -> remote`.

**Inbound.** Content at the remote path propagates to the local path. Notation: `local <- remote`.

**Bidirectional.** Content propagates in both directions, converging toward a common state. Notation: `local <-> remote`.

Direction describes the *logical* flow of content, not the transport mechanism. An outbound flow link may be fulfilled by having the remote pull — the direction is semantic, not mechanical. This directionality is natural for a communication primitive: channels have direction; aliases do not.

### 3.3 Consistency Model

A flow link provides *eventual consistency*: if no new updates are made to the source, the destination will converge to the same state after some finite but unbounded time. This is the standard definition as formalized by Vogels [8] and taxonomized by Viotti and Vukolić [9].

This is the strongest achievable consistency for the partition-normal regime. Immediate consistency (the hard link model) is impossible across separate systems. Synchronous consistency (the symbolic link model) requires both endpoints to be reachable, which contradicts the operating assumptions. Eventual consistency is achievable as long as partitions eventually heal.

More precisely, a flow link satisfies *separation* (when endpoints have converged, the converged state is unique) but not *gluing* (convergence is not guaranteed at any particular time). This decomposition is made rigorous in Section 4.

A flow link also provides *session guarantees* [10] at a single endpoint: monotonic reads and read-your-writes hold locally by construction. They do not hold across endpoints during a partition.

### 3.4 Asynchronous Declaration

A flow link does not require the remote path to be reachable at declaration time. A symbolic link whose target is unreachable is "broken." A flow link whose remote is unreachable is *pending* — fully valid, awaiting connectivity. Requiring reachability at declaration would make the primitive unusable in the scenarios it addresses.

### 3.5 Infrastructure Independence

A flow link declares a relationship without specifying how it is fulfilled. The propagation infrastructure — HTTP, rsync, a shuttle drive, a human carrying media — is external to the declaration. This parallels symbolic links: a symlink to `/mnt/remote/data` does not specify NFSv4 over TCP on port 2049. The link declares the relationship; infrastructure fulfills it.

### 3.6 Composition of Flow Links

Flow links compose into chains. If A has an outbound flow link to B, and B has an outbound flow link to C, content at A propagates through B to C. Same-direction chains compose cleanly; mixed-direction chains require the intermediate node to act as a relay. The behavior of chains is subject to a fundamental bound established in Section 5.

---

## 4. CAP Theorem Correspondence

### 4.1 Mapping Link Types to CAP Regimes

The three filesystem link types correspond to the three regimes of the CAP tradeoff space [11, 12]:

| Link Type | Partition Regime | CAP Position | Behavior During Partition |
|-----------|-----------------|--------------|--------------------------|
| Hard link | Impossible | C + A (trivially) | N/A |
| Symbolic link | Exceptional | CP (consistency over availability) | Returns error |
| Flow link | Normal | AP (availability over consistency) | Operates with local state |

Hard links operate within a single volume. Partitions are structurally impossible. Both consistency and availability are guaranteed without tradeoff. Symbolic links operate across mount points and network filesystems. When a partition occurs, the link fails — consistency over availability. This is appropriate when partitions are rare: failure is a clear signal. Flow links operate across boundaries where partitions are the common case. A link that fails on every partition is a link that almost always fails. The AP choice — availability over consistency — is the only viable option.

Fox and Brewer's harvest-yield framing [13] is more precise than binary CAP for flow links: during partitions, a flow-linked path has reduced harvest (incomplete or stale data) but high yield (always accessible).

### 4.2 Structural Necessity

This correspondence is not a post hoc rationalization. It is a structural argument that a third link type is *required*.

1. Filesystems must express relationships between paths.
2. Paths may reside on systems in three distinct partition regimes.
3. The CAP theorem requires a different consistency/availability tradeoff for each regime.
4. Therefore, a filesystem that expresses relationships at all three scales needs three link primitives.

The theorem does not merely *permit* a third link type. It dictates its properties: eventual consistency and partition tolerance. Any primitive for the partition-normal regime that claims stronger consistency is either restricting its scope (not truly partition-tolerant) or violating the theorem.

### 4.3 The Scale Ladder

The three link types form a scale ladder:

```
                    Scope
                      |
    Flow link --------|  Any (partition-normal)
                      |  Eventual consistency
                      |
    Symbolic link ----|  Network (partition-exceptional)
                      |  Synchronous consistency
                      |
    Hard link --------|  Volume (partition-free)
                      |  Immediate consistency
                      |
                      +----> Consistency strength
```

Scope increases as consistency weakens. This is not a design tradeoff — it is a fundamental property of distributed systems. Broader scope means more partition boundaries, which requires weaker consistency.

### 4.4 Sheaf-Theoretic Proof

The three scopes define three *covering structures* on filesystem sites — specifications of which local observations suffice to determine global state. At volume scope, every observation immediately reveals current state (finest covering). At network scope, observations reveal current state only if the network path is open (coarser). At partition-normal scope, observations reveal only locally cached state (coarsest).

These correspond to three Grothendieck topologies on the category of filesystem sites [14]. The use of sheaf conditions to characterize distributed consistency is established: Goguen [15] showed the gluing axiom captures consistency between concurrent objects; Robinson [16] demonstrated sheaves as the canonical structure for sensor integration; Inoue [17] proposed Grothendieck topologies as foundations for information networks; Felber et al. [18] applied cellular sheaves to distributed task solvability.

Our contribution is the specific instantiation: three named topologies (*J_vol*, *J_net*, *J_flow*) corresponding to three link types, with the proof that consistency degradation along the scale ladder is functorial. Building on the methodology of Rieser [19] for constructing Grothendieck topologies on non-standard categories:

Denote *J_vol* (finest), *J_net* (intermediate), *J_flow* (coarsest), with *J_vol >= J_net >= J_flow*. Hard link data satisfies the sheaf condition over *J_vol*: any two observations of the same inode agree immediately. Symbolic link data satisfies the sheaf condition over *J_net*: successful resolution returns current state, and failures are faithfully reported. Flow link data satisfies the weaker *separated presheaf* condition over *J_flow*: when endpoints have converged, the converged state is unique (separation), but convergence is not guaranteed at any given time (gluing holds only in the limit).

Two results strengthen the scale ladder from pattern to proof:

**Uniqueness.** No fourth consistency model exists between eventual and synchronous for the partition-normal regime. The separated presheaf condition is the strongest achievable for the coarsest covering topology. Any stronger condition would require gluing — guaranteeing convergence at a specific time — which partition-normal operation cannot provide.

**Sheafification cost.** Moving from flow link semantics to symbolic link semantics is the operation of *sheafification*: forcing the separated presheaf to satisfy the full sheaf condition by discarding sections that cannot be glued. In filesystem terms: making unreachable data inaccessible rather than serving stale copies. This is exactly what a symbolic link does when it returns an error. The information loss is not an engineering limitation — it is a mathematical consequence of the sheafification functor.

The consistency degradation along the scale ladder is therefore *functorial*. The chain of topologies forces a corresponding chain of consistency models, and no engineering can produce a model stronger than what the covering structure at each scope permits.

---

## 5. Information-Theoretic Bounds

### 5.1 Information Staleness

"Eventual consistency" says nothing about the *degree* of divergence at any moment. Two flow links with identical clock-time since last sync can have radically different information staleness depending on how fast their sources change.

Let *S_t* denote source state at time *t* and *D_t* destination state. Define the *information staleness* of a flow link as:

> Σ(t) = H(S_t | D_t)

the conditional entropy of the source given the destination — the information about the source that the destination lacks. This is the complement of mutual information: Σ(t) = H(S_t) - I(S_t; D_t). The metric is standard in the Age of Information / Value of Information literature [20, 21]; our contribution is its application to filesystem flow links.

When fully synchronized, Σ(t) = 0. During a partition, staleness grows at the source's entropy rate *h(S)*: a partition of duration *δ* accumulates staleness approximately *h(S) · δ*. This provides a principled signal for synchronization priority: after a partition heals, prioritize the flow link with the highest information staleness, not merely the longest elapsed time.

### 5.2 The Chain Bound

The Data Processing Inequality [22] states that for any Markov chain *X -> Y -> Z*:

> I(X; Z) <= I(X; Y)

Processing data can only destroy information, never create it. This is a theorem, not a design guideline. Applied to a flow link chain A -> B -> C:

> I(A_t; C_t) <= I(A_t; B_t)

Node C's knowledge of A's current state can never exceed B's knowledge. This is a formal bound that constrains *all* flow architectures, not just this proposal. Any system that moves data across partition boundaries through intermediaries is subject to this bound whether it knows it or not. Flow links make the bound visible and manageable.

The practical implications are three. First, chains have a natural depth limit: each hop degrades knowledge of the source, and beyond some depth the tail knows effectively nothing about the head's current state. Second, the depth limit depends on source dynamics — a slowly changing archive tolerates longer chains than a rapidly changing working directory. Third, direct links are provably superior: a direct flow link from A to C always provides at least as much mutual information as a chain A -> B -> C. When a direct connection is possible, even intermittently, it is informationally preferable to relaying.

Content addressing is orthogonal to the DPI bound. It eliminates distortion (every received item is correct) but cannot make C aware of content A has produced that B has not yet received. A content-addressed chain delivers *accurate old data*, not *current data*.

### 5.3 State-Sufficient vs. History-Dependent Flow

During a partition, the source transitions through states *s_0, s_1, ..., s_k*. The destination sees only *s_0* and, after reconnection, *s_k*. Whether this erasure of intermediate states matters depends on content semantics.

**State-sufficient flow:** convergence requires only the current source state. Last-writer-wins on individual files is state-sufficient — the final state of each file is sufficient regardless of intermediate edits.

**History-dependent flow:** convergence requires the complete sequence of transitions. Append-only logs, audit trails, and event streams are history-dependent — the destination has the correct endpoint but has lost the individual events.

This distinction is not captured by CAP, which says only that consistency degrades during partitions. Information theory specifies *what is lost* and *under which semantics the loss matters*. A content-addressed filesystem propagating whole-file states is state-sufficient. The same filesystem propagating a directory change log is history-dependent. Flow link infrastructure must either ensure gap-free delivery for history-dependent content or explicitly acknowledge that partition erasure is data loss within those semantics.

---

## 6. Permission Composition

### 6.1 Flow Composition with Permissions

Flow links interact naturally with existing permission models because they are orthogonal. Permissions govern *local access*. Flow direction governs *propagation*. Their composition creates semantically rich behaviors without special-case logic:

**Write-only + outbound flow = drop slot.** A directory with write-only permissions (`-wx`) and an outbound flow link functions as an ingest point. Users can add files but cannot list or read them; outbound flow drains content to the remote destination. The write-only permission — degenerate for fifty years — gains practical meaning.

**Read-only + inbound flow = subscription.** A directory with read-only permissions (`r-x`) and an inbound flow link functions as a read cache. Data arrives from a remote source; local users can read but not modify it. This is the common pattern of a local mirror, currently implemented through rsync cron jobs and read-only NFS mounts, expressed as a filesystem primitive.

**Read-write + bidirectional flow = collaborative directory.** Full permissions with bidirectional flow produce two-way synchronization — the common file-sync model expressed as a primitive rather than as external tool configuration.

### 6.2 Emergent Semantics

| Permissions | Flow Direction | Emergent Behavior |
|-------------|---------------|-------------------|
| `rwx` | Outbound | Publishing point |
| `rwx` | Inbound | Staging area |
| `rwx` | Bidirectional | Collaborative sync |
| `r-x` | Inbound | Read cache / subscription |
| `r-x` | Outbound | Archive export |
| `-wx` | Outbound | Drop slot / ingest |
| `-wx` | Inbound | Blind relay |

### 6.3 Evidence of Correctness

When a new primitive composes naturally with existing primitives to produce useful emergent behaviors — without special-case logic — that is strong evidence it occupies the correct level of abstraction. A primitive that requires special-case logic to interact with existing filesystem semantics is at the wrong level. A primitive that composes orthogonally, producing a table of useful behaviors including solutions to long-standing practical problems (the uselessness of write-only directories), is at the right level.

The write-only directory has been degenerate for fifty years. Flow links make it useful. That is not a coincidence — it is what happens when a genuine gap in the primitive vocabulary is filled.

---

## 7. Related Work

The individual building blocks of flow links — disconnected operation, optimistic replication, namespace composition, conflict-free convergence — are extensively studied. What was not known is that they converge on a single missing primitive.

### 7.1 Disconnected Operation

**Coda** [23, 24] was the first filesystem to treat disconnected operation as first-class, extending AFS with client-side caching and optimistic replication. Its hoarding/emulation/reintegration model demonstrated that eventual consistency works at the filesystem level. However, Coda treats replication as a system-level property — all files in a volume replicate per the volume's configuration. There is no per-path declaration of intent or direction, and replication relationships are invisible to directory listings.

**Bayou** [25] was designed from the ground up for partition-normal environments, with application-specific conflict detection and resolution that directly informs the flow link decision that conflict resolution is external policy (Section 8.4). But Bayou is an application-level storage system, not a filesystem primitive.

**LOCUS** [26] treated network partition and merge as first-class operating conditions years before Coda, providing early evidence that partition-tolerant filesystems are a long-standing research goal.

All three operate at the system level with transparent replication, not per-path filesystem declarations.

### 7.2 Optimistic Replication and CRDTs

Saito and Shapiro's survey [27] provides a comprehensive taxonomy of optimistic replication but frames it exclusively as a system-level or application-level concern. The concept of a per-path, user-visible replication declaration does not appear — not as an oversight, but reflecting the field's framing of replication as infrastructure.

CRDTs [29, 30] provide conflict-free convergence guarantees directly applicable to bidirectional flow links. When content is modeled as a CRDT (e.g., a set of content-addressed hashes with set-union merge), bidirectional flow converges without conflicts. Under CRDT merge semantics, push and pull on a bidirectional flow link form a Galois connection on information-ordered endpoint states, providing a categorical justification for CRDTs as the natural consistency model for bidirectional flow.

### 7.3 Namespace Composition and Production Systems

**Plan 9** [31] is the closest prior art for flow links as a namespace primitive — its per-process `bind` and `mount` operations compose namespace from multiple sources. But Plan 9's operations are synchronous via the 9P protocol [32] and fail on unreachability. It remains in the CP quadrant.

**Windows DFS + DFSR** is the closest production system: namespace entries backed by eventual replication. But DFS requires Active Directory, operates per-folder (not per-path), lacks explicit direction, and is platform-specific.

**Cloud placeholder files** (OneDrive, iCloud) are implicit inbound flow links at the filesystem level, but are system-managed (not user-declared), unidirectional, fetch-on-demand (synchronous rather than eventually consistent), and do not compose with permissions.

**Unison** [7] is closest in practical behavior — per-path bidirectional sync with conflict detection. But it is a tool, not a primitive: its relationships are invisible to `ls`, non-portable, and inoperable by other tools.

**Reflinks** [35, 36] demonstrate that the link taxonomy is not closed — filesystems have already accepted new link types. But reflinks extend semantics (COW) within a volume, not scope across partitions.

### 7.4 Content-Addressed Storage

Content addressing (Venti [37], Git [38], IPFS [39]) transforms flow link properties from best-effort to structurally guaranteed: idempotent propagation, resumable transfer, corruption detection, and source independence. Under content-addressed semantics, a flow link operates not as a mutable buffer but as a set of independent linear channels carrying immutable data. The robustness is a consequence of this structural change, not a bolted-on property.

### 7.5 Summary of Differentiation

| System | Overlap | What Flow Links Add |
|--------|---------|---------------------|
| Coda [23, 24] | Disconnected operation, optimistic replication | Per-path granularity, explicit direction, permission composability |
| Bayou [25] | Designed for partition-normal operation | Filesystem primitive, per-path declaration |
| Plan 9 [31] | Namespace composition as primitive | Partition tolerance, eventual consistency |
| DFS + DFSR | Namespace links + eventual replication | Per-path, directional, lightweight, cross-platform |
| Cloud placeholders | Implicit inbound flow at filesystem level | User-declared, directional, composable, explicit consistency |
| Unison [7] | Per-path bidirectional sync | Filesystem primitive, inspectable, portable |
| CRDTs [29] | Conflict-free convergence | Filesystem-level application |

---

## 8. Discussion

### 8.1 Kernel Architecture

Flow link *declaration* belongs in the kernel — a new directory entry type (analogous to `DT_LNK`), system calls for creation and query (analogous to `symlink()`, `readlink()`), and filesystem format support for storing flow metadata. Flow link *fulfillment* — actually propagating content — cannot be a kernel operation. The kernel has no business implementing HTTP clients or conflict resolution.

The practical architecture separates declaration from fulfillment: the kernel stores the declaration and notifies userspace daemons of changes; daemons propagate content according to policy. This mirrors early network filesystem architecture — the kernel resolved symbolic links to NFS paths while the NFS client handled network I/O — with the critical addition that the synchronization relationship is declared in the filesystem itself.

Before kernel support, flow links can be expressed in filesystem description files and enforced by userspace daemons — historically precedented, as symlinks existed in some filesystems before the kernel had a unified VFS abstraction.

### 8.2 Content Addressing as Enabler

Content addressing deserves particular attention as a substrate for flow links. It guarantees convergence (reduced to set reconciliation [40]), makes conflict detection trivial (different content = different hashes), eliminates partial-transfer corruption (every received item is complete and verified), and decouples fulfillment from any specific transport or topology. Content-addressed filesystems are to flow links what hierarchical directories are to symbolic links: the natural substrate.

### 8.3 Conflict Resolution

Bidirectional flow links admit conflicts: concurrent modifications at both endpoints during a partition. The CAP theorem guarantees this possibility cannot be eliminated. Conflict resolution is not a property of the flow link primitive — it is a policy of the fulfillment infrastructure, following Bayou's insight [25] that conflict semantics must be application-specific. The primitive provides metadata for detection (content hashes, timestamps, provenance); strategy is configured externally. This separation parallels symbolic links: the link declares a target; the VFS, NFS, and permissions system determine access.

### 8.4 Security Considerations

Flow links introduce data exfiltration risk (malicious outbound flows) and data injection risk (unauthorized inbound flows). The mitigations are standard: flow link creation governed by appropriate permissions, mandatory access control policies in sensitive environments, authenticated remote sources for inbound flow. These concerns are not unique — every cross-system data movement mechanism faces them. The advantage of a filesystem primitive is that the risks become visible and governable through standard access control, rather than hidden in tool configurations.

### 8.5 Type-Theoretic Connections

In substructural type theory, flow links correspond to *eventual contraction* — a structural rule between linear types (no contraction) and full contraction (immediate duplication) that does not appear to have been previously identified. Session types [41] provide another connection: outbound links are send-only channels, inbound are receive-only, and bidirectional links are sessions requiring both operations with conflict handling as protocol.

---

## 9. Conclusion

The filesystem link taxonomy is incomplete. Three partition regimes exist. Three consistency models are required. Two link types exist. One is missing.

The CAP theorem dictates the properties the missing primitive must have: eventual consistency and partition tolerance, with availability maintained during partitions. The sheaf-theoretic analysis proves the consistency model is unique — the separated presheaf condition is the strongest achievable for the coarsest covering topology, and sheafification (demanding stronger consistency) necessarily discards unreachable data. The DPI chain bound proves that staleness accumulates monotonically along flow link chains — a theorem that constrains any multi-hop flow architecture whether it uses flow links or not. These results were known independently in their respective fields. What was not known is that they converge on a single missing filesystem primitive.

Flow links fill that gap. They are directional, asynchronous, and policy-composable. They compose with Unix permission bits to produce useful behaviors — including giving practical semantics to permission modes that have been degenerate for decades — without special-case logic. They make cross-partition relationships visible, inspectable, portable, and composable with other filesystem primitives: the same qualities that justified the introduction of symbolic links forty years ago, applied to the scope boundary that dominates modern computing.

---

## References

[1] S. J. Leffler, M. K. McKusick, M. J. Karels, and J. S. Quarterman, *The Design and Implementation of the 4.3BSD UNIX Operating System*. Addison-Wesley, 1989.

[2] R. Sandberg, D. Goldberg, S. Kleiman, D. Walsh, and B. Lyon, "Design and implementation of the Sun Network Filesystem," in *Proceedings of the USENIX Summer Technical Conference*, 1985.

[3] A. Tridgell and P. Mackerras, "The rsync algorithm," Technical Report TR-CS-96-05, Australian National University, 1996.

[4] D. M. Ritchie and K. Thompson, "The UNIX time-sharing system," *Communications of the ACM*, vol. 17, no. 7, pp. 365-375, 1974.

[5] M. K. McKusick, W. N. Joy, S. J. Leffler, and R. S. Fabry, "A fast file system for UNIX," *ACM Transactions on Computer Systems*, vol. 2, no. 3, pp. 181-197, 1984.

[6] D. Abadi, "Consistency tradeoffs in modern distributed database system design," *IEEE Computer*, vol. 45, no. 2, pp. 37-42, 2012.

[7] B. C. Pierce and J. Vouillon, "What's in Unison? A formal specification and reference implementation of a file synchronizer," Tech. Rep. MS-CIS-03-36, University of Pennsylvania, 2004.

[8] W. Vogels, "Eventually consistent," *Communications of the ACM*, vol. 52, no. 1, pp. 40-44, 2009.

[9] P. Viotti and M. Vukolić, "Consistency in non-transactional distributed storage systems," *ACM Computing Surveys*, vol. 49, no. 1, Article 19, 2016.

[10] D. B. Terry, A. J. Demers, K. Petersen, M. J. Spreitzer, M. M. Theimer, and B. B. Welch, "Session guarantees for weakly consistent replicated data," in *Proceedings of the International Conference on Parallel and Distributed Information Systems*, 1994.

[11] S. Gilbert and N. Lynch, "Brewer's conjecture and the feasibility of consistent, available, partition-tolerant web services," *ACM SIGACT News*, vol. 33, no. 2, pp. 51-59, 2002.

[12] E. A. Brewer, "Towards robust distributed systems (abstract)," in *Proceedings of the 19th Annual ACM Symposium on Principles of Distributed Computing (PODC)*, 2000.

[13] A. Fox and E. A. Brewer, "Harvest, yield, and scalable tolerant systems," in *Proceedings of the Seventh Workshop on Hot Topics in Operating Systems*, 1999.

[14] S. Mac Lane and I. Moerdijk, *Sheaves in Geometry and Logic: A First Introduction to Topos Theory*. Springer-Verlag, 1992.

[15] J. Goguen, "Sheaf semantics for concurrent interacting objects," *Mathematical Structures in Computer Science*, vol. 2, no. 2, pp. 159-191, 1992.

[16] M. Robinson, "Sheaves are the canonical data structure for sensor integration," *Information Fusion*, vol. 36, pp. 208-224, 2017.

[17] T. Inoue, "Grothendieck's geometric universes and a sheaf-theoretic foundation of information network," arXiv preprint arXiv:2602.17160, 2026.

[18] S. Felber, B. Hummes Flores, and H. Rincon-Galeana, "A sheaf-theoretic characterization of tasks in distributed systems," arXiv preprint arXiv:2503.02556, 2025.

[19] A. Rieser, "Grothendieck topologies and sheaf theory for data and graphs: An approach through Čech closure spaces," arXiv preprint arXiv:2109.13867, 2021.

[20] S. Kaul, R. Yates, and M. Gruteser, "Real-time status: How often should one update?" in *Proceedings of IEEE INFOCOM*, 2012.

[21] R. D. Yates, Y. Sun, D. R. Brown, S. K. Kaul, E. Modiano, and S. Ulukus, "Age of information: An introduction and survey," *IEEE Journal on Selected Areas in Communications*, vol. 39, no. 5, pp. 1183-1210, 2021.

[22] T. M. Cover and J. A. Thomas, *Elements of Information Theory*, 2nd ed. Wiley-Interscience, 2006.

[23] M. Satyanarayanan, J. J. Kistler, P. Kumar, M. E. Okasaki, E. H. Siegel, and D. C. Steere, "Coda: A highly available file system for a distributed workstation environment," *IEEE Transactions on Computers*, vol. 39, no. 4, pp. 447-459, 1990.

[24] J. J. Kistler and M. Satyanarayanan, "Disconnected operation in the Coda file system," *ACM Transactions on Computer Systems*, vol. 10, no. 1, pp. 3-25, 1992.

[25] D. B. Terry, M. M. Theimer, K. Petersen, A. J. Demers, M. J. Spreitzer, and C. H. Hauser, "Managing update conflicts in Bayou, a weakly connected replicated storage system," in *Proceedings of the 15th ACM Symposium on Operating Systems Principles (SOSP)*, 1995.

[26] B. Walker, G. Popek, R. English, C. Kline, and G. Thiel, "The LOCUS distributed operating system," in *Proceedings of the Ninth ACM Symposium on Operating Systems Principles*, 1983.

[27] Y. Saito and M. Shapiro, "Optimistic replication," *ACM Computing Surveys*, vol. 37, no. 1, pp. 42-81, 2005.

[28] D. S. Parker, G. J. Popek, G. Rudisin, A. Stoughton, B. J. Walker, E. Walton, J. M. Chow, D. Edwards, S. Kiser, and C. Kline, "Detection of mutual inconsistency in distributed systems," *IEEE Transactions on Software Engineering*, vol. SE-9, no. 3, pp. 240-247, 1983.

[29] M. Shapiro, N. Preguiça, C. Baquero, and M. Zawirski, "Conflict-free replicated data types," in *Proceedings of the 13th International Symposium on Stabilization, Safety, and Security of Distributed Systems (SSS)*, 2011.

[30] M. Shapiro, N. Preguiça, C. Baquero, and M. Zawirski, "A comprehensive study of convergent and commutative replicated data types," INRIA Technical Report RR-7506, 2011.

[31] R. Pike, D. Presotto, S. Dorward, B. Flandrena, K. Thompson, H. Trickey, and P. Winterbottom, "Plan 9 from Bell Labs," *Computing Systems*, vol. 8, no. 3, pp. 221-254, 1995.

[32] R. Pike, D. Presotto, K. Thompson, H. Trickey, and P. Winterbottom, "The use of name spaces in Plan 9," *Operating Systems Review*, vol. 27, no. 2, pp. 72-76, 1993.

[33] R. G. Guy, J. S. Heidemann, W. Mak, T. W. Page Jr., G. J. Popek, and D. Rothmeier, "Implementation of the Ficus replicated file system," in *Proceedings of the USENIX Summer Technical Conference*, 1990.

[34] R. G. Guy, P. L. Reiher, D. Ratner, M. Gunter, W. Ma, and G. J. Popek, "Rumor: Mobile data access through optimistic peer-to-peer replication," in *Proceedings of the ER Workshop on Mobile Data Access*, 1998.

[35] O. Rodeh, J. Bacik, and C. Mason, "BTRFS: The Linux B-tree filesystem," *ACM Transactions on Storage*, vol. 9, no. 3, article 9, 2013.

[36] D. Chinner, C. Hellwig, and D. Howells, "Sharing and deduplication in XFS," in *Proceedings of the Linux Storage, Filesystem, and Memory-Management Summit*, 2016.

[37] S. Quinlan and S. Dorward, "Venti: A new approach to archival storage," in *Proceedings of the USENIX Conference on File and Storage Technologies (FAST)*, 2002.

[38] L. Torvalds, "Git: A distributed version control system," 2005. [Online]. Available: https://git-scm.com/

[39] J. Benet, "IPFS - Content addressed, versioned, P2P file system," arXiv preprint arXiv:1407.3561, 2014.

[40] Y. Minsky, A. Trachtenberg, and R. Zippel, "Set reconciliation with nearly optimal communication complexity," *IEEE Transactions on Information Theory*, vol. 49, no. 9, pp. 2213-2218, 2003.

[41] K. Honda, "Types for dyadic interaction," in *Proceedings of the 4th International Conference on Concurrency Theory (CONCUR)*, Lecture Notes in Computer Science, vol. 715, Springer, 1993, pp. 509-523.
