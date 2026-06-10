import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import DiffView from './DiffView'

describe('DiffView', () => {
  it('marks added and removed lines and counts them in the header', () => {
    render(
      <DiffView
        baseline={'keep\nold line\nkeep too\n'}
        current={'keep\nnew line\nkeep too\nappended\n'}
      />,
    )
    expect(screen.getByTestId('diff-view')).toBeInTheDocument()
    expect(screen.getByText('old line')).toBeInTheDocument()
    expect(screen.getByText('new line')).toBeInTheDocument()
    expect(screen.getByText('appended')).toBeInTheDocument()
    expect(screen.getByText('+2')).toBeInTheDocument()
    expect(screen.getByText('−1')).toBeInTheDocument()
  })

  it('syntax-highlights diff lines when the file language is known', () => {
    const { container } = render(
      <DiffView
        baseline={'func main() {}\n'}
        current={'func main() {}\nfunc added() {}\n'}
        filePath="/project/main.go"
      />,
    )
    // PrismLight emits token spans for recognized syntax (e.g. the func keyword).
    expect(container.querySelectorAll('.token').length).toBeGreaterThan(0)
  })

  it('falls back to plain text for unknown extensions', () => {
    const { container } = render(
      <DiffView baseline={'a\n'} current={'a\nb\n'} filePath="/project/data.weird" />,
    )
    expect(container.querySelectorAll('.token').length).toBe(0)
    expect(screen.getByText('b')).toBeInTheDocument()
  })

  it('shows an explicit no-changes state for identical content', () => {
    render(<DiffView baseline={'same\ncontent\n'} current={'same\ncontent\n'} />)
    expect(screen.getByText(/No uncommitted changes/)).toBeInTheDocument()
    expect(screen.queryByTestId('diff-view')).not.toBeInTheDocument()
  })
})
