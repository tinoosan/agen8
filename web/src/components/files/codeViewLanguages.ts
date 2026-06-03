import type { Extension } from '@codemirror/state'
import { StreamLanguage } from '@codemirror/language'
import { javascript } from '@codemirror/lang-javascript'
import { json } from '@codemirror/lang-json'
import { python } from '@codemirror/lang-python'
import { go } from '@codemirror/lang-go'
import { java } from '@codemirror/lang-java'
import { rust } from '@codemirror/lang-rust'
import { sql } from '@codemirror/lang-sql'
import { html } from '@codemirror/lang-html'
import { css } from '@codemirror/lang-css'
import { markdown } from '@codemirror/lang-markdown'
import { yaml } from '@codemirror/lang-yaml'
import { shell } from '@codemirror/legacy-modes/mode/shell'
import { getFileExtension } from './filePreviewUtils'

export function codeViewLanguageExtensions(filePath?: string): Extension[] {
  const ext = filePath ? getFileExtension(filePath) : ''
  switch (ext) {
    case 'ts':
      return [javascript({ typescript: true })]
    case 'tsx':
      return [javascript({ jsx: true, typescript: true })]
    case 'js':
      return [javascript()]
    case 'jsx':
      return [javascript({ jsx: true })]
    case 'json':
      return [json()]
    case 'py':
      return [python()]
    case 'go':
      return [go()]
    case 'java':
      return [java()]
    case 'rs':
      return [rust()]
    case 'sql':
      return [sql()]
    case 'html':
    case 'xml':
    case 'svg':
      return [html()]
    case 'css':
      return [css()]
    case 'md':
    case 'markdown':
      return [markdown()]
    case 'yaml':
    case 'yml':
      return [yaml()]
    case 'sh':
    case 'bash':
    case 'zsh':
      return [StreamLanguage.define(shell)]
    default:
      return []
  }
}
