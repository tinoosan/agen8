import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Task } from '../../lib/types'

/* The section fetches previews over RPC on click; mock the transport so the
 * real component (including the real ArtifactViewer routing) renders against
 * controlled data without a backend. */
const mockRpcCall = vi.fn()

vi.mock('../../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

const { TaskArtifactsSection, fileArtifactVPath } = await import('./TaskArtifactsSection')

function task(artifacts: string[]): Task {
  return { id: 'task-1', description: 'desc', status: 'in_review', artifacts } as Task
}

function renderSection(artifacts: string[]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TaskArtifactsSection task={task(artifacts)} projectId="proj-1" />
    </QueryClientProvider>,
  )
}

async function expandArtifacts() {
  await userEvent.click(screen.getByRole('button', { name: /artifacts/i }))
}

beforeEach(() => {
  mockRpcCall.mockReset()
  localStorage.clear()
})

describe('fileArtifactVPath', () => {
  it('extracts the vpath from file: refs and rejects everything else', () => {
    expect(fileArtifactVPath('file:/project/.agen8/attachments/task-1/shot.png'))
      .toBe('/project/.agen8/attachments/task-1/shot.png')
    expect(fileArtifactVPath('commit:abc123')).toBeNull()
    expect(fileArtifactVPath('plain prose artifact')).toBeNull()
    expect(fileArtifactVPath('file:')).toBeNull()
    expect(fileArtifactVPath('file:   ')).toBeNull()
  })
})

describe('TaskArtifactsSection', () => {
  it('renders file: refs as buttons and other refs as plain text', async () => {
    renderSection([
      'file:/project/.agen8/attachments/task-1/shot.png',
      'commit:abc123',
      'shipped the thing',
    ])
    await expandArtifacts()

    expect(screen.getByRole('button', { name: 'View shot.png' })).toBeInTheDocument()
    // Non-file refs keep their plain rendering: visible, but not interactive.
    expect(screen.getByText('commit:abc123')).toBeInTheDocument()
    expect(screen.getByText('shipped the thing')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /commit:abc123/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /shipped the thing/ })).not.toBeInTheDocument()
  })

  it('renders nothing for a task without artifacts', () => {
    const { container } = renderSection([])
    expect(container).toBeEmptyDOMElement()
  })

  it('opens the viewer and fetches the file over files.get on click', async () => {
    mockRpcCall.mockResolvedValue({
      artifact: { nodeKey: 'file:/project/notes.txt', kind: 'file', label: 'notes.txt', vpath: '/project/notes.txt' },
      content: 'hello from the attachment',
      contentKind: 'text',
      contentEncoding: 'utf8',
      truncated: false,
      bytesRead: 25,
    })
    renderSection(['file:/project/notes.txt'])
    await expandArtifacts()

    await userEvent.click(screen.getByRole('button', { name: 'View notes.txt' }))

    await waitFor(() => {
      expect(mockRpcCall).toHaveBeenCalledWith('files.get', {
        projectId: 'proj-1',
        path: '/project/notes.txt',
        maxBytes: 2_000_000,
      })
    })
    // Sheet header carries the file identity (the path may also appear in the
    // pane chrome, so assert at-least-one rather than exactly-one).
    expect((await screen.findAllByText('/project/notes.txt')).length).toBeGreaterThan(0)
    expect(await screen.findByText(/hello from the attachment/)).toBeInTheDocument()
  })

  it('shows a calm error state when the file cannot be loaded', async () => {
    mockRpcCall.mockRejectedValue(new Error('file missing'))
    renderSection(['file:/project/.agen8/attachments/task-1/gone.png'])
    await expandArtifacts()

    await userEvent.click(screen.getByRole('button', { name: 'View gone.png' }))

    expect(await screen.findByText('Failed to load file contents.')).toBeInTheDocument()
  })
})
