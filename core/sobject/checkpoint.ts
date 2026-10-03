import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex, concatBytes } from '@noble/hashes/utils.js'

import { verifySignature } from '../../net/peer/signature.js'
import {
  MAX_SO_PARTICIPANTS,
  validateSOAuthorHeads,
  validateSOSequenceHead,
} from './config-chain.js'
import { isSOGroupControl, verifySOCommit } from './control.js'
import { SOBJECT_BASE_CRYPTO_CONTEXT } from './operation-log.js'
import {
  SOCheckpoint,
  SOCheckpointInner,
  SOParticipantRole,
  type SharedObjectConfig,
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
  const authors = inner.authors ?? []
  const prev = inner.prevCheckpointHash ?? new Uint8Array()
  if ((inner.height ?? 0n) === 0n) {
    if (prev.length !== 0 || authors.length !== 0 || inner.sequence) {
      throw new Error('genesis checkpoint must not name previous operations')
    }
    return
  }
  if (prev.length !== 32) {
    throw new Error('checkpoint prev_checkpoint_hash must be a 32-byte hash')
  }
  validateSOSequenceHead('checkpoint sequence', inner.sequence)
  validateSOAuthorHeads('authors', authors)
}

// verifySOCheckpoint authenticates every signature on the checkpoint as
// written for sharedObjectID and returns the body and the signers. A
// checkpoint a group decided carries a commit and may carry no signature. It
// does not check authority; verifySOCheckpointAuthority does. It throws when
// the checkpoint is invalid.
export async function verifySOCheckpoint(
  sharedObjectID: string,
  checkpoint: SOCheckpoint,
): Promise<VerifiedSOCheckpoint> {
  // Decode the well-formed body bound to this object.
  const data = checkpoint.inner ?? new Uint8Array()
  const signatures = checkpoint.signatures ?? []
  const commit = checkpoint.commit ?? []
  if (data.length === 0) {
    throw new Error('checkpoint inner is empty')
  }
  if (signatures.length === 0 && commit.length === 0) {
    throw new Error('checkpoint has no signature or commit')
  }
  if (
    signatures.length > MAX_SO_PARTICIPANTS ||
    commit.length > MAX_SO_PARTICIPANTS
  ) {
    throw new Error('checkpoint has too many signatures')
  }
  const inner = SOCheckpointInner.fromBinary(data)
  validateSOCheckpointInner(inner)
  if (inner.sharedObjectId !== sharedObjectID) {
    throw new Error('checkpoint is bound to another shared object')
  }

  // Every signature covers exactly the encoded body, once per signer.
  const signers = await Promise.all(
    signatures.map((sig) =>
      verifySignature(SO_CHECKPOINT_SIGNATURE_CONTEXT, sig, data),
    ),
  )
  const seen = new Set<string>()
  for (const [i, signer] of signers.entries()) {
    if (!signer) {
      throw new Error(`signatures[${i}]: invalid signature`)
    }
    if (seen.has(signer)) {
      throw new Error(`signatures[${i}]: duplicate signer`)
    }
    seen.add(signer)
  }
  return { inner, signers: [...seen] }
}

// verifySOCheckpointAuthority authenticates the checkpoint and checks that cfg
// authorizes it, matching Go SOCheckpoint.ValidateAuthority. cfg vouches for
// its sealed checkpoint by hash. Otherwise, under group control, a commit of
// cfg's voters decided it under cfg, and under owner control an owner of cfg
// signed it. It throws when either check fails.
export async function verifySOCheckpointAuthority(
  sharedObjectID: string,
  checkpoint: SOCheckpoint,
  cfg: SharedObjectConfig,
): Promise<VerifiedSOCheckpoint> {
  // Authenticate the body and its signatures.
  const verified = await verifySOCheckpoint(sharedObjectID, checkpoint)
  const hash = hashSOCheckpointInner(checkpoint.inner!)
  const sealed = cfg.sealedCheckpoint
  if (
    sealed &&
    (sealed.height ?? 0n) === (verified.inner.height ?? 0n) &&
    bytesToHex(sealed.hash ?? new Uint8Array()) === bytesToHex(hash)
  ) {
    return verified
  }

  // A group decides each checkpoint under its current config.
  if (isSOGroupControl(cfg)) {
    if (
      bytesToHex(verified.inner.configHash ?? new Uint8Array()) !==
      bytesToHex(cfg.configChainHash ?? new Uint8Array())
    ) {
      throw new Error('checkpoint was not decided under the held config')
    }
    await verifySOCommit(
      sharedObjectID,
      cfg,
      verified.inner.height ?? 0n,
      hash,
      checkpoint.commit ?? [],
    )
    return verified
  }

  // An owner signs under owner control.
  const signers = new Set(verified.signers)
  if (
    !(cfg.participants ?? []).some(
      (p) =>
        p.role === SOParticipantRole.SOParticipantRole_OWNER &&
        signers.has(p.peerId ?? ''),
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
