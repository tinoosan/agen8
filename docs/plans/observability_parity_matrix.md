# Observability Parity Matrix

This matrix tracks parity between the unified monitor and the modular CLI surfaces.

| Monitor Surface | Modular Command | Backing RPC |
| --- | --- | --- |
| Team/session list | `agen8 sessions list`, `agen8 dashboard` | `session.list` |
| Team status (pending/active/done) | `agen8 dashboard` | `team.getStatus` |
| Coordinator run focus | `agen8 coordinator`, `agen8 attach` | `team.getManifest`, `agent.list` |
| Session totals (tokens/cost/tasks) | `agen8 dashboard` | `session.getTotals` |
| Event feed | `agen8 logs`, `agen8 activity` | `logs.query`, `activity.stream`, `events.latestSeq` |
| Run activity stream | `agen8 activity --follow` | `activity.stream` |
| Session lifecycle controls | `agen8 sessions pause|resume|stop|delete` | `session.pause|resume|stop|delete` |
| Project-local active context | `agen8 whoami`, `agen8 config` | `project.getContext`, `project.setActiveSession` |

## Regression expectation

- Keep `agen8 monitor` behavior intact.
- Any data visible in monitor should be available via at least one modular command.
