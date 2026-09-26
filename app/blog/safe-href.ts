// safeHref returns url if it is a relative URL or uses a known-safe scheme,
// otherwise '#'. Guards against javascript:/data:/vbscript: URL XSS when
// rendering author or post URLs that may originate from frontmatter or
// hydration JSON. Uses the URL constructor for protocol parsing so static
// analyzers (e.g. CodeQL) recognize the sanitization.
export function safeHref(url: string | undefined): string {
  if (!url) return '#'
  const trimmed = normalizeUrlInput(url)
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

// normalizeUrlInput mirrors the WHATWG URL parser's input preprocessing so
// scheme detection sees what the browser sees: it strips leading and trailing
// C0 control or space characters and removes all ASCII tab and newline
// characters (e.g. "java\tscript:" is parsed as "javascript:"). String.trim
// is applied as well to drop other leading and trailing whitespace.
function normalizeUrlInput(url: string): string {
  let start = 0
  let end = url.length
  while (start < end && url.charCodeAt(start) <= 0x20) start++
  while (end > start && url.charCodeAt(end - 1) <= 0x20) end--
  let out = ''
  for (let i = start; i < end; i++) {
    const c = url.charCodeAt(i)
    if (c === 0x09 || c === 0x0a || c === 0x0d) continue
    out += url[i]
  }
  return out.trim()
}
