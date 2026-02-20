---
name: obsidian-cli
description: Obsidian knowledge-management conventions for durable markdown notes, links, and graph hygiene. Use this as methodology guidance; prefer the runtime's native obsidian tool when available.
compatibility: "Harness-agnostic. Works with any environment that can read/write markdown files and wikilinks."
---

# Obsidian Knowledge Workflow (Harness-Agnostic)

Use this skill for methodology and quality standards. If your runtime provides a native `obsidian` tool, use that tool first for init/search/graph/upsert operations.

## Path policy

- Use the runtime-configured durable knowledge root (for example `/knowledge` or the host's configured vault path).
- Never store durable vault data in ephemeral run-scoped scratch locations.
- If your harness exposes both durable and ephemeral mounts, keep Obsidian vault data on the durable mount only.

## Vault structure

```text
vault/
  inbox/        # Fleeting notes (quick captures)
  notes/        # Permanent + literature notes (atomic, refined)
  mocs/         # Maps of Content (topic indexes)
  journals/     # Session/daily journals
  templates/    # Note templates
```

## Note types

- Fleeting: `F-`
- Literature: `L-`
- Permanent: `P-`
- MOC
- Journal

## Naming

`{TYPE}-{YYYYMMDDHHMMSS}-{slug}.md`

## Required frontmatter

```yaml
---
id: "P-20260220150100-idempotent-pipelines"
type: "P"
title: "Idempotent Pipelines"
created: "2026-02-20T15:01:00Z"
tags: [data-eng, reliability]
aliases: ["safe-reruns"]
source: ""
up: "[[mocs/data-engineering]]"
---
```

## Rules

- One idea per permanent note.
- Prefer linking existing notes over creating duplicates.
- Every refined note must have frontmatter.
- Keep MOCs current.
- Keep inbox small by regularly promoting or deleting fleeting notes.

## Quality checks

- No orphan notes (unless intentionally isolated).
- No broken wikilinks.
- Frontmatter present and parseable.
- MOCs reflect current topic structure.

## Anti-patterns

- Raw dumps without synthesis.
- Giant multi-concept notes.
- Over-tagging without links.
- Stale MOCs.
