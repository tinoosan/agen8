const SERVER_NAME = 'agen8'

export const CODEX_SKILL_COMMAND = 'agen8 skill install --harness codex'
export const CLAUDE_SKILL_COMMAND = 'agen8 skill install --harness claude-cli'

export interface MCPSetupSnippets {
  url: string
  jsonConfig: string
  codexCommand: string
  claudeCommand: string
}

export function buildMCPSetup(secret: string, origin = browserOrigin()): MCPSetupSnippets {
  const token = secret.trim()
  if (!token) {
    throw new Error('MCP token is required')
  }
  const url = buildMCPURL(token, origin)
  return {
    url,
    jsonConfig: JSON.stringify({
      mcpServers: {
        [SERVER_NAME]: {
          type: 'http',
          url,
        },
      },
    }, null, 2),
    codexCommand: `codex mcp add ${SERVER_NAME} --url ${shellQuote(url)}`,
    claudeCommand: `claude mcp add --transport http --scope user ${SERVER_NAME} ${shellQuote(url)}`,
  }
}

function buildMCPURL(token: string, origin: string): string {
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
