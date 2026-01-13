// Package types defines Workbench's core data model types and protocols.
//
// Tool protocol overview (explicit /tools + /results usage)
//
// The host/agent interacts with tools and tool outputs through two VFS mounts:
//   - /tools   (discovery + manifest reads)
//   - /results (responses + artifacts produced by tool calls)
//
// /tools: discovery + manifest only
//
// VFS API contract:
//
//   - fs.List("/tools")
//     => returns tool IDs as directory-like entries
//     e.g. "/tools/github.com.acme.stock"
//
//   - fs.Read("/tools/<toolId>")
//     => returns ONLY the tool manifest JSON bytes
//
// Notes:
//   - The agent does NOT need to know "manifest.json" as a filename.
//   - The VFS may also accept fs.Read("/tools/<toolId>/manifest.json") for explicitness.
//   - The agent should not list inside tool directories; the manifest is the interface surface.
//
// Tool storage (implementation detail; should be invisible to the agent):
//   - Builtins may be in-memory but still appear under /tools.
//   - Custom tools may exist on disk as:
//     data/tools/<toolId>/manifest.json
//     (some deployments may choose:
//     data/tools/custom/<toolId>/manifest.json
//     but the VFS interface remains the same).
//
// /results: Pattern A (callId-first)
//
// After a tool call finishes, outputs are stored under a call directory keyed by callId:
//   - /results/<callId>/response.json
//   - /results/<callId>/<artifact.Path>        (zero or more files)
//   - /results/index.jsonl                     (append-only index; optional)
//
// "artifact" means a file produced by the tool call that can be read later (JSON, CSV, PNG, etc).
// ToolResponse.Artifacts contains paths RELATIVE to the call directory (e.g. "quote.json" or "artifacts/quote.json").
//
// On-disk examples (implementation detail; for a specific runId):
//
//	data/runs/<runId>/results/<callId>/response.json
//	data/runs/<runId>/results/<callId>/<artifact.Path>
//
// Rationale for Pattern A:
//   - avoids encoding tool IDs into path segments
//   - guarantees uniqueness via callId
//   - supports concurrency cleanly
//   - tool identity is recorded in response.json (toolId/actionId), not inferred from directory structure
package types
