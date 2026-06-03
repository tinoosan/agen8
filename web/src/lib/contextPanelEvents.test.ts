import { describe, expect, it, vi } from 'vitest'
import {
  emitContextPanelOpenFile,
  onContextPanelOpenFile,
  pathFromChatHref,
} from './contextPanelEvents'

describe('pathFromChatHref', () => {
  it('returns local absolute paths', () => {
    expect(pathFromChatHref('/workspace/engineering/backend-engineer/agent.py')).toBe('/workspace/engineering/backend-engineer/agent.py')
  })

  it('accepts bare file names', () => {
    expect(pathFromChatHref('ContextPanel.tsx')).toBe('ContextPanel.tsx')
  })

  it('decodes escaped paths', () => {
    expect(pathFromChatHref('%2Fworkspace%2Fengineering%2FREADME.md')).toBe('/workspace/engineering/README.md')
  })

  it('supports file:// links', () => {
    expect(pathFromChatHref('file:///workspace/engineering/README.md')).toBe('/workspace/engineering/README.md')
  })

  it('strips line/column suffixes and fragments', () => {
    expect(pathFromChatHref('/workspace/engineering/README.md:12')).toBe('/workspace/engineering/README.md')
    expect(pathFromChatHref('/workspace/engineering/README.md:12:5')).toBe('/workspace/engineering/README.md')
    expect(pathFromChatHref('/workspace/engineering/README.md#L44')).toBe('/workspace/engineering/README.md')
  })

  it('normalizes workspace-relative chat references before opening files', () => {
    expect(pathFromChatHref('workspace/engineering/backend-engineer/network-scanner/phase2-cli-wiring.md:1')).toBe(
      '/workspace/engineering/backend-engineer/network-scanner/phase2-cli-wiring.md',
    )
    expect(pathFromChatHref('./workspace/engineering/README.md#L44')).toBe('/workspace/engineering/README.md')
    expect(pathFromChatHref('playground/workspace/engineering/backend-engineer/network-scanner/phase1-failure-behavior.md')).toBe(
      '/workspace/engineering/backend-engineer/network-scanner/phase1-failure-behavior.md',
    )
    expect(pathFromChatHref('playground/AGENT_WISHLIST.md')).toBe('playground/AGENT_WISHLIST.md')
  })

  it('rejects web links', () => {
    expect(pathFromChatHref('https://example.com')).toBeNull()
    expect(pathFromChatHref('http://example.com')).toBeNull()
    expect(pathFromChatHref('mailto:person@example.com')).toBeNull()
  })

  it('rejects separator-only path tokens', () => {
    expect(pathFromChatHref('/')).toBeNull()
    expect(pathFromChatHref('//')).toBeNull()
    expect(pathFromChatHref('./')).toBeNull()
    expect(pathFromChatHref('../')).toBeNull()
    expect(pathFromChatHref('file:///')).toBeNull()
  })
})

describe('context panel open-file events', () => {
  it('publishes sanitized path payloads', () => {
    const handler = vi.fn()
    const unsubscribe = onContextPanelOpenFile(handler)

    emitContextPanelOpenFile({ path: '  /workspace/repo/file.ts  ' })

    expect(handler).toHaveBeenCalledTimes(1)
    expect(handler).toHaveBeenCalledWith({ path: '/workspace/repo/file.ts' })

    unsubscribe()
  })
})
