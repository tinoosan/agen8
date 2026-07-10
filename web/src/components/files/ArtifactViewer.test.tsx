import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ArtifactGetResult, ArtifactNode } from '../../lib/types'

vi.mock('./DocumentViewer', () => ({
  default: () => <div data-testid="document-viewer">Rendered markdown</div>,
}))

const { default: ArtifactViewer } = await import('./ArtifactViewer')

function file(path: string): ArtifactNode {
  return {
    kind: 'file',
    label: path.split('/').pop() ?? path,
    vpath: path,
  }
}

function preview(content: string): ArtifactGetResult {
  return {
    artifact: file('/project/.gitignore'),
    content,
    contentKind: 'text',
    contentType: 'text/plain; charset=utf-8',
    truncated: false,
    bytesRead: content.length,
    fileSize: content.length,
  }
}

describe('ArtifactViewer', () => {
  it('renders non-markdown text through the IDE-style code viewer', async () => {
    render(
      <ArtifactViewer
        file={file('/project/scripts/homelab-readiness-check.sh')}
        preview={preview('# Local-only secrets\n**/secrets.yaml')}
        isLoading={false}
        error={false}
        variant="slideover"
      />,
    )

    expect(await screen.findByTestId('code-view')).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Search file content' })).toBeInTheDocument()
    expect(screen.queryByTestId('document-viewer')).not.toBeInTheDocument()
  })

  it('does not duplicate the selected filename in slideover previews', async () => {
    render(
      <ArtifactViewer
        file={file('/project/scripts/homelab-readiness-check.sh')}
        preview={preview('#!/usr/bin/env bash\nset -u -o pipefail')}
        isLoading={false}
        error={false}
        variant="slideover"
      />,
    )

    expect(await screen.findByText('.sh')).toBeInTheDocument()
    expect(screen.queryByText('homelab-readiness-check.sh')).not.toBeInTheDocument()
  })

  it('keeps the selected path title on full page previews', async () => {
    render(
      <ArtifactViewer
        file={file('/project/scripts/homelab-readiness-check.sh')}
        preview={preview('#!/usr/bin/env bash\nset -u -o pipefail')}
        isLoading={false}
        error={false}
        variant="page"
      />,
    )

    expect(await screen.findByText('/project/scripts/homelab-readiness-check.sh')).toBeInTheDocument()
  })

  it('keeps markdown on the rendered document viewer path', async () => {
    render(
      <ArtifactViewer
        file={file('/project/README.md')}
        preview={preview('# README')}
        isLoading={false}
        error={false}
        variant="slideover"
      />,
    )

    expect(await screen.findByTestId('document-viewer')).toBeInTheDocument()
    expect(screen.queryByTestId('code-view')).not.toBeInTheDocument()
  })
})
