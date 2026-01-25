// Package skills manages the skills system for the workbench.
//
// Skills are reusable capabilities that the agent can discover and use. They are
// typically defined as directories within the `/skills` VFS mount, containing a
// `SKILL.md` instruction file and optional helper scripts or resources.
//
// # Key Responsibilities
//
//   - Discovery: Scanning the skills directory to find available skills.
//   - Validation: Ensuring skills follow the required structure and metadata format.
//   - Exposure: Mapping skills to the VFS so the agent can read instructions.
package skills
