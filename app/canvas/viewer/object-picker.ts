import { isHiddenSpaceObject } from '@s4wave/web/space/object-tree.js'

// isCanvasInsertableObject returns whether an object should appear in the
// canvas "Add Existing Object" picker.
export function isCanvasInsertableObject(
  objectKey: string,
  objectType: string,
  canvasObjectKey: string,
): boolean {
  if (!objectKey || objectKey === canvasObjectKey) return false
  return !isHiddenSpaceObject(objectKey, objectType)
}
