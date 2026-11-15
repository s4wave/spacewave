import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex } from '@noble/hashes/utils.js'

import {
  extractPublicKeyFromPeerID,
  peerIDFromPublicKey,
} from '../../net/peer/id.js'
import { verifySignature } from '../../net/peer/signature.js'
import {
  isSOGroupControl,
  validateSOControl,
  verifySOCommit,
} from './control.js'
import {
  SOCheckpointHead,
  SOConfigChange,
  SOConfigChangeType,
  type SOConfigMerge,
  SOOperationPosition,
  SOParticipantConfig,
  SOParticipantRole,
  type SOSequenceHead,
  SOSequencer,
  SOSkippedOperation,
  SharedObjectConfig,
} from './sobject.pb.js'

// MAX_SKIPPED_OPERATIONS is the most skips a config holds, matching Go
// MaxSkippedOperations.
export const MAX_SKIPPED_OPERATIONS = 8

// MAX_SKIP_REASON_SIZE is the largest owner's note on a skip, in bytes,
// matching Go MaxSkipReasonSize.
export const MAX_SKIP_REASON_SIZE = 256

// SO_CONFIG_CHANGE_SIGNATURE_CONTEXT is the signature context of every control
// record. The signed body binds the object, so the context needs no per-record data.
export const SO_CONFIG_CHANGE_SIGNATURE_CONTEXT = 'sobject config change'

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

  // So do an owner's skips, capped so each config copy stays small.
  try {
    validateSOSkippedOperations(cfg.skippedOperations ?? [])
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

// validateSOSkippedOperations checks that skips holds at most
// MAX_SKIPPED_OPERATIONS real positions, strictly sorted by op hash, each with
// a reason of at most MAX_SKIP_REASON_SIZE bytes, matching Go
// validateSkippedOperations. It throws on violation.
export function validateSOSkippedOperations(
  skips: readonly SOSkippedOperation[],
): void {
  if (skips.length > MAX_SKIPPED_OPERATIONS) {
    throw new Error(
      `skipped_operations holds ${skips.length} entries, more than ${MAX_SKIPPED_OPERATIONS}`,
    )
  }
  for (const [i, skip] of skips.entries()) {
    try {
      validateSOPosition(skip.position)
    } catch (err) {
      throw new Error(`skipped_operations[${i}]: ${(err as Error).message}`, {
        cause: err,
      })
    }
    const reasonSize = new TextEncoder().encode(skip.reason ?? '').length
    if (reasonSize > MAX_SKIP_REASON_SIZE) {
      throw new Error(
        `skipped_operations[${i}]: reason is longer than ${MAX_SKIP_REASON_SIZE} bytes`,
      )
    }
    if (i > 0 && compareBytes(skipHash(skips[i - 1]), skipHash(skip)) >= 0) {
      throw new Error('skipped_operations must be strictly sorted by op_hash')
    }
  }
}

// skipHash returns the op hash a skip names.
function skipHash(skip: SOSkippedOperation): Uint8Array {
  return skip.position?.opHash ?? new Uint8Array()
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
// (stale), then authorization, then a MERGE record's branch, then the shape of
// the result. A SELF_ENROLL_PEER record checks only the configuration shape;
// the caller must authenticate the peer-to-entity binding.
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

  // The held configuration authorizes the record, and a merge carries the
  // config its branch yields.
  await verifyConfigChangeAuthority(entry, current)
  await verifyConfigMerge(sharedObjectID, current, entry)

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

// PEER_CONFIG_CHANGE_TYPES are the change types a merge may absorb, matching
// Go isPeerConfigChange: every type but genesis and self-enrollment, which
// need their own proof.
const PEER_CONFIG_CHANGE_TYPES: ReadonlySet<SOConfigChangeType> = new Set([
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_ADD_INVITE,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_SET_ROSTER,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_SET_SEQUENCER,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_SET_CONTROL,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_MERGE,
  SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_SKIP_OPERATION,
])

// verifyConfigMerge checks that a MERGE record's config is the one its branch
// yields on current, and that only MERGE records carry a branch, matching Go
// verifyConfigMerge.
async function verifyConfigMerge(
  sharedObjectID: string,
  current: SharedObjectConfig,
  entry: SOConfigChange,
): Promise<void> {
  // Only a MERGE record carries a branch.
  if (entry.changeType !== SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_MERGE) {
    if (entry.merge) {
      throw new SOConfigChangeError(
        'invalid',
        'only a merge record may carry a merge branch',
      )
    }
    return
  }

  // Recompute the merged config and require the record to carry it.
  const expected = await mergeConfigBranch(sharedObjectID, current, entry.merge)
  if (!equalSOConfigs(entry.config, expected)) {
    throw new SOConfigChangeError(
      'invalid',
      'merge record config differs from its merged branch',
    )
  }
}

// mergeConfigBranch returns the config merge yields on head, matching Go
// mergeConfigBranch. Each branch record must follow the one before it, and the
// first must follow the fork. A record applies only while every signer owns
// the result, so records decided under group control are dropped.
async function mergeConfigBranch(
  sharedObjectID: string,
  head: SharedObjectConfig,
  merge: SOConfigMerge | undefined,
): Promise<SharedObjectConfig> {
  // An owner signs a merge alone, so the head must be under owner control.
  const invalid = (message: string) =>
    new SOConfigChangeError('invalid', `merge record: ${message}`)
  if (isSOGroupControl(head)) {
    throw invalid('merge requires owner control')
  }
  const fork = merge?.fork
  const branch = merge?.branch ?? []
  if (!fork || branch.length === 0) {
    throw invalid('merge requires a fork and a branch')
  }

  // Apply the fields each record changed, while its signers still own.
  let parent = withConfigChainHead(
    fork.config ?? {},
    fork.configSeqno ?? 0n,
    hashSOConfigChange(fork),
  )
  let result = SharedObjectConfig.clone(head)!
  for (const [i, change] of branch.entries()) {
    if (!PEER_CONFIG_CHANGE_TYPES.has(change.changeType ?? 0)) {
      throw invalid(`branch[${i}]: unsupported merged config change`)
    }
    const next = await verifySOConfigChange(
      sharedObjectID,
      parent,
      change,
    ).catch((err: Error) => {
      throw invalid(`branch[${i}]: ${err.message}`)
    })
    if (signedByOwners(change, result)) {
      const candidate = applyConfigDelta(result, parent, next)
      try {
        validateSOConfig(candidate)
        result = candidate
      } catch {
        // An invalid result drops the record.
      }
    }
    parent = next
  }
  return result
}

// signedByOwners reports whether entry carries signatures and every signer is
// an OWNER of cfg. The caller has verified the signatures.
function signedByOwners(
  entry: SOConfigChange,
  cfg: SharedObjectConfig,
): boolean {
  const sigs = entry.signatures ?? []
  return (
    sigs.length !== 0 &&
    sigs.every((sig) => {
      const signer = sig.pubKey ? peerIDFromPublicKey(sig.pubKey) : null
      return !!signer && isOwnerPeer(cfg, signer)
    })
  )
}

// applyConfigDelta returns target with each field that next changed from
// parent set to its value in next, matching Go applyConfigDelta. Participants
// and removed authors merge by peer ID, skipped operations by op hash, and
// dropped roster peers merge as a set. The chain head fields keep target's
// values.
function applyConfigDelta(
  target: SharedObjectConfig,
  parent: SharedObjectConfig,
  next: SharedObjectConfig,
): SharedObjectConfig {
  // Merge the keyed lists by peer ID.
  const out = SharedObjectConfig.clone(target)!
  const peerID = (item: { peerId?: string }) => item.peerId ?? ''
  out.participants = mergeByKey(
    out.participants ?? [],
    parent.participants ?? [],
    next.participants ?? [],
    peerID,
    (a, b) => SOParticipantConfig.equals(a, b),
    (item) => SOParticipantConfig.clone(item)!,
  )
  out.removedAuthors = mergeByKey(
    out.removedAuthors ?? [],
    parent.removedAuthors ?? [],
    next.removedAuthors ?? [],
    peerID,
    (a, b) => SOOperationPosition.equals(a, b),
    (item) => SOOperationPosition.clone(item)!,
  ).sort((a, b) => compareStrings(peerID(a), peerID(b)))
  out.skippedOperations = mergeByKey(
    out.skippedOperations ?? [],
    parent.skippedOperations ?? [],
    next.skippedOperations ?? [],
    (item) => bytesToHex(skipHash(item)),
    (a, b) => SOSkippedOperation.equals(a, b),
    (item) => SOSkippedOperation.clone(item)!,
  ).sort((a, b) => compareBytes(skipHash(a), skipHash(b)))
  out.rosterDroppedPeerIds = mergeByKey(
    out.rosterDroppedPeerIds ?? [],
    parent.rosterDroppedPeerIds ?? [],
    next.rosterDroppedPeerIds ?? [],
    (id) => id,
    (a, b) => a === b,
    (id) => id,
  ).sort(compareStrings)

  // Take each single field the record changed.
  if (!equalPresent(SOSequencer, parent.sequencer, next.sequencer)) {
    out.sequencer = SOSequencer.clone(next.sequencer) ?? undefined
  }
  if ((parent.control ?? 0) !== (next.control ?? 0)) {
    out.control = next.control
  }
  if (
    !equalPresent(
      SOCheckpointHead,
      parent.sealedCheckpoint,
      next.sealedCheckpoint,
    )
  ) {
    out.sealedCheckpoint =
      SOCheckpointHead.clone(next.sealedCheckpoint) ?? undefined
  }
  return out
}

// mergeByKey applies the change from parent to next onto target, matching
// items by key: an item next added or changed replaces or joins target's, and
// an item next removed leaves target. Target keeps its order; new items follow.
function mergeByKey<T>(
  target: readonly T[],
  parent: readonly T[],
  next: readonly T[],
  key: (item: T) => string,
  equal: (a: T, b: T) => boolean,
  clone: (item: T) => T,
): T[] {
  // Index the parent's and next's items by key.
  const before = new Map(parent.map((item) => [key(item), item]))
  const after = new Set(next.map(key))

  // Drop removed items, then add or replace changed ones.
  const out = target.filter(
    (item) => !before.has(key(item)) || after.has(key(item)),
  )
  for (const item of next) {
    const prev = before.get(key(item))
    if (prev !== undefined && equal(prev, item)) {
      continue
    }
    const i = out.findIndex((existing) => key(existing) === key(item))
    if (i === -1) {
      out.push(clone(item))
    } else {
      out[i] = clone(item)
    }
  }
  return out
}

// equalPresent reports whether a and b are both absent, or both present and
// equal, matching Go EqualVT on message pointers.
function equalPresent<T>(
  type: { equals(a: T | undefined, b: T | undefined): boolean },
  a: T | undefined,
  b: T | undefined,
): boolean {
  return (a === undefined) === (b === undefined) && type.equals(a, b)
}

// equalSOConfigs reports whether two configs are equal independent of
// participant order, matching Go EqualSOConfigs.
function equalSOConfigs(
  a: SharedObjectConfig | undefined,
  b: SharedObjectConfig,
): boolean {
  if (!a) {
    return false
  }
  const sorted = (cfg: SharedObjectConfig): SharedObjectConfig => ({
    ...cfg,
    participants: [...(cfg.participants ?? [])].sort((x, y) =>
      compareStrings(x.peerId ?? '', y.peerId ?? ''),
    ),
  })
  return SharedObjectConfig.equals(sorted(a), sorted(b))
}

// compareBytes orders a and b as Go bytes.Compare does.
function compareBytes(a: Uint8Array, b: Uint8Array): number {
  const n = Math.min(a.length, b.length)
  for (let i = 0; i < n; i++) {
    if (a[i] !== b[i]) {
      return a[i] < b[i] ? -1 : 1
    }
  }
  return a.length - b.length
}

// compareStrings orders strings by code unit, which is byte order for the
// ASCII peer IDs it compares.
function compareStrings(a: string, b: string): number {
  if (a === b) {
    return 0
  }
  return a < b ? -1 : 1
}
