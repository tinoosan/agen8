import { describe, expect, it } from 'vitest'
import { buildMCPSetup } from './mcpSetup'

describe('buildMCPSetup', () => {
  it('builds copy-ready MCP snippets from a token', () => {
    const setup = buildMCPSetup('ak_test_secret', 'http://127.0.0.1:7777')

    expect(setup.url).toBe('http://127.0.0.1:7777/mcp?token=ak_test_secret')
    expect(JSON.parse(setup.jsonConfig)).toEqual({
      mcpServers: {
        agen8: {
          type: 'http',
          url: 'http://127.0.0.1:7777/mcp?token=ak_test_secret',
        },
      },
    })
    expect(setup.codexCommand).toBe("codex mcp add agen8 --url 'http://127.0.0.1:7777/mcp?token=ak_test_secret'")
    expect(setup.claudeCommand).toBe("claude mcp add --transport http --scope user agen8 'http://127.0.0.1:7777/mcp?token=ak_test_secret'")
  })

  it('turns wildcard listener origins into a usable loopback URL', () => {
    const setup = buildMCPSetup('ak_test_secret', 'http://0.0.0.0:7777')

    expect(setup.url).toBe('http://127.0.0.1:7777/mcp?token=ak_test_secret')
  })

  it('turns IPv6 wildcard listener origins into a usable loopback URL', () => {
    const setup = buildMCPSetup('ak_test_secret', 'http://[::]:7777')

    expect(setup.url).toBe('http://127.0.0.1:7777/mcp?token=ak_test_secret')
  })

  it('requires a token', () => {
    expect(() => buildMCPSetup('   ', 'http://127.0.0.1:7777')).toThrow('MCP token is required')
  })
})
