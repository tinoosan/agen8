import { pathFromChatHref } from './contextPanelEvents'

type MarkdownNode = {
  type?: string
  value?: string
  children?: MarkdownNode[]
  url?: string
}

const SKIP_TYPES = new Set([
  'link',
  'linkReference',
  'image',
  'imageReference',
  'code',
  'inlineCode',
  'html',
])

const TRAILING_PUNCTUATION_RE = /[),.;!?]+$/
const LEADING_PUNCTUATION_RE = /^[([{]+/
const AUTO_LINK_FILENAME_EXTENSIONS = new Set([
  'bash',
  'c',
  'conf',
  'cpp',
  'css',
  'csv',
  'env',
  'go',
  'h',
  'hpp',
  'html',
  'java',
  'js',
  'json',
  'jsx',
  'lock',
  'log',
  'md',
  'py',
  'rs',
  'sh',
  'sql',
  'svg',
  'toml',
  'ts',
  'tsx',
  'txt',
  'xml',
  'yaml',
  'yml',
  'zsh',
])

export default function remarkLocalFileLinks() {
  return (tree: MarkdownNode) => {
    visit(tree)
  }
}

function visit(node: MarkdownNode, parent?: MarkdownNode, index?: number): void {
  if (!node || typeof node !== 'object') return
  if (node.type && SKIP_TYPES.has(node.type)) return

  if (
    node.type === 'text'
    && parent
    && Array.isArray(parent.children)
    && typeof index === 'number'
    && typeof node.value === 'string'
  ) {
    const transformed = transformTextNode(node.value)
    if (transformed) {
      parent.children.splice(index, 1, ...transformed)
      return
    }
  }

  if (!Array.isArray(node.children)) return
  for (let i = 0; i < node.children.length; i += 1) {
    const child = node.children[i]
    const beforeCount = node.children.length
    visit(child, node, i)
    const afterCount = node.children.length
    if (afterCount > beforeCount) {
      i += afterCount - beforeCount
    }
  }
}

function transformTextNode(value: string): MarkdownNode[] | null {
  const parts = value.split(/(\s+)/)
  const out: MarkdownNode[] = []
  let changed = false

  for (const part of parts) {
    if (part === '') continue
    if (/^\s+$/.test(part)) {
      out.push(textNode(part))
      continue
    }

    const leading = part.match(LEADING_PUNCTUATION_RE)?.[0] ?? ''
    const rawWithoutLeading = leading ? part.slice(leading.length) : part

    const trailing = rawWithoutLeading.match(TRAILING_PUNCTUATION_RE)?.[0] ?? ''
    const core = trailing
      ? rawWithoutLeading.slice(0, rawWithoutLeading.length - trailing.length)
      : rawWithoutLeading

    if (!core) {
      out.push(textNode(part))
      continue
    }

    if (!shouldAutoLinkTextToken(core)) {
      out.push(textNode(part))
      continue
    }

    const resolvedPath = pathFromChatHref(core)
    if (!resolvedPath) {
      out.push(textNode(part))
      continue
    }

    changed = true
    if (leading) out.push(textNode(leading))
    out.push(linkNode(core, resolvedPath))
    if (trailing) out.push(textNode(trailing))
  }

  return changed ? out : null
}

function shouldAutoLinkTextToken(value: string): boolean {
  const normalized = value.trim()
  if (!normalized) return false
  if (/^[./\\]+$/.test(normalized)) return false
  if (
    normalized.startsWith('/')
    || normalized.startsWith('./')
    || normalized.startsWith('../')
    || normalized.toLowerCase().startsWith('file://')
    || normalized.includes('/')
  ) {
    return true
  }

  const ext = normalized.match(/\.([A-Za-z0-9_-]+)$/)?.[1]?.toLowerCase()
  return ext ? AUTO_LINK_FILENAME_EXTENSIONS.has(ext) : false
}

function textNode(value: string): MarkdownNode {
  return { type: 'text', value }
}

function linkNode(label: string, url: string): MarkdownNode {
  return {
    type: 'link',
    url,
    children: [{ type: 'text', value: label }],
  }
}
