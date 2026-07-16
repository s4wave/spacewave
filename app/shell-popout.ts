// buildShellPopoutUrl builds a structured hash URL retaining the stable
// Shell Tab ID for explicit handoff.
export function buildShellPopoutUrl(
  path: string,
  tabIdOrLocation?: string | Pick<Location, 'origin' | 'pathname'>,
  providedLocation?: Pick<Location, 'origin' | 'pathname'>,
): string {
  const location =
    typeof tabIdOrLocation === 'object'
      ? tabIdOrLocation
      : (providedLocation ?? window.location)
  const tabId =
    typeof tabIdOrLocation === 'string' ? tabIdOrLocation : undefined
  const hashlessPath = path.replace(/^#/, '')
  const queryIndex = hashlessPath.indexOf('?')
  const rawPath =
    queryIndex < 0 ? hashlessPath : hashlessPath.slice(0, queryIndex)
  const normalizedPath = rawPath.startsWith('/') ? rawPath : `/${rawPath}`
  const params = new URLSearchParams(
    queryIndex < 0 ? '' : hashlessPath.slice(queryIndex + 1),
  )
  if (tabId) params.set('shellTabId', tabId)
  const encodedParams = params.toString()
  return `${location.origin}${location.pathname}#${normalizedPath}${
    encodedParams ? `?${encodedParams}` : ''
  }`
}

// openShellTabInNewTab opens a shell path in a new browser tab.
export function openShellTabInNewTab(path: string, tabId?: string): void {
  const url = buildShellPopoutUrl(path, tabId)
  window.open(url, '_blank', 'noopener,noreferrer')
}
