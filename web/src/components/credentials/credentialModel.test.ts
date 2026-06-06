import { describe, expect, it } from 'vitest'
import {
  buildSecrets,
  deriveInjection,
  previewInjection,
  emptyAuthDraft,
  type AuthDraft,
} from './credentialModel'

const base: AuthDraft = { injection: 'header', host: 'api.openai.com', fieldName: 'Authorization', value: 'sk-test' }

describe('buildSecrets', () => {
  it('builds a header credential with headerName and no paramName', () => {
    const r = buildSecrets({ ...base, injection: 'header', fieldName: 'Authorization' })
    expect(r.ok).toBe(true)
    expect(r.secrets).toEqual({
      value: 'sk-test',
      host: 'api.openai.com',
      injection: 'header',
      headerName: 'Authorization',
    })
    expect(r.secrets).not.toHaveProperty('paramName')
  })

  it('builds a query credential with paramName and no headerName', () => {
    const r = buildSecrets({ ...base, injection: 'query', fieldName: 'api_key' })
    expect(r.ok).toBe(true)
    expect(r.secrets).toEqual({
      value: 'sk-test',
      host: 'api.openai.com',
      injection: 'query',
      paramName: 'api_key',
    })
  })

  it('builds a bearer credential with neither header nor param name', () => {
    const r = buildSecrets({ ...base, injection: 'bearer', fieldName: '' })
    expect(r.ok).toBe(true)
    expect(r.secrets).toEqual({ value: 'sk-test', host: 'api.openai.com', injection: 'bearer' })
  })

  it('trims surrounding whitespace on every field', () => {
    const r = buildSecrets({ injection: 'header', host: '  api.x.com  ', fieldName: '  X-Key  ', value: '  abc  ' })
    expect(r.secrets).toEqual({ value: 'abc', host: 'api.x.com', injection: 'header', headerName: 'X-Key' })
  })

  // ── failure paths ───────────────────────────────────────────
  it('fails loudly when host is missing', () => {
    const r = buildSecrets({ ...base, host: '   ' })
    expect(r.ok).toBe(false)
    expect(r.secrets).toBeUndefined()
    expect(r.errors).toContain('Host is required.')
  })

  it('fails loudly when value is missing', () => {
    const r = buildSecrets({ ...base, value: '' })
    expect(r.ok).toBe(false)
    expect(r.errors).toContain('Value is required.')
  })

  it('requires a header name in header mode', () => {
    const r = buildSecrets({ ...base, injection: 'header', fieldName: '' })
    expect(r.ok).toBe(false)
    expect(r.errors).toContain('Header name is required.')
  })

  it('requires a param name in query mode', () => {
    const r = buildSecrets({ ...base, injection: 'query', fieldName: '' })
    expect(r.ok).toBe(false)
    expect(r.errors).toContain('Query param name is required.')
  })

  it('does NOT require a field name in bearer mode', () => {
    const r = buildSecrets({ ...base, injection: 'bearer', fieldName: '' })
    expect(r.ok).toBe(true)
  })

  it('reports every missing field at once', () => {
    const r = buildSecrets({ injection: 'query', host: '', fieldName: '', value: '' })
    expect(r.errors).toEqual(expect.arrayContaining(['Host is required.', 'Value is required.', 'Query param name is required.']))
  })
})

describe('deriveInjection', () => {
  it('reads header mode from a headerName field', () => {
    expect(deriveInjection([{ name: 'headerName', kind: 'public', configured: true }])).toBe('header')
  })

  it('reads query mode from a paramName field', () => {
    expect(deriveInjection([{ name: 'paramName', kind: 'public', configured: true }])).toBe('query')
  })

  it('defaults to bearer when there is no field-name marker', () => {
    expect(deriveInjection([{ name: 'value', kind: 'secret', configured: true }])).toBe('bearer')
  })

  it('treats undefined fields as bearer', () => {
    expect(deriveInjection(undefined)).toBe('bearer')
  })
})

describe('previewInjection', () => {
  it('shows a redacted bearer header', () => {
    const p = previewInjection({ ...base, injection: 'bearer' })
    expect(p.effect).toMatch(/^Authorization: Bearer/)
    expect(p.effect).toContain('redacted')
  })

  it('shows the custom header name', () => {
    const p = previewInjection({ ...base, injection: 'header', fieldName: 'X-Api-Key' })
    expect(p.effect).toMatch(/^X-Api-Key:/)
  })

  it('shows the query param name in the URL', () => {
    const p = previewInjection({ ...base, injection: 'query', fieldName: 'api_key' })
    expect(p.effect).toContain('api_key=')
  })

  it('uses placeholders when fields are empty', () => {
    const p = previewInjection(emptyAuthDraft())
    expect(p.action).toContain('‹host›')
  })
})
