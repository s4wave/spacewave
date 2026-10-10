/** bindWebPkgImports selects each shared package's URL before module loading. */
export function bindWebPkgImports(
  webPkgPaths: Record<string, string> | undefined,
  target: Document = document,
): void {
  // Bare prefixes work with both HTTPS and Electron's app: scheme.
  const imports: Record<string, string> = {}
  for (const [id, basePath] of Object.entries(webPkgPaths ?? {})) {
    imports[`bldr-web-pkg/${id}/`] = basePath || `/b/pkg/${id}/`
  }
  if (Object.keys(imports).length === 0) return

  // Browsers retain the first resolution for the document's complete lifetime.
  const script = target.createElement('script')
  script.type = 'importmap'
  script.textContent = JSON.stringify({ imports })
  target.head.append(script)
}
