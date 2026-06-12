import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Task } from '../../lib/types'

const mockRpcCall = vi.fn()
const mockUploadFile = vi.fn()
const mockToastError = vi.fn()
const mockToastSuccess = vi.fn()

vi.mock('../../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  uploadFile: (...args: unknown[]) => mockUploadFile(...args),
}))
vi.mock('sonner', () => ({
  toast: {
    error: (...args: unknown[]) => mockToastError(...args),
    success: (...args: unknown[]) => mockToastSuccess(...args),
  },
}))

const { TaskArtifactsSection, fileArtifactVPath, artifactNote } = await import('./TaskArtifactsSection')

function task(artifacts: string[], status = 'in_review'): Task {
  return { id: 'task-1', description: 'desc', status, artifacts } as Task
}

function renderSection(artifacts: string[], opts: { onOpen?: ReturnType<typeof vi.fn>; status?: string; projectId?: string | null } = {}) {
  const onOpen = opts.onOpen ?? vi.fn()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <TaskArtifactsSection
        task={task(artifacts, opts.status)}
        projectId={opts.projectId === undefined ? 'proj-1' : opts.projectId}
        onOpenArtifact={onOpen}
      />
    </QueryClientProvider>,
  )
  return onOpen
}

async function expandArtifacts() {
  await userEvent.click(screen.getByRole('button', { name: /artifacts/i }))
}

beforeEach(() => {
  mockRpcCall.mockReset()
  mockUploadFile.mockReset()
  mockToastError.mockReset()
  mockToastSuccess.mockReset()
  localStorage.clear()
})

describe('fileArtifactVPath', () => {
  it('extracts the vpath from file: refs', () => {
    expect(fileArtifactVPath('file:/project/.agen8/attachments/task-1/shot.png'))
      .toBe('/project/.agen8/attachments/task-1/shot.png')
    expect(fileArtifactVPath('file:')).toBeNull()
    expect(fileArtifactVPath('file:   ')).toBeNull()
  })

  it('passes through strings that are already vpaths', () => {
    expect(fileArtifactVPath('/project/web/src/App.tsx')).toBe('/project/web/src/App.tsx')
    expect(fileArtifactVPath('/workspace/notes.md')).toBe('/workspace/notes.md')
  })

  it('resolves bare agent relative paths to /project vpaths', () => {
    expect(fileArtifactVPath('internal/services/file/app/service.go'))
      .toBe('/project/internal/services/file/app/service.go')
    expect(fileArtifactVPath('web/src/App.tsx')).toBe('/project/web/src/App.tsx')
    expect(fileArtifactVPath('./web/src/x.ts')).toBe('/project/web/src/x.ts')
    expect(fileArtifactVPath('README.md')).toBe('/project/README.md') // root file via extension
  })

  it('resolves the leading path token from a "path (note)" artifact', () => {
    expect(fileArtifactVPath('internal/services/mission/app/events.go (keyResultProjectID + projectId in KR events)'))
      .toBe('/project/internal/services/mission/app/events.go')
    expect(artifactNote('internal/services/mission/app/events.go (keyResultProjectID in KR events)'))
      .toBe('(keyResultProjectID in KR events)')
  })

  it('rejects scheme refs and prose', () => {
    expect(fileArtifactVPath('commit:abc123')).toBeNull()
    expect(fileArtifactVPath('https://example.com/x')).toBeNull()
    expect(fileArtifactVPath('plain prose artifact')).toBeNull()
    expect(fileArtifactVPath('shipped the thing')).toBeNull()
    expect(fileArtifactVPath('Makefile')).toBeNull() // no slash, no extension -> not resolved
    expect(fileArtifactVPath('commit ecfbe914 fix(web): restore margin')).toBeNull() // prose starting with a word
    expect(fileArtifactVPath('decision dec-97f32d57 made during review')).toBeNull()
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
    expect(screen.getByText('commit:abc123')).toBeInTheDocument()
    expect(screen.getByText('shipped the thing')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /commit:abc123/ })).not.toBeInTheDocument()
  })

  it('reports the clicked vpath to the parent viewer host', async () => {
    const onOpen = renderSection(['file:/project/web/src/App.tsx'])
    await expandArtifacts()
    await userEvent.click(screen.getByRole('button', { name: 'View App.tsx' }))
    expect(onOpen).toHaveBeenCalledWith('/project/web/src/App.tsx')
  })

  it('shows the attach affordance even when the task has no artifacts yet', async () => {
    renderSection([])
    await expandArtifacts()
    expect(screen.getByRole('button', { name: /attach file/i })).toBeInTheDocument()
  })

  it('hides the attach affordance for canceled tasks', () => {
    const { } = { ...renderSection([], { status: 'canceled' }) }
    expect(screen.queryByRole('button', { name: /artifacts/i })).not.toBeInTheDocument()
  })

  it('uploads a picked file then appends the ref server-side', async () => {
    mockRpcCall.mockResolvedValue({})
    mockUploadFile.mockResolvedValue({})
    renderSection([])
    await expandArtifacts()

    const input = screen.getByLabelText('Attachment file') as HTMLInputElement
    const file = new File(['screenshot bytes'], 'build-shot.png', { type: 'image/png' })
    await userEvent.upload(input, file)

    await waitFor(() => {
      expect(mockUploadFile).toHaveBeenCalledWith(expect.objectContaining({
        projectId: 'proj-1',
        path: '/project/.agen8/attachments/task-1/build-shot.png',
        file,
        fileName: 'build-shot.png',
      }))
    })
    expect(mockRpcCall).toHaveBeenCalledWith('task.attachArtifact', {
      taskId: 'task-1',
      ref: 'file:/project/.agen8/attachments/task-1/build-shot.png',
    })
    expect(mockToastSuccess).toHaveBeenCalled()
  })

  it('surfaces upload failures and never appends a ref', async () => {
    mockUploadFile.mockRejectedValue(new Error('disk full'))
    renderSection([])
    await expandArtifacts()

    const input = screen.getByLabelText('Attachment file') as HTMLInputElement
    await userEvent.upload(input, new File(['x'], 'doomed.txt', { type: 'text/plain' }))

    await waitFor(() => expect(mockToastError).toHaveBeenCalled())
    expect(mockRpcCall).not.toHaveBeenCalledWith('task.attachArtifact', expect.anything())
  })

  it('attaches a pasted image from the clipboard', async () => {
    mockRpcCall.mockResolvedValue({})
    mockUploadFile.mockResolvedValue({})
    renderSection([])

    const blob = new File(['png bytes'], 'clip.png', { type: 'image/png' })
    const event = new Event('paste', { bubbles: true, cancelable: true }) as ClipboardEvent
    Object.defineProperty(event, 'clipboardData', {
      value: { items: [{ type: 'image/png', getAsFile: () => blob }] },
    })
    document.dispatchEvent(event)

    await waitFor(() => {
      expect(mockUploadFile).toHaveBeenCalledWith(expect.objectContaining({
        path: expect.stringMatching(/^\/project\/\.agen8\/attachments\/task-1\/pasted-.*\.png$/),
      }))
    })
    expect(mockRpcCall).toHaveBeenCalledWith('task.attachArtifact', expect.objectContaining({ taskId: 'task-1' }))
  })

  it('ignores pastes aimed at editable fields', async () => {
    renderSection([])
    const input = document.createElement('input')
    document.body.appendChild(input)
    const blob = new File(['png'], 'clip.png', { type: 'image/png' })
    const event = new Event('paste', { bubbles: true, cancelable: true }) as ClipboardEvent
    Object.defineProperty(event, 'clipboardData', {
      value: { items: [{ type: 'image/png', getAsFile: () => blob }] },
    })
    input.dispatchEvent(event)
    await new Promise((r) => setTimeout(r, 50))
    expect(mockRpcCall).not.toHaveBeenCalled()
    expect(mockUploadFile).not.toHaveBeenCalled()
    input.remove()
  })
})
