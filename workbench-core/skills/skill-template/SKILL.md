---
name: skill-template
description: Template for creating new skills
---
# Skill Template
Copy this structure when creating a new skill.

## File Structure
my-skill/ SKILL.md # Required: Main instruction file
scripts/ # Optional: Helper scripts
examples/ # Optional: Reference implementations
resources/ # Optional: Templates, assets

## SKILL.md Format
```yaml
---
name: Human-Readable Skill Name
description: One-line summary of what this skill does
---
# Skill Name
## When to Use
Describe the situations where this skill applies.

## Instructions
Step-by-step instructions for executing this skill.

## Examples
Concrete examples of skill application.
```

## User Review Required
> [!IMPORTANT]
> **Writable Skills Location**: Should agent-created skills go to:
> - **Global data dir** (`~/.workbench/skills/`) - persists across all projects
> - **Workdir** ([./skills/](file:///Users/santinoonyeme/personal/dev/Projects/workbench/workbench-core/skills)) - project-specific, version controlled
> - **Configurable** - let the agent/user choose per skill?
> [!WARNING]
> **Security Consideration**: Agent-created skills could contain malicious instructions if the agent is compromised. Consider:
> - Requiring user confirmation before skill creation
> - Sandboxing skill scripts
> - Adding a "review pending skills" workflow
---
