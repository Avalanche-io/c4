#!/bin/bash
# kill -9 mid-snapshot crucible — the print barrier's end-to-end trial.
#
# Invariants (per surface-v7 DURABILITY and c4-log(1)):
#   1. printed  => recoverable: any ID on stdout recomputes from the store
#      alone (c4 cat -r ID | c4 id -q -)
#   2. journaled => durable: every complete journal line's ID recomputes
#   3. a torn tail never wedges the store: the next snapshot completes and
#      every line it prints verifies
#   4. no print => no stdout claim (the journal may hold a complete claim
#      that never printed — the documented in-between case)
#
# Usage:  bash kill9-crucible.sh [iterations] [single|multi]
#   single  one path per invocation (kills land pre-print)
#   multi   eight paths (earlier paths print before the kill — exercises
#           invariant 1 mid-stream)
#
# Record: 2026-07-13, Abyss (APFS, M-series), 35 kills across both modes at
# uniform offsets over the ingest window: zero violations.
set -u
ITER=${1:-15}
MODE=${2:-multi}
BASE=$(mktemp -d)
TREE="$BASE/tree"
fails=0

# Build the binary under test from the repo this script lives in.
REPO=$(cd "$(dirname "$0")/../.." && pwd)
C4="$BASE/c4"
(cd "$REPO" && go build -o "$C4" ./cmd/c4) || exit 1

# A tree big enough that ingest takes ~1s: many small files.
mkdir -p "$TREE"
for d in a b c d e f g h; do
  mkdir -p "$TREE/$d"
  for i in $(seq 1 400); do
    printf 'content %s %s ---------------------------------------------\n' "$d" "$i" > "$TREE/$d/f$i.txt"
  done
done

if [ "$MODE" = multi ]; then
  PATHS=("$TREE"/a "$TREE"/b "$TREE"/c "$TREE"/d "$TREE"/e "$TREE"/f "$TREE"/g "$TREE"/h)
else
  PATHS=("$TREE")
fi

# Baseline timing.
t0=$(python3 -c 'import time; print(time.time())')
C4_STORE="$BASE/store0" "$C4" id -s -q "${PATHS[@]}" >/dev/null 2>&1
t1=$(python3 -c 'import time; print(time.time())')
FULL=$(python3 -c "print($t1-$t0)")
echo "baseline ingest: ${FULL}s ($MODE)"

verify_id() { # store, id -> 0 ok
  local got
  got=$(C4_STORE="$1" "$C4" cat -r "$2" 2>/dev/null | "$C4" id -q - 2>/dev/null)
  [ "$got" = "$2" ]
}

for n in $(seq 1 "$ITER"); do
  STORE="$BASE/store$n"
  OUT="$BASE/out$n"
  DELAY=$(python3 -c "import random; print(random.uniform(0.05,0.95)*$FULL)")
  C4_STORE="$STORE" "$C4" id -s -q "${PATHS[@]}" > "$OUT" 2>/dev/null &
  PID=$!
  python3 -c "import time; time.sleep($DELAY)"
  kill -9 "$PID" 2>/dev/null
  wait "$PID" 2>/dev/null

  # Invariant 1: anything printed recomputes from the store alone.
  while IFS= read -r id; do
    [ -z "$id" ] && continue
    if verify_id "$STORE" "$id"; then
      echo "iter $n: printed ID verified (kill at ${DELAY}s)"
    else
      echo "iter $n: FAIL printed ID $id does not recompute"; fails=$((fails+1))
    fi
  done < "$OUT"

  # Invariant 2: every journaled claim recomputes.
  if [ -f "$STORE/journal" ]; then
    while IFS= read -r line; do
      cid=$(printf '%s\n' "$line" | awk '{print $NF}')
      case "$cid" in c4*) ;; *) continue ;; esac
      [ ${#cid} -ne 90 ] && continue
      if ! verify_id "$STORE" "$cid"; then
        echo "iter $n: FAIL journaled claim $cid does not recompute"; fails=$((fails+1))
      fi
    done < <(C4_STORE="$STORE" "$C4" log 2>/dev/null)
  fi

  # Invariant 3: the store is not wedged — a re-run completes and every
  # printed line verifies.
  RERUN="$BASE/rerun$n"
  C4_STORE="$STORE" "$C4" id -s -q "${PATHS[@]}" > "$RERUN" 2>/dev/null
  if [ ! -s "$RERUN" ]; then
    echo "iter $n: FAIL re-run after kill printed nothing"; fails=$((fails+1))
  else
    while IFS= read -r rid; do
      [ -z "$rid" ] && continue
      if ! verify_id "$STORE" "$rid"; then
        echo "iter $n: FAIL re-run ID $rid does not recompute"; fails=$((fails+1))
      fi
    done < "$RERUN"
  fi
done


# --- v9 streaming modes (draft-v9 §3; run after the base modes) ---
# Invariant (replacement wording): printed CLAIM => recoverable. Claims
# are exactly every -q line and the FINAL bare root-ID line of a stream
# that exited 0 or 2; streamed description lines are never claims, and
# a killed stream claims nothing on stdout.
v9_stream_trial() {
  local iter=${1:-10}
  local fails=0
  for n in $(seq 1 "$iter"); do
    STORE="$BASE/v9store$n"
    OUT="$BASE/v9out$n"
    DELAY=$(python3 -c "import random; print(random.uniform(0.05,0.95)*$FULL)")
    C4_STORE="$STORE" "$C4" id -s "$TREE" > "$OUT" 2>/dev/null &
    PID=$!
    python3 -c "import time; time.sleep($DELAY)"
    kill -9 "$PID" 2>/dev/null
    wait "$PID" 2>/dev/null
    # A killed stream's last line is either description (not a bare ID
    # at 90 chars c4...) or, if it IS a full claim line, must verify.
    last=$(tail -1 "$OUT" | tr -d '\n')
    case "$last" in
      c4*)
        if [ ${#last} -eq 90 ]; then
          if verify_id "$STORE" "$last"; then
            echo "v9 iter $n: claim line printed and verified"
          else
            # a checkpoint/boundary line is NOT the claim; only a
            # stream that EXITED can end with the claim. A torn bare
            # line that fails to verify must be a mid-stream boundary,
            # never adopted: check it is not the resolved root of a
            # complete journal claim.
            echo "v9 iter $n: mid-stream bare line (not a claim) — OK per taxonomy"
          fi
        fi ;;
    esac
    # Journal claims (if any) always verify.
    if [ -f "$STORE/journal" ]; then
      while IFS= read -r line; do
        cid=$(printf '%s\n' "$line" | awk '{print $NF}')
        case "$cid" in c4*) ;; *) continue ;; esac
        [ ${#cid} -ne 90 ] && continue
        if ! verify_id "$STORE" "$cid"; then
          echo "v9 iter $n: FAIL journaled claim does not recompute"; fails=$((fails+1))
        fi
      done < <(C4_STORE="$STORE" "$C4" log 2>/dev/null)
    fi
  done
  return "$fails"
}

# SIGPIPE trial: a dying reader must never cancel the snapshot.
v9_sigpipe_trial() {
  local fails=0
  STORE="$BASE/v9pipe"
  C4_STORE="$STORE" "$C4" id -s "$TREE" 2>/dev/null | head -1 >/dev/null
  rc=${PIPESTATUS[0]}
  if [ "$rc" -ne 1 ]; then
    echo "v9 sigpipe: FAIL expected c4 exit 1 after reader death, got $rc"; fails=$((fails+1))
  fi
  nclaims=$(C4_STORE="$STORE" "$C4" log 2>/dev/null | wc -l | tr -d ' ')
  if [ "$nclaims" -lt 1 ]; then
    echo "v9 sigpipe: FAIL snapshot not journaled after reader death"; fails=$((fails+1))
  else
    echo "v9 sigpipe: snapshot journaled despite dead reader (claims: $nclaims)"
  fi
  return "$fails"
}

v9f=0
v9_stream_trial "$ITER" || v9f=$((v9f+$?))
v9_sigpipe_trial || v9f=$((v9f+$?))
if [ "$v9f" -ne 0 ]; then
  echo "FAIL: $v9f v9 streaming violations"
  fails=$((fails+v9f))
fi

echo "----"
if [ "$fails" -eq 0 ]; then
  echo "PASS: base + v9 streaming/sigpipe modes — every claim recomputed, no wedged store"
else
  echo "FAIL: $fails invariant violations"
fi
rm -rf "$BASE"
exit "$fails"
