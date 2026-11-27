import { Root } from '@s4wave/sdk/root/root.js'
import {
  CREATE_WASIVM_IMAGE_OP_ID,
  WasiVmImageTypeID,
} from '@s4wave/sdk/vm/wasivm.js'
import { CreateWasiVmImageOp, WasiVmImage } from '@s4wave/sdk/vm/wasivm.pb.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { mountCdnImageSpace } from '@s4wave/app/vm/cdn-image-space.js'

// WasiVmImageEntry is a WasiVmImage object and its metadata.
export interface WasiVmImageEntry {
  objectKey: string
  image: WasiVmImage
}

// readWasiVmImage reads the WasiVmImage at objectKey, or returns undefined
// when no image is stored there.
async function readWasiVmImage(
  world: IWorldState,
  objectKey: string,
  signal: AbortSignal,
): Promise<WasiVmImageEntry | undefined> {
  using obj = await world.getObject(objectKey, signal)
  if (!obj) return undefined
  using cursor = await obj.accessWorldState(undefined, signal)
  const resp = await cursor.unmarshal({ blockType: WasiVmImageTypeID }, signal)
  if (!resp.found || !resp.data?.length) return undefined
  return { objectKey, image: WasiVmImage.fromBinary(resp.data) }
}

// listCdnWasiVmImages reads every WasiVmImage published on the default CDN,
// newest first.
export async function listCdnWasiVmImages(
  root: Root,
  signal: AbortSignal,
): Promise<WasiVmImageEntry[]> {
  using catalog = await mountCdnImageSpace(root, '', signal)
  using world = await catalog.space.accessWorldState(false, signal)
  const keys = await listObjectsWithType(world, WasiVmImageTypeID, signal)
  const entries = await Promise.all(
    keys.map((key) => readWasiVmImage(world, key, signal)),
  )
  const created = (entry: WasiVmImageEntry) =>
    entry.image.createdAt?.getTime() ?? 0
  return entries
    .filter((entry): entry is WasiVmImageEntry => !!entry)
    .sort(
      (a, b) =>
        created(b) - created(a) ||
        (a.image.name ?? '').localeCompare(b.image.name ?? ''),
    )
}

// linkCdnWasiVmImage creates a Space image at the published image's key that
// reads its root filesystem from the CDN on first use, so nothing is copied.
// An image already at that key is kept.
export async function linkCdnWasiVmImage(
  world: IWorldState,
  root: Root,
  entry: WasiVmImageEntry,
  sender: string,
  signal: AbortSignal,
) {
  using existing = await world.getObject(entry.objectKey, signal)
  if (existing) return

  // Resolve the CDN Space the image is published in.
  using ownedRoot = root.resourceRef.createResource(
    root.resourceRef.resourceId,
    Root,
  )
  using cdn = (await ownedRoot.getCdn('', signal)).cdn
  const spaceId = await cdn.getCdnSpaceId(signal)
  const op = CreateWasiVmImageOp.create({
    objectKey: entry.objectKey,
    image: {
      ...entry.image,
      cdnSource: { spaceId, objectKey: entry.objectKey },
    },
    timestamp: new Date(),
  })
  await world.applyWorldOp(
    CREATE_WASIVM_IMAGE_OP_ID,
    CreateWasiVmImageOp.toBinary(op),
    sender,
    signal,
  )
}

// formatWasiVmImageLabel formats an image label for display, with its version.
export function formatWasiVmImageLabel(image: WasiVmImage): string {
  const name = image.name || 'VM image'
  return image.version ? `${name} (${image.version})` : name
}
