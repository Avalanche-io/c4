# External tools: `c4 <name>` runs `c4-<name>`

Status: requirements, 2026-10-20. Requested by Joshua so that one install
puts people on a C4 farm: `c4 farm login https://farm.studio.example`,
then submit, watch and fetch, with nothing else to set up.

## What exists

`cmd/c4/main.go` switches on `os.Args[1]` for the built-in verbs; any
other first argument is read as a path (`c4 <path>` is `c4 id <path>`),
and with nothing usable it reads stdin. So `c4 farm up` today scans a
directory called `farm`, or prints usage.

c4-operator releases `c4-farm` (macOS, Linux, Windows) and `c4-agent`
(macOS, Linux) with goreleaser, plus Homebrew formulas `c4-farm` and
`c4-agent` in `mrjoshuak/homebrew-tap`. `c4-farm login HUD-URL` works
against any farm; only `c4-farm up` (a farm on this machine) needs Docker.

## The rule

One rule, no flags, no configuration, the way git and kubectl find
plugins:

1. The built-in verbs win, as today.
2. Otherwise, if the first argument is a **reserved tool name** (`farm`,
   `agent`), c4 runs `c4-<name>` from PATH, even when a file or directory
   of that name exists. A directory named `farm` is scanned with
   `c4 ./farm` or `c4 id farm`. If `c4-<name>` is not on PATH, c4 says so
   on stderr, names the install path, and exits 1. Reserving the two
   names is what makes `c4 farm` reliable in scripts and docs: it never
   depends on what is in the working directory.
3. Otherwise, if the first argument names an **existing file or
   directory**, today's behaviour: `c4 id <path>`. Released c4 has real
   users and this is kept exactly.
4. Otherwise, if the first argument does not start with `-` and
   `c4-<name>` is on PATH, c4 runs it. This is the extension point: any
   tool installed as `c4-<name>` becomes `c4 <name>` without a change to
   c4, and without claiming names that are paths.
5. Otherwise, as today (stdin, or usage and exit 1).

The run passes arguments, stdin, stdout, stderr, the environment and the
exit code through unchanged; c4 adds nothing to the environment and
prints nothing of its own. On Windows `exec.LookPath` finds
`c4-farm.exe` through PATHEXT, so the rule is the same.

`c4 help` (and `--help`, `-h`) prints the top page as today, followed by
the installed tools: every `c4-<name>` executable on PATH, each once,
sorted, as `c4 <name>`, with the two reserved names noted when absent.
This is narration; it goes where the help goes.

## What this does not do

- No plugin manifest, registry, naming convention beyond the prefix, or
  version negotiation. A tool's version is its own business
  (`c4 farm version`).
- No change to any built-in verb, to C4 IDs, to the c4 library's API or
  to ST 2114 behaviour.
- No new flag. The rule is positional and has one knob, PATH, which the
  user already owns.
- `c4-<name>` tools are not listed by `c4 explain` and do not take part
  in the machine contract of the built-in verbs; stdout is theirs.

## Why the reserved list is two names

Every reserved name is a path the bare shortcut can no longer take. `farm`
and `agent` are the two tools the standard release ships; nothing else
earns the exception until it ships the same way. Adding a name is a
one-line change and a changelog entry under the minor tier, because it
changes what `c4 <name>` does for a directory of that name.

## Tests

`cmd/c4/external_test.go` builds the c4 binary and a stand-in `c4-farm`
(a small Go program compiled into a temp dir put first on PATH, so the
same test runs on Windows) and checks:

- `c4 farm a b` runs the tool with `a b`, stdin reaches it, its stdout and
  stderr come back on the right streams, and its exit code is c4's.
- With a directory named `farm` in the working directory, `c4 farm`
  still runs the tool; `c4 ./farm` and `c4 id farm` scan the directory.
- A non-reserved name: `c4 zap` runs `c4-zap` when it is on PATH and
  nothing named `zap` exists; with a file named `zap` present, `c4 zap`
  scans the file even though `c4-zap` is on PATH.
- With `c4-farm` absent from PATH, `c4 farm` exits 1 with the install
  message on stderr and nothing on stdout.
- `c4 help` lists the installed tools.

## Packaging (c4toolkit)

The standard release brings the tools. `releases.json` gains two
components built from c4-operator at a pinned tag: `c4-farm` (all six
platforms) and `c4-agent` (macOS and Linux). They land in the suite
archives beside c4, c4sh and c4git. The Homebrew formula `c4` stays in
`mrjoshuak/homebrew-tap` (Joshua, 2026-10-21: the tap does not move to
Avalanche-io until the service offering is tied in) and depends on that
tap's `c4-farm` and `c4-agent` formulas, generated from each c4-operator
release and pushed from the laptop, so `brew install mrjoshuak/tap/c4`
installs all of it. The pin between the suite
and c4-operator is recorded in `releases.json` and validated with the
rest of the suite.
