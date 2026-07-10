import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Project } from '../../lib/types'

const mockRpcCall = vi.fn()
const mockRelocateProject = vi.fn()

vi.mock('../../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))
vi.mock('../../lib/projectClient', () => ({
  relocateProject: (...args: unknown[]) => mockRelocateProject(...args),
}))

const { default: RelocateProjectDialog } = await import('./RelocateProjectDialog')

const project: Project = {
  id: 'project-1',
  locationId: 'local',
  root: '/repo/old',
  title: 'Project',
  status: 'open',
}

describe('RelocateProjectDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRpcCall.mockImplementation((_method: string, params: { path: string }) => Promise.resolve({
      entries: params.path === '/repo/old'
        ? [{ name: 'renamed', path: '/repo/old/renamed', type: 'directory' }]
        : [],
    }))
    mockRelocateProject.mockResolvedValue({ ...project, root: '/repo/old/renamed' })
  })

  it('browses and confirms a replacement folder without changing the project id', async () => {
    const user = userEvent.setup()
    const onRelocated = vi.fn()
    render(<RelocateProjectDialog project={project} onClose={() => {}} onRelocated={onRelocated} />)

    await user.click(await screen.findByRole('button', { name: /renamed/i }))
    await user.click(screen.getByRole('button', { name: /select folder/i }))
    await user.click(screen.getByRole('button', { name: /use this folder/i }))

    expect(mockRelocateProject).toHaveBeenCalledWith('project-1', '/repo/old/renamed')
    expect(onRelocated).toHaveBeenCalledWith(expect.objectContaining({ id: 'project-1', root: '/repo/old/renamed' }))
  })
})
