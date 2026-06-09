import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Project } from '../../lib/types'

const mockUpdateProject = vi.fn()

vi.mock('../../lib/projectClient', () => ({
  updateProject: (...args: unknown[]) => mockUpdateProject(...args),
}))

const { default: EditProjectDialog } = await import('./EditProjectDialog')

const project: Project = {
  id: 'repo-abc123',
  locationId: 'local',
  root: '/Users/tino/repo',
  title: 'repo',
  status: 'open',
}

describe('EditProjectDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUpdateProject.mockResolvedValue({ ...project })
  })

  it('keeps Save disabled until something changes', () => {
    render(<EditProjectDialog project={project} onClose={() => {}} onSaved={() => {}} />)
    expect(screen.getByRole('button', { name: /^save$/i })).toBeDisabled()
  })

  it('persists the chosen icon and color via customization', async () => {
    const user = userEvent.setup()
    const onSaved = vi.fn()
    render(<EditProjectDialog project={project} onClose={() => {}} onSaved={onSaved} />)

    await user.click(screen.getByRole('button', { name: 'rocket' }))
    await user.click(screen.getByRole('button', { name: 'blue' }))
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    expect(mockUpdateProject).toHaveBeenCalledWith('repo-abc123', {
      title: 'repo',
      customization: { icon: 'rocket', color: '#3b82f6' },
    })
    expect(onSaved).toHaveBeenCalled()
  })

  it('clears a previously set icon and color back to the defaults', async () => {
    const user = userEvent.setup()
    const customized: Project = { ...project, customization: { icon: 'rocket', color: '#3b82f6' } }
    render(<EditProjectDialog project={customized} onClose={() => {}} onSaved={() => {}} />)

    await user.click(screen.getByRole('button', { name: /no icon/i }))
    await user.click(screen.getByRole('button', { name: /default color/i }))
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    expect(mockUpdateProject).toHaveBeenCalledWith('repo-abc123', {
      title: 'repo',
      customization: { icon: '', color: '' },
    })
  })
})
