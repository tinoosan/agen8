/**
 * sessionResume — turn a member's stored harness session into a way to get back
 * into it.
 *
 * Researched reality (see decision dec-ab7c6f66):
 *  - Claude Code has NO resume deep link. Resuming is CLI-only — `claude
 *    --resume <id>` — and it's scoped to the project directory, so the command
 *    has to run there. We prefix `cd <project root>`.
 *  - Codex resumes by id from any directory — `codex resume <id>`. The codex-rs
 *    CLI has no URL scheme, but the Codex *desktop app* registers a semi-official
 *    `codex://threads/<id>` link, which we offer as a best-effort extra.
 *
 * The id we resume with is the member's nativeSessionRef, which is the harness
 * session UUID for claude-code/codex. Other ref shapes agen8 stores — bridge-…,
 * token:…, hand-typed labels — are NOT resumable, so we gate on a UUID shape and
 * a known harness and surface nothing otherwise. Better no affordance than a
 * command that can't work.
 */

export type ResumeHarness = 'claude-code' | 'codex'

export interface ResumeInfo {
  harness: ResumeHarness
  /** The one-line command to copy and run. */
  command: string
  /** Human label for the harness, e.g. "Claude Code". */
  harnessLabel: string
  /** Set when the command must run in a specific directory (Claude Code). */
  cwdNote?: string
  /** Best-effort app deep link (Codex desktop app only). */
  appDeepLink?: string
}

// 8-4-4-4-12 hex. Matches Claude's UUIDv4 and Codex's UUIDv7 session ids, and
// excludes bridge-<hex>, token:<hash>, and hand-typed refs.
const SESSION_UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

function isResumableHarness(kind: string | undefined): kind is ResumeHarness {
  return kind === 'claude-code' || kind === 'codex'
}

/** True when this member's session can actually be resumed. */
export function isResumableSession(
  harnessKind: string | undefined,
  nativeSessionRef: string | undefined,
): boolean {
  return isResumableHarness(harnessKind) && SESSION_UUID.test((nativeSessionRef ?? '').trim())
}

// Single-quote for POSIX shells, escaping embedded quotes — same approach as
// lib/mcpSetup.ts so the snippets behave consistently.
function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\"'\"'")}'`
}

/**
 * Build the resume affordance for a member, or null when it isn't resumable.
 *
 * @param projectRoot the project's working directory — required for Claude Code
 *   (its resume is scoped to that dir); ignored for Codex.
 */
export function buildSessionResume(
  harnessKind: string | undefined,
  nativeSessionRef: string | undefined,
  projectRoot?: string | null,
): ResumeInfo | null {
  if (!isResumableSession(harnessKind, nativeSessionRef)) return null
  const id = (nativeSessionRef ?? '').trim()

  if (harnessKind === 'claude-code') {
    const root = (projectRoot ?? '').trim()
    const command = root
      ? `cd ${shellQuote(root)} && claude --resume ${id}`
      : `claude --resume ${id}`
    return {
      harness: 'claude-code',
      harnessLabel: 'Claude Code',
      command,
      cwdNote: root
        ? `Runs in ${root} — Claude Code resumes are scoped to the project directory.`
        : 'Run this in the project directory — Claude Code resumes are scoped to it.',
    }
  }

  // codex — resumes from any directory; offer the desktop-app link as a bonus.
  return {
    harness: 'codex',
    harnessLabel: 'Codex',
    command: `codex resume ${id}`,
    appDeepLink: `codex://threads/${id}`,
  }
}
