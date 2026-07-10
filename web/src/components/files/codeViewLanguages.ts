import type { Extension } from '@codemirror/state'
import { StreamLanguage } from '@codemirror/language'
import { getFileExtension } from './filePreviewUtils'

// Language packages dominate the code viewer's bundle. Keep every import in
// the matching branch so opening one file type downloads only its grammar.
export async function loadCodeViewLanguageExtensions(filePath?: string): Promise<Extension[]> {
  const ext = filePath ? getFileExtension(filePath) : ''
  switch (ext) {
    case 'ts': {
      const { javascript } = await import('@codemirror/lang-javascript')
      return [javascript({ typescript: true })]
    }
    case 'tsx': {
      const { javascript } = await import('@codemirror/lang-javascript')
      return [javascript({ jsx: true, typescript: true })]
    }
    case 'js': {
      const { javascript } = await import('@codemirror/lang-javascript')
      return [javascript()]
    }
    case 'jsx': {
      const { javascript } = await import('@codemirror/lang-javascript')
      return [javascript({ jsx: true })]
    }
    case 'json': {
      const { json } = await import('@codemirror/lang-json')
      return [json()]
    }
    case 'py': {
      const { python } = await import('@codemirror/lang-python')
      return [python()]
    }
    case 'go': {
      const { go } = await import('@codemirror/lang-go')
      return [go()]
    }
    case 'java': {
      const { java } = await import('@codemirror/lang-java')
      return [java()]
    }
    case 'rs': {
      const { rust } = await import('@codemirror/lang-rust')
      return [rust()]
    }
    case 'sql': {
      const { sql } = await import('@codemirror/lang-sql')
      return [sql()]
    }
    case 'html':
    case 'xml':
    case 'svg': {
      const { html } = await import('@codemirror/lang-html')
      return [html()]
    }
    case 'css': {
      const { css } = await import('@codemirror/lang-css')
      return [css()]
    }
    case 'md':
    case 'markdown': {
      const { markdown } = await import('@codemirror/lang-markdown')
      return [markdown()]
    }
    case 'yaml':
    case 'yml': {
      const { yaml } = await import('@codemirror/lang-yaml')
      return [yaml()]
    }
    case 'sh':
    case 'bash':
    case 'zsh': {
      const { shell } = await import('@codemirror/legacy-modes/mode/shell')
      return [StreamLanguage.define(shell)]
    }
    default:
      return []
  }
}
