// object-ref.ts provides utilities for formatting and parsing ObjectRef to/from base58 strings.
//
// Example usage:
//   import { formatObjectRef, parseObjectRef } from '@s4wave/sdk/world/object-ref.js'
//
//   // Format ObjectRef to base58 string
//   const objectRef = { rootRef: { hash: someHash } }
//   const refStr = await formatObjectRef(root, objectRef, signal)
//
//   // Parse base58 string to ObjectRef
//   const parsed = await parseObjectRef(root, refStr, signal)
//
// Note: These functions only handle the rootRef.hash field. For full ObjectRef
// marshaling including bucketId, transformConf, etc., use the Go implementation.

import type { Root } from '../root/root.js'
import { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'

// formatObjectRef marshals an ObjectRef to a base58-encoded string.
// Returns an empty string if the ref is null or undefined.
// This mimics the Go implementation in hydra/bucket/object-ref.go MarshalB58().
export async function formatObjectRef(
  root: Root,
  ref: ObjectRef | null | undefined,
  signal?: AbortSignal,
): Promise<string> {
  if (!ref) {
    return ''
  }

  // Get the root ref hash
  const rootRef = ref.rootRef
  if (!rootRef?.hash) {
    return ''
  }

  // Marshal the hash to base58
  return await root.marshalHash(rootRef.hash, signal)
}

// parseObjectRef parses a base58-encoded string to an ObjectRef.
// Returns null if the string is empty.
// This mimics the Go implementation in hydra/bucket/object-ref.go ParseObjectRef().
export async function parseObjectRef(
  root: Root,
  refStr: string,
  signal?: AbortSignal,
): Promise<ObjectRef | null> {
  if (!refStr) {
    return null
  }

  // Parse the hash from base58
  const hash = await root.parseHash(refStr, signal)
  if (!hash) {
    return null
  }

  // Create an ObjectRef with the parsed hash
  return {
    rootRef: {
      hash,
    },
  }
}
