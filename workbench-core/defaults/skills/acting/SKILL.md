---
name: Acting
description: Self-provision missing dependencies, runtimes, or services so the agent can continue working autonomously.
---

# Instructions

Use this skill whenever the task depends on software the workspace does not already provide—missing packages, runtimes, language tooling, or service endpoints. It teaches the agent how to diagnose the gap, install what is needed, and verify the new environment before resuming higher-level work.

## When to use

- The logs mention “command not found”, “module missing”, or similar runtime failures for packages, binaries, or interpreters.
- The user asks for new infrastructure (Python virtualenv, Node toolchain, Docker image, etc.) before other coding work can proceed.
- An existing script or build step fails because it expects a dependency that is not installed.

## Workflow

1. **Diagnose clearly.** Inspect the failing command/output, check `which`, `go env`, `pip list`, or equivalent, and confirm the missing dependency/runtime. Capture the reason why the workspace cannot satisfy the task today.
2. **Prefer lean installation commands.** Choose the package manager that best matches the dependency (`apt`, `yum`, `brew`, `pip`, `pipx`, `npm`, `pnpm`, `go install`, etc.). When multiple managers might work, prefer the one already used in this repo (check `package.json`, `pyproject.toml`, Dockerfiles, etc.).
3. **Automate environment creation.** When the task needs an isolated environment (Python venv, Node nvm, Go workspace, etc.), create it programmatically (`python -m venv`, `nvm install`, `go env -w`, etc.), activate it (, ), and record the commands in `/plan/HEAD.md` or in the plan checklist so subsequent steps know where to find the tools.
4. **Install the dependency.** Run the install command and confirm success via version checks (e.g., `pip show`, `npm list <pkg>`, `python --version`). If installation fails, capture the error message and annotate the plan so troubleshooting can continue.
5. **Document the change.** Update `/plan/HEAD.md` or `/plan/CHECKLIST.md` with a summary of what was provisioned and why; include the exact commands used so future walkthroughs reproduce the same environment.

## Decision rules

- Do not proceed with higher-level work until the required tooling can actually run locally in this run (try the failing command once the install completes).
- When a dependency exists but has multiple versions, pick the version range the repo already expects (check lock files or docs) and justify any deviation in the plan notes.
- If a centralized dependency (e.g., system service) cannot be installed, explain why it is blocked and propose manual instructions the user can run.

## Quality checks

- The new tooling must be callable from the agent’s host commands (`shell.exec`, `fs.read` to confirm newly created files).
- Retest the scenario that triggered the installation to prove the failure disappears.
- Keep the plan/notes updated so later turns know what toolchain is now available.
