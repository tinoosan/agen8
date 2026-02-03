// Package skills manages the skills system for the workbench.
//
// Skills are reusable capabilities that the agent can discover and use. They are
// defined as directories within the `/skills` VFS mount (backed by
// <dataDir>/skills/<skill_name>/SKILL.md on disk).
//
// # Key Responsibilities
//
//   - Discovery: Scanning the skills directory to find available skills.
//   - Validation: Ensuring skills follow the required structure and metadata format.
//   - Exposure: Mapping skills to the VFS so the agent can read instructions.
//
// # Package Structure
//
//   - `Manager`: orchestrates skill discovery, caching, and provider lifecycle.
//   - `Provider`: abstracts where skill definitions live (filesystem, HTTP, etc.).
//   - `Skill`: lightweight metadata driven by `SKILL.md` plus optional resources under the skill directory.
//   - `Resource`: the runtime exposes skill files under `/skills/<name>` so constructors can render instructions.

package skills
