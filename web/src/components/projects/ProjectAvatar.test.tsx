import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { Project } from '../../lib/types'
import { ProjectAvatar } from './ProjectAvatar'

const base: Project = {
  id: 'repo-abc123',
  locationId: 'local',
  root: '/Users/tino/payments-api',
  status: 'open',
}

describe('ProjectAvatar', () => {
  it('renders the curated icon glyph when one is set', () => {
    render(<ProjectAvatar project={{ ...base, customization: { icon: 'rocket', color: '#3b82f6' } }} />)
    const avatar = screen.getByTestId('project-avatar')
    // Lucide renders an <svg>; the monogram path is not taken.
    expect(avatar.querySelector('svg')).toBeInTheDocument()
    expect(avatar).toHaveTextContent('')
  })

  it('falls back to the display-name monogram when no icon is set', () => {
    render(<ProjectAvatar project={{ ...base, title: 'Payments API' }} />)
    const avatar = screen.getByTestId('project-avatar')
    expect(avatar.querySelector('svg')).not.toBeInTheDocument()
    // First letter of the title.
    expect(avatar).toHaveTextContent('P')
  })

  it('derives the monogram from the folder name when there is no title', () => {
    render(<ProjectAvatar project={base} />)
    expect(screen.getByTestId('project-avatar')).toHaveTextContent('P')
  })

  it('ignores an unknown icon name and shows the monogram instead', () => {
    render(<ProjectAvatar project={{ ...base, title: 'Zeta', customization: { icon: 'not-a-real-icon' } }} />)
    const avatar = screen.getByTestId('project-avatar')
    expect(avatar.querySelector('svg')).not.toBeInTheDocument()
    expect(avatar).toHaveTextContent('Z')
  })
})
