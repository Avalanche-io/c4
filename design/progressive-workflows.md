# Progressive workflows — a backbone that accretes

Status: design note (2026-07-28, Joshua). The framing correction that
re-scopes `workflow-graph.md`: that document mined one postmortem and
designed for it. **The postmortem is a refinement driver, not the goal.**
The goal is a workflow system for any domain, whose defining property is that
it is useful before it is detailed.

---

## The idea, in Joshua's words

> First you might have a preproduction → production → postproduction
> workflow. As labels alone they provide almost nothing, but they are a
> backbone upon which new details and workflow tasks and edges can be
> created. We want a design, build, test, and run virtuous cycle that is very
> easy to work with as a conceptualization tool, that leads towards a
> detailed hierarchy of workflows.
>
> At the end of the day we want a format and process that is as conceptually
> easy to master as a filesystem and written like c4m, yet be a totally new
> thing that leverages c4 and the advent of AI.

## Why this is not a new mechanism

Three things already in C4 say this, and nobody joined them up:

1. **Partial knowledge is a valid state, not an error.** The philosophy's own
   bullet. Applied to identity it gave nulls; applied to *plans* it gives a
   workflow that is three words long and still valid.
2. **The streaming chain.** A c4m already emits entries with null directory
   IDs, then one refinement patch that fills them in, then the resolved root.
   **That is progressive refinement, shipped.** The format learned the
   pattern for scan-time; workflows need it for design-time.
3. **The identity-basis ladder** — `null` → `nom:` assigned → content
   (`derivation-records.md`). Unknown, then named, then verified. A workflow's
   whole life is a walk up that ladder, one node at a time.

So progressive refinement is not a feature to add. It is the existing
identity model applied to plans instead of bytes.

## The filesystem analogy is load-bearing, not decorative

A filesystem is masterable because it has exactly two ideas: **things nest**,
and **each thing has a name**. Everything else — permissions, sizes, times —
is detail you learn later and can ignore at first.

A workflow has the same two ideas plus one: **things nest**, **each thing has
a name**, and **some things must happen before others**.

That third idea is the only thing a filesystem does not have, and it is the
only thing a user has to learn beyond `mkdir`. Hierarchy is decomposition;
edges are dependency. Conflating them is the mistake every workflow tool
makes — a DAG with no tree is unbrowsable, and a tree with no DAG cannot
sequence.

**c4m already carries hierarchy** — depth, natural order, one entry per line.
A workflow document is that shape with a different line grammar. If we get
this right, someone who understands a directory listing understands a
workflow listing in about a minute.

## The seven rungs

Every rung is a valid document. Nothing is a draft; the whole thing is
always runnable to the extent it is known.

**1 — Labels.** Three words. Worth almost nothing, and the point is that it
costs almost nothing.

```
preproduction
production
postproduction
```

**2 — Decomposition.** Children appear. Still no IDs, no processes.
Filesystem knowledge is sufficient to get this far.

```
preproduction
  casting
  location-scout
production
  principal-photography
postproduction
  edit
  color
  deliver
```

**3 — Dependency.** The one new idea. Some tasks need others first.

```
production            after=preproduction
postproduction/edit   after=production/principal-photography
```

**4 — Process bound.** A task learns *how* it gets done. Crucially the answer
may be "a person does it" — an irreproducible node with an author is
first-class, not a gap (`derivation-records.md`, creation is first-class).

```
postproduction/deliver  process=nom:studio/deliver-spec-v3
postproduction/color    process=human class=volatile authored-by=colorist
```

**5 — Inputs named but not made.** `nom:` tags for artifacts that will exist.
The graph is now plannable and costable before anything is run.

**6 — Run.** Content IDs replace `nom:` tags as things actually get made.
The document becomes a record without changing kind. **Plan and record are
the same document at two rungs** — every other system in the prior art keeps
them in separate artifacts.

**7 — Refinement.** What was one task turns out to be five. An alternate is
tried — a new edge to an existing node, not a new document. A note attaches
to the whole workflow rather than a step. The backbone accretes.

## The virtuous cycle

Design → build → test → run, and the run refines the design:

- **Design** — add structure. Cheap, because rung 1 is three words.
- **Build** — bind processes to tasks. Some are scripts, some are prompts,
  some are people.
- **Test** — run a variant: same graph, one parameter changed. This must be
  the cheapest operation in the system, because it is the most frequent.
- **Run** — execute; content IDs bind; the plan becomes a record.
- **Refine** — the record shows what was slow, what was wrong, what got
  redone. That feeds rung 7 and the loop closes.

The cycle only works if every arrow is cheap. **The one that is usually
expensive is Test**, which is why variants-as-edges is the load-bearing
decision, and why a postmortem full of variant sweeps was the right thing to
mine first.

## Where AI enters, and why now

Not as a feature. As the thing that makes rungs 2 and 7 cheap:

- **Decomposition is drudgery a model is good at.** A human writes three
  labels; an agent proposes forty tasks; the human deletes half. That is a
  better conceptualization tool than either alone.
- **Any step an agent can do is a node.** Joshua to Erik: *"Anything you could
  have claude code do today can be made into a step in a workflow."* The node
  vocabulary is no longer limited to what someone wrote a plugin for.
- **The graph is the shared artifact.** Human and agent read and write the
  same plain-text document. No API, no handoff format.
- **And the honest half:** an agent's step is `volatile` and gets an author,
  because irreproducibility is the signature of creation, not a defect.

This is why the moment is now and not five years ago: rung 4's "process" used
to mean "a program someone wrote." It now means "any digital task," and that
is what turns a workflow tool from pipeline plumbing into a general
instrument.

## What this changes about the mined design

`workflow-graph.md` designed for rung 6 — the fully-bound, fully-run case,
because that is what a postmortem contains. It is not wrong, it is
**partial**. Specifically:

- **Hierarchy is missing entirely.** The mined grammar has edges and no tree.
  Rungs 1–2 cannot be written in it at all. This is the biggest gap.
- **`after=`** (sequence without a data dependency) has no expression. The
  mined design assumes every edge carries data, because in a rendering
  pipeline it does. In a production it does not — casting precedes
  photography without producing a file it consumes.
- **A step with no process yet** has no form. Rung 2 needs one.
- The rest survives: derivation keys, the exempt list, class, `nom:`
  binding, variants-as-edges.

## Open questions

1. **One kind or two?** Is the hierarchy a different document from the edges,
   or one document with two line forms? Filesystem-simplicity argues one.
   The frozen-grammar risk argues care.
2. **Is a task a c4m entry?** If a workflow listing is literally c4m-shaped,
   how much of the entry line survives — does a task have a size, a time?
   Tempting and possibly a trap.
3. **What is a task's ID before it has run?** A `nom:` tag is the obvious
   answer; whether the *task* and its *output* are one node or two is not
   obvious and the mined design assumed one ("a step is its output").
4. **Does `after=` participate in the derivation key?** It is sequencing, not
   data. Probably exempt — but then it cannot invalidate, and sometimes it
   should.

## Standing

This note supersedes `workflow-graph.md`'s scope, not its content. That
document remains the evidence base and the rung-6 design. Neither is built.
Both are downstream of the release blockers and the surface freeze.
