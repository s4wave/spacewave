// PathSeparator is the universally used path separator.
const PATH_SEPARATOR = '/'

// cleanPath normalizes a path (equivalent to Go path.Clean).
// Eliminates multiple slashes, . and .. elements.
function cleanPath(p: string): string {
  if (p === '') {
    return '.'
  }

  const rooted = p[0] === PATH_SEPARATOR
  const n = p.length

  const parts: string[] = []
  let start = 0
  if (rooted) {
    start = 1
  }

  let i = start
  while (i < n) {
    if (p[i] === PATH_SEPARATOR) {
      i++
      continue
    }
    if (p[i] === '.' && (i + 1 === n || p[i + 1] === PATH_SEPARATOR)) {
      i++
      continue
    }
    if (
      p[i] === '.' &&
      p[i + 1] === '.' &&
      (i + 2 === n || p[i + 2] === PATH_SEPARATOR)
    ) {
      if (parts.length > 0 && parts[parts.length - 1] !== '..') {
        parts.pop()
      } else if (!rooted) {
        parts.push('..')
      }
      i += 2
      continue
    }
    let j = i
    while (j < n && p[j] !== PATH_SEPARATOR) {
      j++
    }
    parts.push(p.substring(i, j))
    i = j
  }

  let result = parts.join(PATH_SEPARATOR)
  if (rooted) {
    result = PATH_SEPARATOR + result
  }
  if (result === '') {
    return '.'
  }
  return result
}

// isValidPath checks if a path is a valid fs.ValidPath (Go io/fs.ValidPath equivalent).
// A valid path is a non-empty unrooted slash-separated path without . or .. or empty elements.
function isValidPath(p: string): boolean {
  if (p === '' || p === '.') {
    return true
  }
  if (p[0] === PATH_SEPARATOR) {
    return false
  }
  const parts = p.split(PATH_SEPARATOR)
  for (const part of parts) {
    if (part === '' || part === '.' || part === '..') {
      return false
    }
  }
  return true
}

// splitPath splits a path string.
// Absolute paths are noted and stripped.
// Returns the parts and whether the path was absolute.
export function splitPath(tpath: string): {
  parts: string[]
  isAbsolute: boolean
} {
  tpath = cleanPath(tpath)
  let isAbsolute = false
  if (tpath.length >= 1 && tpath[0] === PATH_SEPARATOR) {
    isAbsolute = true
    tpath = tpath.substring(1)
  }
  if (tpath.length >= 2 && tpath[0] === '.' && tpath[1] === PATH_SEPARATOR) {
    tpath = tpath.substring(2)
  }
  if (tpath.length === 0 || (tpath.length === 1 && tpath[0] === '.')) {
    return { parts: [], isAbsolute }
  }
  return { parts: tpath.split(PATH_SEPARATOR), isAbsolute }
}

// joinPath joins a list of path components to a path.
export function joinPath(parts: string[], isAbsolute: boolean): string {
  let p = parts.join(PATH_SEPARATOR)
  if (isAbsolute) {
    p = PATH_SEPARATOR + p
  }
  let cleaned = cleanPath(p)
  if (!isAbsolute && cleaned[0] === '/') {
    if (cleaned.length === 1) {
      cleaned = '.'
    } else {
      cleaned = cleaned.substring(1)
    }
  }
  return cleaned
}

// joinPathPts joins multiple path parts slices (concats the slices together).
export function joinPathPts(...pts: string[][]): string[] {
  if (pts.length === 0) {
    return []
  }
  const out: string[] = []
  for (const pti of pts) {
    out.push(...pti)
  }
  return out
}

// cleanSplitValidateRelativePath cleans a path, splits it, and validates it.
// Coerces the path to be a relative path, not absolute.
// Throws on invalid paths.
export function cleanSplitValidateRelativePath(
  filePath: string,
): string[] {
  filePath = cleanPath(filePath)
  if (filePath === '/' || filePath === '.') {
    filePath = ''
  }
  if (filePath.length > 0 && filePath[0] === PATH_SEPARATOR) {
    filePath = filePath.substring(1)
  }
  if (filePath.length > 0 && !isValidPath(filePath)) {
    throw new Error('invalid path')
  }
  return splitPath(filePath).parts
}
