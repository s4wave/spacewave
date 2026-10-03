import { sha256 } from '@noble/hashes/sha2.js'
import { concatBytes } from '@noble/hashes/utils.js'

import { extractPublicKeyFromPeerID } from '../../net/peer/id.js'
import { verifySignature } from '../../net/peer/signature.js'
import { validateSOPosition } from './config-chain.js'
import { SOBJECT_BASE_CRYPTO_CONTEXT } from './operation-log.js'
import {
  SOSequence,
  SOSequenceInner,
  type SOOperationPosition,
  type SOSequenceHead,
} from './sobject.pb.js'

// SO_SEQUENCE_SIGNATURE_CONTEXT is the signature context of every sequence
// position. The signed body binds the object, so the context needs no
// per-position data.
export const SO_SEQUENCE_SIGNATURE_CONTEXT = `${SOBJECT_BASE_CRYPTO_CONTEXT}sequence_signature`

// soSequenceHashDomain separates sequence position hashes from other SHA-256
// digests.
const soSequenceHashDomain = new TextEncoder().encode(
  'spacewave/sharedobject/sequence/v1',
)

// hashSOSequenceInner returns the identity of a sequence position from its
// signed body: SHA-256 over the domain, the big-endian body length, and the
// body.
export function hashSOSequenceInner(inner: Uint8Array): Uint8Array {
  const length = new Uint8Array(8)
  new DataView(length.buffer).setBigUint64(0, BigInt(inner.length))
  return sha256(concatBytes(soSequenceHashDomain, length, inner))
}

// validateSOSequenceInner checks the position body's structure, matching Go
// SOSequenceInner.Validate. It throws on the first violation.
export function validateSOSequenceInner(inner: SOSequenceInner): void {
  // The body names an object, its sequencer and a height.
  if (!inner.sharedObjectId) {
    throw new Error('sequence shared_object_id is empty')
  }
  if (!extractPublicKeyFromPeerID(inner.peerId ?? '')) {
    throw new Error('sequence peer_id is invalid')
  }
  const height = inner.height ?? 0n
  if (height === 0n) {
    throw new Error('sequence height must be positive')
  }

  // The first position names nothing before it; every later one names its
  // predecessor.
  const prev = inner.prevHash?.length ?? 0
  if (height === 1n && prev !== 0) {
    throw new Error('first sequence position must not name a previous position')
  }
  if (height > 1n && prev !== 32) {
    throw new Error('sequence prev_hash must be a 32-byte hash')
  }

  // The position places one real operation.
  validateSOPosition(inner.op)
}

// verifySOSequence authenticates the position as signed for sharedObjectID by
// the sequencer it names, and returns its body. It does not check that the
// signer is the appointed sequencer. It throws when the position is invalid.
export async function verifySOSequence(
  sharedObjectID: string,
  record: SOSequence,
): Promise<SOSequenceInner> {
  // Decode the well-formed body bound to this object.
  const data = record.inner ?? new Uint8Array()
  if (data.length === 0) {
    throw new Error('sequence inner is empty')
  }
  const inner = SOSequenceInner.fromBinary(data)
  validateSOSequenceInner(inner)
  if (inner.sharedObjectId !== sharedObjectID) {
    throw new Error('sequence position is bound to another shared object')
  }

  // The signature covers exactly the encoded body, by the named sequencer.
  const signer = await verifySignature(
    SO_SEQUENCE_SIGNATURE_CONTEXT,
    record.signature,
    data,
  )
  if (!signer) {
    throw new Error('invalid sequence signature')
  }
  if (signer !== inner.peerId) {
    throw new Error('sequence signer does not match inner peer ID')
  }
  return inner
}

// encodeSOSequenceInner encodes the body of the position after prev that
// places op, signed by peerId. prev is undefined or height 0 before the first
// position.
export function encodeSOSequenceInner(
  sharedObjectID: string,
  peerId: string,
  prev: SOSequenceHead | undefined,
  op: SOOperationPosition,
): Uint8Array {
  const inner: SOSequenceInner = {
    sharedObjectId: sharedObjectID,
    height: (prev?.height ?? 0n) + 1n,
    prevHash: prev?.hash,
    op,
    peerId,
  }
  validateSOSequenceInner(inner)
  return SOSequenceInner.toBinary(inner)
}
