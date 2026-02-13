---
name: Reporting
description: Produce concise, structured writeups and status updates suitable for handoff.
---

# Instructions

Use this skill when the user needs a clear artifact (summary, memo, report, or status update) that captures decisions, current state, and what happens next.

## When to use

- The scope covers multiple topics, stakeholders, or deliverables that need coordination.
- The user asks for a written summary, briefing, or report.
- You need to record progress, blockers, or outstanding questions for handoff.
- executive_summary or postmortem is needed after major work.

## Workflow

1. **State objective and scope.** Explain why the document exists and what timeframe or pieces it covers.
2. **Collect facts.** Summarize relevant data, status points, or analysis results from your work.
3. **Organize findings.** Use clear sections (e.g., Overview, Progress, Decisions, Risks, Next Steps) and include bullet lists as needed.
4. **Highlight decisions and rationale.** Specify what has been decided, who owns it, and why.
5. **Mention blockers and risks.** Call out what is blocking progress or what could derail the plan.
6. **List next steps and owners.** Be explicit about what will happen next, by whom, and by when.
7. **Keep tone concise and action-oriented.** Aim for clarity; avoid long-winded prose.

## Decision rules

- If multiple stakeholders are involved, state who is responsible for each decision or deliverable.
- When clarity is uncertain, include a "Questions" or "Assumptions" section.
- Avoid detailed technical data unless it aids comprehension; reference attachments if necessary.
- Use visual aids (tables, charts, timelines) when they clarify complex information.

## Quality checks

- Document includes objective, summary, risks, and next actions.
- Reader can understand the status without additional context.
- No contradictory statements appear; ensure dates and owners are consistent.
- All acronyms and technical terms are defined on first use.
- Action items have clear owners and deadlines.

## Templates

### Executive Summary
```markdown
# <Project Name> Status Report

**Date**: YYYY-MM-DD
**Author**: <Name>
**Audience**: <Leadership/Team/Stakeholders>

## Summary
<2-3 sentences: What is this about and what's the current state>

## Key Achievements
- <Milestone 1>
- <Milestone 2>
- <Milestone 3>

## Decisions Made
| Decision | Rationale | Owner | Date |
|----------|-----------|-------|------|
| <what>   | <why>     | <who> | <when> |

## Current Blockers
1. **<Blocker>**: <description> (Owner: <name>, ETA: <date>)
2. **<Blocker>**: <description> (Owner: <name>, ETA: <date>)

## Risks & Mitigation
| Risk | Impact | Likelihood | Mitigation |
|------|--------|-----------|------------|
| <what> | High/Med/Low | High/Med/Low | <action> |

## Next Steps
- [ ] <Action item> (Owner: <name>, Due: <date>)
- [ ] <Action item> (Owner: <name>, Due: <date>)

## Open Questions
1. <Question that needs stakeholder input>
2. <Question that needs stakeholder input>
```

### Sprint/Weekly Update
```markdown
# Week of <Date Range>

## Completed
- ✅ <Item> - <brief description or link>
- ✅ <Item> - <brief description or link>

## In Progress
- 🔄 <Item> - <status, % complete, blockers>
- 🔄 <Item> - <status, % complete, blockers>

## Upcoming (Next Week)
- 📅 <Item> - <planned start>
- 📅 <Item> - <planned start>

## Blockers
- 🚫 <blocker> - <impact, who can unblock>

## Metrics
- <Metric name>: <value> (goal: <target>)
- <Metric name>: <value> (goal: <target>)
```

### Postmortem
```markdown
# Postmortem: <Incident Name>

**Date**: <YYYY-MM-DD>
**Duration**: <start time> - <end time> (<X hours>)
**Severity**: Critical/High/Medium/Low
**Impact**: <# users affected, $ revenue lost, etc.>

## What Happened
<Concise narrative of the incident>

## Timeline
| Time | Event |
|------|-------|
| 10:15 | <First sign of issue> |
| 10:20 | <Detection/alerting> |
| 10:45 | <Mitigation started> |
| 11:30 | <Issue resolved> |

## Root Cause
<Technical explanation of what caused the incident>

## What Went Well
- <Action that helped mitigate>
- <Process that worked>

## What Went Wrong
- <Gap that allowed this to happen>
- <Delay in detection/response>

## Action Items
- [ ] <Prevent recurrence> (Owner: <name>, Due: <date>)
- [ ] <Improve detection> (Owner: <name>, Due: <date>)
- [ ] <Update runbook> (Owner: <name>, Due: <date>)

## Lessons Learned
<Key takeaways for the team>
```

### Meeting Notes
```markdown
# <Meeting Name>

**Date**: YYYY-MM-DD
**Attendees**: <names>
**Facilitator**: <name>
**Notes**: <name>

## Agenda
1. <Topic>
2. <Topic>
3. <Topic>

## Discussion Notes

### <Topic 1>
- <Key point discussed>
- <Decision or outcome>

### <Topic 2>
- <Key point discussed>
- <Decision or outcome>

## Decisions
- ✅ <Decision> (Owner: <name>)
- ✅ <Decision> (Owner: <name>)

## Action Items
- [ ] <Action> (Owner: <name>, Due: <date>)
- [ ] <Action> (Owner: <name>, Due: <date>)

## Parking Lot (deferred topics)
- <Topic for future discussion>
```

## Example

**User request**: "Summarize the Q2 launch status"

**Report**:
```markdown
# Q2 Product Launch - Status Report

**Date**: 2026-04-15
**Status**: On Track with Caveats

## Summary
Beta testers reported API 2.0 latency at 180ms (goal <120ms). Decision: delay release 1 week to fix caching; CS leads notifying clients.

## Risks
- **Missing marketing window**: If patch takes >5 days, we miss keynote mention
- **Mitigation**: Engineering committing weekend sprint if needed

## Next Steps
- Engineering to patch cache by Friday (Owner: Jane)
- Testing to validate over weekend (Owner: Mike)
- Marketing to coordinate new launch date (Owner: Sarah)
```

## Advanced techniques

### Pyramid Principle

Structure reports with **conclusion first**:
1. **Start with the answer** - What's the recommendation/status?
2. **Support with key points** - Why this conclusion?
3. **Provide details** - Data/facts backing each point

Readers can stop at any level and still understand.

### SCQA Framework

For persuasive reports:
- **Situation**: Current context
- **Complication**: Problem/opportunity
- **Question**: What should we do?
- **Answer**: Recommended action

### Traffic Light Status

Use visual indicators:
- 🟢 **On track** - No concerns
- 🟡 **At risk** - Needs attention
- 🔴 **Blocked** - Critical intervention needed

## Anti-patterns

- **Novel-length updates**: Keep it to 1-2 pages max for status reports
- **Burying the lede**: Don't hide critical info at the end
- **Vague action items**: "Look into X" is not actionable - specify what and when
- **Missing context**: Don't assume readers have all background
- **Stale data**: Always date-stamp reports and reference latest numbers

## When NOT to use

- Quick Slack updates (just send the message)
- Personal notes/scratch work (use memory instead)
- Code documentation (belongs in code comments/README)
- Real-time collaboration (use chat/whiteboard instead)

## Scripts

This skill includes helper scripts in the `scripts/` directory for converting reports to deliverable formats.

| Script | Purpose | Example |
|--------|---------|---------|
| `md_to_pdf.sh` | Convert Markdown to PDF | `./scripts/md_to_pdf.sh report.md --style github` |
| `md_to_docx.sh` | Convert Markdown to Word | `./scripts/md_to_docx.sh report.md --toc` |
| `md_to_html.sh` | Convert Markdown to styled HTML | `./scripts/md_to_html.sh report.md --style dark --title "Q2 Report"` |
| `chart_gen.py` | Generate charts from CSV/JSON data | `python3 scripts/chart_gen.py data.csv --type bar --x month --y revenue` |

**Workflow**: Write the report as Markdown, then use scripts to convert to the target format.

**Styles available** for HTML/PDF: `default`, `github`, `dark`, `minimal`

**Chart types**: `bar`, `hbar`, `line`, `area`, `scatter`, `pie` — add `--dark` for dark mode.

**Dependencies** (auto-detected with fallbacks):
- PDF: `pandoc` + LaTeX, or `weasyprint`, or `markdown` (HTML fallback)
- DOCX: `pandoc`, or `python-docx`
- Charts: `matplotlib`
