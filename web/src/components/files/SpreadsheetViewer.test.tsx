import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import SpreadsheetViewer from './SpreadsheetViewer'
import type { ArtifactNode, ArtifactGetResult } from '../../lib/types'

function makeFile(vpath: string): ArtifactNode {
  return {
    nodeKey: 'test',
    kind: 'file',
    label: vpath.split('/').pop() ?? vpath,
    vpath,
  }
}

function makePreview(overrides: Partial<ArtifactGetResult> = {}): ArtifactGetResult {
  return {
    artifact: makeFile('/workspace/test.csv'),
    content: '',
    truncated: false,
    bytesRead: 0,
    ...overrides,
  }
}

describe('SpreadsheetViewer', () => {
  it('renders loading skeleton', () => {
    const { container } = render(
      <SpreadsheetViewer file={makeFile('/workspace/data.csv')} preview={undefined} isLoading={true} error={false} />
    )
    const skeletons = container.querySelectorAll('[class*="skeleton"], [data-slot="skeleton"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('renders error state when fetch fails', () => {
    render(
      <SpreadsheetViewer file={makeFile('/workspace/data.csv')} preview={undefined} isLoading={false} error={true} />
    )
    expect(screen.getByText('Failed to load file')).toBeTruthy()
  })

  it('renders empty state for empty CSV', () => {
    render(
      <SpreadsheetViewer file={makeFile('/workspace/empty.csv')} preview={makePreview({ content: '' })} isLoading={false} error={false} />
    )
    expect(screen.getByText('No data found')).toBeTruthy()
  })

  it('renders CSV data as table', () => {
    const csvContent = 'Name,Age,City\nAlice,30,NYC\nBob,25,LA'
    render(
      <SpreadsheetViewer file={makeFile('/workspace/data.csv')} preview={makePreview({ content: csvContent })} isLoading={false} error={false} />
    )
    expect(screen.getByText('Name')).toBeTruthy()
    expect(screen.getByText('Age')).toBeTruthy()
    expect(screen.getByText('City')).toBeTruthy()
    expect(screen.getByText('Alice')).toBeTruthy()
    expect(screen.getByText('Bob')).toBeTruthy()
  })

  it('shows truncation badge when file is truncated', () => {
    render(
      <SpreadsheetViewer file={makeFile('/workspace/big.csv')} preview={makePreview({ content: 'A,B\n1,2', truncated: true })} isLoading={false} error={false} />
    )
    expect(screen.getByText('Truncated')).toBeTruthy()
  })

  it('renders export buttons', () => {
    render(
      <SpreadsheetViewer file={makeFile('/workspace/data.csv')} preview={makePreview({ content: 'X,Y\n1,2' })} isLoading={false} error={false} />
    )
    expect(screen.getByText('CSV')).toBeTruthy()
    expect(screen.getByText('XLSX')).toBeTruthy()
  })

  it('renders file name in toolbar', () => {
    render(
      <SpreadsheetViewer file={makeFile('/workspace/report.csv')} preview={makePreview({ content: 'A,B\n1,2' })} isLoading={false} error={false} />
    )
    expect(screen.getByText('report.csv')).toBeTruthy()
  })

  it('shows null values as dash', () => {
    const csvContent = 'A,B\n1,'
    render(
      <SpreadsheetViewer file={makeFile('/workspace/data.csv')} preview={makePreview({ content: csvContent })} isLoading={false} error={false} />
    )
    expect(screen.getByText('—')).toBeTruthy()
  })
})
