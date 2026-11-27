import { cleanPath } from '@aptre/bldr'

export const SUBPATH_DELIMITER = '/-/'
export const PATH_SEPARATOR = '/'

/**
 * parseObjectUri parses a URI path to extract object key and subpath
 * components.
 * Uses /-/ as the delimiter between the object key and subpath.
 * A trailing /- is treated the same as /-/ with an empty path.
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
  // Clean the path and force an absolute path
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
    // Split at the explicit delimiter into objectKey and subpath.
    const objectKey = uri.slice(0, delimiterIndex)
    let path = uri.slice(delimiterIndex + SUBPATH_DELIMITER.length)
    // Remove a trailing marker ("-" or "/-") so it does not affect the path.
    if (path === '-') {
      // exact "-" should be ignored
      path = ''
    } else if (path.endsWith('/-')) {
      // trailing "/-" should be removed (removes the last two characters)
      path = path.slice(0, -2)
    }
    return { objectKey, path }
  }

  // No explicit delimiter found. A trailing "/-" marks an empty subpath.
  if (uri.endsWith('/-')) {
    return {
      objectKey: uri.slice(0, -2),
      path: '',
    }
  }

  return { objectKey: uri, path: '' }
}

/**
 * joinObjectUriPath joins path parts into a single path string, filtering out
 * empty segments and removing a trailing "-" if present.
 *
 * @param pathParts - Array of path segments to join
 * @param isAbsolute - If true, path will start with a separator
 * @returns The joined path string
 */
export function joinObjectUriPath(
  pathParts: string[],
  isAbsolute: boolean,
): string {
  const parts = pathParts.flatMap((part) =>
    part !== '' ? [part.replace(/^\/+|\/+$/g, '')] : [],
  )

  const lastIndex = parts.length - 1
  if (lastIndex >= 0 && parts[lastIndex] === '-') {
    parts.pop()
  }

  const path = parts.join(PATH_SEPARATOR)
  return path && isAbsolute ? PATH_SEPARATOR + path : path
}

/**
 * escapeObjectKey escapes an object key for a spacewave:/// address, the
 * inverse of ParseSpacewaveURI in sdk/space/object-uri.go, which cleans the
 * path, splits it on /-/ and then percent-decodes each part. Each segment is
 * percent-encoded, and a whole ".", ".." or "-" segment is escaped so it reads
 * as neither a relative step nor a delimiter. A slash stays raw between two
 * nonempty segments and is "%2F" beside an empty one, so a leading, trailing
 * or doubled slash survives the cleaning.
 *
 * Examples:
 * - "My Repo" -> "My%20Repo"
 * - "a/-/b" -> "a/%2D/b"
 * - "/repos/x" -> "%2Frepos/x"
 *
 * @param objectKey - The object key to escape
 * @returns The escaped key
 */
export function escapeObjectKey(objectKey: string): string {
  const segments = objectKey.split(PATH_SEPARATOR)
  let out = escapeKeySegment(segments[0])
  for (let i = 1; i < segments.length; i++) {
    const sep = segments[i - 1] && segments[i] ? PATH_SEPARATOR : '%2F'
    out += sep + escapeKeySegment(segments[i])
  }
  return out
}

// escapeKeySegment percent-encodes one key segment, escaping a whole ".",
// ".." or "-" segment that encodeURIComponent would leave raw.
function escapeKeySegment(segment: string): string {
  if (segment === '.' || segment === '..') {
    return segment.replaceAll('.', '%2E')
  }
  if (segment === '-') {
    return '%2D'
  }
  return encodeURIComponent(segment)
}
