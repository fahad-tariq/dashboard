#!/usr/bin/env bash
# migrate-sections.sh -- Convert tracker.md section headers into inline [tags: ...].
#
# Usage:
#   ./scripts/migrate-sections.sh tracker.md           # preview (dry-run)
#   ./scripts/migrate-sections.sh tracker.md --apply    # overwrite file

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <tracker.md> [--apply]" >&2
  exit 1
fi

file="$1"
apply=false
if [[ "${2:-}" == "--apply" ]]; then
  apply=true
fi

if [[ ! -f "$file" ]]; then
  echo "error: file not found: $file" >&2
  exit 1
fi

section=""
output=""

while IFS= read -r line || [[ -n "$line" ]]; do
  trimmed="${line#"${line%%[![:space:]]*}"}"

  # Section header: ## Name
  if [[ "$trimmed" =~ ^##[[:space:]]+(.+)$ ]]; then
    section="${BASH_REMATCH[1]}"
    section="${section,,}" # lowercase
    continue
  fi

  # Skip top-level heading
  if [[ "$trimmed" =~ ^#[[:space:]] ]]; then
    output+="$line"$'\n'
    continue
  fi

  # Checkbox line: - [ ] or - [x]
  if [[ "$trimmed" =~ ^-[[:space:]]\[[[:space:]xX]\][[:space:]] ]] && [[ -n "$section" ]]; then
    # Check if line already has [tags: ...]
    if [[ "$line" =~ \[tags:[[:space:]]*([^]]*)\] ]]; then
      existing="${BASH_REMATCH[1]}"
      # Check if section tag already present (case-insensitive)
      already=false
      IFS=',' read -ra parts <<< "$existing"
      for part in "${parts[@]}"; do
        part="${part#"${part%%[![:space:]]*}"}"
        part="${part%"${part##*[![:space:]]}"}"
        if [[ "${part,,}" == "$section" ]]; then
          already=true
          break
        fi
      done
      if [[ "$already" == false ]]; then
        # Append section to existing tags
        line="${line/\[tags: $existing\]/[tags: $existing, $section]}"
      fi
    else
      # No tags yet -- add before any trailing newline
      line="$line [tags: $section]"
    fi
  fi

  output+="$line"$'\n'
done < "$file"

if [[ "$apply" == true ]]; then
  printf '%s' "$output" > "$file"
  echo "migrated: $file" >&2
else
  printf '%s' "$output"
fi
