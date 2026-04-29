#!/bin/bash
set -e

declare -a LABELS=(
  "Needs Review|FBCA04|Awaiting review and approval"
  "P0|B60205|Critical / foundational"
  "P1|D93F0B|Important"
  "P2|FEF2C0|Nice-to-have"
  "core|0E8A16|Core engine"
  "router|1D76DB|Model routing and escalation"
  "identity|5319E7|SOUL / USER identity"
  "memory|6F42C1|Memory layer"
  "hooks|C5DEF5|Hook system"
  "workflow|0052CC|Workflow engine"
  "computer-use|D4C5F9|macOS computer use"
  "mobile|F9D0C4|Mobile device control"
  "widget|FBCA04|iPhone / Watch widgets"
  "voice|BFD4F2|Voice STT and TTS"
  "video|E99695|Conduit Video"
  "design-system|F9D0C4|Conduit Design system"
  "tui|C2E0C6|Terminal UI"
  "gui|C2E0C6|macOS GUI app"
  "spotlight|C2E0C6|Spotlight overlay"
  "ide|C2E0C6|Mini IDE"
  "coding-agent|0E8A16|Coding agent engine"
  "plugin|D4C5F9|Plugin system"
  "skills|D4C5F9|Skills registry"
  "security|B60205|Security and safety"
  "sandbox|B60205|Sandboxed execution"
  "performance|FEF2C0|Token efficiency / performance"
  "analytics|1D76DB|Usage tracking and cost"
  "installation|C5DEF5|Setup and local models"
  "docs|0075CA|Documentation"
  "infra|BFD4F2|Infrastructure and tooling"
  "collab|FBCA04|Collaboration and channels"
  "design-task|FEF2C0|Design / mockup task"
  "eval|1D76DB|Evaluation framework"
  "remote|C5DEF5|Remote / pairing"
  "accessibility|0075CA|Accessibility"
)

for L in "${LABELS[@]}"; do
  NAME=$(echo "$L" | cut -d'|' -f1)
  COLOR=$(echo "$L" | cut -d'|' -f2)
  DESC=$(echo "$L" | cut -d'|' -f3)
  if gh label create "$NAME" --color "$COLOR" --description "$DESC" 2>&1 | grep -q "already exists"; then
    gh label edit "$NAME" --color "$COLOR" --description "$DESC" >/dev/null 2>&1 || true
  fi
done

echo "Created/updated labels:"
gh label list --limit 100 | wc -l
