import { describe, it, expect } from 'vitest'
import { buildSessionResume, isResumableSession } from './sessionResume'

const CLAUDE_UUID = '1af9287d-2b8b-48a7-8e3c-ec2e4c8d8746'
const CODEX_UUID = '0199d68d-14ef-70c0-bf1e-4b001a0992c1' // UUIDv7 shape

describe('isResumableSession', () => {
  it('accepts a UUID ref for claude-code and codex', () => {
    expect(isResumableSession('claude-code', CLAUDE_UUID)).toBe(true)
    expect(isResumableSession('codex', CODEX_UUID)).toBe(true)
  })

  it('rejects non-session refs and unknown harnesses', () => {
    expect(isResumableSession('claude-code', 'bridge-a1b2c3d4')).toBe(false)
    expect(isResumableSession('claude-code', 'token:abcdef0123')).toBe(false)
    expect(isResumableSession('claude-code', 'my hand typed ref')).toBe(false)
    expect(isResumableSession('bridge', CLAUDE_UUID)).toBe(false)
    expect(isResumableSession('unknown', CLAUDE_UUID)).toBe(false)
    expect(isResumableSession(undefined, CLAUDE_UUID)).toBe(false)
    expect(isResumableSession('claude-code', undefined)).toBe(false)
  })
})

describe('buildSessionResume', () => {
  it('returns null when not resumable', () => {
    expect(buildSessionResume('bridge', CLAUDE_UUID, '/repo')).toBeNull()
    expect(buildSessionResume('claude-code', 'token:x', '/repo')).toBeNull()
  })

  it('claude-code: cd into the project root then claude --resume', () => {
    const info = buildSessionResume('claude-code', CLAUDE_UUID, '/work/app')
    expect(info).not.toBeNull()
    expect(info!.harness).toBe('claude-code')
    expect(info!.command).toBe(`cd '/work/app' && claude --resume ${CLAUDE_UUID}`)
    expect(info!.cwdNote).toContain('/work/app')
    expect(info!.appDeepLink).toBeUndefined()
  })

  it('claude-code: falls back to a bare command + generic note when no root', () => {
    const info = buildSessionResume('claude-code', CLAUDE_UUID, null)
    expect(info!.command).toBe(`claude --resume ${CLAUDE_UUID}`)
    expect(info!.cwdNote).toMatch(/project directory/i)
  })

  it('claude-code: shell-quotes a root with spaces/quotes safely', () => {
    const info = buildSessionResume('claude-code', CLAUDE_UUID, "/work/it's here")
    expect(info!.command).toBe(`cd '/work/it'"'"'s here' && claude --resume ${CLAUDE_UUID}`)
  })

  it('codex: resumes from any dir and offers the desktop-app link', () => {
    const info = buildSessionResume('codex', CODEX_UUID, '/work/app')
    expect(info!.harness).toBe('codex')
    expect(info!.command).toBe(`codex resume ${CODEX_UUID}`)
    expect(info!.command).not.toContain('cd ')
    expect(info!.cwdNote).toBeUndefined()
    expect(info!.appDeepLink).toBe(`codex://threads/${CODEX_UUID}`)
  })
})
