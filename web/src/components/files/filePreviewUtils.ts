const CODE_LANGUAGE_BY_EXT: Record<string, string> = {
  ts: 'typescript',
  tsx: 'tsx',
  js: 'javascript',
  jsx: 'jsx',
  py: 'python',
  go: 'go',
  rs: 'rust',
  java: 'java',
  c: 'markup',
  cpp: 'markup',
  h: 'markup',
  hpp: 'markup',
  css: 'css',
  html: 'markup',
  xml: 'markup',
  svg: 'markup',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  sh: 'bash',
  bash: 'bash',
  zsh: 'bash',
  sql: 'sql',
  md: 'markdown',
  markdown: 'markdown',
  toml: 'toml',
}

export function basename(path: string): string {
  const parts = path.split('/')
  return parts[parts.length - 1] || path
}

export function displayVirtualPathPart(part: string, index: number, projectRoot?: string | null): string {
  if (index !== 0 || part !== 'project') return part
  const normalizedRoot = (projectRoot ?? '').replace(/\/+$/, '')
  const rootName = normalizedRoot ? basename(normalizedRoot) : ''
  return rootName || part
}

export function getFileExt(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return ext ? `.${ext}` : ''
}

export function getFileExtension(path: string): string {
  return path.split('.').pop()?.toLowerCase() ?? ''
}

export function isMarkdownFile(path: string): boolean {
  const ext = getFileExtension(path)
  return ext === 'md' || ext === 'markdown'
}

export function isCodeFile(path: string): boolean {
  const ext = getFileExtension(path)
  return ['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs', 'java', 'c', 'cpp', 'h', 'json', 'yaml', 'yml', 'toml', 'xml', 'html', 'css', 'sql', 'sh', 'bash', 'zsh', 'svg'].includes(ext)
}

export function isSvgFile(path: string, contentType?: string): boolean {
  return getFileExtension(path) === 'svg' || (contentType ?? '').toLowerCase().includes('image/svg+xml')
}

export function getCodeLanguage(path?: string): string {
  const ext = path ? getFileExtension(path) : ''
  return CODE_LANGUAGE_BY_EXT[ext] ?? 'markup'
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function decodeBase64(base64: string): ArrayBuffer {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)
}

export function isDocxFile(path: string): boolean {
  return getFileExt(path).toLowerCase() === '.docx'
}

const SPREADSHEET_EXTS = new Set(['.csv', '.tsv', '.xlsx', '.xls'])

export function isSpreadsheetFile(path: string): boolean {
  return SPREADSHEET_EXTS.has(getFileExt(path).toLowerCase())
}

export function isTextSpreadsheet(path: string): boolean {
  const ext = getFileExt(path).toLowerCase()
  return ext === '.csv' || ext === '.tsv'
}

export function downloadBlob(filename: string, blob: Blob) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}
