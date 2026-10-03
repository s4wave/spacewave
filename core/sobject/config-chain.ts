import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex } from '@noble/hashes/utils.js'

import { extractPublicKeyFromPeerID } from '../../net/peer/id.js'
import { verifySignature } from '../../net/peer/signature.js'
import {
  isSOGroupControl,
  validateSOControl,
  verifySOCommit,
} from './control.js'
import {
  SOConfigChange,
  SOConfigChangeType,
  SOParticipantConfig,
  type SOOperationPosition,
  SOParticipantRole,
  type SOSequenceHead,
  type SOSequencer,
  type SharedObjectConfig,
} from './sobject.pb.js'

// SO_CONFIG_CHANGE_SIGNATURE_CONTEXT is the signature context of every control
// record. The signed body binds the object, so the context needs no per-record data.
export const SO_CONFIG_CHANGE_SIGNATURE_CONTEXT = 'sobject config change'

// MAX_SO_PARTICIPANTS bounds the participants of one configuration.
export const MAX_SO_PARTICIPANTS = 100

// SOConfigChangeErrorKind classifies a rejected control record.
// invalid: the record is malformed or bound to another object.
// stale: the record does not extend the held head; a fresher base may succeed.
// unauthorized: the held configuration does not authorize the record.
export type SOConfigChangeErrorKind = 'invalid' | 'stale' | 'unauthorized'

// SOConfigChangeError is thrown when a control record is rejected.
export class SOConfigChangeError extends Error {
  // constructor records the rejection kind with its message.
  constructor(
    readonly kind: SOConfigChangeErrorKind,
    message: string,
  ) {
    super(message)
    this.name = 'SOConfigChangeError'
  }
}

// hashSOConfigChange returns the identity of a control record: the SHA-256 of
// its encoding with signatures and commit cleared, which is also the signed
// body and the value a group decides.
export function hashSOConfigChange(entry: SOConfigChange): Uint8Array {
  return sha256(configChangeSignedBody(entry))
}

// configChangeSignedBody encodes the record with its authorization, the
// signatures and the commit, cleared.
export function configChangeSignedBody(entry: SOConfigChange): Uint8Array {
  return SOConfigChange.toBinary({ ...entry, signatures: [], commit: [] })
}

// validateSOConfig checks that a configuration can authorize later changes,
// matching Go SharedObjectConfig.Validate. A signed head may retain the final
// departure with no participants. It throws on the first violation.
export function validateSOConfig(cfg: SharedObjectConfig): void {
  // Pinned operations of removed authors outlive the final departure too.
  try {
    validateSOAuthorHeads('removed_authors', cfg.removedAuthors ?? [])
  } catch (err) {
    throw new SOConfigChangeError('invalid', (err as Error).message)
  }

  // The roster drops each peer once, in order.
  const dropped = cfg.rosterDroppedPeerIds ?? []
  for (const [i, peerID] of dropped.entries()) {
    if (!extractPublicKeyFromPeerID(peerID)) {
      throw new SOConfigChangeError(
        'invalid',
        `roster_dropped_peer_ids[${i}]: invalid peer id`,
      )
    }
    if (i > 0 && dropped[i - 1] >= peerID) {
      throw new SOConfigChangeError(
        'invalid',
        'roster_dropped_peer_ids must be strictly sorted',
      )
    }
  }

  // An appointed sequencer names a key and a real start.
  try {
    validateSOSequencer(cfg.sequencer)
  } catch (err) {
    throw new SOConfigChangeError('invalid', (err as Error).message)
  }

  // The control setting matches its voting weights.
  try {
    validateSOControl(cfg)
  } catch (err) {
    throw new SOConfigChangeError('invalid', (err as Error).message)
  }

  // A signed history head can retain the final departure; an empty bootstrap cannot grant authority.
  const participants = cfg.participants ?? []
  if (participants.length === 0) {
    if ((cfg.configChainHash?.length ?? 0) !== 32) {
      throw new SOConfigChangeError('invalid', 'config has no participants')
    }
    return
  }
  if (participants.length > MAX_SO_PARTICIPANTS) {
    throw new SOConfigChangeError('invalid', 'config has too many participants')
  }

  // Each remaining peer has one unambiguous role in this configuration.
  const seen = new Set<string>()
  for (const [i, p] of participants.entries()) {
    const peerID = p.peerId ?? ''
    if (!extractPublicKeyFromPeerID(peerID)) {
      throw new SOConfigChangeError(
        'invalid',
        `participants[${i}]: invalid peer id`,
      )
    }
    const role = p.role ?? SOParticipantRole.SOParticipantRole_UNKNOWN
    if (
      role < SOParticipantRole.SOParticipantRole_READER ||
      role > SOParticipantRole.SOParticipantRole_OWNER
    ) {
      throw new SOConfigChangeError(
        'invalid',
        `participants[${i}]: invalid role`,
      )
    }
    if (seen.has(peerID)) {
      throw new SOConfigChangeError(
        'invalid',
        `participants[${i}]: duplicate peer id: ${peerID}`,
      )
    }
    seen.add(peerID)
  }

  // Remaining participants need an owner to authorize any later change.
  if (
    !participants.some(
      (p) => p.role === SOParticipantRole.SOParticipantRole_OWNER,
    )
  ) {
    throw new SOConfigChangeError('invalid', 'config has no owner')
  }
}

// validateSOAuthorHeads checks that each author appears once, in peer ID
// order, at a real operation, matching Go validateAuthorHeads. field names the
// list in errors. Base58 peer IDs are ASCII, so string order is byte order.
export function validateSOAuthorHeads(
  field: string,
  authors: readonly SOOperationPosition[],
): void {
  for (const [i, author] of authors.entries()) {
    try {
      validateSOPosition(author)
    } catch (err) {
      throw new Error(`${field}[${i}]: ${(err as Error).message}`, {
        cause: err,
      })
    }
    if (i > 0 && (authors[i - 1].peerId ?? '') >= (author.peerId ?? '')) {
      throw new Error(`${field} must be strictly sorted by peer_id`)
    }
  }
}

// validateSOPosition checks that pos names a real operation, matching Go
// validatePosition. It throws on violation.
export function validateSOPosition(pos: SOOperationPosition | undefined): void {
  if (!extractPublicKeyFromPeerID(pos?.peerId ?? '')) {
    throw new Error('position peer_id is invalid')
  }
  if ((pos?.nonce ?? 0n) === 0n || (pos?.opHash?.length ?? 0) !== 32) {
    throw new Error('position must name an operation')
  }
}

// validateSOSequenceHead checks that head is unset, height 0 with no hash, or
// a real position, matching Go validateSequenceHead. field names the head in
// errors.
export function validateSOSequenceHead(
  field: string,
  head: SOSequenceHead | undefined,
): void {
  const height = head?.height ?? 0n
  const length = head?.hash?.length ?? 0
  if (height === 0n && length !== 0) {
    throw new Error(`${field} at height 0 must not name a position`)
  }
  if (height !== 0n && length !== 32) {
    throw new Error(`${field} hash must be a 32-byte hash`)
  }
}

// validateSOSequencer checks the sequencer's structure, matching Go
// SOSequencer.Validate. An unset sequencer is Merge.
export function validateSOSequencer(sequencer: SOSequencer | undefined): void {
  const peerId = sequencer?.peerId ?? ''
  if (peerId && !extractPublicKeyFromPeerID(peerId)) {
    throw new Error('sequencer peer_id is invalid')
  }
  validateSOSequenceHead('sequencer start', sequencer?.start)
}

// verifySOConfigChain verifies a control chain from genesis and returns the
// resulting configuration with its chain head. Genesis is signed by an owner
// of its own configuration, which starts under owner control; each later
// record is authorized by its parent.
// It throws SOConfigChangeError when the chain is invalid.
export async function verifySOConfigChain(
  sharedObjectID: string,
  entries: readonly SOConfigChange[],
): Promise<SharedObjectConfig> {
  // Genesis is the unlinked first record of this object.
  const [genesis, ...rest] = entries
  if (!genesis) {
    throw new SOConfigChangeError('invalid', 'config chain is empty')
  }
  if (genesis.sharedObjectId !== sharedObjectID) {
    throw new SOConfigChangeError(
      'invalid',
      'genesis entry is bound to another shared object',
    )
  }
  if (
    (genesis.configSeqno ?? 0n) !== 0n ||
    (genesis.previousHash?.length ?? 0) !== 0 ||
    genesis.changeType !== SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_GENESIS
  ) {
    throw new SOConfigChangeError(
      'invalid',
      'genesis entry must be an unlinked GENESIS at seqno 0',
    )
  }

  // Genesis grants owner authority to its own signers.
  const config = genesis.config
  if (!config?.participants?.length) {
    throw new SOConfigChangeError(
      'invalid',
      'genesis entry has no participants',
    )
  }
  validateSOConfig(config)
  if (isSOGroupControl(config)) {
    throw new SOConfigChangeError(
      'invalid',
      'genesis entry must start under owner control',
    )
  }
  await verifyConfigChangeAuthority(genesis, config)

  // Verify each transition under the preceding configuration.
  let current = withConfigChainHead(config, 0n, hashSOConfigChange(genesis))
  for (const entry of rest) {
    current = await verifySOConfigChange(sharedObjectID, current, entry)
  }
  return current
}

// verifySOConfigChange verifies one signed transition against the held
// configuration and returns a fresh configuration carrying the new head.
// Checks run in this order: object binding, then the link to the held head
// (stale), then authorization, then the shape of the result. A
// SELF_ENROLL_PEER record checks only the configuration shape; the caller
// must authenticate the peer-to-entity binding.
// It throws SOConfigChangeError when the record is rejected.
export async function verifySOConfigChange(
  sharedObjectID: string,
  current: SharedObjectConfig,
  entry: SOConfigChange,
): Promise<SharedObjectConfig> {
  // The record carries a configuration for this object.
  const next = entry.config
  if (!next) {
    throw new SOConfigChangeError(
      'invalid',
      'config change requires the next configuration',
    )
  }
  if (entry.sharedObjectId !== sharedObjectID) {
    throw new SOConfigChangeError(
      'invalid',
      'config change is bound to another shared object',
    )
  }

  // The record extends the held head.
  const head = current.configChainHash ?? new Uint8Array()
  if (bytesToHex(entry.previousHash ?? new Uint8Array()) !== bytesToHex(head)) {
    throw new SOConfigChangeError(
      'stale',
      'config change previous_hash does not match current config_chain_hash',
    )
  }
  const expected =
    head.length === 0 ? 0n : (current.configChainSeqno ?? 0n) + 1n
  if (expected > 0xffffffffffffffffn) {
    throw new SOConfigChangeError('invalid', 'config change sequence exhausted')
  }
  if ((entry.configSeqno ?? 0n) !== expected) {
    throw new SOConfigChangeError(
      'stale',
      `config change seqno ${entry.configSeqno ?? 0n} does not match expected ${expected}`,
    )
  }

  // The held configuration authorizes the record.
  await verifyConfigChangeAuthority(entry, current)

  // An authorized signer still cannot produce an unusable configuration.
  const result = withConfigChainHead(next, expected, hashSOConfigChange(entry))
  validateSOConfig(result)
  return result
}

// withConfigChainHead returns a copy of cfg carrying a verified head.
function withConfigChainHead(
  cfg: SharedObjectConfig,
  seqno: bigint,
  hash: Uint8Array,
): SharedObjectConfig {
  return { ...cfg, configChainSeqno: seqno, configChainHash: hash }
}

// verifyConfigChangeAuthority checks that cfg authorizes the record, matching
// Go verifyConfigChangeAuthority. Under group control, a commit of cfg's
// voters decides every record but a self-enrollment, at one more than the
// height of the checkpoint the record seals, with no signature. Otherwise the
// record carries no commit and at least one signature, each valid over the
// body, from a distinct signer, and each signer an OWNER of cfg. A
// SELF_ENROLL_PEER record carries exactly one signature, by the enrolling
// peer.
async function verifyConfigChangeAuthority(
  entry: SOConfigChange,
  cfg: SharedObjectConfig,
): Promise<void> {
  // A group decides every change but a device joining its own entity.
  const deny = (message: string) =>
    new SOConfigChangeError('unauthorized', message)
  const sigs = entry.signatures ?? []
  const selfEnroll =
    entry.changeType ===
    SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER
  if (isSOGroupControl(cfg) && !selfEnroll) {
    if (sigs.length !== 0) {
      throw deny('group control record must not carry signatures')
    }
    const sealed = entry.config?.sealedCheckpoint
    if (!sealed) {
      throw deny('group control record must seal a checkpoint')
    }
    await verifySOCommit(
      entry.sharedObjectId ?? '',
      cfg,
      (sealed.height ?? 0n) + 1n,
      hashSOConfigChange(entry),
      entry.commit ?? [],
    ).catch((err: Error) => {
      throw deny(err.message)
    })
    return
  }

  // Require signatures in the count the change type allows, and no commit.
  if ((entry.commit?.length ?? 0) !== 0) {
    throw deny('signed control record must not carry a commit')
  }
  if (sigs.length === 0) {
    throw deny('missing signature')
  }
  if (selfEnroll && sigs.length !== 1) {
    throw deny('self-enroll must carry exactly one signature')
  }

  // Verify every signature over the body.
  const body = configChangeSignedBody(entry)
  const signers = await Promise.all(
    sigs.map((sig) =>
      verifySignature(SO_CONFIG_CHANGE_SIGNATURE_CONTEXT, sig, body),
    ),
  )

  // Each signer is distinct and authorized by cfg.
  const seen = new Set<string>()
  for (const [i, signer] of signers.entries()) {
    if (!signer) {
      throw deny(`signatures[${i}]: invalid signature`)
    }
    if (seen.has(signer)) {
      throw deny(`signatures[${i}]: duplicate signer ${signer}`)
    }
    seen.add(signer)
    if (selfEnroll) {
      validateSelfEnrollPeerChange(entry, cfg, signer)
    } else if (!isOwnerPeer(cfg, signer)) {
      throw deny(`signer ${signer} is not an OWNER in the config`)
    }
  }
}

// isOwnerPeer reports whether a peer holds owner authority in the configuration.
function isOwnerPeer(cfg: SharedObjectConfig, peerID: string): boolean {
  return (cfg.participants ?? []).some(
    (p) =>
      p.peerId === peerID &&
      p.role === SOParticipantRole.SOParticipantRole_OWNER,
  )
}

// validateSelfEnrollPeerChange checks that a self-enrollment adds only its
// signer, under an entity already present, without raising the entity's role,
// renaming it, or changing configuration metadata. The caller must separately
// authenticate that the signer belongs to the added participant's entity.
function validateSelfEnrollPeerChange(
  entry: SOConfigChange,
  cfg: SharedObjectConfig,
  signer: string,
): void {
  // Enrollment preserves all configuration metadata.
  const deny = (message: string) =>
    new SOConfigChangeError('unauthorized', message)
  const next = entry.config ?? {}
  if (
    bytesToHex(next.configChainHash ?? new Uint8Array()) !==
      bytesToHex(cfg.configChainHash ?? new Uint8Array()) ||
    (next.configChainSeqno ?? 0n) !== (cfg.configChainSeqno ?? 0n)
  ) {
    throw deny('self-enroll may not mutate config metadata')
  }

  // Exactly one peer is added and every existing participant is unchanged.
  const prev = new Map((cfg.participants ?? []).map((p) => [p.peerId ?? '', p]))
  const added = (next.participants ?? []).filter(
    (p) => !prev.has(p.peerId ?? ''),
  )
  if (
    (next.participants ?? []).length !== prev.size + 1 ||
    added.length !== 1
  ) {
    throw deny('self-enroll must add exactly one participant')
  }
  for (const [peerID, p] of prev) {
    const kept = (next.participants ?? []).find(
      (q) => (q.peerId ?? '') === peerID,
    )
    if (!kept || !SOParticipantConfig.equals(p, kept)) {
      throw deny('self-enroll may not modify existing participants')
    }
  }

  // The added participant is the signer, bound to an existing entity.
  const participant = added[0]
  if (participant.peerId !== signer) {
    throw deny('self-enroll signer must match the added participant')
  }
  const entityID = participant.entityId ?? ''
  if (!entityID) {
    throw deny('self-enroll participant requires entity_id')
  }

  // A device joins without a vote; the group grants votes.
  if ((participant.votingWeight ?? 0) !== 0) {
    throw deny('self-enroll may not grant a vote')
  }

  // The enrollment role is bounded by the entity's existing authority.
  let role = SOParticipantRole.SOParticipantRole_UNKNOWN
  let username = ''
  for (const p of cfg.participants ?? []) {
    if ((p.entityId ?? '') !== entityID) {
      continue
    }
    role = Math.max(role, p.role ?? SOParticipantRole.SOParticipantRole_UNKNOWN)
    username ||= p.username ?? ''
  }
  if (role === SOParticipantRole.SOParticipantRole_UNKNOWN) {
    throw deny('self-enroll entity is not a current participant')
  }
  if (
    (participant.role ?? SOParticipantRole.SOParticipantRole_UNKNOWN) > role
  ) {
    throw deny('self-enroll role escalation is not allowed')
  }

  // The enrolling peer may not rename its entity.
  if ((participant.username ?? '') !== username) {
    throw deny("self-enroll username must match the entity's recorded username")
  }
}
