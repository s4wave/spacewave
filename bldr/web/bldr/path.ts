// pathSeparator is the path separator.
export const pathSeparator = '/' as const

// splitPath splits a path string.
// absolute paths are ignored (converted to relative paths starting at ./).
// returns if the path was absolute or relative.
export function splitPath(tpath: string): {
  pathParts: string[]
  isAbsolute: boolean
} {
  tpath = cleanPath(tpath)
  const isAbsolute = tpath.startsWith(pathSeparator)
  if (isAbsolute) {
    tpath = tpath.substring(1)
  } else if (tpath.startsWith(`.${pathSeparator}`)) {
    tpath = tpath.substring(2)
  }
  if (tpath.length == 0) {
    return { pathParts: [], isAbsolute }
  }

  return { pathParts: tpath.split(pathSeparator), isAbsolute }
}

// joinPath joins a list of path parts.
export function joinPath(pathParts: string[], isAbsolute: boolean) {
  let out = pathParts.join(pathSeparator)
  if (isAbsolute) {
    out = pathSeparator + out
  }
  return cleanPath(out)
}

// navigateUpPath returns a path for navigating up depth dirs.
// For example:
// - 0: .
// - 1: ..
// - 2: ../..
// - 3: ../../..
// ...and so on.
export function navigateUpPath(depth: number) {
  return depth < 1 ? '.' : Array(depth).fill('..').join('/')
}

// cleanPath returns the shortest path name equivalent to path
// by purely lexical processing.
//
// Based on the Go path.Clean function (BSD-3 license):
// https://github.com/golang/go/blob/go1.21.5/src/path/path.go#L72
//
// It applies the following rules iteratively until no further processing can be done:
//
//  1. Replace multiple slashes with a single slash.
//  2. Eliminate each . path name element (the current directory).
//  3. Eliminate each inner .. path name element (the parent directory)
//     along with the non-.. element that precedes it.
//  4. Eliminate .. elements that begin a rooted path:
//     that is, replace "/.." by "/" at the beginning of a path.
//
// The returned path ends in a slash only if it is the root "/".
//
// If the result of this process is an empty string, Clean
// returns the string ".".
//
// See also Rob Pike, “Lexical File Names in Plan 9 or
// Getting Dot-Dot Right,”
// https://9p.io/sys/doc/lexnames.html
export function cleanPath(path: string): string {
  // Rule 1: Replace multiple slashes with a single slash
  path = path.replace(/\/+/g, pathSeparator)

  // Split the path into segments
  const segments = path.split(pathSeparator)
  const stack: string[] = []

  for (const segment of segments) {
    if (segment === '..') {
      if (stack.length > 0 && stack[stack.length - 1] !== '..') {
        stack.pop()
      } else if (!path.startsWith(pathSeparator)) {
        // Keep '..' for non-rooted paths when it cannot be resolved further
        stack.push(segment)
      }
    } else if (segment !== '.' && segment !== '') {
      stack.push(segment)
    }
  }

  // Reconstruct the path
  let result = stack.join(pathSeparator)
  if (path.startsWith(pathSeparator)) {
    result = pathSeparator + result
  }

  // Special handling for non-rooted paths that start with '..'
  if (path.startsWith('..') && result === '') {
    return '..'
  }

  // If the result is empty and the path was rooted, return "/"
  if (result.length === 0) {
    return path.startsWith(pathSeparator) ? pathSeparator : '.'
  }

  return result
}
