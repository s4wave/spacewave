// SAFE_HREF_SCHEMES is the allowlist of URL schemes safe to use in href.
const SAFE_HREF_SCHEMES = ['http:', 'https:', 'mailto:', 'tel:']

// safeHref returns url if it is a relative URL or uses a known-safe scheme,
// otherwise '#'. Guards against javascript:/data:/vbscript: URL XSS when
// rendering author or post URLs that may originate from frontmatter or
// hydration JSON.
export function safeHref(url: string | undefined): string {
  if (!url) return '#'
  const trimmed = url.trim()
  if (!trimmed) return '#'
  // Relative URLs (no scheme) are safe.
  if (!/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) return trimmed
  const lower = trimmed.toLowerCase()
  for (const scheme of SAFE_HREF_SCHEMES) {
    if (lower.startsWith(scheme)) return trimmed
  }
  return '#'
}
