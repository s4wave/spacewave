import type { Root } from '@s4wave/sdk/root/root.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import { V86Image } from '@s4wave/sdk/vm/v86.pb.js'
import { V86ImageTypeID } from '@s4wave/sdk/vm/v86image.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { EntityDecoder, KeyedSet } from '@s4wave/sdk/world/entity-set.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { mountCdnImageSpace } from './cdn-image-space.js'
import {
  compareV86ImageNewestFirst,
  isDefaultV86Image,
} from './v86-wizard-config.js'

// V86ImageEntry is a V86Image object and its decoded metadata. metadataError
// is set when the metadata could not be decoded; the image then carries a
// placeholder.
export interface V86ImageEntry {
  objectKey: string
  image: V86Image
  metadataError?: string
}

// readV86Image reads the V86Image at objectKey, or returns undefined when no
// image metadata is stored there.
export async function readV86Image(
  world: IWorldState,
  objectKey: string,
  signal?: AbortSignal,
): Promise<V86ImageEntry | undefined> {
  // Unmarshal the image metadata at the object root.
  using obj = await world.getObject(objectKey, signal)
  if (!obj) return undefined
  using cursor = await obj.accessWorldState(undefined, signal)
  const resp = await cursor.unmarshal({ blockType: V86ImageTypeID }, signal)
  if (!resp.found || !resp.data?.length) return undefined
  return v86ImageEntry(objectKey, decodeV86Metadata(resp.data))
}

// V86Metadata is decoded image metadata, or why it could not be decoded.
export type V86Metadata = { image: V86Image } | { metadataError: string }

// decodeV86Metadata decodes the metadata stored at a V86Image object root.
export function decodeV86Metadata(data: Uint8Array): V86Metadata {
  try {
    return { image: V86Image.fromBinary(data) }
  } catch (err) {
    return {
      metadataError:
        err instanceof Error ? err.message : 'metadata decode failed',
    }
  }
}

// v86ImageDecoder decodes V86Image bodies in a followed set of images.
export const v86ImageDecoder: EntityDecoder<V86Metadata> = {
  name: 'v86-image',
  decode: decodeV86Metadata,
}

// v86ImageEntry is the entry of metadata read at objectKey; metadata that
// could not be decoded gets a placeholder image.
function v86ImageEntry(objectKey: string, meta: V86Metadata): V86ImageEntry {
  if ('image' in meta) return { objectKey, image: meta.image }
  return {
    objectKey,
    image: {
      name: objectKey,
      platform: 'v86',
      description: 'Metadata could not be decoded.',
      tags: [],
    },
    metadataError: meta.metadataError,
  }
}

// v86ImageEntries lists the images of a followed set, newest first.
export function v86ImageEntries(
  set: KeyedSet<V86Metadata> | null,
): V86ImageEntry[] {
  if (!set) return []
  return set.keys
    .flatMap((key) => {
      const meta = set.rows.get(key)
      return meta ? [v86ImageEntry(key, meta)] : []
    })
    .sort(compareV86ImageNewestFirst)
}

// listV86Images reads every V86Image in world, newest first.
export async function listV86Images(
  world: IWorldState,
  signal: AbortSignal,
): Promise<V86ImageEntry[]> {
  const keys = await listObjectsWithType(world, V86ImageTypeID, signal)
  const entries = await Promise.all(
    keys.map((key) => readV86Image(world, key, signal)),
  )
  return entries
    .filter((entry): entry is V86ImageEntry => !!entry)
    .sort(compareV86ImageNewestFirst)
}

// listCdnV86Images reads every V86Image published in a CDN catalog Space.
export async function listCdnV86Images(
  space: Space,
  signal: AbortSignal,
): Promise<V86ImageEntry[]> {
  using world = await space.accessWorldState(false, signal)
  return await listV86Images(world, signal)
}

// loadCdnV86Image reads the catalog image at objectKey from the CDN cdnId, or
// the catalog's default image when objectKey is empty.
export async function loadCdnV86Image(
  root: Root,
  cdnId: string,
  objectKey: string,
  signal: AbortSignal,
): Promise<V86ImageEntry | undefined> {
  using catalog = await mountCdnImageSpace(root, cdnId, signal)
  if (!objectKey) {
    const entries = await listCdnV86Images(catalog.space, signal)
    return entries.find((entry) => isDefaultV86Image(entry.image))
  }
  using world = await catalog.space.accessWorldState(false, signal)
  return await readV86Image(world, objectKey, signal)
}

// formatV86ImageLabel names an image for display, with its version.
export function formatV86ImageLabel(image: V86Image): string {
  const name = image.name || image.distro || 'VM image'
  return image.version ? `${name} (${image.version})` : name
}

// V86CatalogErrorCopy is the user-facing description of a catalog failure.
export interface V86CatalogErrorCopy {
  title: string
  detail: string
  // unpublished is set when the catalog is empty rather than unreachable, so
  // retrying cannot help.
  unpublished: boolean
}

// getV86CatalogErrorCopy describes a catalog load failure without exposing
// backend detail.
export function getV86CatalogErrorCopy(error: unknown): V86CatalogErrorCopy {
  const message =
    error instanceof Error
      ? error.message
      : typeof error === 'string'
        ? error
        : ''
  const unpublished =
    /published head|not published|no published|empty catalog/i.test(message)
  return unpublished
    ? {
        title: 'No VM images are published yet',
        detail: 'This image catalog has no published images yet.',
        unpublished: true,
      }
    : {
        title: 'Image catalog unavailable',
        detail: 'The VM image catalog could not be loaded. Try again.',
        unpublished: false,
      }
}
