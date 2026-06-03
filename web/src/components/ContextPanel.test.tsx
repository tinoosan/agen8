import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { FilesListDirResult } from '../lib/types'

const mockUseArtifactFiles = vi.fn()
const mockUseProjectArtifactFiles = vi.fn()
const mockRpcCall = vi.fn()
let mockFocusedProjectRoot: string | null = null

vi.mock('../hooks/useArtifactFiles', () => ({
  useArtifactFiles: (...args: unknown[]) => mockUseArtifactFiles(...args),
  useProjectArtifactFiles: (...args: unknown[]) => mockUseProjectArtifactFiles(...args),
}))

vi.mock('../lib/routing', () => ({
  useNavigation: () => ({ projectId: 'project-1', focusedProjectRoot: mockFocusedProjectRoot }),
}))

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  onNotification: () => () => {},
}))

const { default: ContextPanel } = await import('./ContextPanel')

function renderPanel(props?: Partial<ComponentProps<typeof ContextPanel>>) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ContextPanel spaceId="space-1" {...props} />
    </QueryClientProvider>,
  )
}

describe('ContextPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFocusedProjectRoot = null
    try { localStorage.clear() } catch { /* ignore */ }
    const queryState = { data: [], isFetched: true, isFetching: false, isLoading: false }
    mockUseArtifactFiles.mockReturnValue(queryState)
    mockUseProjectArtifactFiles.mockReturnValue(queryState)
    mockRpcCall.mockResolvedValue({ artifact: { kind: 'file', label: 'placeholder' }, content: '', truncated: false, bytesRead: 0 })
  })

  it('renders the context rail without the removed Plan surface by default', () => {
    renderPanel({ projectId: 'project-1' })
    expect(screen.queryByRole('button', { name: 'Plan' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open file' })).toBeInTheDocument()
  })

  it('shows task requests without persisting a Plan fallback', async () => {
    renderPanel({
      spaceId: 'space-ad-hoc',
      taskOpenRequest: {
        id: 1,
        status: 'active',
        task: {
          id: 'task-1',
          spaceId: 'space-ad-hoc',
          title: 'Review plan',
          status: 'active',
        },
      },
    })

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Review plan/i })).toBeInTheDocument()
    })
    expect(localStorage.getItem('oa-context-view:space-ad-hoc')).toBeNull()
  })

  it('opens absolute .agen8/workspace links by mapping them to /workspace vpath', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/playground'
    const onFileOpenRequestHandled = vi.fn()
    renderPanel({
      spaceId: null,
      fileOpenRequest: {
        id: 7,
        path: '/Users/santino.onyeme/Projects/agen8/playground/.agen8/workspace/dummy_research_report_4.md',
      },
      onFileOpenRequestHandled,
    })

    await waitFor(() => {
      expect(onFileOpenRequestHandled).toHaveBeenCalledWith(7)
    })
    await waitFor(() => {
      expect(screen.getByTitle('/workspace/dummy_research_report_4.md')).toBeInTheDocument()
    })
  })

  it('opens project-name-prefixed root files as project files', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/playground'
    const onFileOpenRequestHandled = vi.fn()
    renderPanel({
      spaceId: null,
      fileOpenRequest: {
        id: 8,
        path: 'playground/AGENT_WISHLIST.md',
      },
      onFileOpenRequestHandled,
    })

    await waitFor(() => {
      expect(onFileOpenRequestHandled).toHaveBeenCalledWith(8)
    })
    await waitFor(() => {
      expect(screen.getByTitle('/project/AGENT_WISHLIST.md')).toBeInTheDocument()
    })
  })

  it('opens bare relative project file links as project files', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/homelab'
    const onFileOpenRequestHandled = vi.fn()
    renderPanel({
      spaceId: null,
      fileOpenRequest: {
        id: 9,
        path: 'docs/homelab-inventory-risk-report-2026-05-31.md',
      },
      onFileOpenRequestHandled,
    })

    await waitFor(() => {
      expect(onFileOpenRequestHandled).toHaveBeenCalledWith(9)
    })
    await waitFor(() => {
      expect(screen.getByTitle('/project/docs/homelab-inventory-risk-report-2026-05-31.md')).toBeInTheDocument()
    })
  })

  it('shows the actual project root name in file viewer breadcrumbs', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/homelab'
    mockRpcCall.mockImplementation(async (method: string, params?: Record<string, unknown>) => {
      if (method === 'files.listDir') {
        const path = typeof params?.path === 'string' ? params.path : '/project'
        return { path, entries: [] }
      }
      if (method === 'files.get' || method === 'artifact.get') {
        return {
          artifact: { kind: 'file', label: 'ks-apps.yaml', vpath: '/project/clusters/mugiwara/ks-apps.yaml' },
          content: 'apiVersion: kustomize.toolkit.fluxcd.io/v1',
          truncated: false,
          bytesRead: 44,
        }
      }
      throw new Error(`unexpected rpc method: ${method}`)
    })

    renderPanel({
      projectId: 'project-1',
      fileOpenRequest: { id: 12, path: 'homelab/clusters/mugiwara/ks-apps.yaml' },
    })

    const breadcrumb = await screen.findByLabelText('breadcrumb')
    await waitFor(() => {
      expect(within(breadcrumb).getByText('homelab')).toBeInTheDocument()
    })
    expect(within(breadcrumb).queryByText('project')).not.toBeInTheDocument()
  })

  it('opens a typed project path from the open-file modal without recursively indexing the project', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/playground'
    mockRpcCall.mockImplementation(async (method: string, params?: Record<string, unknown>) => {
      if (method === 'files.listDir') {
        const path = typeof params?.path === 'string' ? params.path : '/project'
        const table: Record<string, FilesListDirResult> = {
          '/project': {
            path: '/project',
            entries: [
              { name: 'src', path: '/project/src', isDir: true, writable: true },
            ],
          },
          '/project/src': {
            path: '/project/src',
            entries: [
              { name: 'main.ts', path: '/project/src/main.ts', isDir: false, writable: true },
            ],
          },
        }
        return table[path] ?? { path, entries: [] }
      }
      if (method === 'artifact.get') {
        return {
          artifact: { kind: 'file', label: 'main.ts', vpath: '/project/src/main.ts' },
          content: 'console.log("hello")',
          truncated: false,
          bytesRead: 20,
        }
      }
      if (method === 'files.get') {
        return {
          artifact: { kind: 'file', label: 'main.ts', vpath: '/project/src/main.ts' },
          content: 'console.log("hello")',
          truncated: false,
          bytesRead: 20,
        }
      }
      throw new Error(`unexpected rpc method: ${method}`)
    })

    renderPanel({ projectId: 'project-1' })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Open file' }))

    const searchInput = await screen.findByRole('textbox', { name: 'Search files' })
    expect(searchInput).toBeInTheDocument()
    await waitFor(() => {
      expect(
        mockRpcCall.mock.calls.some(([method, params]) =>
          method === 'files.listDir'
          && typeof params === 'object'
          && params !== null
          && (params as Record<string, unknown>).includeHidden === true
          && (params as Record<string, unknown>).projectId === 'project-1'
          && (params as Record<string, unknown>).path === '/project'),
      ).toBe(true)
    })

    await user.type(searchInput, 'src/main.ts')
    await waitFor(() => {
      expect(screen.getByText('/project/src/main.ts')).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: /main\.ts/i }))

    await waitFor(() => {
      expect(screen.getByTitle('/project/src/main.ts')).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(
        mockRpcCall.mock.calls.some(([method, params]) =>
          method === 'files.get'
          && typeof params === 'object'
          && params !== null
          && (params as Record<string, unknown>).path === '/project/src/main.ts'),
      ).toBe(true)
    })
  })

  it('shows the file tree rail only while a file tab is active', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/playground'
    // The rail is fed by the project filesystem index (files.listDir), not agent artifacts.
    mockRpcCall.mockImplementation(async (method: string, params?: Record<string, unknown>) => {
      if (method === 'files.listDir') {
        const path = typeof params?.path === 'string' ? params.path : '/project'
        if (path !== '/project') return { path, entries: [] }
        return {
          path: '/project',
          entries: [
            { name: 'report.md', path: '/project/report.md', isDir: false, writable: true },
            { name: 'alpha.md', path: '/project/alpha.md', isDir: false, writable: true },
          ],
        }
      }
      if (method === 'files.get' || method === 'artifact.get') {
        return {
          artifact: { kind: 'file', label: 'report.md', vpath: '/project/report.md' },
          content: '# report',
          truncated: false,
          bytesRead: 8,
        }
      }
      throw new Error(`unexpected rpc method: ${method}`)
    })

    const user = userEvent.setup()
    renderPanel({
      spaceId: 'space-1',
      projectId: 'project-1',
      fileOpenRequest: { id: 11, path: 'playground/report.md' },
    })

    // In a file view the viewer opens first and the filesystem rail stays hidden until requested.
    await waitFor(() => {
      expect(screen.getByTitle('/project/report.md')).toBeInTheDocument()
    })
    expect(screen.getByTitle('/project/report.md')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /alpha\.md/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show file rail' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Show file rail' }))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /alpha\.md/i })).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: 'Hide file rail' }))
    expect(screen.getByTitle('/project/report.md')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /alpha\.md/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show file rail' })).toBeInTheDocument()

    // Closing the active file tab keeps the rail absent instead of falling back to the removed Plan surface.
    await user.click(screen.getByRole('button', { name: /Close report\.md/i }))
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /alpha\.md/i })).not.toBeInTheDocument()
    })
    expect(screen.queryByRole('button', { name: 'Plan' })).not.toBeInTheDocument()
  })

  it('populates the file tree rail from the project file index when no agent artifacts exist', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/playground'
    mockRpcCall.mockImplementation(async (method: string, params?: Record<string, unknown>) => {
      if (method === 'files.listDir') {
        const path = typeof params?.path === 'string' ? params.path : '/project'
        if (path !== '/project') return { path, entries: [] }
        return {
          path: '/project',
          entries: [
            { name: 'kustomization.yaml', path: '/project/kustomization.yaml', isDir: false, writable: true },
            { name: 'README.md', path: '/project/README.md', isDir: false, writable: true },
          ],
        }
      }
      if (method === 'files.get' || method === 'artifact.get') {
        return {
          artifact: { kind: 'file', label: 'kustomization.yaml', vpath: '/project/kustomization.yaml' },
          content: 'resources:\n  - clusters/mugiwara',
          truncated: false,
          bytesRead: 30,
        }
      }
      throw new Error(`unexpected rpc method: ${method}`)
    })

    const user = userEvent.setup()
    renderPanel({
      spaceId: 'space-1',
      projectId: 'project-1',
      fileOpenRequest: { id: 21, path: 'playground/kustomization.yaml' },
    })

    // The opened file renders in the viewer.
    await waitFor(() => {
      expect(screen.getByTitle('/project/kustomization.yaml')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: 'Show file rail' }))

    // A sibling that exists ONLY in the project file index appears in the rail —
    // proving the rail is fed by the filesystem index, not the empty agent-artifact list.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /README\.md/i })).toBeInTheDocument()
    })

    // The root folder is labeled with the repo name (basename of the project root),
    // not the virtual "project" namespace segment.
    expect(screen.getByRole('button', { name: /playground/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^project\b/i })).not.toBeInTheDocument()
  })

  it('does not recursively index heavyweight project directories in the rail', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/playground'
    mockRpcCall.mockImplementation(async (method: string, params?: Record<string, unknown>) => {
      if (method === 'files.listDir') {
        const path = typeof params?.path === 'string' ? params.path : '/project'
        if (path === '/project') {
          return {
            path: '/project',
            entries: [
              { name: 'src', path: '/project/src', isDir: true, writable: true },
              { name: 'node_modules', path: '/project/node_modules', isDir: true, writable: true },
            ],
          }
        }
        if (path === '/project/src') {
          return {
            path,
            entries: [
              { name: 'main.ts', path: '/project/src/main.ts', isDir: false, writable: true },
            ],
          }
        }
        if (path === '/project/node_modules') {
          throw new Error('node_modules should not be indexed')
        }
        return { path, entries: [] }
      }
      if (method === 'files.get' || method === 'artifact.get') {
        return {
          artifact: { kind: 'file', label: 'main.ts', vpath: '/project/src/main.ts' },
          content: 'console.log("hello")',
          truncated: false,
          bytesRead: 20,
        }
      }
      throw new Error(`unexpected rpc method: ${method}`)
    })

    renderPanel({
      spaceId: 'space-1',
      projectId: 'project-1',
      fileOpenRequest: { id: 22, path: 'playground/src/main.ts' },
    })

    await waitFor(() => {
      expect(screen.getByTitle('/project/src/main.ts')).toBeInTheDocument()
    })
    expect(
      mockRpcCall.mock.calls.some(([method, params]) =>
        method === 'files.listDir'
        && typeof params === 'object'
        && params !== null
        && (params as Record<string, unknown>).path === '/project/node_modules'),
    ).toBe(false)
  })

  it('shows root folders in the rail and loads folder contents on expansion', async () => {
    mockFocusedProjectRoot = '/Users/santino.onyeme/Projects/agen8/playground'
    mockRpcCall.mockImplementation(async (method: string, params?: Record<string, unknown>) => {
      if (method === 'files.listDir') {
        const path = typeof params?.path === 'string' ? params.path : '/project'
        if (path === '/project') {
          return {
            path,
            entries: [
              { name: 'kustomization.yaml', path: '/project/kustomization.yaml', isDir: false, writable: true },
              { name: 'clusters', path: '/project/clusters', isDir: true, writable: true },
            ],
          }
        }
        if (path === '/project/clusters') {
          return {
            path,
            entries: [
              { name: 'mugiwara', path: '/project/clusters/mugiwara', isDir: true, writable: true },
              { name: 'cluster.yaml', path: '/project/clusters/cluster.yaml', isDir: false, writable: true },
            ],
          }
        }
        return { path, entries: [] }
      }
      if (method === 'files.get' || method === 'artifact.get') {
        return {
          artifact: { kind: 'file', label: 'kustomization.yaml', vpath: '/project/kustomization.yaml' },
          content: 'resources:\n  - clusters/mugiwara',
          truncated: false,
          bytesRead: 30,
        }
      }
      throw new Error(`unexpected rpc method: ${method}`)
    })

    const user = userEvent.setup()
    renderPanel({
      spaceId: 'space-1',
      projectId: 'project-1',
      fileOpenRequest: { id: 23, path: 'playground/kustomization.yaml' },
    })

    await waitFor(() => {
      expect(screen.getByTitle('/project/kustomization.yaml')).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: 'Show file rail' }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /clusters/i })).toBeInTheDocument()
    })
    expect(screen.queryByRole('button', { name: /cluster\.yaml/i })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /clusters/i }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /cluster\.yaml/i })).toBeInTheDocument()
    })
    expect(
      mockRpcCall.mock.calls.some(([method, params]) =>
        method === 'files.listDir'
        && typeof params === 'object'
        && params !== null
        && (params as Record<string, unknown>).path === '/project/clusters'),
    ).toBe(true)
  })

})
