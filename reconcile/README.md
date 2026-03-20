# Reconcile

Filesystem reconciliation for c4m manifests.

## Reconcile a directory

Make a directory match a c4m description:

```go
rec := reconcile.New(
    reconcile.WithSource(reconcile.NewDirSource(srcManifest, srcDir)),
    reconcile.WithSource(store),
)
plan, err := rec.Plan(target, dirPath)
result, err := rec.Apply(plan, dirPath)
```

The planner diffs the target c4m against the current directory state.
The applier creates, moves, and removes files as needed. Operations
are idempotent — safe to re-run after interruption. Nothing starts
until all required content is confirmed available.

## Distribute to multiple targets

Read a source directory once, write to N destinations simultaneously:

```go
result, err := reconcile.Distribute("/mnt/card/",
    reconcile.ToDir("/mnt/shuttle/"),
    reconcile.ToDir("/mnt/backup/"),
    reconcile.ToStore(myStore),
)

result.Manifest   // c4m file with C4 IDs computed during the single read
result.Targets    // per-target: Created count, Errors
```

Content is hashed during the read — no second pass. Each destination
gets the same bytes. Useful for on-set DIT workflows where speed and
verification matter.
