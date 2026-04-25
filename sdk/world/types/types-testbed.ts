import type { BackendAPI } from '@aptre/bldr-sdk'
import type { TestbedRoot } from '../../../sdk/testbed/testbed.js'
import {
  buildTypeObjectKey,
  checkObjectType,
  ensureTypeExists,
  getObjectType,
  listObjectsWithType,
  setObjectType,
} from './types.js'
import { ErrTypeIDEmpty } from './errors.js'

export default async function main(
  _backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  testbedRoot: TestbedRoot,
) {
  console.log('starting world types test...')

  // Create a world engine
  console.log('creating world engine...')
  using engine = await testbedRoot.createWorld('test-types-engine')
  console.log('created world engine')

  // Create a write transaction
  using tx = await engine.newTransaction(true, abortSignal)

  // Test 1: Create an object and set its type
  console.log('test 1: creating object and setting type...')
  const testObjKey = 'test-object-1'
  const testTypeID = 'test-type'

  await tx.createObject(testObjKey, {})
  console.log(`created object: ${testObjKey}`)

  // Set the object type
  await setObjectType(tx, testObjKey, testTypeID, abortSignal)
  console.log(`set object type: ${testTypeID}`)

  // Get the object type back
  const retrievedType = await getObjectType(tx, testObjKey, abortSignal)
  if (retrievedType !== testTypeID) {
    throw new Error(`expected type ${testTypeID} but got ${retrievedType}`)
  }
  console.log(`verified object type: ${retrievedType}`)

  // Test 2: Check object type
  console.log('test 2: checking object type...')
  await checkObjectType(tx, testObjKey, testTypeID, abortSignal)
  console.log('checkObjectType passed')

  // Try checking with wrong type (should throw)
  let wrongTypeThrew = false
  try {
    await checkObjectType(tx, testObjKey, 'wrong-type', abortSignal)
  } catch {
    wrongTypeThrew = true
    console.log('checkObjectType correctly threw for wrong type')
  }
  if (!wrongTypeThrew) {
    throw new Error('expected checkObjectType to throw for wrong type')
  }

  // Test 3: Ensure type object exists
  console.log('test 3: ensuring type object exists...')
  const typeObjKey = buildTypeObjectKey(testTypeID)
  const typeObj = await tx.getObject(typeObjKey)
  if (!typeObj) {
    throw new Error('type object should exist')
  }
  console.log(`verified type object exists: ${typeObjKey}`)

  // Test 4: Create multiple objects with the same type
  console.log('test 4: creating multiple objects with same type...')
  const testObjKey2 = 'test-object-2'
  const testObjKey3 = 'test-object-3'

  await tx.createObject(testObjKey2, {})
  await setObjectType(tx, testObjKey2, testTypeID, abortSignal)
  console.log(`created and typed object: ${testObjKey2}`)

  await tx.createObject(testObjKey3, {})
  await setObjectType(tx, testObjKey3, testTypeID, abortSignal)
  console.log(`created and typed object: ${testObjKey3}`)

  // Test 5: List objects with type
  console.log('test 5: listing objects with type...')
  const objectsWithType = await listObjectsWithType(tx, testTypeID, abortSignal)
  console.log(`found ${objectsWithType.length} objects with type ${testTypeID}`)

  if (objectsWithType.length !== 3) {
    throw new Error(
      `expected 3 objects with type ${testTypeID} but got ${objectsWithType.length}`,
    )
  }

  const expectedKeys = [testObjKey, testObjKey2, testObjKey3].sort()
  const actualKeys = objectsWithType.sort()
  for (let i = 0; i < expectedKeys.length; i++) {
    if (expectedKeys[i] !== actualKeys[i]) {
      throw new Error(
        `expected key ${expectedKeys[i]} but got ${actualKeys[i]}`,
      )
    }
  }
  console.log('verified all objects are listed correctly')

  // Test 6: Get type of object with no type
  console.log('test 7: getting type of object with no type...')
  const untypedObjKey = 'untyped-object'
  await tx.createObject(untypedObjKey, {})
  const untypedObjType = await getObjectType(tx, untypedObjKey, abortSignal)
  if (untypedObjType !== '') {
    throw new Error(`expected empty type but got ${untypedObjType}`)
  }
  console.log('verified untyped object returns empty string')

  // Test 7: Error handling - empty type ID
  console.log('test 7: testing error handling for empty type ID...')
  let emptyTypeThrew = false
  try {
    await listObjectsWithType(tx, '', abortSignal)
  } catch (err) {
    if (err instanceof ErrTypeIDEmpty) {
      emptyTypeThrew = true
      console.log('correctly threw ErrTypeIDEmpty')
    }
  }
  if (!emptyTypeThrew) {
    throw new Error('expected ErrTypeIDEmpty to be thrown')
  }

  // Test 8: Ensure type exists (idempotent)
  console.log('test 8: testing ensureTypeExists idempotency...')
  const newTypeID = 'another-type'
  const created1 = await ensureTypeExists(tx, newTypeID, abortSignal)
  if (!created1) {
    throw new Error('expected ensureTypeExists to return true for new type')
  }
  console.log('created new type')

  const created2 = await ensureTypeExists(tx, newTypeID, abortSignal)
  if (created2) {
    throw new Error(
      'expected ensureTypeExists to return false for existing type',
    )
  }
  console.log('verified ensureTypeExists is idempotent')

  // Test 9: Change object type
  console.log('test 9: changing object type...')
  const changeableObjKey = 'changeable-object'
  const type1 = 'type-one'
  const type2 = 'type-two'

  await tx.createObject(changeableObjKey, {})
  await setObjectType(tx, changeableObjKey, type1, abortSignal)
  console.log(`set initial type: ${type1}`)

  const initialType = await getObjectType(tx, changeableObjKey, abortSignal)
  if (initialType !== type1) {
    throw new Error(`expected type ${type1} but got ${initialType}`)
  }

  // Change the type
  await setObjectType(tx, changeableObjKey, type2, abortSignal)
  console.log(`changed type to: ${type2}`)

  const changedType = await getObjectType(tx, changeableObjKey, abortSignal)
  if (changedType !== type2) {
    throw new Error(`expected type ${type2} but got ${changedType}`)
  }
  console.log('verified type change worked correctly')

  // Verify it's no longer in the old type's list
  const type1Objects = await listObjectsWithType(tx, type1, abortSignal)
  if (type1Objects.includes(changeableObjKey)) {
    throw new Error('object should not be in old type list')
  }

  // Verify it's in the new type's list
  const type2Objects = await listObjectsWithType(tx, type2, abortSignal)
  if (!type2Objects.includes(changeableObjKey)) {
    throw new Error('object should be in new type list')
  }
  console.log('verified object moved to new type list')

  // Commit the transaction
  console.log('committing transaction...')
  await tx.commit(abortSignal)
  console.log('transaction committed')

  // Done
  console.log('all world types tests passed successfully!')
}
