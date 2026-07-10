const SERVER_NAME = 'agen8'

export const CODEX_SKILL_COMMAND = 'agen8 skill install --harness codex'
export const CLAUDE_SKILL_COMMAND = 'agen8 skill install --harness claude-cli'

export interface MCPSetupSnippets {
  url: string
  compatibilityUrl: string
  jsonConfig: string
  codexCommand: string
  claudeCommand: string
  /** Provisions the agen8 hooks ("waiting on you" alerts) for Claude Code. */
  hooksClaudeCommand: string
  /** Provisions the agen8 hooks for Codex (user-level ~/.codex/hooks.json). */
  hooksCodexCommand: string
}

export function buildMCPSetup(secret: string, origin = browserOrigin()): MCPSetupSnippets {
  const token = secret.trim()
  if (!token) {
    throw new Error('MCP token is required')
  }
  const url = buildMCPURL(origin)
  const compatibilityUrl = buildMCPCompatibilityURL(token, origin)
  const daemonOrigin = normalizeOrigin(origin)
  return {
    url,
    compatibilityUrl,
    jsonConfig: JSON.stringify({
      mcpServers: {
        [SERVER_NAME]: {
          type: 'http',
          url,
          bearer_token_env_var: 'AGEN8_MCP_TOKEN',
        },
      },
    }, null, 2),
    codexCommand: `export AGEN8_MCP_TOKEN=${shellQuote(token)}\ncodex mcp add ${SERVER_NAME} --url ${shellQuote(url)} --bearer-token-env-var AGEN8_MCP_TOKEN`,
    claudeCommand: `agen8 client setup --harness claude --url ${shellQuote(daemonOrigin)} --token ${shellQuote(token)}`,
    hooksClaudeCommand: `agen8 hooks install --harness claude --url ${shellQuote(daemonOrigin)} --token ${shellQuote(token)}`,
    hooksCodexCommand: `agen8 hooks install --harness codex --url ${shellQuote(daemonOrigin)} --token ${shellQuote(token)}`,
  }
}

function buildMCPURL(origin: string): string {
  const base = normalizeOrigin(origin)
  const url = new URL('/mcp', base)
  return url.toString()
}

function buildMCPCompatibilityURL(token: string, origin: string): string {
  const base = normalizeOrigin(origin)
  const url = new URL('/mcp', base)
  url.searchParams.set('token', token)
  return url.toString()
}

function normalizeOrigin(origin: string): string {
  const trimmed = origin.trim().replace(/\/+$/, '')
  if (!trimmed) {
    throw new Error('MCP origin is required')
  }
  const parsed = new URL(trimmed)
  if (parsed.hostname === '0.0.0.0' || parsed.hostname === '[::]' || parsed.hostname === '::') {
    parsed.hostname = '127.0.0.1'
  }
  return parsed.origin
}

function browserOrigin(): string {
  if (typeof window === 'undefined' || !window.location?.origin) {
    return 'http://127.0.0.1:7777'
  }
  return window.location.origin
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\"'\"'")}'`
}
