import { getUnixFSFileInfoKind } from '@s4wave/sdk/unixfs/file-kind.js'
import { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import {
  getUnixFSParentPath,
  joinUnixFSDisplayPath,
  normalizeUnixFSLookupPath,
} from '@s4wave/sdk/unixfs/path.js'
import { getMimeType } from '@s4wave/web/hooks/useUnixFSHandle.js'

// UnixFSGalleryCandidate describes one discovered gallery item.
export interface UnixFSGalleryCandidate {
  path: string
  name: string
  label: string
  mimeType: string
}

// UnixFSGalleryDiscoveryState is the image index of a gallery scope: the
// images in path order, whether the server limit hid further images, and the
// directories that could not be read.
export interface UnixFSGalleryDiscoveryState {
  scopePath: string
  items: UnixFSGalleryCandidate[]
  errors: string[]
  truncated: boolean
}

// galleryExtensions are the file name extensions of the image types the
// gallery previews.
const galleryExtensions = [
  '.png',
  '.jpg',
  '.jpeg',
  '.gif',
  '.webp',
  '.svg',
  '.bmp',
  '.tif',
  '.tiff',
  '.avif',
]

// resolveGalleryScope opens the directory the gallery indexes: the requested
// path, or its parent when the path selects a file.
async function resolveGalleryScope(
  rootHandle: FSHandle,
  requestedScopePath: string,
  signal: AbortSignal,
): Promise<{ handle: FSHandle; scopePath: string }> {
  const requestedHandle = await openGalleryPath(
    rootHandle,
    requestedScopePath,
    signal,
  )
  const info = await requestedHandle.getFileInfo(signal)
  if (getUnixFSFileInfoKind(info) === 'directory') {
    return { handle: requestedHandle, scopePath: requestedScopePath }
  }
  requestedHandle[Symbol.dispose]()

  const scopePath = getUnixFSParentPath(requestedScopePath)
  const handle = await openGalleryPath(rootHandle, scopePath, signal)
  return { handle, scopePath }
}

// openGalleryPath opens the handle at path below rootHandle.
async function openGalleryPath(
  rootHandle: FSHandle,
  path: string,
  signal: AbortSignal,
): Promise<FSHandle> {
  const lookupPath = normalizeUnixFSLookupPath(path)
  if (!lookupPath) {
    return rootHandle.clone(signal)
  }
  return (await rootHandle.lookupPath(lookupPath, signal)).handle
}

// buildGalleryCandidate returns the gallery item for the image at relPath
// below scopePath.
function buildGalleryCandidate(
  scopePath: string,
  relPath: string,
): UnixFSGalleryCandidate {
  const name = relPath.slice(relPath.lastIndexOf('/') + 1)
  return {
    path: joinUnixFSDisplayPath(scopePath, relPath),
    name,
    label: relPath,
    mimeType: getMimeType(name),
  }
}

// compareGalleryCandidates orders candidates by path, the order the server
// takes matches in.
function compareGalleryCandidates(
  a: UnixFSGalleryCandidate,
  b: UnixFSGalleryCandidate,
): number {
  if (a.label === b.label) return 0
  return a.label < b.label ? -1 : 1
}

// streamUnixFSGalleryCandidates streams the image index of the scoped UnixFS
// subtree. The server indexes the subtree and sends the images each change
// adds and removes. Each yielded state is the complete current index.
export async function* streamUnixFSGalleryCandidates(
  rootHandle: FSHandle,
  requestedScopePath: string,
  signal: AbortSignal,
): AsyncIterable<UnixFSGalleryDiscoveryState> {
  const scope = await resolveGalleryScope(
    rootHandle,
    requestedScopePath || '/',
    signal,
  )
  using handle = scope.handle
  const items = new Map<string, UnixFSGalleryCandidate>()
  for await (const resp of handle.watchFiles(galleryExtensions, 0, signal)) {
    for (const relPath of resp.removed ?? []) {
      items.delete(relPath)
    }
    for (const relPath of resp.added ?? []) {
      items.set(relPath, buildGalleryCandidate(scope.scopePath, relPath))
    }
    yield {
      scopePath: scope.scopePath,
      items: [...items.values()].sort(compareGalleryCandidates),
      errors: resp.errors ?? [],
      truncated: resp.truncated ?? false,
    }
  }
}
