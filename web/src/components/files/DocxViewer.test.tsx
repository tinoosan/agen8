import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import DocxViewer from './DocxViewer'
import type { ArtifactNode, ArtifactGetResult } from '../../lib/types'

// Mock mammoth to return controlled HTML output
vi.mock('mammoth', () => ({
  default: {
    convertToHtml: vi.fn(),
  },
}))

import mammoth from 'mammoth'

function makeFile(vpath: string): ArtifactNode {
  return {
    nodeKey: 'test',
    kind: 'file',
    label: vpath.split('/').pop() ?? vpath,
    vpath,
  }
}

function makePreview(overrides: Partial<ArtifactGetResult> = {}): ArtifactGetResult {
  return {
    artifact: makeFile('/workspace/test.docx'),
    content: '',
    truncated: false,
    bytesRead: 0,
    bytesB64: 'AAAA', // minimal non-empty base64
    ...overrides,
  }
}

describe('DocxViewer', () => {
  it('sanitizes script tags from mammoth HTML output', async () => {
    const maliciousHtml = '<p>Hello</p><script>alert("xss")</script>'
    vi.mocked(mammoth.convertToHtml).mockResolvedValue({ value: maliciousHtml, messages: [] })

    const { container } = render(
      <DocxViewer file={makeFile('/workspace/evil.docx')} preview={makePreview()} isLoading={false} error={false} />
    )

    await waitFor(() => {
      const prose = container.querySelector('.md-prose')
      expect(prose).toBeTruthy()
      expect(prose!.innerHTML).toContain('Hello')
      expect(prose!.innerHTML).not.toContain('<script>')
      expect(prose!.innerHTML).not.toContain('alert')
    })
  })

  it('sanitizes event handler attributes from mammoth HTML output', async () => {
    const maliciousHtml = '<img src="x" onerror="alert(1)"><p>Safe</p>'
    vi.mocked(mammoth.convertToHtml).mockResolvedValue({ value: maliciousHtml, messages: [] })

    const { container } = render(
      <DocxViewer file={makeFile('/workspace/evil2.docx')} preview={makePreview()} isLoading={false} error={false} />
    )

    await waitFor(() => {
      const prose = container.querySelector('.md-prose')
      expect(prose).toBeTruthy()
      expect(prose!.innerHTML).toContain('Safe')
      expect(prose!.innerHTML).not.toContain('onerror')
    })
  })

  it('sanitizes javascript: URIs from mammoth HTML output', async () => {
    const maliciousHtml = '<a href="javascript:alert(1)">Click</a>'
    vi.mocked(mammoth.convertToHtml).mockResolvedValue({ value: maliciousHtml, messages: [] })

    const { container } = render(
      <DocxViewer file={makeFile('/workspace/evil3.docx')} preview={makePreview()} isLoading={false} error={false} />
    )

    await waitFor(() => {
      const prose = container.querySelector('.md-prose')
      expect(prose).toBeTruthy()
      const anchor = prose!.querySelector('a')
      expect(anchor).toBeTruthy()
      // DOMPurify removes javascript: URIs entirely — href should be null or not contain javascript:
      const href = anchor!.getAttribute('href')
      expect(href === null || !href.includes('javascript:')).toBe(true)
    })
  })

  it('renders loading skeleton', () => {
    const { container } = render(
      <DocxViewer file={makeFile('/workspace/doc.docx')} preview={undefined} isLoading={true} error={false} />
    )
    const skeletons = container.querySelectorAll('[class*="skeleton"], [data-slot="skeleton"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('renders error state when fetch fails', () => {
    render(
      <DocxViewer file={makeFile('/workspace/doc.docx')} preview={undefined} isLoading={false} error={true} />
    )
    expect(screen.getByText('Failed to load file')).toBeTruthy()
  })
})
