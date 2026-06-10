import { useMemo } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { Decoration, EditorView, highlightSpecialChars, keymap, lineNumbers } from '@codemirror/view'
import { EditorState, type Extension } from '@codemirror/state'
import { defaultKeymap, historyKeymap } from '@codemirror/commands'
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { codeViewLanguageExtensions } from './codeViewLanguages'

interface CodeViewProps {
  content: string
  filePath?: string
  search?: string
}

function searchHighlighter(query?: string): Extension {
  const needle = query?.trim()
  if (!needle) return []
  return EditorView.decorations.compute(['doc'], (state) => {
    const decorations = []
    const lowerNeedle = needle.toLowerCase()
    for (let lineNo = 1; lineNo <= state.doc.lines; lineNo += 1) {
      const line = state.doc.line(lineNo)
      if (!line.text.toLowerCase().includes(lowerNeedle)) continue
      decorations.push(Decoration.line({ class: 'cm-agen8-search-hit' }).range(line.from))
    }
    return Decoration.set(decorations, true)
  })
}

const agen8EditorTheme = EditorView.theme({
  '&': {
    height: '100%',
    color: 'var(--text-1)',
    backgroundColor: 'transparent',
    fontFamily: 'var(--font-mono, "SF Mono", "Fira Code", "JetBrains Mono", ui-monospace, monospace)',
    fontSize: '0.75rem',
  },
  '.cm-scroller': {
    fontFamily: 'inherit',
    lineHeight: '1.7',
    overflow: 'auto',
  },
  '.cm-content': {
    padding: '10px 0 18px',
    caretColor: 'var(--accent)',
  },
  '.cm-line': {
    padding: '0 18px 0 10px',
  },
  '.cm-gutters': {
    backgroundColor: 'transparent',
    color: 'var(--text-3)',
    border: '0',
    userSelect: 'none',
  },
  '.cm-lineNumbers .cm-gutterElement': {
    minWidth: '3.2em',
    padding: '0 12px 0 8px',
    fontSize: '0.625rem',
    opacity: '0.68',
  },
  '.cm-activeLine': {
    backgroundColor: 'color-mix(in srgb, var(--accent) 8%, transparent)',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'transparent',
    color: 'var(--text-2)',
  },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
    backgroundColor: 'color-mix(in srgb, var(--accent) 28%, transparent)',
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-matchingBracket, .cm-nonmatchingBracket': {
    backgroundColor: 'color-mix(in srgb, var(--accent) 16%, transparent)',
    outline: '1px solid color-mix(in srgb, var(--accent) 35%, transparent)',
  },
  '.cm-agen8-search-hit': {
    backgroundColor: 'var(--amber-dim)',
  },
})

const agen8HighlightStyle = HighlightStyle.define([
  {
    tag: [
      tags.keyword,
      tags.operatorKeyword,
      tags.modifier,
      tags.atom,
      tags.bool,
      tags.null,
    ],
    color: 'var(--accent)',
  },
  {
    tag: [tags.string, tags.special(tags.string), tags.regexp],
    color: 'var(--green)',
  },
  {
    tag: [tags.number, tags.integer, tags.float],
    color: 'var(--amber)',
  },
  {
    tag: [tags.comment, tags.lineComment, tags.blockComment],
    color: 'var(--text-3)',
    fontStyle: 'italic',
  },
  {
    tag: [tags.variableName],
    color: 'var(--text-1)',
  },
  {
    tag: [tags.propertyName, tags.attributeName, tags.labelName],
    color: 'color-mix(in srgb, var(--accent) 78%, var(--text-1))',
  },
  {
    tag: [tags.definition(tags.variableName), tags.function(tags.variableName), tags.className, tags.typeName, tags.namespace],
    color: 'color-mix(in srgb, var(--text-1) 82%, var(--accent))',
  },
  {
    tag: [tags.literal, tags.derefOperator, tags.separator],
    color: 'var(--text-2)',
  },
  {
    tag: [tags.heading, tags.strong],
    color: 'var(--text-1)',
    fontWeight: '600',
  },
  {
    tag: [tags.emphasis],
    color: 'var(--text-2)',
    fontStyle: 'italic',
  },
  {
    tag: [tags.link, tags.url],
    color: 'var(--accent)',
    textDecoration: 'underline',
    textUnderlineOffset: '2px',
  },
  {
    tag: [tags.invalid],
    color: 'var(--red)',
  },
])

export default function CodeView({ content, filePath, search: searchQuery }: CodeViewProps) {
  const extensions = useMemo<Extension[]>(
    () => [
      lineNumbers(),
      highlightSpecialChars(),
      EditorState.readOnly.of(true),
      EditorView.editable.of(false),
      EditorView.lineWrapping,
      syntaxHighlighting(agen8HighlightStyle, { fallback: true }),
      search({ top: true }),
      highlightSelectionMatches(),
      keymap.of([...defaultKeymap, ...historyKeymap, ...searchKeymap]),
      agen8EditorTheme,
      searchHighlighter(searchQuery),
      ...codeViewLanguageExtensions(filePath),
    ],
    [filePath, searchQuery],
  )

  return (
    <div data-testid="code-view" className="h-full min-h-0 overflow-hidden bg-transparent">
      <CodeMirror
        value={content}
        extensions={extensions}
        basicSetup={false}
        theme="none"
        editable={false}
        readOnly
        height="100%"
        // The height="100%" prop styles .cm-editor, but it resolves against the
        // wrapper react-codemirror renders, which is auto-height by default — so
        // the editor grew to full content and got clipped by overflow-hidden.
        // Forcing the root to 100% gives a definite height chain so .cm-scroller
        // scrolls internally instead.
        style={{ height: '100%' }}
      />
    </div>
  )
}
