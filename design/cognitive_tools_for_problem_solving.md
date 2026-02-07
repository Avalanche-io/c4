# Cognitive Tools for Problem Solving

This document collects cognitive strategies, traps, and ideation techniques
that affect how we frame and solve problems. It is a companion to
[algorithmic_reframings.md](algorithmic_reframings.md), which catalogs specific
algorithmic patterns where changing the problem scope yields surprising
improvements.

That document answers "what can you do differently?" This one asks the prior
question: "why don't we see these options in the first place, and what habits
of thought make them visible?"

This is incomplete and will grow.

---

## Part I: Cognitive Traps

These are systematic ways that human cognition locks onto a problem frame and
resists alternatives. Knowing they exist doesn't make them go away, but it
does tell you where to look when you're stuck.

### 1. Functional Fixedness

**What it is:** Seeing an object, tool, or concept only in terms of its
conventional use. The classic demonstration is Duncker's candle problem (1945):
given a candle, a box of tacks, and matches, mount the candle on the wall. Most
people fail because they see the box as "a container for tacks" rather than "a
shelf." When the tacks are presented next to the box rather than inside it,
success rates jump — because the box is no longer performing its conventional
function.

**How it blocks reframing:** If you see rsync as "a file transfer tool," you
optimize file transfer. You don't ask whether transfer is the right frame. If
you see a manifest as "overhead before the real work," you try to minimize it
rather than recognizing it as a useful object in its own right. The conventional
category obscures the unconventional use.

**Counter-strategy:** Explicitly list the properties of each component
independent of its current role. A manifest has properties: it's small,
portable, diffable, content-addressed, machine-readable. These properties exist
regardless of whether you think of it as "transfer metadata" or "a filesystem
document." The properties suggest uses; the label suppresses them.

---

### 2. Einstellung Effect

**What it is:** A known solution method blocks perception of a better one. In
chess experiments (Luchins, 1942; Bilalic et al., 2008), players who see a
familiar 5-move solution literally do not see a 3-move solution — eye-tracking
shows they don't even look at the relevant squares. The known approach captures
attention so completely that the search for alternatives never begins.

**How it blocks reframing:** Once you know how to do something, you optimize
the known approach rather than questioning whether it's the right approach.
rsync works. It's well-understood. It handles the problem. The fact that it
re-negotiates from scratch every time is an optimization opportunity within the
rsync frame, not a signal that the frame is wrong. Einstellung keeps you in the
frame.

**Counter-strategy:** Before optimizing, explicitly ask: "What would a solution
look like if this tool/approach didn't exist?" Remove the known solution from
consideration and see what the problem looks like fresh. This is uncomfortable
because the known solution is right there, and it works. The discipline is in
setting it aside temporarily.

---

### 3. Anchoring

**What it is:** The first number, approach, or frame you encounter
disproportionately influences subsequent thinking. In estimation tasks, even
obviously irrelevant anchors (spinning a wheel of fortune) shift estimates
toward the anchor value (Tversky & Kahneman, 1974).

**How it blocks reframing:** The way a problem is first presented becomes the
anchor. "How do we make file transfer faster?" anchors on transfer speed. The
question already contains the frame — it presupposes that transfer is the
operation to optimize. The reframing "how do we minimize unnecessary transfer?"
is a different question that the anchor obscures.

**Counter-strategy:** Rephrase the problem multiple ways before solving it.
"How do we transfer files faster?" "How do we avoid transferring files?"
"What if both sides already have most of the data?" "What if we separated
knowing about files from having them?" Each rephrasing shifts the anchor and
opens different solution paths.

---

### 4. Problem Representation Effects

**What it is:** The encoding of a problem determines which operations feel
natural and which feel forced. Kaplan & Simon (1990) showed this with the
mutilated checkerboard problem: remove two opposite corners from a checkerboard;
can you tile the remainder with dominoes? Presented geometrically, people try
(and fail) to construct tilings. Presented with colors — each domino covers one
black and one white square, but two same-colored corners were removed — the
impossibility is immediate. Same problem, different encoding, different
difficulty.

**How it blocks reframing:** If filesystem sync is represented as "a set of
files to move," the natural operations are move, copy, diff-and-patch. If it's
represented as "two state descriptions to reconcile," the natural operations
are diff, merge, identify-missing. The problem is the same; the representation
makes different solution strategies cognitively accessible.

**Counter-strategy:** Deliberately re-represent the problem in a different
formalism. Draw it instead of describing it. Write it as math instead of prose.
Describe the end state instead of the process. Each representation makes
different aspects salient.

---

### 5. Availability Bias in Solution Search

**What it is:** Solutions that come to mind easily (because they're familiar,
recent, or dramatic) dominate the search at the expense of solutions that are
less available but potentially better. You reach for the tool you used last
week, not the tool that fits best.

**How it blocks reframing:** The algorithmic reframings in the companion
document are powerful precisely because they're not the first thing you think
of. Preprocessing, amortized analysis, communication complexity — these are not
the default cognitive tools. You reach for brute force, caching, or parallelism
because those are available. The less-available reframing might be better, but
it doesn't surface without deliberate effort.

**Counter-strategy:** Maintain a catalog of reframings (like the companion
document) and consult it deliberately when stuck. The catalog makes unfamiliar
strategies available. This is the cognitive equivalent of a reference book:
it compensates for the limits of what you happen to remember.

---

## Part II: Ideation Strategies

These are deliberate thinking moves that help escape the traps above.

### 6. Inversion

**What it is:** Instead of asking "how do I achieve X?" ask "what would make X
impossible?" or "what conditions would guarantee failure?" Then work to remove
those conditions.

**Example:** Instead of "how do we make file sync fast?" ask "what makes file
sync slow?" Answers: re-scanning unchanged files, re-transmitting unchanged
content, negotiating state that both sides already know. Each answer points to
a specific mechanism to eliminate (persistent state descriptions, content-
addressed deduplication, receiver-side diff). The solutions emerge from removing
obstacles rather than adding capabilities.

**Why it works:** Inversion shifts you from a generative search (infinitely many
ways to "make things fast") to an eliminative search (finite list of things that
make it slow). The eliminative search is more constrained and often more
productive.

**Read more:** Munger, "The Psychology of Human Misjudgment" (the "invert,
always invert" principle). Polya, "How to Solve It" (working backwards).

---

### 7. Representation Change

**What it is:** Redescribe the same problem in a different formalism — visual
instead of symbolic, algebraic instead of geometric, frequency domain instead
of time domain, graph instead of matrix.

**Examples:**
- Fourier analysis: convolution in the time domain is multiplication in the
  frequency domain. An O(n²) operation becomes O(n log n) via FFT — not
  because of a better algorithm in the original domain, but because the
  operation is simpler in the transformed domain.
- Graphs vs matrices: graph connectivity questions become eigenvalue questions.
  The Fiedler vector (second-smallest eigenvalue of the Laplacian) tells you
  the best bipartition — a combinatorial question answered algebraically.
- Geometry vs algebra: the dot product test for perpendicularity replaces
  geometric construction with arithmetic. What was hard to construct becomes
  easy to compute.

**Why it works:** Different representations make different operations cheap.
A problem that's hard in one representation may be trivial in another, not
because the problem changed but because the machinery available to you changed.

**Connection to algorithmic reframings:** Many entries in the companion document
are representation changes in disguise. Lifting/relaxation represents discrete
points as continuous vectors. Dimensionality reduction represents high-dimensional
data in low dimensions. Sketching represents large datasets as small summaries.
The algorithmic insight is often "find the right representation" rather than
"find the right algorithm."

---

### 8. Analogical Transfer

**What it is:** Recognize structural similarity between your problem and a
solved problem in a different domain, and adapt the known solution.

**Examples:**
- Epidemic models (SIR) applied to information propagation in networks
  (gossip protocols).
- Supply-chain optimization applied to data transfer scheduling (source
  selection, priority queuing, backpressure).
- Version control (git's content-addressed object store) as a model for
  filesystem state management (C4M's content-addressed manifests).

**Why it's hard:** Surface features differ between domains. A "graph cut" and
a "filesystem sync" don't look alike, but both involve partitioning a structure
into components that minimize cross-boundary traffic. Recognizing the
structural similarity requires abstracting past the surface.

**Why it works:** Many problem structures recur across domains. Optimization
under constraints, state reconciliation, caching/memoization, compression —
these show up everywhere. A solution that works in one domain often transfers
to another if you can identify the structural correspondence.

**Counter-trap:** Analogies can also mislead. A false structural
correspondence leads you to apply a solution from a domain where it works to
a domain where it doesn't. The discipline is in verifying that the relevant
structural properties actually hold, not just that the surface looks similar.

**Read more:** Gentner, "Structure-Mapping: A Theoretical Framework for
Analogy." Holyoak & Thagard, "Mental Leaps: Analogy in Creative Thought."

---

### 9. Decomposition and Unification

**What it is:** Two complementary moves:
- **Decompose:** Split a complex problem into independent subproblems that can
  be solved separately.
- **Unify:** Recognize that apparently separate problems are instances of the
  same underlying problem and solve them together.

Both are reframings. Decomposition changes the problem from "solve this complex
thing" to "solve these simpler things." Unification changes the problem from
"solve these many things" to "solve this one thing that covers all of them."

**When to decompose:** The problem has identifiable substructure with weak
coupling between parts. Each part can be solved independently without losing
important interactions. The subproblems are simpler than the whole.

**When to unify:** You're solving the same structural problem repeatedly in
different guises. The separate solutions have redundancy. A single abstraction
would serve all cases.

**The tension:** Decomposition is almost always the right first move.
Unification is often premature — it creates abstractions before the problem
space is understood. But when unification is right, it can collapse multiple
complex problems into one simple one. The skill is in timing: decompose early,
unify late, and only unify when the structural correspondence is genuine rather
than superficial.

**In C4:** The manifest format unifies several apparently separate problems
(filesystem description, change detection, integrity verification, transfer
coordination) into a single abstraction. Whether this unification is justified
is exactly the kind of question the design analysis plan (DESIGN_ANALYSIS_PLAN.md)
is meant to answer.

---

### 10. Constraint Relaxation and Tightening

**What it is:** Deliberately add or remove constraints to explore the solution
space.

- **Relax constraints** to find an upper bound on what's possible. "If we had
  unlimited bandwidth, what would the ideal workflow look like?" This reveals
  the shape of the solution before practical constraints narrow it.
- **Tighten constraints** to expose what's truly necessary. "What if we could
  only send 1 KB? What information would we include?" This forces you to
  identify the essential information and discard the rest.

**Why it works:** Real problems come with a mix of genuine constraints and
assumed constraints. Relaxing everything shows you the ideal. Tightening
everything shows you the essentials. The actual solution lives somewhere
between, and exploring both extremes reveals which constraints actually matter.

**Example in C4:** "What if the manifest had to fit in a single TCP packet
(~1400 bytes)?" Forces you to think about what information is essential for
the receiver to begin useful work. Names? Sizes? Hashes? Structure? The answer
reveals the priority ordering of progressive resolution — which is exactly
the ordering C4M uses.

**Connection to algorithmic reframings:** Lifting/relaxation in the companion
document is the formal version of constraint relaxation applied to optimization
problems. The cognitive strategy is the same: remove a constraint, solve the
easier problem, then figure out how to re-introduce the constraint.

---

### 11. Perspective Shifting

**What it is:** Deliberately adopt the viewpoint of a different actor in the
system — the receiver instead of the sender, the user instead of the
implementer, the adversary instead of the designer.

**Why it works:** Different actors have different information, different goals,
and different constraints. A solution that looks elegant from the implementer's
perspective may be unusable from the user's perspective. A protocol that makes
sense from the sender's viewpoint may be inefficient from the receiver's.

**Example in C4:** The entire push-intent-pull-content insight came from
shifting perspective from sender to receiver. From the sender's perspective,
the natural operation is "send files." From the receiver's perspective, the
natural operation is "tell me what you have; I'll take what I need." The
receiver has information (what they already have) that the sender lacks. The
protocol should put the decision where the information is.

**Counter-trap:** Perspective-shifting can degenerate into "designing by
committee" where every perspective gets equal weight. Not all perspectives are
equally relevant for a given decision. The discipline is in choosing *which*
perspective to adopt based on where the binding constraint or critical
information lives.

---

## Part III: Meta-Observations

### 12. Why Reframings Are Hard to See

The algorithmic reframings cataloged in the companion document share a common
property: they seem obvious in retrospect. Once you see that filesystem sync
is a preprocessing problem, or that push-intent-pull-content is a communication
complexity argument, the insight feels natural. But it wasn't seen initially.

Several cognitive factors conspire against seeing reframings:

- **Problem statements carry implicit frames.** "Transfer files faster" already
  frames the problem as transfer optimization. The frame is embedded in the
  language, and you absorb it before you start thinking.

- **Expertise deepens the current frame.** The more you know about file transfer
  protocols, the more sophisticated your transfer optimizations become — and the
  less likely you are to question whether transfer is the right frame. Expertise
  creates Einstellung.

- **Reframings cross domain boundaries.** Seeing that filesystem sync is a
  preprocessing problem requires knowledge of data structure preprocessing.
  Seeing it as a communication complexity problem requires knowledge of that
  field. Reframings live at the intersection of domains, which is the least
  populated part of the expertise landscape.

- **Success discourages reframing.** If your current approach works, there is
  no pressure to seek alternatives. rsync works. It's only when the current
  approach fails — or when you encounter a problem large enough that the
  current approach's scaling becomes visible — that the search for reframings
  begins.

### 13. Catalogs as Cognitive Prosthetics

A catalog of reframings (or cognitive strategies, or traps) functions as a
prosthetic for the limitations listed above. You can't rely on spontaneously
seeing the right reframing, because the cognitive deck is stacked against it.
But you can systematically consult a reference:

- "Am I stuck? Let me check the traps list. Am I exhibiting functional
  fixedness? Einstellung? Am I anchored on the first approach?"
- "Is this problem actually hard, or is my representation making it hard? What
  would it look like in a different formalism?"
- "Does this problem structure match any of the algorithmic reframings? Is
  there a preprocessing opportunity? An amortization argument? A communication
  complexity reduction?"

The catalog doesn't think for you. It tells you where to look. The cognitive
value is in converting "I don't know what I don't know" into "here is a finite
list of things to check."

This document and its companion are both instances of this pattern: catalogs
that compensate for the systematic blind spots in how we approach problems.

---

## Related Documents

- [algorithmic_reframings.md](algorithmic_reframings.md) — Catalog of specific
  algorithmic patterns where scope changes yield surprising improvements.
- [push_intent_pull_content.md](push_intent_pull_content.md) — A specific
  design argument that illustrates several reframings and cognitive shifts.
- [reframing_file_transfer.md](reframing_file_transfer.md) — The formal
  algorithmic argument for manifest-based sync as a problem reframing.

---

*This document collects observations about how we think about problems. Like
the solutions it aims to support, it works better when consulted deliberately
than when left to chance.*
