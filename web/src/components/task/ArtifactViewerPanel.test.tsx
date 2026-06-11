import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

/* The panel fetches previews and git baselines over RPC; mock the transport
 * so the real component (including the real ArtifactViewer routing and
 * DiffView) renders against controlled data without a backend. */
const mockRpcCall = vi.fn()

vi.mock('../../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

const { ArtifactViewerPanel } = await import('./ArtifactViewerPanel')

function renderPanel(vpath: string, layout: 'sheet' | 'inline' = 'sheet', onClose = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ArtifactViewerPanel projectId="proj-1" vpath={vpath} onClose={onClose} layout={layout} />
    </QueryClientProvider>,
  )
  return onClose
}

const textPreview = (vpath: string, content: string) => ({
  artifact: { nodeKey: 'file:' + vpath, kind: 'file', label: vpath.split('/').pop(), vpath },
  content,
  contentKind: 'text',
  contentEncoding: 'utf8',
  truncated: false,
  bytesRead: content.length,
})

beforeEach(() => {
  mockRpcCall.mockReset()
})

describe('ArtifactViewerPanel', () => {
  it('fetches the file over files.get and renders text content', async () => {
    mockRpcCall.mockResolvedValue(textPreview('/project/notes.txt', 'hello from the attachment'))
    renderPanel('/project/notes.txt')

    await waitFor(() => {
      expect(mockRpcCall).toHaveBeenCalledWith('files.get', {
        projectId: 'proj-1',
        path: '/project/notes.txt',
        maxBytes: 2_000_000,
      })
    })
    expect((await screen.findAllByText('/project/notes.txt')).length).toBeGreaterThan(0)
    expect(await screen.findByText(/hello from the attachment/)).toBeInTheDocument()
  })

  it('shows a calm error state when the file cannot be loaded', async () => {
    mockRpcCall.mockRejectedValue(new Error('file missing'))
    renderPanel('/project/.agen8/attachments/task-1/gone.png')
    expect(await screen.findByText('Failed to load file contents.')).toBeInTheDocument()
  })

  it('offers the diff toggle for text files and diffs against the git baseline', async () => {
    mockRpcCall.mockImplementation(async (method: unknown) => {
      if (method === 'files.get') return textPreview('/project/main.go', 'line one\nline two changed\n')
      if (method === 'files.baseline') return { path: '/project/main.go', tracked: true, content: 'line one\nline two\n' }
      throw new Error('unexpected method: ' + String(method))
    })
    renderPanel('/project/main.go')

    await userEvent.click(await screen.findByRole('button', { name: 'Diff' }))

    expect(await screen.findByTestId('diff-view')).toBeInTheDocument()
    expect(screen.getByText('line two')).toBeInTheDocument()
    expect(screen.getByText('line two changed')).toBeInTheDocument()
    expect(mockRpcCall).toHaveBeenCalledWith('files.baseline', {
      projectId: 'proj-1',
      path: '/project/main.go',
    })
  })

  it('never offers the diff toggle for images', async () => {
    mockRpcCall.mockResolvedValue({
      artifact: { nodeKey: 'file:/project/shot.png', kind: 'file', label: 'shot.png', vpath: '/project/shot.png' },
      content: '',
      contentKind: 'image',
      contentEncoding: 'base64',
      bytesB64: 'aGk=',
      truncated: false,
      bytesRead: 2,
    })
    renderPanel('/project/shot.png')
    await waitFor(() => expect(mockRpcCall).toHaveBeenCalled())
    expect(screen.queryByRole('button', { name: 'Diff' })).not.toBeInTheDocument()
  })

  it('degrades to normal view with a notice when the file has no git baseline', async () => {
    mockRpcCall.mockImplementation(async (method: unknown) => {
      if (method === 'files.get') return textPreview('/project/new.txt', 'brand new file\n')
      if (method === 'files.baseline') return { path: '/project/new.txt', tracked: false }
      throw new Error('unexpected method: ' + String(method))
    })
    renderPanel('/project/new.txt')
    await userEvent.click(await screen.findByRole('button', { name: 'Diff' }))

    expect(await screen.findByTestId('diff-unavailable-notice')).toBeInTheDocument()
    // The normal content is still visible underneath the notice — no dead pane.
    expect(screen.getByText(/brand new file/)).toBeInTheDocument()
    expect(screen.queryByTestId('diff-view')).not.toBeInTheDocument()
  })

  it('shows the structured unsupported reason for remote-location files', async () => {
    mockRpcCall.mockImplementation(async (method: unknown) => {
      if (method === 'files.get') return textPreview('/project/remote.txt', 'remote file body\n')
      if (method === 'files.baseline') return { path: '/project/remote.txt', tracked: false, unsupported: 'Diff is not available for files on remote locations yet.' }
      throw new Error('unexpected method: ' + String(method))
    })
    renderPanel('/project/remote.txt')
    await userEvent.click(await screen.findByRole('button', { name: 'Diff' }))

    const notice = await screen.findByTestId('diff-unavailable-notice')
    expect(notice.textContent).toContain('remote locations')
    expect(screen.getByText(/remote file body/)).toBeInTheDocument()
  })

  it('renders inline layout as a side panel with a working close button', async () => {
    mockRpcCall.mockResolvedValue(textPreview('/project/notes.txt', 'inline content'))
    const onClose = renderPanel('/project/notes.txt', 'inline')

    expect(await screen.findByTestId('artifact-inline-panel')).toBeInTheDocument()
    expect(await screen.findByText(/inline content/)).toBeInTheDocument()
    // Inline layout is a plain column, not a dialog overlay.
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Close artifact viewer' }))
    expect(onClose).toHaveBeenCalled()
  })

  it('toggles a file browser and loads a picked file into the same viewer', async () => {
    mockRpcCall.mockImplementation(async (method: unknown, params: unknown) => {
      const p = params as { path?: string }
      if (method === 'files.listDir') {
        return {
          path: p.path,
          entries: [
            { name: 'other.txt', path: '/project/other.txt', isDir: false, writable: false },
          ],
        }
      }
      if (method === 'files.get') {
        const label = p.path?.split('/').pop()
        return textPreview(p.path ?? '', `contents of ${label}`)
      }
      throw new Error('unexpected method: ' + String(method))
    })
    renderPanel('/project/notes.txt')
    expect(await screen.findByText(/contents of notes.txt/)).toBeInTheDocument()

    // Browser hidden by default; toggle reveals it.
    expect(screen.queryByTestId('file-browser-pane')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Browse files' }))
    expect(await screen.findByTestId('file-browser-pane')).toBeInTheDocument()

    // Pick a different file — the same viewer (no remount) shows it.
    await userEvent.click(await screen.findByText('other.txt'))
    expect(await screen.findByText(/contents of other.txt/)).toBeInTheDocument()
    await waitFor(() =>
      expect(mockRpcCall).toHaveBeenCalledWith('files.get', expect.objectContaining({ path: '/project/other.txt' })),
    )

    // Toggling off returns to the plain single-file view.
    await userEvent.click(screen.getByRole('button', { name: 'Browse files' }))
    expect(screen.queryByTestId('file-browser-pane')).not.toBeInTheDocument()
  })

  it('keeps the browser pane visible beside shrinkable wide content', async () => {
    mockRpcCall.mockImplementation(async (method: unknown, params: unknown) => {
      const p = params as { path?: string }
      if (method === 'files.listDir') {
        return {
          path: p.path,
          entries: [
            { name: 'ArtifactPreviewPane.tsx', path: '/project/web/src/components/files/ArtifactPreviewPane.tsx', isDir: false, writable: false },
          ],
        }
      }
      if (method === 'files.get') {
        return textPreview(p.path ?? '', 'const longLine = "' + 'x'.repeat(240) + '"')
      }
      throw new Error('unexpected method: ' + String(method))
    })

    renderPanel('/project/web/src/components/files/CodeView.tsx', 'inline')
    await userEvent.click(await screen.findByRole('button', { name: 'Browse files' }))

    const browserPane = await screen.findByTestId('file-browser-pane')
    expect(browserPane).toHaveClass('w-[clamp(160px,34%,240px)]')
    expect(browserPane.previousElementSibling).toHaveClass('min-w-0', 'overflow-hidden')
    expect(await screen.findByText('ArtifactPreviewPane.tsx')).toBeInTheDocument()
  })

  it('keeps the diff toggle working in inline layout', async () => {
    mockRpcCall.mockImplementation(async (method: unknown) => {
      if (method === 'files.get') return textPreview('/project/main.go', 'a\nb changed\n')
      if (method === 'files.baseline') return { path: '/project/main.go', tracked: true, content: 'a\nb\n' }
      throw new Error('unexpected method: ' + String(method))
    })
    renderPanel('/project/main.go', 'inline')
    await userEvent.click(await screen.findByRole('button', { name: 'Diff' }))
    expect(await screen.findByTestId('diff-view')).toBeInTheDocument()
  })
})
