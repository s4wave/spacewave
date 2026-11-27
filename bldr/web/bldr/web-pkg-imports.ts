/** bindWebPkgImports selects each shared package's URL before module loading. */
export function bindWebPkgImports(
  webPkgPaths: Record<string, string> | undefined,
  target: Document = document,
): void {
  // Browsers retain the first resolution for the document's complete lifetime,
  // and Firefox rejects every import map after the document's first import map,
  // so a package whose URL the document's maps already resolve adds no map. The
  // maps in the document are the record, because separately bundled callers
  // cannot share module state. Bare prefixes work with both HTTPS and
  // Electron's app: scheme.
  const bound = new Map<string, string>()
  for (const script of target.querySelectorAll('script[type="importmap"]')) {
    const imports: Record<string, string> =
      JSON.parse(script.textContent!).imports ?? {}
    for (const [prefix, url] of Object.entries(imports)) {
      if (!bound.has(prefix)) bound.set(prefix, url)
    }
  }

  const imports: Record<string, string> = {}
  for (const [id, basePath] of Object.entries(webPkgPaths ?? {})) {
    const prefix = `bldr-web-pkg/${id}/`
    if (bound.has(prefix)) continue
    const url = basePath || `/b/pkg/${id}/`
    if (resolvePrefix(bound, prefix) === url) continue
    imports[prefix] = url
  }
  if (Object.keys(imports).length === 0) return

  const script = target.createElement('script')
  script.type = 'importmap'
  script.textContent = JSON.stringify({ imports })
  target.head.append(script)
}

// resolvePrefix resolves a specifier prefix through the longest bound prefix
// that contains it, as the browser does, or returns undefined.
function resolvePrefix(
  bound: Map<string, string>,
  specifier: string,
): string | undefined {
  let match = ''
  for (const prefix of bound.keys()) {
    if (
      prefix.endsWith('/') &&
      specifier.startsWith(prefix) &&
      prefix.length > match.length
    ) {
      match = prefix
    }
  }
  if (!match) return undefined
  return bound.get(match) + specifier.slice(match.length)
}
