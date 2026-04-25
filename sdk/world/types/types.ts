import type { Quad as GraphQuad } from '@go/github.com/s4wave/spacewave/db/block/quad/quad.pb.js'
import type { IWorldState } from '../world-state.js'
import { keyToIRI } from '../graph-utils.js'
import { ErrTypeIDEmpty } from './errors.js'

// TypesPrefix is the prefix string for all types identifiers
export const TypesPrefix = 'types/'

// TypePred is the predicate linking an object to its type (in IRI format)
export const TypePred = '<type>'

// BuildTypeObjectKey returns the object key referring to the type
export function buildTypeObjectKey(typeID: string): string {
  if (!typeID) {
    return ''
  }
  return TypesPrefix + typeID
}

// BuildTypeQuad returns a type quad for a key and type
export function buildTypeQuad(objKey: string, typeID: string): GraphQuad {
  return {
    subject: keyToIRI(objKey),
    predicate: TypePred,
    obj: keyToIRI(buildTypeObjectKey(typeID)),
  }
}

// GetObjectType returns the type of a given object
// Returns empty string if the object has no type
export async function getObjectType(
  ws: IWorldState,
  key: string,
  abortSignal?: AbortSignal,
): Promise<string> {
  const result = await ws.lookupGraphQuads(
    keyToIRI(key),
    TypePred,
    undefined,
    undefined,
    1,
    abortSignal,
  )

  if (!result.quads || result.quads.length === 0) {
    return ''
  }

  const typeIRI = result.quads[0].obj
  if (!typeIRI) {
    return getObjectTypeFromTypeIndex(ws, key, abortSignal)
  }

  // Convert IRI to key and check prefix
  let typeKey: string
  try {
    typeKey = typeIRI.startsWith('<') ? typeIRI.slice(1, -1) : typeIRI
  } catch {
    return ''
  }

  if (!typeKey.startsWith(TypesPrefix)) {
    return ''
  }

  return typeKey.slice(TypesPrefix.length)
}

// getObjectTypeFromTypeIndex falls back to the type index when a graph quad
// response omits the object field.
async function getObjectTypeFromTypeIndex(
  ws: IWorldState,
  key: string,
  abortSignal?: AbortSignal,
): Promise<string> {
  const iter = await ws.iterateObjects(TypesPrefix, false, abortSignal)
  try {
    while (await iter.next(abortSignal)) {
      const typeKey = await iter.key(abortSignal)
      if (!typeKey.startsWith(TypesPrefix)) {
        continue
      }
      const typeID = typeKey.slice(TypesPrefix.length)
      const keys = await ws.listObjectsWithType(typeID, abortSignal)
      if (keys.includes(key)) {
        return typeID
      }
    }
    return ''
  } finally {
    await iter.close(abortSignal)
  }
}

// CheckObjectType asserts that the object key exists and has the given type
export async function checkObjectType(
  ws: IWorldState,
  key: string,
  typeID: string,
  abortSignal?: AbortSignal,
): Promise<void> {
  const objType = await getObjectType(ws, key, abortSignal)
  if (objType !== typeID) {
    if (!objType) {
      throw new Error(`object ${key}: expected object to exist w/ a valid type`)
    }
    throw new Error(
      `object ${key}: expected type ${typeID} but got "${objType}"`,
    )
  }
}

// SetObjectType sets the type of a given object by writing a graph quad
export async function setObjectType(
  ws: IWorldState,
  key: string,
  typeID: string,
  abortSignal?: AbortSignal,
): Promise<void> {
  if (!key || !typeID) {
    throw new Error('empty object key or type ID')
  }

  const nextQuad = buildTypeQuad(key, typeID)

  // Look up existing type quads for this object
  const existing = await ws.lookupGraphQuads(
    keyToIRI(key),
    TypePred,
    undefined,
    undefined,
    undefined,
    abortSignal,
  )

  // Delete any existing type quads that don't match
  let exists = false
  if (existing.quads) {
    for (const q of existing.quads) {
      if (q.obj === nextQuad.obj) {
        exists = true
      } else {
        await ws.deleteGraphQuad(
          q.subject ?? '',
          q.predicate ?? '',
          q.obj ?? '',
          q.label,
          abortSignal,
        )
      }
    }
  }

  // Ensure the type object exists BEFORE setting the quad
  // (SetGraphQuad validates that both subject and object exist)
  await ensureTypeExists(ws, typeID, abortSignal)

  // Add the new type quad if it doesn't exist
  if (!exists) {
    await ws.setGraphQuad(
      nextQuad.subject ?? '',
      nextQuad.predicate ?? '',
      nextQuad.obj ?? '',
      nextQuad.label,
      abortSignal,
    )
  }
}

// EnsureTypeExists creates the object representing the type ID if it doesn't exist
// Returns true if the type was created, false if it already existed
export async function ensureTypeExists(
  ws: IWorldState,
  typeID: string,
  abortSignal?: AbortSignal,
): Promise<boolean> {
  const objKey = buildTypeObjectKey(typeID)
  const obj = await ws.getObject(objKey, abortSignal)
  if (obj) {
    return false
  }
  await ws.createObject(objKey, {}, abortSignal)
  return true
}

// IterateObjectsWithType iterates over object keys with the given type ID
// The callback receives each object key and should return true to continue iteration
export async function iterateObjectsWithType(
  ws: IWorldState,
  typeID: string,
  cb: (objKey: string) => Promise<boolean> | boolean,
  abortSignal?: AbortSignal,
): Promise<void> {
  if (!typeID) {
    throw new ErrTypeIDEmpty()
  }
  if (!cb) {
    return
  }

  const objKeys = await ws.listObjectsWithType(typeID, abortSignal)
  for (const objKey of objKeys) {
    const ctnu = await cb(objKey)
    if (!ctnu) {
      break
    }
  }
}

// ListObjectsWithType returns the list of object keys with the given type id.
export async function listObjectsWithType(
  ws: IWorldState,
  typeID: string,
  abortSignal?: AbortSignal,
): Promise<string[]> {
  const objKeys: string[] = []
  await iterateObjectsWithType(
    ws,
    typeID,
    (objKey) => {
      objKeys.push(objKey)
      return true
    },
    abortSignal,
  )
  return objKeys
}
