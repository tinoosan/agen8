import { describe, expect, it } from 'vitest'
import { buildMCPSetup } from './mcpSetup'

describe('buildMCPSetup', () => {
  it('builds copy-ready MCP snippets from a token', () => {
    const setup = buildMCPSetup('ak_test_secret', 'http://127.0.0.1:7777')

    expect(setup.url).toBe('http://127.0.0.1:7777/mcp')
    expect(setup.compatibilityUrl).toBe('http://127.0.0.1:7777/mcp?token=ak_test_secret')
    expect(JSON.parse(setup.jsonConfig)).toEqual({
      mcpServers: {
        agen8: {
          type: 'http',
          url: 'http://127.0.0.1:7777/mcp',
          bearer_token_env_var: 'AGEN8_MCP_TOKEN',
        },
      },
    })
    expect(setup.codexCommand).toBe("export AGEN8_MCP_TOKEN='ak_test_secret'\ncodex mcp add agen8 --url 'http://127.0.0.1:7777/mcp' --bearer-token-env-var AGEN8_MCP_TOKEN")
    expect(setup.claudeCommand).toBe("agen8 client setup --harness claude --url 'http://127.0.0.1:7777' --token 'ak_test_secret'")
    expect(setup.hooksClaudeCommand).toBe("agen8 hooks install --harness claude --url 'http://127.0.0.1:7777' --token 'ak_test_secret'")
    expect(setup.hooksCodexCommand).toBe("agen8 hooks install --harness codex --url 'http://127.0.0.1:7777' --token 'ak_test_secret'")
  })

  it('turns wildcard listener origins into a usable loopback URL', () => {
    const setup = buildMCPSetup('ak_test_secret', 'http://0.0.0.0:7777')

    expect(setup.url).toBe('http://127.0.0.1:7777/mcp')
    expect(setup.compatibilityUrl).toBe('http://127.0.0.1:7777/mcp?token=ak_test_secret')
  })

  it('turns IPv6 wildcard listener origins into a usable loopback URL', () => {
    const setup = buildMCPSetup('ak_test_secret', 'http://[::]:7777')

    expect(setup.url).toBe('http://127.0.0.1:7777/mcp')
    expect(setup.compatibilityUrl).toBe('http://127.0.0.1:7777/mcp?token=ak_test_secret')
  })

  it('requires a token', () => {
    expect(() => buildMCPSetup('   ', 'http://127.0.0.1:7777')).toThrow('MCP token is required')
  })
})
