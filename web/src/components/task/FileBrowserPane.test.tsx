import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockRpcCall = vi.fn()
vi.mock('../../lib/rpc', () => ({ rpcCall: (...a: unknown[]) => mockRpcCall(...a) }))

const { FileBrowserPane } = await import('./FileBrowserPane')

function listResult(path: string, entries: Array<{ name: string; isDir: boolean }>) {
  return {
    path,
    entries: entries.map((e) => ({ name: e.name, path: `${path}/${e.name}`.replace('//', '/'), isDir: e.isDir, writable: false })),
  }
}

function renderPane(onSelect = vi.fn(), initialDir = '/project/src') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <FileBrowserPane projectId="p1" initialDir={initialDir} activeVPath="/project/src/App.tsx" onSelectFile={onSelect} />
    </QueryClientProvider>,
  )
  return onSelect
}

beforeEach(() => mockRpcCall.mockReset())

describe('FileBrowserPane', () => {
  it('lists the initial directory with folders before files', async () => {
    mockRpcCall.mockResolvedValue(listResult('/project/src', [
      { name: 'zeta.ts', isDir: false },
      { name: 'components', isDir: true },
      { name: 'App.tsx', isDir: false },
    ]))
    renderPane()

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledWith('files.listDir', { projectId: 'p1', path: '/project/src' }))
    const folder = await screen.findByText('components')
    const file = await screen.findByText('App.tsx')
    // The directory sorts ahead of the files: DOCUMENT_POSITION_FOLLOWING (4)
    // means `file` comes after `folder` in document order.
    expect(folder.compareDocumentPosition(file) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('navigates into a folder on click (new files.listDir)', async () => {
    mockRpcCall.mockImplementation(async (_m: unknown, params?: { path?: string }) => {
      if (params?.path === '/project/src') return listResult('/project/src', [{ name: 'components', isDir: true }])
      return listResult('/project/src/components', [{ name: 'Button.tsx', isDir: false }])
    })
    renderPane()

    await userEvent.click(await screen.findByText('components'))
    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledWith('files.listDir', { projectId: 'p1', path: '/project/src/components' }))
    expect(await screen.findByText('Button.tsx')).toBeInTheDocument()
  })

  it('reports a file click to the parent', async () => {
    mockRpcCall.mockResolvedValue(listResult('/project/src', [{ name: 'main.go', isDir: false }]))
    const onSelect = renderPane()
    await userEvent.click(await screen.findByText('main.go'))
    expect(onSelect).toHaveBeenCalledWith('/project/src/main.go')
  })

  it('goes up a directory with the up control', async () => {
    mockRpcCall.mockResolvedValue(listResult('/project/src', [{ name: 'a.ts', isDir: false }]))
    renderPane()
    await screen.findByText('a.ts')
    await userEvent.click(screen.getByRole('button', { name: /up one directory/i }))
    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledWith('files.listDir', { projectId: 'p1', path: '/project' }))
  })
})
