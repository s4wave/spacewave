// safeHref returns url if it is a relative URL or uses a known-safe scheme,
// otherwise '#'. Guards against javascript:/data:/vbscript: URL XSS when
// rendering author or post URLs that may originate from frontmatter or
// hydration JSON. Uses the URL constructor for protocol parsing so static
// analyzers (e.g. CodeQL) recognize the sanitization.
export function safeHref(url: string | undefined): string {
  if (!url) return '#'
  const trimmed = url.trim()
  if (!trimmed) return '#'
  // Relative URLs (no scheme) are safe.
  if (!/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) return trimmed
  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    return '#'
  }
  switch (parsed.protocol) {
    case 'http:':
    case 'https:':
    case 'mailto:':
    case 'tel:':
      return trimmed
    default:
      return '#'
  }
}
