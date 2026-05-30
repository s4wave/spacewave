const projectedPathMaxSessionIndex = 0xffffffff

export interface ProjectedObjectPathInput {
  sessionIndex: number
  sharedObjectId: string
  objectKey: string
  path?: string
}

export interface ProjectedSpaceRootPathInput {
  sessionIndex: number
  sharedObjectId: string
}

export interface ParsedProjectedSpacePath {
  sessionIndex: number
  sharedObjectId: string
  path: string
  segments: string[]
  objectKey?: string
  objectPath?: string
}

function assertSessionIndex(sessionIndex: number): void {
  if (
    !Number.isInteger(sessionIndex) ||
    sessionIndex < 0 ||
    sessionIndex > projectedPathMaxSessionIndex
  ) {
    throw new Error('invalid projected path session index')
  }
}

function assertSharedObjectId(sharedObjectId: string): void {
  if (!sharedObjectId) {
    throw new Error('projected path requires a shared object id')
  }
}

function encodedPathSegments(path: string): string[] {
  return path
    .split('/')
    .flatMap((part) => (part ? [encodeURIComponent(part)] : []))
}

function decodedPathSegments(rawSegments: string[]): string[] {
  return rawSegments.map((segment) => decodeURIComponent(segment))
}

function parseSessionIndex(rawIndex: string): number {
  if (!/^[0-9]+$/.test(rawIndex)) {
    throw new Error('parse session index')
  }
  const sessionIndex = Number(rawIndex)
  assertSessionIndex(sessionIndex)
  return sessionIndex
}

function withObjectParts(
  parsed: Omit<ParsedProjectedSpacePath, 'objectKey' | 'objectPath'>,
): ParsedProjectedSpacePath {
  if (parsed.segments[4] !== '-') {
    return parsed
  }

  const objectSegments = parsed.segments.slice(5)
  const subpathMarkerIndex = objectSegments.lastIndexOf('-')
  if (subpathMarkerIndex < 0) {
    return {
      ...parsed,
      objectKey: objectSegments.join('/'),
      objectPath: '',
    }
  }

  return {
    ...parsed,
    objectKey: objectSegments.slice(0, subpathMarkerIndex).join('/'),
    objectPath: objectSegments.slice(subpathMarkerIndex + 1).join('/'),
  }
}

export function normalizeProjectedSubpath(path: string): string {
  if (!path || path === '/') {
    return ''
  }
  return path.split('/').filter(Boolean).join('/')
}

export function joinProjectedSubpath(parts: string[]): string {
  return parts.map(normalizeProjectedSubpath).filter(Boolean).join('/')
}

export function buildProjectedSpaceRootPath({
  sessionIndex,
  sharedObjectId,
}: ProjectedSpaceRootPathInput): string {
  assertSessionIndex(sessionIndex)
  assertSharedObjectId(sharedObjectId)
  return `u/${sessionIndex}/so/${encodeURIComponent(sharedObjectId)}`
}

export function buildProjectedObjectPath({
  sessionIndex,
  sharedObjectId,
  objectKey,
  path = '',
}: ProjectedObjectPathInput): string {
  const objectPath =
    `${buildProjectedSpaceRootPath({ sessionIndex, sharedObjectId })}/-/` +
    encodedPathSegments(objectKey).join('/')
  const normalizedPath = normalizeProjectedSubpath(path)
  if (!normalizedPath) {
    return objectPath
  }
  return `${objectPath}/-/${encodedPathSegments(normalizedPath).join('/')}`
}

export function parseProjectedSpacePath(
  projectedPath: string,
): ParsedProjectedSpacePath {
  const trimmedPath = projectedPath.replace(/^\/+|\/+$/g, '')
  if (!trimmedPath) {
    throw new Error('empty projected path')
  }

  const rawSegments = trimmedPath.split('/')
  if (
    rawSegments.length < 4 ||
    rawSegments[0] !== 'u' ||
    rawSegments[2] !== 'so'
  ) {
    throw new Error('invalid projected path format')
  }

  const segments = decodedPathSegments(rawSegments)
  const parsed = {
    sessionIndex: parseSessionIndex(rawSegments[1]),
    sharedObjectId: segments[3],
    path: segments.join('/'),
    segments,
  }
  assertSharedObjectId(parsed.sharedObjectId)
  return withObjectParts(parsed)
}
