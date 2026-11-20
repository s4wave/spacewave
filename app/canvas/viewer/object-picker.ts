import type { ObjectMetadata } from '@s4wave/sdk/world/world.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import {
  isInternalObject,
  listInternalTypes,
} from '@s4wave/web/space/object-tree.js'

// canvasPickerPageSize bounds the listing page the object picker searches.
const canvasPickerPageSize = 1000

// canvasPickerPrefixPageSize bounds the keys read under the typed query.
const canvasPickerPrefixPageSize = 100

// isCanvasInsertableObject returns whether a listed object may go on the
// canvas: any object but the canvas itself.
export function isCanvasInsertableObject(
  objectKey: string,
  canvasObjectKey: string,
): boolean {
  return !!objectKey && objectKey !== canvasObjectKey
}

// listCanvasPickerObjects lists the objects the canvas object picker searches:
// the first listing page and the keys that start with query, in key order,
// leaving out internal objects.
export async function listCanvasPickerObjects(
  world: IWorldState,
  query: string,
  signal?: AbortSignal,
): Promise<ObjectMetadata[]> {
  const [internalTypes, first, prefixed] = await Promise.all([
    listInternalTypes(world, signal),
    world.listObjects({ limit: canvasPickerPageSize }, signal),
    query
      ? world.listObjects(
          { prefix: query, limit: canvasPickerPrefixPageSize },
          signal,
        )
      : undefined,
  ])
  const byKey = new Map<string, ObjectMetadata>()
  for (const object of [
    ...(first.objects ?? []),
    ...(prefixed?.objects ?? []),
  ]) {
    if (isInternalObject(object.typeId, internalTypes)) continue
    byKey.set(object.objectKey ?? '', object)
  }
  return Array.from(byKey.values()).toSorted((a, b) =>
    (a.objectKey ?? '') < (b.objectKey ?? '') ? -1 : 1,
  )
}
