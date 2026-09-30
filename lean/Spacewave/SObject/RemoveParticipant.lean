import Spacewave.SObject.Host
import Spacewave.SObject.KeyRotation

/-!
# Participant removal

Mirrors `core/sobject/remove-participant.go` at Spacewave `c9eaf16de`.
Removal filters recipients and replaces proofs from removed signers under the
existing host lock. Pending operations and rejections that no longer verify
under the next audience are dropped, so the validator can still admit the
resulting state. It does not rotate the content key. Future-epoch exclusion
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

/-- retainOperation mirrors SOOperation.ValidateSignature against the next audience. -/
def retainOperation (config : Config) (o : Operation) : Bool :=
  o.parsed && o.peer == o.sig.signer &&
    o.signedBy config.participants [Role.writer, Role.validator, Role.owner]

/-- retainRejection mirrors SOOperationRejection.ValidateSignature against the next audience. -/
def retainRejection (config : Config) (r : Operation) : Bool :=
  r.innerValid && r.signedBy config.participants [Role.validator, Role.owner]

/-- pruneRejections drops unverifiable rejections and the groups they leave empty. -/
def pruneRejections (config : Config) (groups : List Rejections) : List Rejections :=
  (groups.map fun g => {g with entries := g.entries.filter (retainRejection config)}).filter
    (!·.entries.isEmpty)

/-- removalRoot retains surviving proofs or signs the unchanged content once. -/
def removalRoot (config : Config) (targets : List String) (signer : String)
    (crypto : RemovalCrypto) (root : Root) : Option Root :=
  if !root.hasInner || config.participants.isEmpty then some root
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

/--
pruneRemovedParticipants mirrors the Go function of the same name. The state
already holds the next configuration; oldConfig authenticates the signer's own
grant. Pending operations, rejections, grants and the root must remain valid
under the next configuration.
-/
def pruneRemovedParticipants (oldConfig : Config) (targets : List String)
    (signer : String) (crypto : RemovalCrypto) (s : State) : Option State := do
  if signer = "" then none
  else
    let retained := s.grants.filter fun g => !removalTarget targets g.peer
    let own := retained.find? (·.peer == signer)
    let grants ← rewrapGrants oldConfig s.config targets signer own crypto.decrypted
      crypto.wraps retained
    let root ← removalRoot s.config targets signer crypto s.root
    some {s with
      ops := s.ops.filter (retainOperation s.config)
      rejections := pruneRejections s.config s.rejections
      grants, root}

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
      let entry := removalEntry oldConfig targets sig hash
      let outcome ← applyConfigChange previous (some entry)
        (pruneRemovedParticipants oldConfig targets sig.signer crypto) lockOK writeOK
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
      (root.hasInner = true → config.participants ≠ [] →
        rootAuthorized config out = true) := by
  unfold removalRoot at h
  dsimp only at h
  split at h
  · rename_i empty
    cases h
    refine ⟨rfl, rfl, rfl, fun inner nonempty => ?_⟩
    simp_all
  · split at h
    · contradiction
    · split at h
      · contradiction
      · split at h
        · split at h
          · rename_i authorized
            cases h
            exact ⟨rfl, rfl, rfl, fun _ _ => authorized⟩
          · contradiction
        · split at h
          · rename_i authorized
            cases h
            exact ⟨rfl, rfl, rfl, fun _ _ => authorized⟩
          · contradiction

/-- Pruning removes precisely targeted grant recipients and preserves authority and content. -/
theorem pruneRemovedParticipants_spec {oldConfig : Config} {targets : List String}
    {signer : String} {crypto : RemovalCrypto} {s out : State}
    (h : pruneRemovedParticipants oldConfig targets signer crypto s = some out) :
    out.config = s.config ∧
      out.grants.map (·.peer) =
        (s.grants.filter fun g => !removalTarget targets g.peer).map (·.peer) ∧
      out.root.content = s.root.content ∧ out.root.seqno = s.root.seqno ∧
      out.root.nonces = s.root.nonces := by
  unfold pruneRemovedParticipants at h
  split at h
  · contradiction
  · cases wraps : rewrapGrants oldConfig s.config targets signer
        ((s.grants.filter fun g => !removalTarget targets g.peer).find? (·.peer == signer))
        crypto.decrypted crypto.wraps (s.grants.filter fun g => !removalTarget targets g.peer) with
    | none => simp only [Option.bind_eq_bind, wraps, Option.bind_none] at h; cases h
    | some grants =>
      simp only [Option.bind_eq_bind, wraps, Option.bind_some] at h
      cases root : removalRoot s.config targets signer crypto s.root with
      | none => simp [root] at h
      | some next =>
        simp [root] at h
        subst out
        obtain ⟨content, seqno, nonces, _⟩ := removalRoot_content root
        exact ⟨rfl, rewrapGrants_recipients wraps, content, seqno, nonces⟩

/-- Pruning keeps exactly the pending proofs that verify under the next audience. -/
theorem pruneRemovedParticipants_pending_eq {oldConfig : Config} {targets : List String}
    {signer : String} {crypto : RemovalCrypto} {s out : State}
    (h : pruneRemovedParticipants oldConfig targets signer crypto s = some out) :
    out.ops = s.ops.filter (retainOperation s.config) ∧
      out.rejections = pruneRejections s.config s.rejections := by
  unfold pruneRemovedParticipants at h
  split at h
  · contradiction
  · cases wraps : rewrapGrants oldConfig s.config targets signer
        ((s.grants.filter fun g => !removalTarget targets g.peer).find? (·.peer == signer))
        crypto.decrypted crypto.wraps (s.grants.filter fun g => !removalTarget targets g.peer) with
    | none => simp only [Option.bind_eq_bind, wraps, Option.bind_none] at h; cases h
    | some grants =>
      simp only [Option.bind_eq_bind, wraps, Option.bind_some] at h
      cases root : removalRoot s.config targets signer crypto s.root with
      | none => simp [root] at h
      | some next =>
        simp [root] at h
        subst out
        exact ⟨rfl, rfl⟩

/-- Pending operations and rejections that were well formed remain admissible after pruning,
so a removal cannot leave validation wedged on proofs from departed signers. -/
theorem pruneRemovedParticipants_pending {oldConfig : Config} {targets : List String}
    {signer : String} {crypto : RemovalCrypto} {s out : State}
    (h : pruneRemovedParticipants oldConfig targets signer crypto s = some out)
    (ops : s.ops.all (·.valid oldConfig)) (rejections : s.rejections.all (·.valid oldConfig)) :
    out.ops.all (·.valid out.config) ∧ out.rejections.all (·.valid out.config) := by
  obtain ⟨opsEq, rejectionsEq⟩ := pruneRemovedParticipants_pending_eq h
  rw [(pruneRemovedParticipants_spec h).1, opsEq, rejectionsEq]
  constructor
  · simp only [List.all_eq_true, List.mem_filter]
    intro o ⟨member, keep⟩
    have old := List.all_eq_true.mp ops o member
    simp only [Operation.valid, retainOperation, Bool.and_eq_true] at old keep ⊢
    exact ⟨old.1, keep.2, keep.1.2⟩
  · simp only [pruneRejections, List.all_eq_true, List.mem_filter, List.mem_map]
    rintro _ ⟨⟨g, member, rfl⟩, _⟩
    have old := List.all_eq_true.mp rejections g member
    simp only [Rejections.valid, Bool.and_eq_true, List.all_eq_true, List.mem_filter,
      decide_eq_true_eq] at old ⊢
    refine ⟨⟨⟨old.1.1.1, fun r ⟨entry, keep⟩ => ?_⟩, ?_⟩, ?_⟩
    · have := old.1.1.2 r entry
      simp only [retainRejection, Bool.and_eq_true] at keep this ⊢
      exact ⟨this.1, keep.2⟩
    · exact (List.filter_sublist.map _).nodup old.1.2
    · exact (List.filter_sublist.map _).nodup old.2

/-- Every published removal passes the signed host transition; no-op results never publish. -/
theorem removeParticipants_published {previous : State} {snapshot : Option Config}
    {targets : List String} {sig : Sig} {hash : String} {crypto : RemovalCrypto}
    {watchOK buildOK lockOK writeOK : Bool} {out : RemovalResult}
    (h : removeParticipants previous snapshot targets sig hash crypto
      watchOK buildOK lockOK writeOK = some out)
    (wrote : out.outcome.wrote = true) :
    ∃ config, snapshot = some config ∧
      applyConfigChange previous (some (removalEntry config targets sig hash))
        (pruneRemovedParticipants config targets sig.signer crypto)
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
                (pruneRemovedParticipants config targets sig.signer crypto)
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
    (fun _ _ accepted => (pruneRemovedParticipants_spec accepted).1) applied
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
  obtain ⟨_, recipients, content, seqno, nonces⟩ := pruneRemovedParticipants_spec called
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
