import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex, concatBytes } from '@noble/hashes/utils.js'

import { extractPublicKeyFromPeerID } from '../../net/peer/id.js'
import { verifySignature } from '../../net/peer/signature.js'
import { MAX_SO_PARTICIPANTS } from './config-chain.js'
import { SOBJECT_BASE_CRYPTO_CONTEXT } from './operation-log.js'
import {
  SOCheckpoint,
  SOCheckpointInner,
  SOParticipantRole,
  type SOParticipantConfig,
} from './sobject.pb.js'

// SO_REPLAY_VERSION is the replay rule version this build accepts.
export const SO_REPLAY_VERSION = 1

// MAX_SO_STATE_DATA_SIZE bounds the encrypted state a checkpoint carries.
export const MAX_SO_STATE_DATA_SIZE = 10 * 1024 * 1024

// SO_CHECKPOINT_SIGNATURE_CONTEXT is the signature context of every checkpoint.
// The signed body binds the object, so the context needs no per-checkpoint data.
export const SO_CHECKPOINT_SIGNATURE_CONTEXT = `${SOBJECT_BASE_CRYPTO_CONTEXT}checkpoint_signature`

// soCheckpointHashDomain separates checkpoint hashes from other SHA-256 digests.
const soCheckpointHashDomain = new TextEncoder().encode(
  'spacewave/sharedobject/checkpoint/v1',
)

// VerifiedSOCheckpoint is an authenticated checkpoint body and its signers.
export interface VerifiedSOCheckpoint {
  // inner is the decoded body.
  inner: SOCheckpointInner
  // signers are the peer IDs that signed the body, in signature order.
  signers: string[]
}

// hashSOCheckpointInner returns the identity of a checkpoint from its signed
// body: SHA-256 over the domain, the big-endian body length, and the body.
export function hashSOCheckpointInner(inner: Uint8Array): Uint8Array {
  const length = new Uint8Array(8)
  new DataView(length.buffer).setBigUint64(0, BigInt(inner.length))
  return sha256(concatBytes(soCheckpointHashDomain, length, inner))
}

// validateSOCheckpointInner checks the body's structure, matching Go
// SOCheckpointInner.Validate. It throws on the first violation.
export function validateSOCheckpointInner(inner: SOCheckpointInner): void {
  // The body names an object, a config and a known replay rule.
  if (!inner.sharedObjectId) {
    throw new Error('checkpoint shared_object_id is empty')
  }
  if ((inner.configHash?.length ?? 0) !== 32) {
    throw new Error('checkpoint config_hash must be a 32-byte hash')
  }
  if (inner.replayVersion !== SO_REPLAY_VERSION) {
    throw new Error(
      `unsupported checkpoint replay version ${inner.replayVersion ?? 0}`,
    )
  }
  if ((inner.stateData?.length ?? 0) > MAX_SO_STATE_DATA_SIZE) {
    throw new Error('checkpoint state_data exceeds the maximum size')
  }

  // Genesis starts the chain; every later checkpoint names its predecessor.
  const frontier = inner.frontier ?? []
  const authors = inner.authors ?? []
  const prev = inner.prevCheckpointHash ?? new Uint8Array()
  if ((inner.height ?? 0n) === 0n) {
    if (prev.length !== 0 || frontier.length !== 0 || authors.length !== 0) {
      throw new Error('genesis checkpoint must not name previous operations')
    }
    return
  }
  if (prev.length !== 32) {
    throw new Error('checkpoint prev_checkpoint_hash must be a 32-byte hash')
  }

  // The frontier holds distinct hashes in byte order.
  let last = ''
  for (const [i, h] of frontier.entries()) {
    if (h.length !== 32) {
      throw new Error(`frontier[${i}] must be a 32-byte hash`)
    }
    const hex = bytesToHex(h)
    if (hex <= last) {
      throw new Error('checkpoint frontier must be strictly sorted')
    }
    last = hex
  }

  // Each author appears once, in peer ID order, at a real operation. Base58
  // peer IDs are ASCII, so string order is byte order.
  let lastPeer = ''
  for (const [i, author] of authors.entries()) {
    const peerId = author.peerId ?? ''
    if (!extractPublicKeyFromPeerID(peerId)) {
      throw new Error(`authors[${i}]: peer_id is invalid`)
    }
    if ((author.nonce ?? 0n) === 0n || (author.opHash?.length ?? 0) !== 32) {
      throw new Error(`authors[${i}] must name an operation`)
    }
    if (i > 0 && peerId <= lastPeer) {
      throw new Error('checkpoint authors must be strictly sorted by peer_id')
    }
    lastPeer = peerId
  }
}

// verifySOCheckpoint authenticates every signature on the checkpoint as
// written for sharedObjectID and returns the body and the signers. It does not
// check that a signer holds authority; verifySOCheckpointAuthority does. It
// throws when the checkpoint is invalid.
export async function verifySOCheckpoint(
  sharedObjectID: string,
  checkpoint: SOCheckpoint,
): Promise<VerifiedSOCheckpoint> {
  // Decode the well-formed body bound to this object.
  const data = checkpoint.inner ?? new Uint8Array()
  const signatures = checkpoint.signatures ?? []
  if (data.length === 0) {
    throw new Error('checkpoint inner is empty')
  }
  if (signatures.length === 0) {
    throw new Error('checkpoint has no signature')
  }
  if (signatures.length > MAX_SO_PARTICIPANTS) {
    throw new Error('checkpoint has too many signatures')
  }
  const inner = SOCheckpointInner.fromBinary(data)
  validateSOCheckpointInner(inner)
  if (inner.sharedObjectId !== sharedObjectID) {
    throw new Error('checkpoint is bound to another shared object')
  }

  // Every signature covers exactly the encoded body, once per signer.
  const signers: string[] = []
  for (const [i, sig] of signatures.entries()) {
    const signer = await verifySignature(
      SO_CHECKPOINT_SIGNATURE_CONTEXT,
      sig,
      data,
    )
    if (!signer) {
      throw new Error(`signatures[${i}]: invalid signature`)
    }
    if (signers.includes(signer)) {
      throw new Error(`signatures[${i}]: duplicate signer`)
    }
    signers.push(signer)
  }
  return { inner, signers }
}

// verifySOCheckpointAuthority authenticates the checkpoint and checks that an
// owner under participants signed it, matching Go
// SOCheckpoint.ValidateAuthority. It throws when either check fails.
export async function verifySOCheckpointAuthority(
  sharedObjectID: string,
  checkpoint: SOCheckpoint,
  participants: readonly SOParticipantConfig[],
): Promise<VerifiedSOCheckpoint> {
  const verified = await verifySOCheckpoint(sharedObjectID, checkpoint)
  if (
    !participants.some(
      (p) =>
        p.role === SOParticipantRole.SOParticipantRole_OWNER &&
        verified.signers.includes(p.peerId ?? ''),
    )
  ) {
    throw new Error('checkpoint is not signed by an owner')
  }
  return verified
}

// soCheckpointCovers reports whether the checkpoint covers the operation at
// nonce in peerId's chain, matching Go SOOperationSet.Covers.
export function soCheckpointCovers(
  inner: SOCheckpointInner | undefined,
  peerId: string,
  nonce: bigint,
): boolean {
  const author = inner?.authors?.find((a) => a.peerId === peerId)
  return author !== undefined && nonce <= (author.nonce ?? 0n)
}
