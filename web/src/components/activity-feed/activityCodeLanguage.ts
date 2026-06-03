const SUPPORTED_LANGUAGES = new Set([
  'bash',
  'css',
  'go',
  'javascript',
  'json',
  'jsx',
  'markdown',
  'markup',
  'python',
  'rust',
  'sql',
  'toml',
  'tsx',
  'typescript',
  'yaml',
])

const LANGUAGE_ALIASES: Record<string, string> = {
  html: 'markup',
  js: 'javascript',
  md: 'markdown',
  py: 'python',
  rs: 'rust',
  sh: 'bash',
  text: 'markup',
  ts: 'typescript',
  yml: 'yaml',
  xml: 'markup',
}

export function normalizeActivityCodeLanguage(language: string): string {
  const normalized = language.trim().toLowerCase()
  const resolved = LANGUAGE_ALIASES[normalized] ?? normalized
  return SUPPORTED_LANGUAGES.has(resolved) ? resolved : 'markup'
}
