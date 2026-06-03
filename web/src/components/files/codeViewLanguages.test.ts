import { describe, expect, it } from 'vitest'
import { codeViewLanguageExtensions } from './codeViewLanguages'

describe('codeViewLanguageExtensions', () => {
  it('uses a shell parser for shell scripts', () => {
    expect(codeViewLanguageExtensions('/project/scripts/homelab-readiness-check.sh')).toHaveLength(1)
    expect(codeViewLanguageExtensions('/project/scripts/deploy.bash')).toHaveLength(1)
    expect(codeViewLanguageExtensions('/project/scripts/dev.zsh')).toHaveLength(1)
  })

  it('leaves unknown extensions as plain text', () => {
    expect(codeViewLanguageExtensions('/project/README.unknown')).toHaveLength(0)
  })
})
