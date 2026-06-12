export const FILE_REF_PREFIX = 'file:'

/**
 * Resolves an artifact ref to an openable file vpath, or null when it isn't a
 * file. Handles three shapes:
 *  - file:<vpath>                explicit ref (what attach mints)
 *  - /project/.. /workspace/..   already a vpath
 *  - bare project-relative path  what agents store for files they edited
 *    (e.g. internal/services/file/app/service.go) -> /project/<rel>
 * Scheme refs (commit:, http:, ...) and prose (anything with whitespace) stay
 * non-file so they keep their plain rendering.
 */
export function fileArtifactVPath(ref: string): string | null {
  const trimmed = ref.trim()
  if (!trimmed) return null
  // Agents commonly store "path (note)" - the path then a human note. Resolve
  // off the first whitespace-delimited token; the rest is shown as context.
  const token = trimmed.split(/\s/)[0]
  if (token.startsWith(FILE_REF_PREFIX)) {
    const vpath = token.slice(FILE_REF_PREFIX.length).trim()
    return vpath || null
  }
  // Already a project/workspace vpath.
  if (token.startsWith('/project/') || token.startsWith('/workspace/')) return token
  // Any other scheme ref (commit:, http:, agen8:, ...) is not a file.
  if (/^[a-z][a-z0-9+.-]*:/i.test(token)) return null
  // A bare relative path: has a directory separator, or a real (alpha-led) file
  // extension so "v2.0" / "1.5" prose tokens don't get mistaken for files.
  const looksLikePath = token.includes('/') || /\.[A-Za-z][A-Za-z0-9]{0,7}$/.test(token)
  if (!looksLikePath) return null
  return '/project/' + token.replace(/^\.?\/+/, '')
}

/** The human note an agent appended after the path, e.g. "(reason ...)" - or ''. */
export function artifactNote(ref: string): string {
  const trimmed = ref.trim()
  const firstSpace = trimmed.search(/\s/)
  return firstSpace === -1 ? '' : trimmed.slice(firstSpace).trim()
}
