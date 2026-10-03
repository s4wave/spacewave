import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex, concatBytes } from '@noble/hashes/utils.js'

import { extractPublicKeyFromPeerID } from '../../net/peer/id.js'
import { verifySignature } from '../../net/peer/signature.js'
import { hashSOCheckpointInner, MAX_SO_STATE_DATA_SIZE } from './checkpoint.js'
import { configChangeSignedBody, MAX_SO_PARTICIPANTS } from './config-chain.js'
import { SOBJECT_BASE_CRYPTO_CONTEXT } from './operation-log.js'
import {
  SOConfigChange,
  SOControl,
  SOControlMessageInner,
  SOControlMessageType,
  SODecisionKind,
  SOParticipantRole,
  type SOCheckpointHead,
  type SOControlMessage,
  type SharedObjectConfig,
} from './sobject.pb.js'

// MAX_SO_VOTING_WEIGHT bounds one device's voting weight, matching Go
// MaxVotingWeight.
export const MAX_SO_VOTING_WEIGHT = 1 << 16

// SO_CONTROL_MESSAGE_SIGNATURE_CONTEXT is the signature context of every
// control message. The signed body binds the object and the instance, so the
// context needs no per-message data.
export const SO_CONTROL_MESSAGE_SIGNATURE_CONTEXT = `${SOBJECT_BASE_CRYPTO_CONTEXT}control_message_signature`

// soControlMessageHashDomain separates control message hashes from other
// SHA-256 digests.
const soControlMessageHashDomain = new TextEncoder().encode(
  'spacewave/sharedobject/control/v1',
)

// isSOGroupControl reports whether the config's voters, not an owner, decide
// its control records and checkpoints.
export function isSOGroupControl(cfg: SharedObjectConfig): boolean {
  return cfg.control === SOControl.SO_CONTROL_GROUP
}

// soVotingWeight returns peerID's voting weight under group control, or zero.
export function soVotingWeight(
  cfg: SharedObjectConfig,
  peerID: string,
): number {
  if (!isSOGroupControl(cfg)) {
    return 0
  }
  const p = (cfg.participants ?? []).find((p) => p.peerId === peerID)
  return p?.votingWeight ?? 0
}

// soTotalVotingWeight returns the sum of every voter's weight under group
// control, or zero.
export function soTotalVotingWeight(cfg: SharedObjectConfig): number {
  if (!isSOGroupControl(cfg)) {
    return 0
  }
  return (cfg.participants ?? []).reduce(
    (total, p) => total + (p.votingWeight ?? 0),
    0,
  )
}

// soHasQuorum reports whether weight is more than two thirds of total. Two
// such quorums share an honest voter while dishonest voters hold less than
// one third.
export function soHasQuorum(weight: number, total: number): boolean {
  return total !== 0 && weight * 3 > total * 2
}

// validateSOControl checks the control setting and voting weights, matching
// Go validateControl: under owner control nobody votes; under group control
// only writers and owners vote, at least one does, and the config names the
// checkpoint it sealed. It throws on the first violation.
export function validateSOControl(cfg: SharedObjectConfig): void {
  // Owner control has no voters.
  const participants = cfg.participants ?? []
  const control = cfg.control ?? SOControl.SO_CONTROL_OWNER
  if (control === SOControl.SO_CONTROL_OWNER) {
    for (const [i, p] of participants.entries()) {
      if ((p.votingWeight ?? 0) !== 0) {
        throw new Error(`participants[${i}]: voting weight under owner control`)
      }
    }
    if (cfg.sealedCheckpoint) {
      validateSOCheckpointHead(cfg.sealedCheckpoint)
    }
    return
  }
  if (control !== SOControl.SO_CONTROL_GROUP) {
    throw new Error(`unknown control ${Number(cfg.control)}`)
  }

  // Group control needs a writing voter and the sealed checkpoint.
  let total = 0
  for (const [i, p] of participants.entries()) {
    const w = p.votingWeight ?? 0
    if (w > MAX_SO_VOTING_WEIGHT) {
      throw new Error(
        `participants[${i}]: voting weight above ${MAX_SO_VOTING_WEIGHT}`,
      )
    }
    if (w !== 0 && !canWriteOps(p.role)) {
      throw new Error(`participants[${i}]: only writers and owners vote`)
    }
    total += w
  }
  if (total === 0) {
    throw new Error('group control needs a voter')
  }
  if (!cfg.sealedCheckpoint) {
    throw new Error('group control needs a sealed checkpoint')
  }
  validateSOCheckpointHead(cfg.sealedCheckpoint)
}

// canWriteOps reports whether role may write operations, and so vote.
function canWriteOps(role: SOParticipantRole | undefined): boolean {
  return (
    role === SOParticipantRole.SOParticipantRole_WRITER ||
    role === SOParticipantRole.SOParticipantRole_OWNER
  )
}

// validateSOCheckpointHead checks that head names a checkpoint.
function validateSOCheckpointHead(head: SOCheckpointHead): void {
  if ((head.hash?.length ?? 0) !== 32) {
    throw new Error('checkpoint head hash must be a 32-byte hash')
  }
}

// hashSOControlMessageInner returns the identity of a control message from
// its signed body: SHA-256 over the domain, the big-endian body length, and
// the body.
export function hashSOControlMessageInner(inner: Uint8Array): Uint8Array {
  const length = new Uint8Array(8)
  new DataView(length.buffer).setBigUint64(0, BigInt(inner.length))
  return sha256(concatBytes(soControlMessageHashDomain, length, inner))
}

// soControlValueHash returns the identity of a value of kind: the hash of a
// control record, which must be encoded with signatures and commit cleared,
// or of a checkpoint body. It throws on an unknown kind or a control record
// that is not its signed body.
export function soControlValueHash(
  kind: SODecisionKind | undefined,
  value: Uint8Array,
): Uint8Array {
  // A checkpoint is named by its body.
  if (kind === SODecisionKind.SO_DECISION_KIND_CHECKPOINT) {
    return hashSOCheckpointInner(value)
  }
  if (kind !== SODecisionKind.SO_DECISION_KIND_CONFIG) {
    throw new Error(`unknown decision kind ${kind ?? 0}`)
  }

  // A control record's hash is over its canonical encoding, so the value
  // must be that encoding.
  const body = configChangeSignedBody(SOConfigChange.fromBinary(value))
  if (bytesToHex(body) !== bytesToHex(value)) {
    throw new Error('control record value is not its signed body')
  }
  return sha256(body)
}

// validateSOControlMessageInner checks the message body's structure, matching
// Go SOControlMessageInner.Validate: the instance it belongs to, and the
// fields its type carries. It throws on the first violation.
export function validateSOControlMessageInner(
  inner: SOControlMessageInner,
): void {
  // The body names an object, a voter and an instance.
  if (!inner.sharedObjectId) {
    throw new Error('control message shared_object_id is empty')
  }
  if (!extractPublicKeyFromPeerID(inner.peerId ?? '')) {
    throw new Error('control message peer_id is invalid')
  }
  if ((inner.configHash?.length ?? 0) !== 32) {
    throw new Error('control message config_hash must be a 32-byte hash')
  }
  const valueHashLength = inner.valueHash?.length ?? 0
  if (valueHashLength !== 0 && valueHashLength !== 32) {
    throw new Error(
      'control message value_hash must be empty or a 32-byte hash',
    )
  }

  // Each type carries its own fields.
  const height = inner.height ?? 0n
  const round = inner.round ?? 0
  const validRound = inner.validRound ?? 0
  switch (inner.type) {
    case SOControlMessageType.SO_CONTROL_MESSAGE_TYPE_AGREE:
      if (inner.kind !== SODecisionKind.SO_DECISION_KIND_CONFIG) {
        throw new Error('only control records are agreed to')
      }
      if (height !== 0n || round !== 0 || validRound !== 0) {
        throw new Error('agreement must not name a height or round')
      }
      validateSOControlValue(inner)
      return
    case SOControlMessageType.SO_CONTROL_MESSAGE_TYPE_PROPOSAL:
      if (height === 0n || round === 0 || validRound >= round) {
        throw new Error(
          'proposal must name a height and a round after its valid round',
        )
      }
      validateSOControlValue(inner)
      return
    case SOControlMessageType.SO_CONTROL_MESSAGE_TYPE_PREVOTE:
    case SOControlMessageType.SO_CONTROL_MESSAGE_TYPE_PRECOMMIT:
      if ((inner.kind ?? 0) !== SODecisionKind.SO_DECISION_KIND_UNKNOWN) {
        throw new Error('vote must not name a value kind')
      }
      if (
        height === 0n ||
        round === 0 ||
        validRound !== 0 ||
        (inner.value?.length ?? 0) !== 0
      ) {
        throw new Error('vote must name a height and round and carry no value')
      }
      return
    default:
      throw new Error(`unknown control message type ${inner.type ?? 0}`)
  }
}

// validateSOControlValue checks that the message carries the bounded value it
// names. A value holds a checkpoint body with its World, or a control record.
function validateSOControlValue(inner: SOControlMessageInner): void {
  // The value must be present and bounded.
  const value = inner.value ?? new Uint8Array()
  if (
    value.length === 0 ||
    value.length > MAX_SO_STATE_DATA_SIZE + 1024 * 1024
  ) {
    throw new Error('control message must carry a bounded value')
  }

  // The value must match its hash.
  const hash = soControlValueHash(inner.kind, value)
  if (bytesToHex(hash) !== bytesToHex(inner.valueHash ?? new Uint8Array())) {
    throw new Error('control message value does not match value_hash')
  }
}

// verifySOControlMessage authenticates the message as signed for
// sharedObjectID by the voter it names and returns its body, matching Go
// SOControlMessage.Verify. It does not check that the signer votes. It throws
// when the message is invalid.
export async function verifySOControlMessage(
  sharedObjectID: string,
  msg: SOControlMessage,
): Promise<SOControlMessageInner> {
  // Decode the well-formed body bound to this object.
  const data = msg.inner ?? new Uint8Array()
  if (data.length === 0) {
    throw new Error('control message inner is empty')
  }
  const inner = SOControlMessageInner.fromBinary(data)
  validateSOControlMessageInner(inner)
  if (inner.sharedObjectId !== sharedObjectID) {
    throw new Error('control message is bound to another shared object')
  }

  // The signature covers exactly the encoded body, by the named voter.
  const signer = await verifySignature(
    SO_CONTROL_MESSAGE_SIGNATURE_CONTEXT,
    msg.signature,
    data,
  )
  if (!signer) {
    throw new Error('control message signature is invalid')
  }
  if (signer !== inner.peerId) {
    throw new Error('control message is not signed by its peer')
  }
  return inner
}

// verifySOCommit checks that commit decides valueHash for the instance at
// height under cfg, matching Go VerifyCommit: precommits of one round for
// valueHash under cfg's head, each from a distinct voter of cfg, together
// holding more than two thirds of its voting weight. It throws when the
// commit does not decide the value.
export async function verifySOCommit(
  sharedObjectID: string,
  cfg: SharedObjectConfig,
  height: bigint,
  valueHash: Uint8Array,
  commit: readonly SOControlMessage[],
): Promise<void> {
  // Only group control decides by commit.
  if (!isSOGroupControl(cfg)) {
    throw new Error('commit requires group control')
  }
  if (commit.length === 0 || commit.length > MAX_SO_PARTICIPANTS) {
    throw new Error(
      'commit must hold between one and the participant limit of precommits',
    )
  }

  // Verify every precommit before judging the decision.
  const inners = await Promise.all(
    commit.map((msg, i) =>
      verifySOControlMessage(sharedObjectID, msg).catch((err: Error) => {
        throw new Error(`commit[${i}]: ${err.message}`, { cause: err })
      }),
    ),
  )

  // Sum the weight of distinct voters precommitting valueHash in one round.
  const configHash = bytesToHex(cfg.configChainHash ?? new Uint8Array())
  const value = bytesToHex(valueHash)
  const seen = new Set<string>()
  let weight = 0
  for (const [i, inner] of inners.entries()) {
    if (
      inner.type !== SOControlMessageType.SO_CONTROL_MESSAGE_TYPE_PRECOMMIT ||
      (inner.height ?? 0n) !== height ||
      bytesToHex(inner.configHash ?? new Uint8Array()) !== configHash ||
      bytesToHex(inner.valueHash ?? new Uint8Array()) !== value
    ) {
      throw new Error(`commit[${i}]: not a precommit of this decision`)
    }
    if ((inner.round ?? 0) !== (inners[0].round ?? 0)) {
      throw new Error(`commit[${i}]: precommits span rounds`)
    }
    const peerID = inner.peerId ?? ''
    if (seen.has(peerID)) {
      throw new Error(`commit[${i}]: duplicate voter`)
    }
    seen.add(peerID)
    const w = soVotingWeight(cfg, peerID)
    if (w === 0) {
      throw new Error(`commit[${i}]: ${peerID} does not vote`)
    }
    weight += w
  }

  // The voters hold more than two thirds of the weight.
  if (!soHasQuorum(weight, soTotalVotingWeight(cfg))) {
    throw new Error('commit lacks more than two thirds of the voting weight')
  }
}
