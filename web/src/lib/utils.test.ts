import { afterEach, describe, expect, it, vi } from 'vitest'
import { copyText } from './utils'

describe('copyText', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('writes the text to the clipboard', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await copyText('hello')
    expect(writeText).toHaveBeenCalledWith('hello')
  })

  it('throws when the clipboard API is unavailable', async () => {
    vi.stubGlobal('navigator', {})
    await expect(copyText('hello')).rejects.toThrow('Clipboard is unavailable')
  })
})
