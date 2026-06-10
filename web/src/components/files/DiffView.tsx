import { useMemo } from 'react'
import { diffLines } from 'diff'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import { useStore } from '../../lib/store'
import { prismStyleFor } from './prismTheme'
import tsx from 'react-syntax-highlighter/dist/esm/languages/prism/tsx'
import typescript from 'react-syntax-highlighter/dist/esm/languages/prism/typescript'
import javascript from 'react-syntax-highlighter/dist/esm/languages/prism/javascript'
import jsx from 'react-syntax-highlighter/dist/esm/languages/prism/jsx'
import json from 'react-syntax-highlighter/dist/esm/languages/prism/json'
import go from 'react-syntax-highlighter/dist/esm/languages/prism/go'
import python from 'react-syntax-highlighter/dist/esm/languages/prism/python'
import rust from 'react-syntax-highlighter/dist/esm/languages/prism/rust'
import java from 'react-syntax-highlighter/dist/esm/languages/prism/java'
import sql from 'react-syntax-highlighter/dist/esm/languages/prism/sql'
import bash from 'react-syntax-highlighter/dist/esm/languages/prism/bash'
import yaml from 'react-syntax-highlighter/dist/esm/languages/prism/yaml'
import markdown from 'react-syntax-highlighter/dist/esm/languages/prism/markdown'
import css from 'react-syntax-highlighter/dist/esm/languages/prism/css'
import markup from 'react-syntax-highlighter/dist/esm/languages/prism/markup'
import { getFileExtension } from './filePreviewUtils'

SyntaxHighlighter.registerLanguage('tsx', tsx)
SyntaxHighlighter.registerLanguage('typescript', typescript)
SyntaxHighlighter.registerLanguage('javascript', javascript)
SyntaxHighlighter.registerLanguage('jsx', jsx)
SyntaxHighlighter.registerLanguage('json', json)
SyntaxHighlighter.registerLanguage('go', go)
SyntaxHighlighter.registerLanguage('python', python)
SyntaxHighlighter.registerLanguage('rust', rust)
SyntaxHighlighter.registerLanguage('java', java)
SyntaxHighlighter.registerLanguage('sql', sql)
SyntaxHighlighter.registerLanguage('bash', bash)
SyntaxHighlighter.registerLanguage('yaml', yaml)
SyntaxHighlighter.registerLanguage('markdown', markdown)
SyntaxHighlighter.registerLanguage('css', css)
SyntaxHighlighter.registerLanguage('markup', markup)

/** Extension → Prism language id, mirroring codeViewLanguages.ts coverage. */
function diffLanguageFor(filePath?: string): string | null {
  switch (filePath ? getFileExtension(filePath) : '') {
    case 'ts': return 'typescript'
    case 'tsx': return 'tsx'
    case 'js': case 'mjs': case 'cjs': return 'javascript'
    case 'jsx': return 'jsx'
    case 'json': return 'json'
    case 'go': return 'go'
    case 'py': return 'python'
    case 'rs': return 'rust'
    case 'java': return 'java'
    case 'sql': return 'sql'
    case 'sh': case 'bash': case 'zsh': return 'bash'
    case 'yml': case 'yaml': return 'yaml'
    case 'md': case 'markdown': return 'markdown'
    case 'css': return 'css'
    case 'html': case 'xml': case 'svg': return 'markup'
    default: return null
  }
}

interface DiffViewProps {
  /** Committed (git HEAD) content — the "before". */
  baseline: string
  /** Working-tree content — the "after". */
  current: string
  /** Used to pick the syntax-highlight language by extension. */
  filePath?: string
}

interface DiffRow {
  kind: 'context' | 'added' | 'removed'
  oldLine: number | null
  newLine: number | null
  text: string
}

function buildRows(baseline: string, current: string): DiffRow[] {
  const rows: DiffRow[] = []
  let oldLine = 1
  let newLine = 1
  for (const part of diffLines(baseline, current)) {
    const lines = part.value.split('\n')
    // diffLines values end with \n except possibly the last part; drop the
    // trailing empty segment so it doesn't render as a phantom line.
    if (lines[lines.length - 1] === '') lines.pop()
    for (const text of lines) {
      if (part.added) {
        rows.push({ kind: 'added', oldLine: null, newLine: newLine++, text })
      } else if (part.removed) {
        rows.push({ kind: 'removed', oldLine: oldLine++, newLine: null, text })
      } else {
        rows.push({ kind: 'context', oldLine: oldLine++, newLine: newLine++, text })
      }
    }
  }
  return rows
}

const ROW_STYLES: Record<DiffRow['kind'], { background: string; color?: string; marker: string }> = {
  context: { background: 'transparent', marker: ' ' },
  added: { background: 'color-mix(in srgb, var(--green, #3fb950) 12%, transparent)', marker: '+' },
  removed: { background: 'color-mix(in srgb, var(--red, #f85149) 12%, transparent)', marker: '-' },
}

/**
 * Unified line diff of a file's uncommitted working-tree changes against its
 * git HEAD baseline. Renders the full file with added/removed highlighting —
 * the Codex-style review view.
 */
type PrismStyle = ReturnType<typeof prismStyleFor>

/** One diff line, syntax-highlighted when the file's language is known. */
function DiffLineText({ text, language, prismStyle }: { text: string; language: string | null; prismStyle: PrismStyle }) {
  if (!language || text === '') {
    return <>{text}</>
  }
  return (
    <SyntaxHighlighter
      language={language}
      style={prismStyle}
      PreTag="span"
      CodeTag="span"
      customStyle={{
        display: 'inline',
        background: 'transparent',
        padding: 0,
        margin: 0,
        fontFamily: 'inherit',
        fontSize: 'inherit',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-all',
      }}
      codeTagProps={{ style: { fontFamily: 'inherit', fontSize: 'inherit', whiteSpace: 'pre-wrap', wordBreak: 'break-all', background: 'transparent' } }}
    >
      {text}
    </SyntaxHighlighter>
  )
}

export default function DiffView({ baseline, current, filePath }: DiffViewProps) {
  const theme = useStore((s) => s.theme)
  const prismStyle = prismStyleFor(theme)
  const rows = useMemo(() => buildRows(baseline, current), [baseline, current])
  const language = useMemo(() => diffLanguageFor(filePath), [filePath])
  const hasChanges = rows.some((row) => row.kind !== 'context')

  if (!hasChanges) {
    return (
      <div className="flex items-center justify-center h-full px-4 py-8 text-xs text-[var(--text-3)]">
        No uncommitted changes — the file matches git HEAD.
      </div>
    )
  }

  const added = rows.filter((r) => r.kind === 'added').length
  const removed = rows.filter((r) => r.kind === 'removed').length

  return (
    <div className="h-full overflow-auto" data-testid="diff-view">
      <div className="px-4 py-2 text-[11px] text-[var(--text-3)] border-b border-[var(--border)] sticky top-0 bg-[var(--bg-surface,var(--bg-elevated))] z-10">
        Uncommitted changes vs git HEAD ·{' '}
        <span style={{ color: 'var(--green, #3fb950)' }}>+{added}</span>{' '}
        <span style={{ color: 'var(--red, #f85149)' }}>−{removed}</span>
      </div>
      <table className="w-full border-collapse" style={{ fontFamily: 'var(--font-mono, monospace)', fontSize: '0.75rem', lineHeight: 1.6 }}>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} style={{ background: ROW_STYLES[row.kind].background }}>
              <td className="select-none text-right pr-2 pl-3 text-[var(--text-3)] align-top w-10 tabular-nums" style={{ opacity: 0.6 }}>
                {row.oldLine ?? ''}
              </td>
              <td className="select-none text-right pr-2 text-[var(--text-3)] align-top w-10 tabular-nums" style={{ opacity: 0.6 }}>
                {row.newLine ?? ''}
              </td>
              <td className="select-none w-4 text-center align-top" style={{ color: row.kind === 'added' ? 'var(--green, #3fb950)' : row.kind === 'removed' ? 'var(--red, #f85149)' : 'var(--text-3)' }}>
                {ROW_STYLES[row.kind].marker}
              </td>
              <td className="align-top pr-4" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                <DiffLineText text={row.text} language={language} prismStyle={prismStyle} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
