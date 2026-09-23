import Spacewave.SObject.Host
import Spacewave.SObject.KeyRotation

/-!
# Participant removal

Mirrors `core/sobject/remove-participant.go` at Spacewave `79e5a444d`.
Removal filters recipients and replaces proofs from removed signers under the
existing host lock. It does not rotate the content key. Future-epoch exclusion
composes removal's audience with KeyRotation's recipient theorem.

Public-key extraction, decryption, encryption, signing and serialized output
identities are explicit primitive inputs. Their success does not include role,
recipient, configuration or consensus admission. Rewrapping preserves the old
material under the encryption contract; conformance decrypts actual Go grants.
-/

namespace Spacewave.SObject

/-- removalTarget mirrors the nonempty peer-ID set used by Go. -/
def removalTarget (targets : List String) (peer : String) : Bool :=
  peer != "" && targets.contains peer

/-- removalConfig preserves configuration fields while filtering the audience. -/
def removalConfig (config : Config) (targets : List String) : Config :=
  {config with participants := config.participants.filter fun p => !removalTarget targets p.peer}

/-- RewrapInput supplies public-key extraction and a fresh encrypted serialization. -/
structure RewrapInput where
  publicKey : Bool
  encryptOK : Bool
  data : String
  deriving DecidableEq, Repr, Inhabited

/-- RemovalCrypto carries primitive outcomes for the retained grants and root. -/
structure RemovalCrypto where
  decrypted : Bool
  wraps : List RewrapInput
  rootSignOK : Bool
  rootData : String
  rootFormat : Bool
  deriving DecidableEq, Repr

/-- rewrapGrant preserves a retained recipient and authenticates any replacement proof. -/
def rewrapGrant (oldConfig nextConfig : Config) (targets : List String)
    (signer : String) (own : Option Grant) (decrypted : Bool)
    (input : RewrapInput) (grant : Grant) : Option Grant := do
  if grant.sig.signer = "" then none
  else if !removalTarget targets grant.sig.signer then some grant
  else
    let own ← own
    if !(own.valid oldConfig.participants) || !decrypted || !input.publicKey ||
        !input.encryptOK then none
    else
      let next : Grant := ⟨input.data, grant.peer, true, ⟨signer, true⟩⟩
      if next.valid nextConfig.participants then some next else none

/-- rewrapGrants checks each retained proof in order, reusing the same decrypted material. -/
def rewrapGrants (oldConfig nextConfig : Config) (targets : List String)
    (signer : String) (own : Option Grant) (decrypted : Bool) :
    List RewrapInput → List Grant → Option (List Grant)
  | _, [] => some []
  | inputs, grant :: rest => do
    let next ← rewrapGrant oldConfig nextConfig targets signer own decrypted
      (inputs.head?.getD default) grant
    let tail ← rewrapGrants oldConfig nextConfig targets signer own decrypted inputs.tail rest
    some (next :: tail)

/-- removalRoot retains surviving proofs or signs the unchanged content once. -/
def removalRoot (config : Config) (targets : List String) (signer : String)
    (crypto : RemovalCrypto) (root : Root) : Option Root :=
  if !root.hasInner then some root
  else if root.sigs.any (·.signer == "") then none
  else
    let retained := root.sigs.filter fun sig => !removalTarget targets sig.signer
    if retained.isEmpty && !crypto.rootSignOK then none
    else
      let next := {root with
        sigs := if retained.isEmpty then [⟨signer, true⟩] else retained
        data := crypto.rootData
        format := crypto.rootFormat}
      if rootAuthorized config next then some next else none

/-- removalCallback owns grant/proof edits and leaves the verified configuration intact. -/
def removalCallback (oldConfig nextConfig : Config) (targets : List String)
    (signer : String) (crypto : RemovalCrypto) (s : State) : Option State := do
  if signer = "" then none
  else
    let retained := s.grants.filter fun g => !removalTarget targets g.peer
    let own := retained.find? (·.peer == signer)
    let grants ← rewrapGrants oldConfig nextConfig targets signer own crypto.decrypted
      crypto.wraps retained
    let root ← removalRoot nextConfig targets signer crypto s.root
    some {s with grants, root}

/-- RemovalResult includes no-op results and the exact ordered removed audience. -/
structure RemovalResult where
  removed : List String
  outcome : HostResult
  deriving DecidableEq, Repr

/-- removalEntry binds the filtered audience to the watched configuration head. -/
def removalEntry (config : Config) (targets : List String) (sig : Sig) (hash : String) : Entry :=
  ⟨expectedSeqno config % seqnoLimit, some (removalConfig config targets),
    some sig, config.hash, ChangeType.removeParticipant, hash⟩

/-- removeParticipants follows the watched audience, signed change, callback and publication. -/
def removeParticipants (previous : State) (snapshot : Option Config) (targets : List String)
    (sig : Sig) (hash : String) (crypto : RemovalCrypto)
    (watchOK buildOK lockOK writeOK : Bool) : Option RemovalResult := do
  let noop : RemovalResult := ⟨[], ⟨previous, false, false⟩⟩
  if !(targets.any (· != "")) then some noop
  else if !watchOK then none
  else match snapshot with
  | none => some noop
  | some oldConfig =>
    let removed := (oldConfig.participants.filter
      fun p => removalTarget targets p.peer).map (·.peer)
    if removed.isEmpty then some noop
    else if !buildOK then none
    else
      let nextConfig := removalConfig oldConfig targets
      let entry := removalEntry oldConfig targets sig hash
      let outcome ← applyConfigChange previous (some entry)
        (removalCallback oldConfig nextConfig targets sig.signer crypto) lockOK writeOK
      some ⟨removed, outcome⟩

/-- The removal audience contains exactly old participants not targeted for removal. -/
theorem removalConfig_members {config : Config} {targets : List String} {p : Participant} :
    p ∈ (removalConfig config targets).participants ↔
      p ∈ config.participants ∧ removalTarget targets p.peer = false := by
  simp [removalConfig]

/-- A subsequent rotation cannot grant its new key to a removed peer. -/
theorem removal_rotation_excludes {config : Config} {targets : List String}
    {peers : List RotationPeer} {epoch seqno : Nat} {key : String} {cryptoOK : Bool}
    {out : Rotation} {peer : String}
    (audience : peers.map (·.participant) = (removalConfig config targets).participants)
    (target : removalTarget targets peer = true)
    (h : rotateTransformKey peers epoch seqno key cryptoOK = some out) :
    ∀ grant ∈ out.grants, grant.peer ≠ peer := by
  apply rotateTransformKey_excludes ?_ h
  intro p present equal
  have configured : p.participant ∈ (removalConfig config targets).participants := by
    rw [← audience]
    exact List.mem_map.mpr ⟨p, present, rfl⟩
  have excluded := (removalConfig_members.mp configured).2
  simp [equal, target] at excluded

/-- Every remaining readable participant gets the subsequent rotation's new key. -/
theorem removal_rotation_coverage {config : Config} {targets : List String}
    {peers : List RotationPeer} {epoch seqno : Nat} {key : String} {cryptoOK : Bool}
    {out : Rotation} {p : Participant}
    (audience : peers.map (·.participant) = (removalConfig config targets).participants)
    (member : p ∈ config.participants) (retained : removalTarget targets p.peer = false)
    (readable : p.role ∈ [Role.reader, Role.writer, Role.validator, Role.owner])
    (h : rotateTransformKey peers epoch seqno key cryptoOK = some out) :
    KeyGrant.mk p.peer key ∈ out.grants := by
  have configured := removalConfig_members.mpr ⟨member, retained⟩
  rw [← audience] at configured
  obtain ⟨peer, present, equal⟩ := List.mem_map.mp configured
  apply (rotateTransformKey_coverage h p.peer).mpr
  exact ⟨peer, present, by simp [equal], by simpa [equal] using readable⟩

/-- Rewrapping cannot redirect a retained grant to another peer. -/
theorem rewrapGrant_recipient {oldConfig nextConfig : Config} {targets : List String}
    {signer : String} {own : Option Grant} {decrypted : Bool} {input : RewrapInput}
    {grant out : Grant}
    (h : rewrapGrant oldConfig nextConfig targets signer own decrypted input grant = some out) :
    out.peer = grant.peer := by
  unfold rewrapGrant at h
  split at h
  · contradiction
  · split at h
    · cases h; rfl
    · cases own with
      | none => simp at h
      | some g =>
        simp only [Option.bind_eq_bind, Option.bind_some] at h
        split at h
        · contradiction
        · split at h
          · cases h; rfl
          · contradiction

/-- Successful rewrapping preserves the full ordered recipient list. -/
theorem rewrapGrants_recipients {oldConfig nextConfig : Config} {targets : List String}
    {signer : String} {own : Option Grant} {decrypted : Bool} {inputs : List RewrapInput}
    {grants out : List Grant}
    (h : rewrapGrants oldConfig nextConfig targets signer own decrypted inputs grants = some out) :
    out.map (·.peer) = grants.map (·.peer) := by
  induction grants generalizing inputs out with
  | nil => simp [rewrapGrants] at h; subst out; rfl
  | cons grant rest ih =>
    simp only [rewrapGrants, Option.bind_eq_bind] at h
    cases changed : rewrapGrant oldConfig nextConfig targets signer own decrypted
        (inputs.head?.getD default) grant with
    | none => simp [changed] at h
    | some next =>
      simp only [changed, Option.bind_some] at h
      cases tail : rewrapGrants oldConfig nextConfig targets signer own decrypted
          inputs.tail rest with
      | none => simp [tail] at h
      | some after =>
        simp [tail] at h
        subst out
        simp [rewrapGrant_recipient changed, ih tail]

/-- Root proof replacement preserves content, sequence and the committed nonce map. -/
theorem removalRoot_content {config : Config} {targets : List String} {signer : String}
    {crypto : RemovalCrypto} {root out : Root}
    (h : removalRoot config targets signer crypto root = some out) :
    out.content = root.content ∧ out.seqno = root.seqno ∧ out.nonces = root.nonces ∧
      (root.hasInner = true → rootAuthorized config out = true) := by
  unfold removalRoot at h
  dsimp only at h
  split at h
  · rename_i empty
    cases h
    exact ⟨rfl, rfl, rfl, by simp_all⟩
  · split at h
    · contradiction
    · split at h
      · contradiction
      · split at h
        · split at h
          · rename_i authorized
            cases h
            exact ⟨rfl, rfl, rfl, fun _ => authorized⟩
          · contradiction
        · split at h
          · rename_i authorized
            cases h
            exact ⟨rfl, rfl, rfl, fun _ => authorized⟩
          · contradiction

/-- A callback removes precisely targeted grant recipients and preserves authority and content. -/
theorem removalCallback_spec {oldConfig nextConfig : Config} {targets : List String}
    {signer : String} {crypto : RemovalCrypto} {s out : State}
    (h : removalCallback oldConfig nextConfig targets signer crypto s = some out) :
    out.config = s.config ∧
      out.grants.map (·.peer) =
        (s.grants.filter fun g => !removalTarget targets g.peer).map (·.peer) ∧
      out.root.content = s.root.content ∧ out.root.seqno = s.root.seqno ∧
      out.root.nonces = s.root.nonces := by
  unfold removalCallback at h
  split at h
  · contradiction
  · cases wraps : rewrapGrants oldConfig nextConfig targets signer
        ((s.grants.filter fun g => !removalTarget targets g.peer).find? (·.peer == signer))
        crypto.decrypted crypto.wraps (s.grants.filter fun g => !removalTarget targets g.peer) with
    | none => simp only [Option.bind_eq_bind, wraps, Option.bind_none] at h; cases h
    | some grants =>
      simp only [Option.bind_eq_bind, wraps, Option.bind_some] at h
      cases root : removalRoot nextConfig targets signer crypto s.root with
      | none => simp [root] at h
      | some next =>
        simp [root] at h
        subst out
        obtain ⟨content, seqno, nonces, _⟩ := removalRoot_content root
        exact ⟨rfl, rewrapGrants_recipients wraps, content, seqno, nonces⟩

/-- Every published removal passes the signed host transition; no-op results never publish. -/
theorem removeParticipants_published {previous : State} {snapshot : Option Config}
    {targets : List String} {sig : Sig} {hash : String} {crypto : RemovalCrypto}
    {watchOK buildOK lockOK writeOK : Bool} {out : RemovalResult}
    (h : removeParticipants previous snapshot targets sig hash crypto
      watchOK buildOK lockOK writeOK = some out)
    (wrote : out.outcome.wrote = true) :
    ∃ config, snapshot = some config ∧
      applyConfigChange previous (some (removalEntry config targets sig hash))
        (removalCallback config (removalConfig config targets) targets sig.signer crypto)
        lockOK writeOK = some out.outcome := by
  unfold removeParticipants at h
  dsimp only at h
  split at h
  · cases h; contradiction
  · split at h
    · contradiction
    · cases snapshot with
      | none => simp at h; subst out; contradiction
      | some config =>
        dsimp only at h
        split at h
        · cases h; contradiction
        · split at h
          · contradiction
          · cases applied : applyConfigChange previous
                (some (removalEntry config targets sig hash))
                (removalCallback config (removalConfig config targets) targets sig.signer crypto)
                lockOK writeOK with
            | none => simp only [Option.bind_eq_bind, applied, Option.bind_none] at h; cases h
            | some result =>
              simp only [Option.bind_eq_bind, applied, Option.bind_some, Option.some.injEq] at h
              subst out
              exact ⟨config, rfl, applied⟩

/-- Published removal requires the held owner and preserves the filtered audience. -/
theorem removeParticipants_authorized {previous : State} {snapshot : Option Config}
    {targets : List String} {sig : Sig} {hash : String} {crypto : RemovalCrypto}
    {watchOK buildOK lockOK writeOK : Bool} {out : RemovalResult}
    (h : removeParticipants previous snapshot targets sig hash crypto
      watchOK buildOK lockOK writeOK = some out)
    (wrote : out.outcome.wrote = true) :
    sig.valid = true ∧ isOwner previous.config sig.signer = true ∧
      ∃ config, snapshot = some config ∧
        out.outcome.state.config.participants = (removalConfig config targets).participants := by
  obtain ⟨config, snapshotEq, applied⟩ := removeParticipants_published h wrote
  obtain ⟨entry, equal, verified⟩ := applyConfigChange_authorized
    (fun _ _ accepted => (removalCallback_spec accepted).1) applied
  cases equal
  obtain ⟨signed, present, valid, owner⟩ := verifyChange_authorized verified
  simp only [removalEntry, Option.some.injEq] at present
  subst signed
  obtain ⟨next, nextEq, shape, _⟩ := verifyChange_spec verified
  simp only [removalEntry, Option.some.injEq] at nextEq
  subst next
  exact ⟨valid,
    owner (by simp [removalEntry, ChangeType.removeParticipant, ChangeType.selfEnrollPeer]),
    config, snapshotEq, by simp [shape, Config.withHead]⟩

/-- Published removal preserves the accepted root and excludes targeted grant recipients. -/
theorem removeParticipants_state {previous : State} {snapshot : Option Config}
    {targets : List String} {sig : Sig} {hash : String} {crypto : RemovalCrypto}
    {watchOK buildOK lockOK writeOK : Bool} {out : RemovalResult}
    (h : removeParticipants previous snapshot targets sig hash crypto
      watchOK buildOK lockOK writeOK = some out)
    (wrote : out.outcome.wrote = true) :
    out.outcome.state.root.content = previous.root.content ∧
      out.outcome.state.root.seqno = previous.root.seqno ∧
      out.outcome.state.root.nonces = previous.root.nonces ∧
      ∀ grant ∈ out.outcome.state.grants, removalTarget targets grant.peer = false := by
  obtain ⟨_, _, applied⟩ := removeParticipants_published h wrote
  obtain ⟨_, _, _, _, called, _, _⟩ := applyConfigChange_spec applied
  obtain ⟨_, recipients, content, seqno, nonces⟩ := removalCallback_spec called
  refine ⟨content, seqno, nonces, ?_⟩
  intro grant member
  have present : grant.peer ∈ out.outcome.state.grants.map (·.peer) :=
    List.mem_map.mpr ⟨grant, member, rfl⟩
  rw [recipients] at present
  obtain ⟨original, retained, equal⟩ := List.mem_map.mp present
  have excluded : removalTarget targets original.peer = false := by
    simpa using (List.mem_filter.mp retained).2
  simpa [equal] using excluded

end Spacewave.SObject
