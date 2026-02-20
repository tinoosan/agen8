---
name: obsidian-cli
description: Manage a local Obsidian-compatible markdown vault as a Zettelkasten system with linked notes, MOCs, journals, and graph audits across sessions.
compatibility: "Requires bash and python3 only. No external non-standard dependencies."
---

# Instructions

Use this skill to persist durable knowledge in an Obsidian-style vault (plain markdown + `[[wikilinks]]`). The vault is filesystem-native and can be opened directly in Obsidian Desktop.

## When to use

- You learn something worth preserving across sessions.
- The user asks you to take notes, capture insights, or build long-term context.
- You are doing research and need traceable source-to-insight synthesis.
- At session start/end when continuity or recap is needed.

## Vault path resolution

Resolve vault path in this order:

1. `$OBSIDIAN_VAULT_PATH`
2. `~/.agents/vault.conf` (first non-empty non-comment line)
3. default `~/.agents/vault/`

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

- Fleeting: `F-` prefix, fast capture, temporary until processed.
- Literature: `L-` prefix, source-grounded summaries and claims.
- Permanent: `P-` prefix, atomic ideas in your own words.
- MOC: topic map/index notes under `mocs/`.
- Journal: session or daily logs under `journals/`.

## Naming convention

Use:

`{TYPE}-{YYYYMMDDHHMMSS}-{slug}.md`

Example:

`P-20260220150100-idempotent-pipelines.md`

## Required frontmatter

Every non-template note must include frontmatter:

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

## Workflow

1. Initialize vault with `scripts/vault_init.sh`.
2. Capture quick ideas in `inbox/` as fleeting notes.
3. Process inbox: merge duplicates, remove noise, promote valuable ideas.
4. Create literature notes from sources and connect them to MOCs.
5. Create permanent notes (one idea per note, own words, explicit links).
6. Update MOCs to keep topic navigation current.
7. Maintain journals for session continuity and decisions.
8. Run search/audit regularly to detect orphans, broken links, stale maps.

## Decision rules

- One permanent idea per note.
- Always link notes; avoid isolated notes.
- Prefer linking existing notes over creating near-duplicates.
- Frontmatter is mandatory for all non-template notes.
- Add an `up` link for literature/permanent notes to a parent MOC.

## Quality checks

- No orphan permanent/literature notes unless intentional.
- No broken wikilinks.
- Frontmatter present and parseable.
- Inbox remains small and processed regularly.
- MOCs reflect current key notes and relationships.

## Templates

### Fleeting

```markdown
---
id: "F-YYYYMMDDHHMMSS-slug"
type: "F"
title: ""
created: ""
tags: []
source: ""
up: ""
---

- Observation:
- Why it might matter:
- Link candidates:
```

### Literature

```markdown
---
id: "L-YYYYMMDDHHMMSS-slug"
type: "L"
title: ""
created: ""
tags: []
source: ""
up: "[[mocs/topic]]"
---

## Source summary
## Key claims
## Evidence / quotes
## Open questions
## Related notes
```

### Permanent

```markdown
---
id: "P-YYYYMMDDHHMMSS-slug"
type: "P"
title: ""
created: ""
tags: []
aliases: []
source: ""
up: "[[mocs/topic]]"
---

## Idea
## Rationale
## Implications
## Connected notes
```

### MOC

```markdown
---
id: "MOC-YYYYMMDDHHMMSS-topic"
type: "MOC"
title: ""
created: ""
tags: [moc]
---

## Scope
## Core notes
## Subtopics
## Gaps
```

### Journal

```markdown
---
id: "JOURNAL-YYYYMMDDHHMMSS-session"
type: "JOURNAL"
title: ""
created: ""
tags: [journal]
---

## Goals
## Work log
## Decisions
## Next actions
```

## Example walkthrough

1. Research a topic and capture raw points as fleeting notes in `inbox/`.
2. Convert source-backed points into literature notes in `notes/`.
3. Synthesize atomic permanent notes with explicit `[[wikilinks]]`.
4. Update `mocs/<topic>.md` with new note links.
5. Run graph audit and fix orphan/broken links.

## Anti-patterns

- Dumping raw text without synthesis.
- Creating orphan notes with no incoming or outgoing links.
- Over-tagging without meaningful link structure.
- Writing giant multi-concept notes instead of atomic notes.
- Letting MOCs go stale while notes proliferate.

## Scripts

- Initialize vault: `scripts/vault_init.sh [--path <dir>]`
- Search notes: `scripts/vault_search.sh [--tag ...] [--link ...] [--type ...] [--query ...] [--in ...] [--path ...]`
- Audit graph: `scripts/vault_graph.sh [--path <dir>] [--top <n>] [--json-pretty]`
