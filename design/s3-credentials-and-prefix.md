# The CLI's S3 store: temporary credentials, and the prefix rule

Status: design, 2026-11-03, for Joshua. Two items raised
by the operator's S3 work (c4-operator branch `s3-session-creds`).
Nothing built.

## 1. Credentials: the standard chain, in core, used by both

**Today** `store/config.go` reads `AWS_ACCESS_KEY_ID` and
`AWS_SECRET_ACCESS_KEY` and refuses without them, and `store/s3sign.go`
signs with a static key pair. On EC2 or EKS a user of `c4 id -s` to S3
must mint long-lived keys, which is what those platforms exist to avoid.

**The operator's chain** (`internal/storeurl/creds.go`, 411 lines, standard library
only): a `Credentials` value with `SessionToken` and `Expires`; a
`CredentialSource` interface with `Static` and `Ambient`; `Ambient` is the
environment (including `AWS_SESSION_TOKEN`), then a web identity
(`AWS_WEB_IDENTITY_TOKEN_FILE` plus `AWS_ROLE_ARN` through STS
`AssumeRoleWithWebIdentity`, the token file re-read at each renewal,
regional endpoint, `AWS_ENDPOINT_URL_STS` honoured), then the instance
role through IMDSv2 (2 s timeout, `AWS_EC2_METADATA_DISABLED` honoured);
renewal five minutes before expiry, single flight, holders of valid keys
never wait, a failed renewal keeps valid keys and retries. The signer
sends `X-Amz-Security-Token` and signs it. Tested against AWS's SigV4
vectors and with fakes for STS and IMDS, under the race detector.

**Proposal: the chain moves into c4 core, and the operator imports it from
there.** One implementation per idea: today the operator carries its own
copy because core has none; once core has it, the operator's `storeurl`
package uses `store.CredentialSource` from c4 (it already pins c4 in
go.mod). Zero dependencies holds: the chain imports only the standard
library, as the signer already does.

Shape in core:

- `store.Credentials{AccessKeyID, SecretAccessKey, SessionToken, Expires}`
  and `store.CredentialSource` with `Retrieve(ctx)`, lifted as they are.
- `store.Static(c)` and `store.Ambient()`.
- `S3Store` holds a `CredentialSource` instead of two strings;
  `signRequest` asks it for keys per request and adds the token header
  when present. `NewS3Store` keeps its signature for callers that pass
  static keys (it wraps them in `Static`).
- `openS3Configured` uses `Ambient()`: the environment's static keys still
  win when set, so every configuration that works today works unchanged;
  without them, a web identity or an instance role is used, and only when
  all three are absent does it refuse, naming the three.
- Not covered, as in c4d's: `~/.aws/config`, profiles, SSO, and ECS
  container credentials. The aws CLI's own chain covers those; say so in
  the error.

Scope for Joshua: this is about 400 lines of core, the largest single
addition to `store/` since the S3 store itself. It is data-plane code the
operator needs identically, so the alternative is two copies.

## 2. The prefix: refuse what would land outside the folder

**Today** `openS3Configured` takes the URL path as the prefix verbatim and
`objectKey` is `prefix + id`. So `s3://bucket/runs` stores
`runsc4abc…`, beside the folder rather than in it, and `s3://bucket/c4`,
the form the package's own documentation uses in two places, stores
`c4c4abc…`. The operator now refuses a prefix without a trailing slash at
open, naming where the objects would land, and keeps bucket roots
(`s3://bucket`, `s3://bucket/`) working.

**Proposal: the CLI refuses the same way**, with the same message shape,
so the farm and the CLI name the same keys for the same URL and a shared
bucket is never a silent false miss. The doc examples change to
`s3://bucket/c4/`.

The cost, to be stated plainly: a user whose config already says
`s3://bucket/c4` has objects under `c4<id>` keys today. After the change
that URL is refused; with the slash added, the store reads `c4/<id>` and
does not see them. Options:

- A. Refuse, and say in the release notes that objects stored under a
  slashless prefix stay under their old keys, which `aws s3 mv` can move
  (`c4<id>` → `c4/<id>`). Clean; one rule; matches the operator.
- B. Refuse new configurations but keep reading the legacy key as a
  fallback on miss. Hides the inconsistency it means to remove, and adds a
  second key shape forever.
- C. Silently add the slash. Rejected for the farm, for the same
  reason: it would make the two sides disagree about existing data.

Recommendation: A. The S3 store is young, the fix is a one-line config
edit plus an `aws s3 mv` for anyone who has stored under the old shape,
and the notes say exactly that.

## Tests

- SigV4 vectors from botocore (`get-vanilla`, `post-sts-header-before`),
  shared with the operator's testdata.
- The chain with fake STS and IMDS endpoints: environment first, web
  identity with rotation and renewal timing, an outage while keys still
  hold, instance role, 64 concurrent callers making one exchange.
- The prefix rule: `s3://b/runs` refused naming `runsc4…`; `s3://b`,
  `s3://b/`, `s3://b/runs/` accepted; the error names the fix.
- Every existing S3 test unchanged.
