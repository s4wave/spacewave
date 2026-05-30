export function splitUnixFSPath(path: string): string[] {
  const rooted = path.startsWith('/')
  const parts: string[] = []
  for (const segment of path.split('/')) {
    if (!segment || segment === '.') {
      continue
    }
    if (segment === '..') {
      if (parts.length > 0 && parts.at(-1) !== '..') {
        parts.pop()
        continue
      }
      if (!rooted) {
        parts.push(segment)
      }
      continue
    }
    parts.push(segment)
  }
  return parts
}

export function normalizeUnixFSLookupPath(path: string): string {
  if (!path || path === '/' || path === '.') {
    return ''
  }
  return splitUnixFSPath(path).join('/')
}

export function normalizeUnixFSDisplayPath(path: string): string {
  const normalized = normalizeUnixFSLookupPath(path)
  return normalized ? `/${normalized}` : '/'
}

export function joinUnixFSDisplayPath(
  basePath: string,
  ...segments: string[]
): string {
  return normalizeUnixFSDisplayPath([basePath, ...segments].join('/'))
}

export function getUnixFSParentPath(path: string): string {
  const parts = splitUnixFSPath(path)
  if (parts.length <= 1) {
    return '/'
  }
  return `/${parts.slice(0, -1).join('/')}`
}

export function getUnixFSBaseName(path: string): string {
  return splitUnixFSPath(path).at(-1) ?? ''
}

export function getUnixFSRelativePath(rootPath: string, path: string): string {
  const normalizedRoot = normalizeUnixFSLookupPath(rootPath)
  const normalizedPath = normalizeUnixFSLookupPath(path)
  if (!normalizedRoot) {
    return normalizedPath
  }
  if (!normalizedPath.startsWith(`${normalizedRoot}/`)) {
    return normalizedPath
  }
  return normalizedPath.slice(normalizedRoot.length + 1)
}

export function isSameOrChildUnixFSPath(
  parentPath: string,
  childPath: string,
): boolean {
  const parentParts = splitUnixFSPath(parentPath)
  const childParts = splitUnixFSPath(childPath)
  if (childParts.length < parentParts.length) {
    return false
  }
  return parentParts.every((part, index) => childParts[index] === part)
}
