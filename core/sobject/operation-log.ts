import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex, concatBytes } from '@noble/hashes/utils.js'

import { extractPublicKeyFromPeerID } from '../../net/peer/id.js'
import { verifySignature } from '../../net/peer/signature.js'
import {
  SOOperation,
  SOOperationInner,
  SOParticipantRole,
  type SOParticipantConfig,
} from './sobject.pb.js'

// SOBJECT_BASE_CRYPTO_CONTEXT matches the Go baseCryptoContext.
export const SOBJECT_BASE_CRYPTO_CONTEXT =
  'sobject 2024-05-22T20:10:42.613604Z shared object crypto ctx v1.'

// SO_OPERATION_PROTOCOL_VERSION is the operation format this build writes and accepts.
export const SO_OPERATION_PROTOCOL_VERSION = 1

// MAX_SO_OPERATION_PARENTS bounds the causal parents named by one operation.
export const MAX_SO_OPERATION_PARENTS = 256

// MAX_SO_INNER_DATA_SIZE bounds an operation body and its payload.
export const MAX_SO_INNER_DATA_SIZE = 1024 * 1024

// SO_OPERATION_SIGNATURE_CONTEXT is the signature context of every operation.
// The signed body binds the object, so the context needs no per-operation data.
export const SO_OPERATION_SIGNATURE_CONTEXT = `${SOBJECT_BASE_CRYPTO_CONTEXT}participant_operation_signature`

// soOperationHashDomain separates operation hashes from other SHA-256 digests.
const soOperationHashDomain = new TextEncoder().encode(
  'spacewave/sharedobject/operation/v1',
)

// soOperationLocalIDPattern matches the lowercase ULID local IDs Go accepts.
const soOperationLocalIDPattern = /^[0-7][0-9a-hjkmnp-tv-z]{25}$/

// hashSOOperationInner returns the identity of an operation from its signed
// body: SHA-256 over the domain, the big-endian body length, and the body.
export function hashSOOperationInner(inner: Uint8Array): Uint8Array {
  const length = new Uint8Array(8)
  new DataView(length.buffer).setBigUint64(0, BigInt(inner.length))
  return sha256(concatBytes(soOperationHashDomain, length, inner))
}

// validateSOOperationInner checks the body's identity, payload and links,
// matching Go SOOperationInner.Validate. It throws on the first violation.
export function validateSOOperationInner(inner: SOOperationInner): void {
  // The author, local ID, and sequence identify the operation.
  if (!extractPublicKeyFromPeerID(inner.peerId ?? '')) {
    throw new Error('operation peer_id is invalid')
  }
  if (!soOperationLocalIDPattern.test(inner.localId ?? '')) {
    throw new Error('operation local_id is invalid')
  }
  const nonce = inner.nonce ?? 0n
  if (nonce === 0n) {
    throw new Error('operation nonce must be positive')
  }

  // The payload is present and bounded.
  const opData = inner.opData ?? new Uint8Array()
  if (opData.length === 0 || opData.length > MAX_SO_INNER_DATA_SIZE) {
    throw new Error('operation op_data is empty or too large')
  }

  // The body names this protocol and an object.
  if (inner.protocolVersion !== SO_OPERATION_PROTOCOL_VERSION) {
    throw new Error(
      `unsupported operation protocol version ${inner.protocolVersion ?? 0}`,
    )
  }
  if (!inner.sharedObjectId) {
    throw new Error('operation shared_object_id is empty')
  }

  // The author's first operation has no predecessor; every later one names it.
  const prev = inner.prevOpHash ?? new Uint8Array()
  if (nonce === 1n && prev.length !== 0) {
    throw new Error(
      'first operation of an author must not name a previous operation',
    )
  }
  if (nonce > 1n && prev.length !== 32) {
    throw new Error('operation prev_op_hash must be a 32-byte hash')
  }

  // Parents are distinct hashes in byte order and never repeat prev.
  const parents = inner.parentHashes ?? []
  if (parents.length > MAX_SO_OPERATION_PARENTS) {
    throw new Error('operation parent_hashes exceeds the maximum count')
  }
  const prevHex = bytesToHex(prev)
  let last = ''
  for (const [i, parent] of parents.entries()) {
    if (parent.length !== 32) {
      throw new Error(`parent_hashes[${i}] must be a 32-byte hash`)
    }
    const hex = bytesToHex(parent)
    if (hex <= last) {
      throw new Error('operation parent_hashes must be strictly sorted')
    }
    if (hex === prevHex) {
      throw new Error('operation parent_hashes must not repeat prev_op_hash')
    }
    last = hex
  }

  // The operation names the config it was written under.
  if ((inner.configHash?.length ?? 0) !== 32) {
    throw new Error('operation config_hash must be a 32-byte hash')
  }
}

// verifySOOperation authenticates the operation as written for sharedObjectID
// by the author it names, and returns its body. It does not check
// authorization. It throws when the operation is invalid.
export async function verifySOOperation(
  sharedObjectID: string,
  op: SOOperation,
): Promise<SOOperationInner> {
  // Decode the well-formed body.
  const data = op.inner ?? new Uint8Array()
  if (data.length === 0 || data.length > MAX_SO_INNER_DATA_SIZE) {
    throw new Error('operation inner is empty or too large')
  }
  const inner = SOOperationInner.fromBinary(data)
  validateSOOperationInner(inner)

  // An operation signed for another object is not valid here.
  if (inner.sharedObjectId !== sharedObjectID) {
    throw new Error('operation is bound to another shared object')
  }

  // The signature covers exactly the encoded body, by the author.
  const signer = await verifySignature(
    SO_OPERATION_SIGNATURE_CONTEXT,
    op.signature,
    data,
  )
  if (!signer) {
    throw new Error('invalid operation signature')
  }
  if (signer !== inner.peerId) {
    throw new Error('signer peer ID does not match inner peer ID')
  }
  return inner
}

// canWriteOps reports whether a role may write operations.
export function canWriteOps(role: SOParticipantRole | undefined): boolean {
  return (
    role === SOParticipantRole.SOParticipantRole_WRITER ||
    role === SOParticipantRole.SOParticipantRole_OWNER
  )
}

// verifySOOperationWriter authenticates the operation and checks that its
// author may write operations under participants. It returns the body and
// throws when the operation is invalid or its author is not a writer.
export async function verifySOOperationWriter(
  sharedObjectID: string,
  op: SOOperation,
  participants: readonly SOParticipantConfig[],
): Promise<SOOperationInner> {
  const inner = await verifySOOperation(sharedObjectID, op)
  if (
    !participants.some((p) => p.peerId === inner.peerId && canWriteOps(p.role))
  ) {
    throw new Error('operation author is not a writer')
  }
  return inner
}

// SOEquivocation is evidence that an author signed several operations at one sequence.
export interface SOEquivocation {
  // peerId is the author.
  peerId: string
  // nonce is the author sequence.
  nonce: bigint
  // hashes are the conflicting operation hashes in byte order, as lowercase hex.
  hashes: string[]
}

// SOOperationSet is a set of verified operations of one shared object, keyed
// by lowercase hex operation hash. Adding operations in any order yields the
// same set, heads and evidence.
export class SOOperationSet {
  private readonly ops = new Map<string, SOOperationInner>()
  private readonly byAuthorSeq = new Map<string, string[]>()

  // constructor binds the set to one shared object.
  constructor(private readonly sharedObjectID: string) {}

  // add verifies op and adds it. It resolves false when the set already holds
  // it. An author's second operation at one sequence is kept as evidence.
  async add(op: SOOperation): Promise<boolean> {
    // Verify the operation and skip one the set holds.
    const inner = await verifySOOperation(this.sharedObjectID, op)
    const key = bytesToHex(hashSOOperationInner(op.inner ?? new Uint8Array()))
    if (this.ops.has(key)) {
      return false
    }

    // Index it by hash and by its author position. The author is a base58
    // peer ID, so the space separator cannot collide.
    this.ops.set(key, inner)
    const pos = `${inner.peerId} ${inner.nonce}`
    this.byAuthorSeq.set(pos, [...(this.byAuthorSeq.get(pos) ?? []), key])
    return true
  }

  // size is the number of operations in the set.
  get size(): number {
    return this.ops.size
  }

  // heads returns a fresh list of the operations no other operation in the
  // set names, in byte order, as lowercase hex.
  heads(): string[] {
    // Collect every hash an operation names.
    const named = new Set<string>()
    for (const inner of this.ops.values()) {
      named.add(bytesToHex(inner.prevOpHash ?? new Uint8Array()))
      for (const parent of inner.parentHashes ?? []) {
        named.add(bytesToHex(parent))
      }
    }

    // The heads are the operations left unnamed.
    return [...this.ops.keys()].filter((key) => !named.has(key)).sort()
  }

  // equivocations returns every author sequence with more than one operation,
  // ordered by author bytes and sequence.
  equivocations(): SOEquivocation[] {
    // Gather each position holding several operations.
    const out: SOEquivocation[] = []
    for (const keys of this.byAuthorSeq.values()) {
      if (keys.length < 2) {
        continue
      }
      const inner = this.ops.get(keys[0]!)!
      out.push({
        peerId: inner.peerId ?? '',
        nonce: inner.nonce ?? 0n,
        hashes: [...keys].sort(),
      })
    }

    // Order like Go: base58 peer IDs are ASCII, so string order is byte order.
    return out.sort((a, b) =>
      a.peerId !== b.peerId
        ? a.peerId < b.peerId
          ? -1
          : 1
        : a.nonce < b.nonce
          ? -1
          : a.nonce > b.nonce
            ? 1
            : 0,
    )
  }
}
