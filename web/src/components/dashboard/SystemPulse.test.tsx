import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import SystemPulse from './SystemPulse'

vi.mock('../../hooks/useCountUp', () => ({
  useCountUp: (value: number | string) => String(value),
}))

describe('SystemPulse', () => {
  it('shows ready spaces instead of running agents when work is quiet', () => {
    const { container } = render(
      <SystemPulse
        spaceCount={0}
        activeMissionCount={0}
        pendingEscalationCount={0}
        pendingOACount={2}
        escalationUrgencies={[]}
        focusMode={false}
      />,
    )

    expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('Waiting for your first space to start.')
    expect(screen.queryByText('cost')).toBeNull()
    expect(screen.queryByText('running')).toBeNull()
    expect(screen.queryByText('agents')).toBeNull()
    expect(container.querySelector('.system-glow-idle')).toBeTruthy()
  })

  it('shows spaces as moving work while spaces exist', () => {
    render(
      <SystemPulse
        spaceCount={1}
        activeMissionCount={0}
        pendingEscalationCount={0}
        pendingOACount={0}
        escalationUrgencies={[]}
        focusMode={false}
      />,
    )

    expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('1 space is moving.')
    expect(screen.getByText('moving')).toBeTruthy()
  })

  it('treats active missions as moving work', () => {
    const { container } = render(
      <SystemPulse
        spaceCount={0}
        activeMissionCount={1}
        pendingEscalationCount={0}
        pendingOACount={0}
        escalationUrgencies={[]}
        focusMode={false}
      />,
    )

    expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('1 active mission is moving.')
    expect(screen.getByText('active mission')).toBeTruthy()
    expect(container.querySelector('.system-glow-active')).toBeTruthy()
  })
})
