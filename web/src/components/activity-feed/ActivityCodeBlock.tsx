import { memo } from 'react'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import bash from 'react-syntax-highlighter/dist/esm/languages/prism/bash'
import css from 'react-syntax-highlighter/dist/esm/languages/prism/css'
import go from 'react-syntax-highlighter/dist/esm/languages/prism/go'
import javascript from 'react-syntax-highlighter/dist/esm/languages/prism/javascript'
import json from 'react-syntax-highlighter/dist/esm/languages/prism/json'
import jsx from 'react-syntax-highlighter/dist/esm/languages/prism/jsx'
import markdown from 'react-syntax-highlighter/dist/esm/languages/prism/markdown'
import markup from 'react-syntax-highlighter/dist/esm/languages/prism/markup'
import python from 'react-syntax-highlighter/dist/esm/languages/prism/python'
import rust from 'react-syntax-highlighter/dist/esm/languages/prism/rust'
import sql from 'react-syntax-highlighter/dist/esm/languages/prism/sql'
import toml from 'react-syntax-highlighter/dist/esm/languages/prism/toml'
import tsx from 'react-syntax-highlighter/dist/esm/languages/prism/tsx'
import typescript from 'react-syntax-highlighter/dist/esm/languages/prism/typescript'
import yaml from 'react-syntax-highlighter/dist/esm/languages/prism/yaml'
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { normalizeActivityCodeLanguage } from './activityCodeLanguage'

SyntaxHighlighter.registerLanguage('bash', bash)
SyntaxHighlighter.registerLanguage('css', css)
SyntaxHighlighter.registerLanguage('go', go)
SyntaxHighlighter.registerLanguage('javascript', javascript)
SyntaxHighlighter.registerLanguage('json', json)
SyntaxHighlighter.registerLanguage('jsx', jsx)
SyntaxHighlighter.registerLanguage('markdown', markdown)
SyntaxHighlighter.registerLanguage('markup', markup)
SyntaxHighlighter.registerLanguage('python', python)
SyntaxHighlighter.registerLanguage('rust', rust)
SyntaxHighlighter.registerLanguage('sql', sql)
SyntaxHighlighter.registerLanguage('toml', toml)
SyntaxHighlighter.registerLanguage('tsx', tsx)
SyntaxHighlighter.registerLanguage('typescript', typescript)
SyntaxHighlighter.registerLanguage('yaml', yaml)

interface ActivityCodeBlockProps {
  code: string
  language: string
  compact?: boolean
  showLineNumbers?: boolean
  /** Force dark syntax theme regardless of current app theme */
  forceDark?: boolean
}

export default memo(function ActivityCodeBlock({
  code,
  language,
  compact = false,
  showLineNumbers = false,
  forceDark = false,
}: ActivityCodeBlockProps) {
  return (
    <SyntaxHighlighter
      language={normalizeActivityCodeLanguage(language)}
      style={vscDarkPlus}
      customStyle={compact
        ? { margin: 0, padding: '8px 10px', fontSize: '0.6875rem', borderRadius: 'var(--r-md)', background: forceDark ? 'transparent' : 'var(--bg-app)' }
        : { margin: 0, padding: '12px', fontSize: '0.6875rem', borderRadius: '4px', background: forceDark ? 'transparent' : 'var(--bg-code)' }}
      showLineNumbers={showLineNumbers}
      lineNumberStyle={showLineNumbers
        ? { color: 'var(--text-3)', fontSize: '0.5625rem', minWidth: '2em', paddingRight: '8px' }
        : undefined}
    >
      {code}
    </SyntaxHighlighter>
  )
})
