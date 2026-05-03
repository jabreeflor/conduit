#!/usr/bin/env bash
# gen-changelog.sh — regenerate CHANGELOG.md from merged PR titles.
#
# Reads merged PR titles via `gh pr list --state merged --search ...` and
# bins them into Keep-a-Changelog sections by Conventional Commits type.
#
# Usage:
#   .scripts/gen-changelog.sh --since vX.Y.Z --preview     # print proposed [Unreleased] block to stdout
#   .scripts/gen-changelog.sh --release vX.Y.Z             # rewrite CHANGELOG.md, promoting [Unreleased] to vX.Y.Z

set -euo pipefail

SINCE=""
RELEASE=""
PREVIEW=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --since)   SINCE="$2"; shift 2 ;;
    --release) RELEASE="$2"; shift 2 ;;
    --preview) PREVIEW=1; shift ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

command -v gh >/dev/null || { echo "gh CLI required" >&2; exit 1; }

# When --release is given without --since, fall back to the most recent tag.
if [[ -n "$RELEASE" && -z "$SINCE" ]]; then
  SINCE=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
fi

# Compute the merged-after date for the search.
if [[ -n "$SINCE" ]]; then
  if git rev-parse "$SINCE" >/dev/null 2>&1; then
    SINCE_DATE=$(git log -1 --format=%cI "$SINCE")
  else
    echo "tag/ref not found: $SINCE" >&2
    exit 1
  fi
else
  SINCE_DATE=""
fi

# Pull merged PR titles + numbers as TSV.
search_args=(--state merged --limit 500 --json number,title,mergedAt --jq '.[] | "\(.number)\t\(.mergedAt)\t\(.title)"')
prs=$(gh pr list "${search_args[@]}")

# Bin by conventional-commit type. Hidden types (test/build/ci/chore) drop out.
declare -a added=() fixed=() changed=() removed=()
while IFS=$'\t' read -r num merged_at title; do
  [[ -z "$title" ]] && continue
  if [[ -n "$SINCE_DATE" && "$merged_at" < "$SINCE_DATE" ]]; then
    continue
  fi
  # Strip leading "type(scope):" or "type:".
  type=$(echo "$title" | sed -E 's/^([a-z]+)(\([^)]*\))?(!)?:.*/\1\3/' | tr -d '[:space:]')
  desc=$(echo "$title" | sed -E 's/^[a-z]+(\([^)]*\))?!?:[[:space:]]*//')
  bullet="- ${desc} (#${num})"
  case "$type" in
    feat)            added+=("$bullet") ;;
    feat\!)          removed+=("**BREAKING:** $bullet") ;;
    fix)             fixed+=("$bullet") ;;
    fix\!)           removed+=("**BREAKING:** $bullet") ;;
    perf|refactor|docs) changed+=("$bullet") ;;
    perf\!|refactor\!|docs\!) removed+=("**BREAKING:** $bullet") ;;
    *) ;;  # test / build / ci / chore / non-conforming → hidden
  esac
done <<< "$prs"

emit_section () {
  local heading="$1"; shift
  local -a items=("$@")
  if (( ${#items[@]} > 0 )); then
    printf '\n### %s\n' "$heading"
    printf '%s\n' "${items[@]}"
  fi
}

render () {
  local heading="$1"
  printf '## %s\n' "$heading"
  emit_section "Added"   "${added[@]:-}"
  emit_section "Changed" "${changed[@]:-}"
  emit_section "Fixed"   "${fixed[@]:-}"
  emit_section "Removed" "${removed[@]:-}"
}

if (( PREVIEW )); then
  render "[Unreleased]"
  exit 0
fi

if [[ -z "$RELEASE" ]]; then
  echo "Either --preview or --release <vX.Y.Z> is required." >&2
  exit 2
fi

today=$(date -u +%Y-%m-%d)
new_block=$(render "[$RELEASE] - $today")

# Replace the existing [Unreleased] block with a fresh empty one + the new
# release block, leaving prior history intact.
tmp=$(mktemp)
awk -v new="$new_block" '
  BEGIN { skip=0 }
  /^## \[Unreleased\]/ { print "## [Unreleased]\n"; print new; skip=1; next }
  skip && /^## / { skip=0 }
  !skip { print }
' CHANGELOG.md > "$tmp"
mv "$tmp" CHANGELOG.md

echo "CHANGELOG.md updated for $RELEASE."
