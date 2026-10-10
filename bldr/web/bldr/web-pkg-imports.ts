/** bindWebPkgImports selects each shared package's URL before module loading. */
export function bindWebPkgImports(
  webPkgPaths: Record<string, string> | undefined,
  target: Document = document,
): void {
  // Browsers retain the first resolution for the document's complete lifetime,
  // so a prefix is mapped once. The document records its bindings because
  // separately bundled callers cannot share module state. Bare prefixes work
  // with both HTTPS and Electron's app: scheme.
  const bound = new Set<string>()
  for (const script of target.querySelectorAll('script[data-bldr-web-pkg]')) {
    for (const prefix of Object.keys(JSON.parse(script.textContent!).imports)) {
      bound.add(prefix)
    }
  }

  const imports: Record<string, string> = {}
  for (const [id, basePath] of Object.entries(webPkgPaths ?? {})) {
    const prefix = `bldr-web-pkg/${id}/`
    if (bound.has(prefix)) continue
    imports[prefix] = basePath || `/b/pkg/${id}/`
  }
  if (Object.keys(imports).length === 0) return

  const script = target.createElement('script')
  script.type = 'importmap'
  script.dataset.bldrWebPkg = ''
  script.textContent = JSON.stringify({ imports })
  target.head.append(script)
}
