# Verify on pull: a client setting of the library, default on, trust per source

Status: design, 2026-11-03, for Joshua. Nothing built.

## The ruling

Joshua: "I think the id on pull question is a c4d, c4 cli, c4 lib client
config question. Right? Like some HPC applications in trusted environments
built to be error correcting should not rehash on each pulling node, while
file exchanges between untrusted parties should."

So: one implementation, in the c4 library, used by the CLI and by c4d and
c4-operator. Default verify (safe by default, fast by flag). A trusted
environment opts out explicitly, scoped to the sources it trusts, never
inferred.

## Where verification happens today

| Pull path                                   | Verified?                                   |
| ------------------------------------------- | ------------------------------------------- |
| `c4 cat ID`                                 | yes: `readVerified` rehashes the whole object before writing a byte |
| reconcile (`c4 restore`, `c4 patch`)        | **no** on pull: `openContent(id)` is copied straight to the destination; restore rehashes the as-built tree afterwards, a second full pass |
| c4-operator node and regional fills         | only when a Project sets `verifyFill`, off by default ("bytes are verified once, when they enter the store") |
| `Put` (store ingest)                        | yes by construction: Put computes the ID    |

The library never verifies what it pulls; the two verbs that do each do it
their own way; and the operator's default is the opposite of the ruling.

## The setting

**Trust is named per source; everything else is verified.**

- `~/.c4/config`: `trust = <endpoint>` lines, one per source the client
  trusts, in the same spelling as its `store =` line (a path, or an
  `s3://` URL). `C4_TRUST=<endpoint>,...` is the environment form, like
  `C4_STORE`.
- No URL option. A `?verify=false` on a store URL would travel with the
  URL into a Project spec, a shared config, a pasted command; trust is a
  property of the client's relationship to a source, not of the source's
  address. Keeping URLs identical between the CLI and the operator also
  keeps the key-naming rule simple.
- A source that is not configured as a store cannot be trusted (the line
  is reported and ignored). Nothing is inferred: not from `localhost`,
  not from a Unix path, not from a cluster membership.
- **Matching is exact, after one normalisation, and one line trusts one
  source.** A `trust` value is compared to each configured endpoint after
  both are normalised the same way: a path is cleaned (`filepath.Clean`)
  and made absolute against the config file's directory for a config
  line or the working directory for `C4_TRUST`; an `s3://` URL keeps its
  bucket, prefix and query exactly, only the scheme and host lowercased.
  The normalised strings must be byte-equal. No globs, no prefixes, no
  "all S3": a line that matches two endpoints is a configuration error,
  reported, and trusts neither.

Library surface, in `store`:

```go
// Verified returns src with every object it opens hashed as it streams.
// The reader withholds the object's last byte until the underlying
// stream ends and the digest matches the ID; a mismatch returns
// ErrCorrupt in place of the last byte, so no consumer ever sees EOF on
// bytes that did not verify.
func Verified(src Source, name string) Source

type ErrCorrupt struct{ ID, Got c4.ID; Source string }
```

`OpenStore` and `OpenConfigured` wrap every configured source in
`Verified` unless its endpoint is trusted; `MultiStore` wraps per member,
so a mixed configuration verifies exactly the untrusted ones. `Put` is
unchanged (it hashes). `Has` is unchanged (a presence probe asserts
nothing about bytes; a pull verifies them).

## What "verify" means for a streaming reader

The reader keeps one byte back. Every byte read from the source goes into
a SHA-512 and all but the most recent one go to the consumer; at the
source's EOF the digest is compared to the ID; on a match the held byte
is delivered and then EOF; on a mismatch the held byte is never delivered
and the read returns `ErrCorrupt`. A consumer that copies to a file (the
durable writer in reconcile) therefore never closes a complete file on
bad bytes: the copy fails, the temporary file is discarded, nothing is
renamed into place, exactly as for any other read error. This is the
rule the operator's tier applies now (`tier.Fetcher.Verify`); the library takes it
over so there is one reader.

Where a source reports a size (S3 HEAD, a file's stat), a short stream
fails the digest anyway; no second check.

## The error, and what the verbs do with it

`ErrCorrupt`: `object c4… from <source> does not hash to its ID (got c4…);
refusing it`. It names the source so a mixed configuration points at the
bad one.

- `c4 cat`: prints nothing, the error on stderr, exit 1 (as today).
- `c4 restore` and `c4 patch`: the create for that path fails, is listed
  under "error:", and the verb exits 3 ("restore incomplete: cure the
  failures and re-run"); the pre-image is durable, nothing claims the
  target. Restore's post-build rehash stays: it also verifies moves,
  metadata and the tree's root ID. Files created through a verified pull
  are thereby hashed twice; a later change can let the post-build scan
  reuse the pull's digest through the reuse guide. Not in this note.
- A `-s` snapshot is not a pull; unchanged.

## c4-operator's mapping

`tier.Fetcher.Verify` becomes a call to `store.Verified` around the
upstream reader: one implementation. The Project field `verifyFill`
(default off) inverts to the library's default: a Project verifies every
fill unless it names the sources it trusts. Proposed field:

```yaml
store:
  trust: [regional, peers]   # sources this project does not rehash on fill
```

with `verifyFill: true` kept for one release as a no-op alias of "trust
nothing" and a CEL rule refusing both together. An existing Project that
never set `verifyFill` starts verifying on fill after the upgrade: that is
the ruling, and the release notes say so with the cost table below so a
cluster that measured it opts out deliberately.

`store.trust` lowers a protection, so it is set the way strict
accountability is turned down: only a farm admin may set it, it is a
signed, booked control (who, when, which sources), and the HUD shows it
on the project: "fills from peers aren't verified for this project, set
by <admin> on <date>". A Shared partition trusts a source only if every
project in it does; one project's trust never lowers another's
verification.

Trusting `peers` means one compromised node can poison another's cache:
a node that serves bad bytes under a good ID is believed by every peer
that trusts it, and the corruption travels until something downstream
rehashes. That is the trade the cost table prices; it belongs in the
HUD's line and the release notes, next to the numbers.

## Cost

Measured today, SHA-512 over pulled bytes:

| Hardware             | CPU-seconds per GB |
| -------------------- | ------------------ |
| x86 c7i, c7a         | 1.28 to 1.40       |
| Graviton3            | 0.99               |
| Apple M3 Max         | 0.62               |

At 1 GB/s of fills a node spends about 1.3 x86 cores verifying; a trusted
HPC environment built to be error-correcting is the case for `trust`.

## Tests

- A file store with one object's bytes flipped: `c4 restore --force` of a
  description naming it exits 3, names the ID and the source, writes
  nothing for that path, and leaves the rest restored; `c4 cat` of it
  prints nothing and exits 1.
- The same store trusted (`trust =` line, and `C4_TRUST`): the bad bytes
  are written untouched, and restore's post-build rehash is what catches
  them.
- A streaming consumer sees `ErrCorrupt` before the last byte, never a
  clean EOF, for an object whose source stream ends early or late.
- Two stores, one trusted: only the untrusted one's objects are hashed
  (counted through a test source).
- A `trust` line naming an endpoint that is not a configured store is
  reported and ignored.
- `c4 cat` through `Verified` behaves as `readVerified` did; `readVerified`
  is deleted.
