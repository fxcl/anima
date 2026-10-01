#!/bin/bash
# Go port vs Java anima (com.hellokaton.anima v0.3.1) API alignment benchmark.
# Deterministic: verifies each manifest row both ways.
#   covered/partial -> every go doc token must EXIST (src: prefix greps Go source)
#   gap             -> every token must be ABSENT (stale-gap tripwire: presence = manifest drift = run fails)
# Emits METRIC lines consumed by the autoresearch runner.
set -uo pipefail
cd "$(dirname "$0")/.."
JAVA_SRC="${JAVA_SRC:-$HOME/go/anima/anima/src/main/java/com/hellokaton/anima}"
MANIFEST="autoresearch/manifest.tsv"

if [ ! -d "$JAVA_SRC" ]; then
  echo "FATAL java source not found: $JAVA_SRC" >&2
  exit 2
fi

go_doc="$( { go doc -all . 2>&1; for d in dialect; do go doc -all "./$d" 2>&1; done; } )"
if [ -z "$go_doc" ]; then
  echo "FATAL go doc produced no output" >&2
  exit 2
fi

covered=0; partial=0; gap=0; total=0; errors=0

while IFS=$'\t' read -r id status anchor syms note; do
  [ -z "$id" ] && continue
  case "$id" in \#*) continue ;; esac
  if [ "$status" = excluded ]; then continue; fi
  total=$((total+1))

  jfile="${anchor%%:*}"; jtok="${anchor#*:}"
  if ! grep -qF "$jtok" "$JAVA_SRC/$jfile" 2>/dev/null; then
    echo "ANCHOR-MISS $id: '$jtok' not in $jfile"
    errors=$((errors+1)); continue
  fi

  if [ "$status" = gap ]; then
    present=0
    IFS=, read -ra gtoks <<< "$syms"
    for s in "${gtoks[@]}"; do
      if echo "$go_doc" | grep -qF "$s"; then
        echo "STALE-GAP $id: token '$s' now exists in Go API - update manifest status"
        present=1
      fi
    done
    if [ $present -eq 1 ]; then errors=$((errors+1)); else gap=$((gap+1)); fi
    continue
  fi

  ok=1
  IFS=, read -ra ctoks <<< "$syms"
  for s in "${ctoks[@]}"; do
    case "$s" in
      src:*) grep -rqF "${s#src:}" --include='*.go' . || { echo "MISS $id: '$s' not in Go source"; ok=0; } ;;
      *)     echo "$go_doc" | grep -qF "$s" || { echo "MISS $id: '$s' not in go doc"; ok=0; } ;;
    esac
  done
  if [ $ok -eq 0 ]; then
    errors=$((errors+1)); continue
  fi
  case "$status" in
    covered) covered=$((covered+1)) ;;
    partial) partial=$((partial+1)) ;;
    *) echo "BAD-STATUS $id: $status"; errors=$((errors+1)) ;;
  esac
done < "$MANIFEST"

score=$(awk "BEGIN {printf \"%.4f\", ($covered + 0.5*$partial) / $total}")
echo "METRIC alignment_ratio=$score"
echo "METRIC java_api_total=$total"
echo "METRIC covered=$covered"
echo "METRIC partial=$partial"
echo "METRIC gap=$gap"
echo "METRIC manifest_errors=$errors"
[ "$errors" -eq 0 ]
