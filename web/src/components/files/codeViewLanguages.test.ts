import { describe, expect, it } from 'vitest'
import { loadCodeViewLanguageExtensions } from './codeViewLanguages'

describe('loadCodeViewLanguageExtensions', () => {
  it('uses a shell parser for shell scripts', async () => {
    await expect(loadCodeViewLanguageExtensions('/project/scripts/homelab-readiness-check.sh')).resolves.toHaveLength(1)
    await expect(loadCodeViewLanguageExtensions('/project/scripts/deploy.bash')).resolves.toHaveLength(1)
    await expect(loadCodeViewLanguageExtensions('/project/scripts/dev.zsh')).resolves.toHaveLength(1)
  })

  it('leaves unknown extensions as plain text', async () => {
    await expect(loadCodeViewLanguageExtensions('/project/README.unknown')).resolves.toHaveLength(0)
  })
})
