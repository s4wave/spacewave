import Spacewave.SObject.Host
import Spacewave.SObject.KeyRotation

/-!
# Clean SharedObject re-encryption

Mirrors `core/sobject/reencrypt.go` at Spacewave `1c4b31957`.
Source configuration, root, grants and reader authority are checked separately
from operational queues, which are intentionally not imported. Destination
membership is copied without its source history, and root state starts at one.

Decoding returns plaintext and its sequence, not an admission decision. The
model checks the sequence and size. Random key generation, block transforms,
signatures and encoded byte identities are primitive boundaries. Conformance
uses actual encryption and decryption to connect these identities to Go values.
-/

namespace Spacewave.SObject

/-- PlainRoot retains the decoded sequence and hexadecimal state-data bytes. -/
structure PlainRoot where
  seqno : Nat
  data : String
  deriving DecidableEq, Repr

/-- ReencryptInput separates source state from cryptographic and encoding outcomes. -/
structure ReencryptInput where
  sourceId : String
  destinationId : String
  sourcePeer : String
  destinationPeer : String
  factory : Bool
  source : Option State
  participants : List RotationPeer
  decoded : Option PlainRoot
  key : String
  cryptoOK : Bool
  grantData : List String
  rootData : String
  rootContent : String
  rootDigest : String
  deriving DecidableEq, Repr

/-- Reencrypted carries a fresh state and its decoded data and key capabilities. -/
structure Reencrypted where
  state : State
  plain : PlainRoot
  keys : List KeyGrant
  deriving DecidableEq, Repr

/-- firstReadable mirrors participant.go's first matching participant lookup. -/
def firstReadable (participants : List Participant) (peer : String) : Bool :=
  match participants.find? (·.peer == peer) with
  | none => false
  | some p => p.role ∈ [Role.reader, Role.writer, Role.validator, Role.owner]

/-- reencryptSource validates the source root and each unique grant before reading it. -/
def reencryptSource (source : State) (reader : String) : Bool :=
  source.config.validate && source.root.validate && rootAuthorized source.config source.root &&
    decide (source.grants.map (·.peer)).Nodup && source.grants.all (fun g =>
      g.valid source.config.participants && firstReadable source.config.participants g.peer) &&
    firstReadable source.config.participants reader && source.grants.any (·.peer == reader)

/-- reencryptDestination requires a readable validator or owner to sign the new root. -/
def reencryptDestination (participants : List Participant) (signer : String) : Bool :=
  match participants.find? (·.peer == signer) with
  | none => false
  | some p => p.role == Role.validator || p.role == Role.owner

/-- reencryptedState constructs only destination membership, fresh root and fresh grants. -/
def reencryptedState (input : ReencryptInput) (rotation : Rotation) : State :=
  { config := ⟨input.participants.map (·.participant), 0, "", 0⟩
    root := ⟨input.rootData, input.rootContent, 1, input.rootDigest,
      input.rootContent.length / 2 ≤ 1024 * 1024, input.rootContent != "", [],
      [⟨input.destinationPeer, true⟩]⟩
    grants := rotation.grants.zipIdx.map fun (g, index) =>
      ⟨input.grantData[index]?.getD "", g.peer, true, ⟨input.destinationPeer, true⟩⟩
    invites := []
    ops := []
    queued := []
    rejections := [] }

/-- reencryptState checks source authority and builds a clean destination under fresh material. -/
def reencryptState (input : ReencryptInput) : Option Reencrypted := do
  if input.sourceId = "" || input.destinationId = "" || input.sourcePeer = "" ||
      input.destinationPeer = "" || !input.factory || input.participants.isEmpty ||
      !input.participants.all (·.participant.valid) then none
  else
    let source ← input.source
    if !reencryptSource source input.sourcePeer then none
    else
      let plain ← input.decoded
      if plain.seqno != source.root.seqno || plain.seqno = 0 ||
          plain.data.length / 2 > 10 * 1024 * 1024 ||
          !reencryptDestination (input.participants.map (·.participant)) input.destinationPeer then
        none
      else
        let rotation ← rotateTransformKey input.participants 0 0 input.key input.cryptoOK
        let next := reencryptedState input rotation
        if next.validate then some ⟨next, ⟨1, plain.data⟩, rotation.grants⟩ else none

/-- An accepted clone has authenticated readable source data and a valid fresh destination. -/
theorem reencryptState_spec {input : ReencryptInput} {out : Reencrypted}
    (h : reencryptState input = some out) :
    ∃ source plain rotation, input.source = some source ∧ input.decoded = some plain ∧
      reencryptSource source input.sourcePeer = true ∧
      plain.seqno = source.root.seqno ∧
      reencryptDestination (input.participants.map (·.participant)) input.destinationPeer = true ∧
      rotateTransformKey input.participants 0 0 input.key input.cryptoOK = some rotation ∧
      out.state = reencryptedState input rotation ∧ out.plain = ⟨1, plain.data⟩ ∧
      out.keys = rotation.grants ∧ out.state.validate = true := by
  unfold reencryptState at h
  split at h
  · contradiction
  · cases sourceEq : input.source with
    | none => simp [sourceEq] at h
    | some source =>
      simp only [sourceEq, Option.bind_eq_bind, Option.bind_some] at h
      split at h
      · contradiction
      · rename_i sourceOK
        cases plainEq : input.decoded with
        | none => simp [plainEq] at h
        | some plain =>
          simp only [plainEq, Option.bind_some] at h
          split at h
          · contradiction
          · rename_i plainOK
            cases rotationEq : rotateTransformKey input.participants 0 0 input.key
                input.cryptoOK with
            | none => simp [rotationEq] at h
            | some rotation =>
              simp only [rotationEq, Option.bind_some] at h
              split at h
              · rename_i nextOK
                cases h
                simp only [Bool.or_eq_true, bne_iff_ne, decide_eq_true_eq, not_or] at plainOK
                exact ⟨source, plain, rotation, rfl, rfl, by simpa using sourceOK,
                  by omega, by simpa using plainOK.2, rfl, rfl, rfl, rfl, nextOK⟩
              · contradiction

/-- Re-encryption copies only decoded state data; every destination history starts empty. -/
theorem reencryptState_clean {input : ReencryptInput} {out : Reencrypted}
    (h : reencryptState input = some out) :
    out.state.root.seqno = 1 ∧ out.state.root.nonces = [] ∧
      out.state.config.seqno = 0 ∧ out.state.config.hash = "" ∧
      out.state.ops = [] ∧ out.state.queued = [] ∧ out.state.rejections = [] ∧
      out.state.invites = [] ∧
      ∃ plain, input.decoded = some plain ∧ out.plain = ⟨1, plain.data⟩ := by
  obtain ⟨_, plain, _, _, decoded, _, _, _, _, state, payload, _, _⟩ := reencryptState_spec h
  simp [state, reencryptedState]
  exact ⟨plain, decoded, payload⟩

/-- Every destination reader, and only a destination reader, receives the new transform key. -/
theorem reencryptState_coverage {input : ReencryptInput} {out : Reencrypted}
    (h : reencryptState input = some out) (peer : String) :
    KeyGrant.mk peer input.key ∈ out.keys ↔
      ∃ p ∈ input.participants, p.participant.peer = peer ∧
        p.participant.role ∈ [Role.reader, Role.writer, Role.validator, Role.owner] := by
  obtain ⟨_, _, _, _, _, _, _, _, rotated, _, _, keys, _⟩ := reencryptState_spec h
  rw [keys]
  exact rotateTransformKey_coverage rotated peer

/-- The destination's actual grants have exactly the rotated capability recipients in order. -/
theorem reencryptState_recipients {input : ReencryptInput} {out : Reencrypted}
    (h : reencryptState input = some out) :
    out.state.grants.map (·.peer) = out.keys.map (·.peer) := by
  obtain ⟨_, _, rotation, _, _, _, _, _, _, state, _, keys, _⟩ := reencryptState_spec h
  simp only [state, keys, reencryptedState, List.map_map]
  simpa only [List.map_map, Function.comp_def] using
    congrArg (List.map (fun g : KeyGrant => g.peer)) (List.zipIdx_map_fst 0 rotation.grants)

/-- The destination's first matching signer record must be a validator or owner. -/
theorem reencryptState_signer {input : ReencryptInput} {out : Reencrypted}
    (h : reencryptState input = some out) :
    ∃ p, (input.participants.map (·.participant)).find? (·.peer == input.destinationPeer) = some p ∧
      (p.role = Role.validator ∨ p.role = Role.owner) := by
  obtain ⟨_, _, _, _, _, _, _, signer, _⟩ := reencryptState_spec h
  unfold reencryptDestination at signer
  split at signer
  · contradiction
  · rename_i p found
    exact ⟨p, found, by simpa using signer⟩

/-- Successful re-encryption requires the source peer to be a current readable participant. -/
theorem reencryptState_reader {input : ReencryptInput} {out : Reencrypted}
    (h : reencryptState input = some out) :
    ∃ source, input.source = some source ∧
      firstReadable source.config.participants input.sourcePeer = true ∧
      rootAuthorized source.config source.root = true := by
  obtain ⟨source, _, _, sourceEq, _, authorized, _⟩ := reencryptState_spec h
  simp only [reencryptSource, Bool.and_eq_true] at authorized
  exact ⟨source, sourceEq, authorized.1.2, authorized.1.1.1.1.2⟩

end Spacewave.SObject
