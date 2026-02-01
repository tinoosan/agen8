// Package skills manages the skills system for the workbench.
//
// Skills are reusable capabilities that the agent can discover and use. They are
// defined as markdown files within the `/skills` VFS mount (backed by
// <dataDir>/skills/<skill_name>.md on disk).
//
// # Key Responsibilities
//
//   - Discovery: Scanning the skills directory to find available skills.
//   - Validation: Ensuring skills follow the required structure and metadata format.
//   - Exposure: Mapping skills to the VFS so the agent can read instructions.
package skills
