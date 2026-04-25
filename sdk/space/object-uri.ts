import { cleanPath } from '@aptre/bldr'

export const SUBPATH_DELIMITER = '/-/'
export const PATH_SEPARATOR = '/'

/**
 * Parses a URI path to extract object key and subpath components.
 * Uses /-/ as delimiter between object key and subpath.
 * A trailing /- is treated the same as /-/ with empty path.
 *
 * Examples:
 * - "some/object/key" -> {objectKey: "some/object/key", path: ""}
 * - "some/object/key/-/foo/bar" -> {objectKey: "some/object/key", path: "foo/bar"}
 * - "some/object/key/-" -> {objectKey: "some/object/key", path: ""}
 *
 * @param uri - The URI path to parse
 * @returns Object containing parsed components
 */
export function parseObjectUri(uri: string): {
  objectKey: string
  path: string
} {
  // Clean the path and force absolute
  uri = cleanPath('/' + uri)

  // Trim the / off the beginning
  uri = uri.substring(1)

  // A bare "-" is the subpath delimiter with no key and no path.
  if (uri === '-') {
    return { objectKey: '', path: '' }
  }

  // If URI starts with subpath delimiter, remove it
  if (uri.startsWith(SUBPATH_DELIMITER.substring(1))) {
    uri = uri.substring(SUBPATH_DELIMITER.length - 1)
  }

  // Find the first occurrence of the full subpath delimiter
  const delimiterIndex = uri.indexOf(SUBPATH_DELIMITER)
  if (delimiterIndex !== -1) {
    // We have an explicit delimiter; split into objectKey and subpath.
    const objectKey = uri.slice(0, delimiterIndex)
    let path = uri.slice(delimiterIndex + SUBPATH_DELIMITER.length)
    // If the subpath ends with a trailing marker (i.e. "-" or "/-"),
    // remove it so it does not affect the path.
    // Remove any trailing marker from the subpath if present.
    if (path === '-') {
      // exact "-" should be ignored
      path = ''
    } else if (path.endsWith('/-')) {
      // trailing "/-" should be removed (removes the last two characters)
      path = path.slice(0, -2)
    }
    return { objectKey, path }
  }

  // No explicit delimiter found.
  // For backward compatibility, if the URI ends with a trailing marker "/-",
  // remove it.
  if (uri.endsWith('/-')) {
    return {
      objectKey: uri.slice(0, -2),
      path: '',
    }
  }

  return { objectKey: uri, path: '' }
}

/**
 * Joins path parts into a single path string, filtering out empty segments
 * and removing trailing "-" if present.
 *
 * @param pathParts - Array of path segments to join
 * @param isAbsolute - If true, path will start with a separator
 * @returns The joined path string
 */
export function joinObjectUriPath(
  pathParts: string[],
  isAbsolute: boolean,
): string {
  const parts = pathParts
    .filter((part) => part !== '')
    .map((part) => part.replace(/^\/+|\/+$/g, ''))

  const lastIndex = parts.length - 1
  if (lastIndex >= 0 && parts[lastIndex] === '-') {
    parts.pop()
  }

  const path = parts.join(PATH_SEPARATOR)
  return path && isAbsolute ? PATH_SEPARATOR + path : path
}
