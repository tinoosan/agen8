import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import Sparkline from './Sparkline'

describe('Sparkline', () => {
  it('draws one bar per non-zero bucket (zero days are blank, not interpolated)', () => {
    const { container } = render(<Sparkline data={[0, 1, 2, 0, 3]} />)
    // 3 non-zero values → 3 <rect> bars; the two zero days render nothing.
    expect(container.querySelectorAll('rect').length).toBe(3)
  })

  it('renders a flat baseline (no bars) when there is no activity', () => {
    const { container } = render(<Sparkline data={[0, 0, 0]} />)
    expect(container.querySelectorAll('rect').length).toBe(0)
    expect(container.querySelector('line')).toBeTruthy()
  })

  it('renders a flat baseline for an empty series', () => {
    const { container } = render(<Sparkline data={[]} />)
    expect(container.querySelectorAll('rect').length).toBe(0)
    expect(container.querySelector('line')).toBeTruthy()
  })

  it('exposes an accessible label describing the trend', () => {
    const { container } = render(<Sparkline data={[1, 2]} label="3 completed in the last 2 days" />)
    const svg = container.querySelector('svg')
    expect(svg?.getAttribute('role')).toBe('img')
    expect(svg?.getAttribute('aria-label')).toBe('3 completed in the last 2 days')
  })

  it('falls back to a derived label when none is given', () => {
    const { container } = render(<Sparkline data={[2, 3]} />)
    expect(container.querySelector('svg')?.getAttribute('aria-label')).toBe('5 completed across 2 days')
  })
})
