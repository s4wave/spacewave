import { Root } from '@s4wave/sdk/root'
import { formatObjectRef, parseObjectRef } from '@s4wave/sdk/world/object-ref'
import {
  Hash,
  HashType,
} from '@go/github.com/s4wave/spacewave/net/hash/hash.pb.js'

// testHashFunctions tests ObjectRef formatting/parsing and hash operations.
export async function testHashFunctions(
  rootResource: Root,
  abortSignal: AbortSignal,
) {
  // test ObjectRef formatting and parsing
  console.log('testing ObjectRef formatting/parsing...')
  const testHash: Hash = {
    hashType: 3, // BLAKE3
    hash: new Uint8Array([
      1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
      22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
    ]),
  }
  const testObjectRef = {
    rootRef: {
      hash: testHash,
    },
  }

  // format ObjectRef to base58 string
  const refStr = await formatObjectRef(rootResource, testObjectRef, abortSignal)
  console.log('formatted ObjectRef to base58:', refStr)
  if (!refStr) {
    throw new Error('formatObjectRef returned empty string')
  }

  // parse base58 string back to ObjectRef
  const parsedRef = await parseObjectRef(rootResource, refStr, abortSignal)
  console.log('parsed ObjectRef from base58')
  if (!parsedRef?.rootRef?.hash) {
    throw new Error('parseObjectRef failed to return valid ObjectRef')
  }

  // verify hash matches
  const originalHash = testHash.hash
  const parsedHash = parsedRef.rootRef.hash.hash
  if (
    !originalHash ||
    !parsedHash ||
    originalHash.length !== parsedHash.length
  ) {
    throw new Error('parsed hash length mismatch')
  }
  for (let i = 0; i < originalHash.length; i++) {
    if (originalHash[i] !== parsedHash[i]) {
      throw new Error(`parsed hash mismatch at byte ${i}`)
    }
  }
  console.log('ObjectRef round-trip test passed')

  // test HashSum
  console.log('testing HashSum...')
  const testData = new TextEncoder().encode('Hello, World!')
  const computedHash = await rootResource.hashSum(
    HashType.HashType_BLAKE3,
    testData,
    abortSignal,
  )
  const hashB58 = await rootResource.marshalHash(computedHash, abortSignal)
  console.log(`computed BLAKE3 hash: ${hashB58}`)
  if (!computedHash?.hash || computedHash.hash.length !== 32) {
    throw new Error('hashSum failed to return valid hash')
  }
  if (computedHash.hashType !== HashType.HashType_BLAKE3) {
    throw new Error('hashSum returned wrong hash type')
  }
  console.log('HashSum test passed')

  // test HashValidate with valid hash
  console.log('testing HashValidate with valid hash...')
  const validateResult = await rootResource.hashValidate(
    computedHash,
    abortSignal,
  )
  if (!validateResult.valid) {
    throw new Error(`hashValidate failed: ${validateResult.error}`)
  }
  console.log('HashValidate (valid) test passed')

  // test HashValidate with invalid hash
  console.log('testing HashValidate with invalid hash...')
  const invalidHash: Hash = {
    hashType: HashType.HashType_BLAKE3,
    hash: new Uint8Array([1, 2, 3]), // wrong length for BLAKE3
  }
  const invalidResult = await rootResource.hashValidate(
    invalidHash,
    abortSignal,
  )
  if (invalidResult.valid) {
    throw new Error('hashValidate should have failed for invalid hash')
  }
  if (!invalidResult.error) {
    throw new Error('hashValidate should return error message')
  }
  console.log('HashValidate (invalid) test passed')
}
