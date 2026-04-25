// buildShellPopoutUrl builds the full URL for opening a shell path in a new tab.
export function buildShellPopoutUrl(
  path: string,
  location: Pick<Location, 'origin' | 'pathname'> = window.location,
): string {
  const hashlessPath = path.replace(/^#/, '')
  const normalizedPath =
    hashlessPath.startsWith('/') ? hashlessPath : `/${hashlessPath}`
  return `${location.origin}${location.pathname}#${normalizedPath}`
}

// openShellTabInNewTab opens a shell path in a new browser tab.
export function openShellTabInNewTab(path: string): void {
  const url = buildShellPopoutUrl(path)
  window.open(url, '_blank', 'noopener,noreferrer')
}
