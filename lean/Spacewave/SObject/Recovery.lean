import Spacewave.SObject.State

/-!
# Entity recovery and self-enrollment

Mirrors `core/sobject/recovery.go` at Spacewave `cde42d69c`. Envelope and grant
cryptography, serialization and provider calls are explicit boundaries.
Construction preserves supplied metadata. Configuration admission checks shape
and roles; its caller must authenticate the peer-to-entity relationship.
Optional entity tags follow the resolver's actual checks.

The resolver result distinguishes failure from a successful nil material value.
A successful decoder acquisition supplies a callable provider decoder.
-/

namespace Spacewave.SObject

/-- RecoveryMaterial retains the recovered role, entity and opaque grant plaintext. -/
structure RecoveryMaterial where
  data : String
  entity : String
  role : Role
  grant : Option String
  deriving DecidableEq, Repr

/-- RecoveryEnvelope retains encrypted bytes and the configuration/key-epoch metadata. -/
structure RecoveryEnvelope where
  entity : String
  epoch : Nat
  seqno : Nat
  hash : String
  data : String
  deriving DecidableEq, Repr

/-- buildRecoveryEnvelope validates constructor inputs and copies their exact head metadata. -/
def buildRecoveryEnvelope (entity : String) (epoch : Nat) (config : Option Config)
    (material : Option RecoveryMaterial) (recipients : List String)
    (cryptoOK : Bool) (encoded : String) : Option RecoveryEnvelope := do
  if entity = "" then none
  else
    let config ← config
    let _ ← material
    if recipients.isEmpty || !cryptoOK then none
    else some ⟨entity, epoch, config.seqno, config.hash, encoded⟩

/-- unlockRecovery checks envelope/credential presence before the primitive authenticated decode. -/
def unlockRecovery (keys : List String) (envelope : Option RecoveryEnvelope)
    (decoded : Option RecoveryMaterial) : Option RecoveryMaterial := do
  let envelope ← envelope
  if keys.isEmpty || envelope.data = "" then none else decoded

/-- resolveRecovery binds every nonempty entity tag to the provider account's current entity. -/
def resolveRecovery (featureOK : Bool) (entity : Option String)
    (readOK : Bool) (envelope : Option RecoveryEnvelope) (decoderOK decodeOK : Bool)
    (material : Option RecoveryMaterial) : Option (Option RecoveryMaterial) := do
  if !featureOK then none
  else
    let entity ← entity
    if entity = "" || !readOK then none
    else
      let envelopeEntity := (envelope.map (·.entity)).getD ""
      if envelopeEntity != "" && envelopeEntity != entity then none
      else if !decoderOK then none
      else if !decodeOK then none
      else
        let materialEntity := (material.map (·.entity)).getD ""
        if materialEntity != "" && materialEntity != entity then none
        else some material

/-- buildSelfEnroll constructs a signed proposal without assuming its membership admission. -/
def buildSelfEnroll (current : Option Config) (sig : Option Sig) (peer entity : String)
    (role : Role) (hash : String) (signOK : Bool) : Option Entry := do
  let current ← current
  let sig ← sig
  if peer = "" || entity = "" || role < Role.reader || role > Role.owner || !signOK then none
  else
    let next := {current with participants := current.participants ++ [⟨peer, role, entity⟩]}
    some ⟨expectedSeqno current % seqnoLimit, some next, some sig,
      current.hash, ChangeType.selfEnrollPeer, hash⟩

/-- RecoveryGrant carries actual encrypted grant bytes and their recovered plaintext identity. -/
structure RecoveryGrant where
  grant : Grant
  material : String
  deriving DecidableEq, Repr

/-- buildSelfEnrollGrant delegates only key extraction and grant encryption to primitives. -/
def buildSelfEnrollGrant (signer peer object : String) (material : Option RecoveryMaterial)
    (publicKey cryptoOK : Bool) (encoded inner : String) : Option RecoveryGrant := do
  if signer = "" || peer = "" || object = "" then none
  else
    let material ← material
    let grant ← material.grant
    if !publicKey || !cryptoOK then none
    else some ⟨⟨encoded, peer, inner != "" && inner.length / 2 ≤ 1024 * 1024,
      ⟨signer, true⟩⟩, grant⟩

/-- Accepted envelope construction preserves the supplied configuration head and key epoch. -/
theorem buildRecoveryEnvelope_spec {entity : String} {epoch : Nat} {config : Option Config}
    {material : Option RecoveryMaterial} {recipients : List String}
    {cryptoOK : Bool} {encoded : String} {out : RecoveryEnvelope}
    (h : buildRecoveryEnvelope entity epoch config material recipients cryptoOK encoded = some out) :
    entity ≠ "" ∧ ∃ c m, config = some c ∧ material = some m ∧
      recipients ≠ [] ∧ cryptoOK = true ∧ out = ⟨entity, epoch, c.seqno, c.hash, encoded⟩ := by
  unfold buildRecoveryEnvelope at h
  split at h
  · contradiction
  · rename_i nonempty
    cases config with
    | none => simp at h
    | some c =>
      simp only [Option.bind_eq_bind, Option.bind_some] at h
      cases material with
      | none => simp at h
      | some m =>
        simp only [Option.bind_some] at h
        split at h
        · contradiction
        · rename_i encrypted
          cases h
          simp only [Bool.or_eq_true, Bool.not_eq_true', not_or] at encrypted
          exact ⟨nonempty, c, m, rfl, rfl, by simpa using encrypted.1,
            by simpa using encrypted.2, rfl⟩

/-- Accepted unlocks return exactly authenticated decoded material and require credentials. -/
theorem unlockRecovery_spec {keys : List String} {envelope : Option RecoveryEnvelope}
    {decoded : Option RecoveryMaterial} {out : RecoveryMaterial}
    (h : unlockRecovery keys envelope decoded = some out) :
    keys ≠ [] ∧ decoded = some out ∧ ∃ env, envelope = some env ∧ env.data ≠ "" := by
  unfold unlockRecovery at h
  cases envelope with
  | none => simp at h
  | some env =>
    simp only [Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · contradiction
    · rename_i present
      simp only [Bool.or_eq_true, decide_eq_true_eq, not_or] at present
      exact ⟨by simpa using present.1, h, env, rfl, present.2⟩

/-- Resolution rejects nonempty entity tags that disagree with the selected provider account. -/
theorem resolveRecovery_entities {featureOK readOK decoderOK decodeOK : Bool}
    {entity : Option String} {envelope : Option RecoveryEnvelope}
    {material out : Option RecoveryMaterial}
    (h : resolveRecovery featureOK entity readOK envelope decoderOK decodeOK material = some out) :
    ∃ id, entity = some id ∧ id ≠ "" ∧ out = material ∧
      ((envelope.map (·.entity)).getD "" = "" ∨ (envelope.map (·.entity)).getD "" = id) ∧
      ((out.map (·.entity)).getD "" = "" ∨ (out.map (·.entity)).getD "" = id) := by
  unfold resolveRecovery at h
  split at h
  · contradiction
  · cases entity with
    | none => simp at h
    | some id =>
      simp only [Option.bind_eq_bind, Option.bind_some] at h
      split at h
      · contradiction
      · rename_i present
        split at h
        · contradiction
        · rename_i envelopeOK
          split at h
          · contradiction
          · split at h
            · contradiction
            · split at h
              · contradiction
              · rename_i materialOK
                cases h
                simp only [Bool.or_eq_true, decide_eq_true_eq, not_or] at present
                simp only [Bool.and_eq_true, bne_iff_ne, not_and] at envelopeOK materialOK
                refine ⟨id, rfl, present.1, rfl, ?_, ?_⟩
                · by_cases empty : (envelope.map (·.entity)).getD "" = ""
                  · exact Or.inl empty
                  · exact Or.inr (by simpa using envelopeOK empty)
                · by_cases empty : (material.map (·.entity)).getD "" = ""
                  · exact Or.inl empty
                  · exact Or.inr (by simpa using materialOK empty)

/-- A successful self-enrollment builder appends exactly the requested peer and preserves old records. -/
theorem buildSelfEnroll_spec {current : Option Config} {sig : Option Sig} {peer entity : String}
    {role : Role} {hash : String} {signOK : Bool} {out : Entry}
    (h : buildSelfEnroll current sig peer entity role hash signOK = some out) :
    ∃ c signature, current = some c ∧ sig = some signature ∧ peer ≠ "" ∧ entity ≠ "" ∧
      Role.reader ≤ role ∧ role ≤ Role.owner ∧
      out = ⟨expectedSeqno c % seqnoLimit,
        some {c with participants := c.participants ++ [⟨peer, role, entity⟩]}, some signature,
        c.hash, ChangeType.selfEnrollPeer, hash⟩ := by
  unfold buildSelfEnroll at h
  cases current with
  | none => simp at h
  | some c =>
    simp only [Option.bind_eq_bind, Option.bind_some] at h
    cases sig with
    | none => simp at h
    | some signature =>
      simp only [Option.bind_some] at h
      split at h
      · contradiction
      · rename_i valid
        cases h
        simp only [Bool.or_eq_true, decide_eq_true_eq, not_or] at valid
        exact ⟨c, signature, rfl, rfl, valid.1.1.1.1, valid.1.1.1.2,
          Int.le_of_not_gt valid.1.1.2, Int.le_of_not_gt valid.1.2, rfl⟩

/-- enrollRecovery composes the proposal builder with actual current-configuration admission. -/
def enrollRecovery (current : Config) (sig : Sig) (peer entity : String)
    (role : Role) (hash : String) (signOK : Bool) : Option Config := do
  let entry ← buildSelfEnroll (some current) (some sig) peer entity role hash signOK
  verifyChange current entry

/-- Recovery cannot enroll above the currently admitted entity's strongest role. -/
theorem enrollRecovery_roleBound {current out : Config} {sig : Sig} {peer entity : String}
    {role : Role} {hash : String} {signOK : Bool}
    (h : enrollRecovery current sig peer entity role hash signOK = some out) :
    sig.valid = true ∧ sig.signer = peer ∧ entity ≠ "" ∧
      entityRole current entity ≠ Role.unknown ∧ role ≤ entityRole current entity ∧
      out.participants = current.participants ++ [⟨peer, role, entity⟩] ∧
      Role.reader ≤ role ∧ role ≤ Role.owner := by
  unfold enrollRecovery at h
  cases built : buildSelfEnroll (some current) (some sig) peer entity role hash signOK with
  | none => simp [built] at h
  | some entry =>
    simp only [built, Option.bind_eq_bind, Option.bind_some] at h
    obtain ⟨c, signature, ceq, seq, _, nonempty, lower, upper, shape⟩ := buildSelfEnroll_spec built
    cases ceq; cases seq; subst entry
    obtain ⟨next, nextEq, stateEq, _, _, _, signed, _⟩ := verifyChange_spec h
    simp only [Option.some.injEq] at nextEq
    subst next
    simp only [verifySignature, ChangeType.selfEnrollPeer, beq_self_eq_true, ↓reduceIte,
      Bool.and_eq_true] at signed
    obtain ⟨added, member, signer, entityKnown, ceiling, _⟩ := validateSelfEnroll_added signed.2
    have absent : current.participants.all (·.peer != sig.signer) = true := by
      have checked := signed.2
      simp only [validateSelfEnroll, Bool.and_eq_true] at checked
      exact checked.1.2
    rcases List.mem_append.mp member with old | appended
    · have unequal := List.all_eq_true.mp absent added old
      simp [signer] at unequal
    · have equal := List.mem_singleton.mp appended
      subst added
      exact ⟨signed.1, signer.symm, nonempty, entityKnown, ceiling,
        by simp [stateEq, Config.withHead], lower, upper⟩

/-- A built recovery grant encrypts exactly the recovered grant material for its named recipient. -/
theorem buildSelfEnrollGrant_spec {signer peer object encoded inner : String}
    {material : Option RecoveryMaterial} {publicKey cryptoOK : Bool} {out : RecoveryGrant}
    (h : buildSelfEnrollGrant signer peer object material publicKey cryptoOK encoded inner = some out) :
    signer ≠ "" ∧ ∃ m plain, material = some m ∧ m.grant = some plain ∧
      out.material = plain ∧ out.grant.peer = peer ∧ out.grant.sig = ⟨signer, true⟩ := by
  unfold buildSelfEnrollGrant at h
  split at h
  · contradiction
  · rename_i args
    cases material with
    | none => simp at h
    | some m =>
      simp only [Option.bind_eq_bind, Option.bind_some] at h
      cases plain : m.grant with
      | none => simp [plain] at h
      | some data =>
        simp only [plain, Option.bind_some] at h
        split at h
        · contradiction
        · cases h
          simp only [Bool.or_eq_true, decide_eq_true_eq, not_or] at args
          exact ⟨args.1.1, m, data, rfl, plain, rfl, rfl, rfl⟩

/-- A same-peer recovery grant is authorized after the bounded enrollment is admitted. -/
theorem recoveryGrant_admitted {current admitted : Config} {sig : Sig}
    {peer entity hash object encoded inner : String} {role : Role} {signOK publicKey cryptoOK : Bool}
    {material : Option RecoveryMaterial} {out : RecoveryGrant}
    (enrolled : enrollRecovery current sig peer entity role hash signOK = some admitted)
    (built : buildSelfEnrollGrant sig.signer peer object material publicKey cryptoOK encoded inner = some out)
    (format : out.grant.format = true) : out.grant.valid admitted.participants = true := by
  obtain ⟨_, signer, _, _, _, audience, lower, upper⟩ := enrollRecovery_roleBound enrolled
  obtain ⟨nonempty, _, _, _, _, _, recipient, signature⟩ := buildSelfEnrollGrant_spec built
  have member : Participant.mk peer role entity ∈ admitted.participants := by
    rw [audience]
    simp
  have readable : role ∈ [Role.reader, Role.writer, Role.validator, Role.owner] := by
    simp only [Role.reader, Role.writer, Role.validator, Role.owner] at lower upper ⊢
    simp only [List.mem_cons, List.not_mem_nil, or_false]
    change (1 : Int) ≤ role at lower
    change role ≤ (4 : Int) at upper
    change role = (1 : Int) ∨ role = 2 ∨ role = 3 ∨ role = 4
    rcases Int.le_iff_eq_or_lt.mp lower with one | greater
    · exact Or.inl one.symm
    · have lowerTwo : (2 : Int) ≤ role := Int.add_one_le_of_lt greater
      rcases Int.le_iff_eq_or_lt.mp lowerTwo with two | greater
      · exact Or.inr (Or.inl two.symm)
      · have lowerThree : (3 : Int) ≤ role := Int.add_one_le_of_lt greater
        rcases Int.le_iff_eq_or_lt.mp lowerThree with three | greater
        · exact Or.inr (Or.inr (Or.inl three.symm))
        · exact Or.inr (Or.inr (Or.inr (Int.le_antisymm upper (Int.add_one_le_of_lt greater))))
  simp only [Grant.valid, format, signature, recipient]
  simp only [Bool.true_and, Bool.and_eq_true, bne_iff_ne]
  refine ⟨nonempty, List.any_eq_true.mpr ⟨⟨peer, role, entity⟩, member, ?_⟩⟩
  simp [signer, readable]

/-- Caller-authenticated entity ownership composes with checked enrollment and grant admission. -/
theorem recoveryGrant_withEntityBinding {current admitted : Config} {sig : Sig}
    {peer entity hash object encoded inner : String} {role : Role} {signOK publicKey cryptoOK : Bool}
    {material : Option RecoveryMaterial} {out : RecoveryGrant}
    (authenticated : String → String → Prop)
    (binding : authenticated sig.signer entity)
    (enrolled : enrollRecovery current sig peer entity role hash signOK = some admitted)
    (built : buildSelfEnrollGrant sig.signer peer object material publicKey cryptoOK encoded inner = some out)
    (format : out.grant.format = true) :
    authenticated peer entity ∧ role ≤ entityRole current entity ∧
      out.grant.valid admitted.participants = true := by
  have checked := enrollRecovery_roleBound enrolled
  exact ⟨by simpa [checked.2.1] using binding, checked.2.2.2.2.1,
    recoveryGrant_admitted enrolled built format⟩

end Spacewave.SObject
