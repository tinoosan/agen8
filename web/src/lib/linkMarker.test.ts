import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  buildMarkerFiles, MARKER_DIR, supportsDirectoryPicker, writeMarkerToDirectory,
} from './linkMarker'

describe('buildMarkerFiles', () => {
  it('composes the three marker files with a project-only pointer', () => {
    const files = buildMarkerFiles({
      serverUrl: 'https://app.agen8.dev',
      projectId: 'repo-abc123',
      token: 'wlt_secrettoken',
    })

    expect(files.map((f) => f.name)).toEqual(['workspace.json', 'token', '.gitignore'])

    const pointer = JSON.parse(files[0].contents)
    expect(pointer).toEqual({ server_url: 'https://app.agen8.dev', project_id: 'repo-abc123' })
    expect('workspace_id' in pointer).toBe(false)

    expect(files[1].contents).toBe('wlt_secrettoken\n')
    expect(files[2].contents).toBe('token\n')
  })

  it('includes workspace_id only when provided', () => {
    const withWs = buildMarkerFiles({
      serverUrl: 'https://app.agen8.dev',
      projectId: 'repo-abc123',
      workspaceId: 'ws-1',
      token: 'wlt_x',
    })
    expect(JSON.parse(withWs[0].contents)).toEqual({
      server_url: 'https://app.agen8.dev',
      project_id: 'repo-abc123',
      workspace_id: 'ws-1',
    })

    const blankWs = buildMarkerFiles({
      serverUrl: 'https://app.agen8.dev',
      projectId: 'repo-abc123',
      workspaceId: '   ',
      token: 'wlt_x',
    })
    expect('workspace_id' in JSON.parse(blankWs[0].contents)).toBe(false)
  })

  it('trims whitespace from inputs', () => {
    const files = buildMarkerFiles({
      serverUrl: '  https://app.agen8.dev  ',
      projectId: '  repo-abc123  ',
      token: '  wlt_x  ',
    })
    expect(JSON.parse(files[0].contents)).toEqual({
      server_url: 'https://app.agen8.dev',
      project_id: 'repo-abc123',
    })
    expect(files[1].contents).toBe('wlt_x\n')
  })

  it('throws loudly rather than emitting a half-written marker', () => {
    const base = { serverUrl: 'https://x', projectId: 'p', token: 'wlt_x' }
    expect(() => buildMarkerFiles({ ...base, serverUrl: '' })).toThrow(/server URL/)
    expect(() => buildMarkerFiles({ ...base, projectId: '  ' })).toThrow(/project id/)
    expect(() => buildMarkerFiles({ ...base, token: '' })).toThrow(/link token/)
  })
})

describe('directory picker support', () => {
  afterEach(() => {
    delete (window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker
    vi.restoreAllMocks()
  })

  it('reports false when the File System Access API is absent', () => {
    delete (window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker
    expect(supportsDirectoryPicker()).toBe(false)
  })

  it('reports true when showDirectoryPicker exists', () => {
    ;(window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker = () => Promise.resolve({})
    expect(supportsDirectoryPicker()).toBe(true)
  })

  it('fails loudly when asked to write without the API', async () => {
    delete (window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker
    await expect(
      writeMarkerToDirectory([{ name: 'token', contents: 'wlt_x\n' }]),
    ).rejects.toThrow(/cannot write folders/)
  })

  it('writes every marker file into a created .agen8 directory', async () => {
    const writes: Array<{ name: string; data: string }> = []
    const fileHandle = (name: string) => ({
      createWritable: async () => ({
        write: async (data: string) => { writes.push({ name, data }) },
        close: async () => {},
      }),
    })
    const dirHandle = {
      getFileHandle: async (name: string, opts?: { create?: boolean }) => {
        expect(opts?.create).toBe(true)
        return fileHandle(name)
      },
    }
    const getDirectoryHandle = vi.fn(async (name: string, opts?: { create?: boolean }) => {
      expect(name).toBe(MARKER_DIR)
      expect(opts?.create).toBe(true)
      return dirHandle
    })
    ;(window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker = vi.fn(
      async () => ({ getDirectoryHandle }),
    )

    const files = [
      { name: 'workspace.json', contents: '{}\n' },
      { name: 'token', contents: 'wlt_x\n' },
      { name: '.gitignore', contents: 'token\n' },
    ]
    await writeMarkerToDirectory(files)

    expect(getDirectoryHandle).toHaveBeenCalledTimes(1)
    expect(writes).toEqual([
      { name: 'workspace.json', data: '{}\n' },
      { name: 'token', data: 'wlt_x\n' },
      { name: '.gitignore', data: 'token\n' },
    ])
  })
})
