import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import CodeView from './CodeView'

describe('CodeView', () => {
  it('syntax-highlights shell scripts', async () => {
    render(
      <CodeView
        filePath="/project/scripts/homelab-readiness-check.sh"
        content={[
          '#!/usr/bin/env bash',
          'set -u -o pipefail',
          'for arg in "$@"; do',
          '  case "$arg" in',
          '    --live) LIVE_CHECKS=true ;;',
          '  esac',
          'done',
        ].join('\n')}
      />,
    )

    const codeView = screen.getByTestId('code-view')
    await waitFor(() => {
      expect(codeView.querySelectorAll('.cm-content span[class]').length).toBeGreaterThan(0)
    })
  })
})
