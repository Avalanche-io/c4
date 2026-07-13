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

echo "----"
if [ "$fails" -eq 0 ]; then
  echo "PASS: $ITER kills, every printed and journaled ID recomputed, no wedged store"
else
  echo "FAIL: $fails invariant violations"
fi
rm -rf "$BASE"
exit "$fails"
