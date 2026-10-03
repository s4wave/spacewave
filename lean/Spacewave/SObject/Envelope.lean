import Spacewave.SObject.ConfigChain

/-!
# Operation and checkpoint envelopes

Models the envelope checks a member runs before an operation or a checkpoint
enters its state, checked against Spacewave `fc01bb918`:

- `verifyOperation` mirrors `SOOperation.Verify` with `SOOperation.Validate`,
  `SOOperationInner.Validate` and `validateLinks` in `core/sobject`.
- `authorizeOperation` mirrors `SOOperation.ValidateSignature`.
- `verifyCheckpoint` mirrors `SOCheckpoint.Verify` with
  `SOCheckpointInner.Validate`.
- `authorizeCheckpoint` mirrors `SOCheckpoint.ValidateAuthority`.

Abstraction. As in the configuration chain, a peer ID that fails to parse
projects to the empty string, hashes are lowercase hex, and a signature is its
signer, empty when its key is missing or does not parse, and whether it
verified over the encoded body. Byte fields enter as their lengths. A body that
fails to decode is none. `sigFormat` is the outcome of `Signature.Validate`,
and `localID` whether the local ID parses as a ULID. Lowercase hex of equal
length sorts as its bytes do, so hash order is string order.
-/

namespace Spacewave.SObject

/-- maxInnerBytes mirrors Go `MaxInnerDataSize`. -/
def maxInnerBytes : Nat := 1024 * 1024

/-- maxStateBytes mirrors Go `MaxStateDataSize`. -/
def maxStateBytes : Nat := 10 * 1024 * 1024

/-- maxParents mirrors Go `MaxSOOperationParents`. -/
def maxParents : Nat := 256

/-- operationVersion mirrors Go `SOOperationProtocolVersion`. -/
def operationVersion : Nat := 1

/-- supportedReplayVersion mirrors Go `SOReplayVersion`. -/
def supportedReplayVersion : Nat := 1

/-- Position is one `SOOperationPosition`: an author, a nonce and a hash. -/
structure Position where
  peer : String
  nonce : Nat
  hash : String
  deriving DecidableEq, Repr

/-- Position.valid reports whether a position names a real operation. -/
def Position.valid (p : Position) : Bool :=
  p.peer != "" && p.nonce > 0 && p.hash.length == digestLength

/-- OpBody is a decoded `SOOperationInner`. -/
structure OpBody where
  object : String
  version : Nat
  peer : String
  localID : Bool
  nonce : Nat
  dataBytes : Nat
  prev : String
  parents : List Position
  configHash : String
  deriving DecidableEq, Repr

/-- Operation is one `SOOperation`. -/
structure Operation where
  innerBytes : Nat
  sigFormat : Bool
  body : Option OpBody
  sig : Sig
  deriving DecidableEq, Repr

/--
OpBody.linksValid mirrors `validateLinks`: this protocol and an object, a
previous operation exactly when the nonce is above one, and bounded parents
that name real operations in strict hash order, never the previous one, under a
named configuration.
-/
def OpBody.linksValid (b : OpBody) : Bool :=
  b.version == operationVersion && b.object != "" &&
    (if b.nonce == 1 then b.prev == "" else b.prev.length == digestLength) &&
    b.parents.length ≤ maxParents &&
    b.parents.all (fun p => p.valid && p.hash != b.prev) &&
    decide (b.parents.Pairwise (·.hash < ·.hash)) &&
    b.configHash.length == digestLength

/-- OpBody.valid mirrors `SOOperationInner.Validate`. -/
def OpBody.valid (b : OpBody) : Bool :=
  b.peer != "" && b.localID && b.nonce > 0 && b.dataBytes ≤ maxInnerBytes && b.linksValid

/--
verifyOperation mirrors `SOOperation.Verify`: a bounded, well-formed body
bound to `obj`, signed over its encoding by the author it names. It returns
the body.
-/
def verifyOperation (obj : String) (op : Operation) : Option OpBody := do
  let b ← op.body
  if op.innerBytes > 0 && op.innerBytes ≤ maxInnerBytes && op.sigFormat && b.valid &&
      b.object == obj && op.sig.signer == b.peer && op.sig.valid then
    some b
  else
    none

/-- canWriteOps mirrors `CanWriteOps`. -/
def canWriteOps (role : Role) : Bool :=
  role == Role.writer || role == Role.owner

/--
authorizeOperation mirrors `SOOperation.ValidateSignature`: a verified
operation whose author may write under `participants`.
-/
def authorizeOperation (obj : String) (participants : List Participant) (op : Operation) :
    Bool :=
  match verifyOperation obj op with
  | none => false
  | some b => participants.any fun p => p.peer == b.peer && canWriteOps p.role

/-- CheckpointBody is a decoded `SOCheckpointInner`. -/
structure CheckpointBody where
  object : String
  configHash : String
  replayVersion : Nat
  stateBytes : Nat
  height : Nat
  prev : String
  authors : List Position
  deriving DecidableEq, Repr

/-- Checkpoint is one `SOCheckpoint`. -/
structure Checkpoint where
  innerBytes : Nat
  body : Option CheckpointBody
  sigs : List Sig
  deriving DecidableEq, Repr

/--
CheckpointBody.valid mirrors `SOCheckpointInner.Validate`: an object, a
configuration, this replay rule and bounded state. A genesis checkpoint names
nothing before it; a later one names its predecessor and each author's last
covered operation once, in peer ID order.
-/
def CheckpointBody.valid (b : CheckpointBody) : Bool :=
  b.object != "" && b.configHash.length == digestLength &&
    b.replayVersion == supportedReplayVersion && b.stateBytes ≤ maxStateBytes &&
    if b.height == 0 then b.prev == "" && b.authors.isEmpty
    else
      b.prev.length == digestLength && b.authors.all Position.valid &&
        decide (b.authors.Pairwise (·.peer < ·.peer))

/--
verifyCheckpoint mirrors `SOCheckpoint.Verify`: a nonempty well-formed body
bound to `obj` with one to `maxParticipants` verified signatures from distinct
signers. It returns the body and the signers.
-/
def verifyCheckpoint (obj : String) (cp : Checkpoint) : Option (CheckpointBody × List String) := do
  let b ← cp.body
  let signers := cp.sigs.map (·.signer)
  if cp.innerBytes > 0 && !cp.sigs.isEmpty && cp.sigs.length ≤ maxParticipants && b.valid &&
      b.object == obj && cp.sigs.all (fun s => s.signer != "" && s.valid) &&
      decide signers.Nodup then
    some (b, signers)
  else
    none

/--
authorizeCheckpoint mirrors `SOCheckpoint.ValidateAuthority`: a verified
checkpoint signed by an OWNER under `participants`.
-/
def authorizeCheckpoint (obj : String) (participants : List Participant) (cp : Checkpoint) :
    Bool :=
  match verifyCheckpoint obj cp with
  | none => false
  | some (_, signers) =>
    participants.any fun p => p.role == Role.owner && signers.contains p.peer

/-! ## Operations -/

/--
A verified operation is bound to the verifying object, and its signature is a
valid one by the author it names.
-/
theorem verifyOperation_spec {obj : String} {op : Operation} {b : OpBody}
    (h : verifyOperation obj op = some b) :
    op.body = some b ∧ b.valid = true ∧ b.object = obj ∧ op.sig.signer = b.peer ∧
      op.sig.valid = true := by
  unfold verifyOperation at h
  cases body : op.body with
  | none => simp [body] at h
  | some b' =>
    simp only [body, Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · rename_i ok
      cases h
      simp only [Bool.and_eq_true, beq_iff_eq] at ok
      exact ⟨rfl, ok.1.1.1.2, ok.1.1.2, ok.1.2, ok.2⟩
    · contradiction

/--
An authorized operation is signed by its author, who holds WRITER or OWNER
under the participants.
-/
theorem authorizeOperation_writer {obj : String} {participants : List Participant}
    {op : Operation} (h : authorizeOperation obj participants op = true) :
    ∃ b, verifyOperation obj op = some b ∧ op.sig.valid = true ∧
      ∃ p ∈ participants, p.peer = op.sig.signer ∧
        (p.role = Role.writer ∨ p.role = Role.owner) := by
  unfold authorizeOperation at h
  split at h
  · contradiction
  · rename_i b verified
    obtain ⟨_, _, _, signer, valid⟩ := verifyOperation_spec verified
    obtain ⟨p, mem, ok⟩ := List.any_eq_true.mp h
    simp only [canWriteOps, Bool.and_eq_true, Bool.or_eq_true, beq_iff_eq] at ok
    exact ⟨b, verified, valid, p, mem, ok.1.trans signer.symm, ok.2⟩

/-- An operation bound to another object is never authorized. -/
theorem authorizeOperation_object {obj : String} {participants : List Participant}
    {op : Operation} {b : OpBody} (body : op.body = some b) (other : b.object ≠ obj) :
    authorizeOperation obj participants op = false := by
  unfold authorizeOperation
  split
  · rfl
  · rename_i b' verified
    obtain ⟨body', _, bound, _⟩ := verifyOperation_spec verified
    rw [body] at body'
    cases body'
    exact absurd bound other

/-! ## Checkpoints -/

/--
A verified checkpoint is bound to the verifying object, and every signer is
named once with a valid signature.
-/
theorem verifyCheckpoint_spec {obj : String} {cp : Checkpoint} {b : CheckpointBody}
    {signers : List String} (h : verifyCheckpoint obj cp = some (b, signers)) :
    cp.body = some b ∧ b.valid = true ∧ b.object = obj ∧ signers = cp.sigs.map (·.signer) ∧
      signers.Nodup ∧ cp.sigs ≠ [] ∧ ∀ s ∈ cp.sigs, s.valid = true := by
  unfold verifyCheckpoint at h
  cases body : cp.body with
  | none => simp [body] at h
  | some b' =>
    simp only [body, Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · rename_i ok
      cases h
      simp only [Bool.and_eq_true, beq_iff_eq, decide_eq_true_eq, Bool.not_eq_true',
        List.isEmpty_eq_false_iff, List.all_eq_true] at ok
      obtain ⟨⟨⟨⟨⟨⟨_, nonempty⟩, _⟩, valid⟩, bound⟩, signed⟩, nodup⟩ := ok
      exact ⟨rfl, valid, bound, rfl, nodup, nonempty, fun s mem => (signed s mem).2⟩
    · contradiction

/-- An authorized checkpoint carries a valid signature from an OWNER. -/
theorem authorizeCheckpoint_owner {obj : String} {participants : List Participant}
    {cp : Checkpoint} (h : authorizeCheckpoint obj participants cp = true) :
    ∃ s ∈ cp.sigs, s.valid = true ∧
      ∃ p ∈ participants, p.role = Role.owner ∧ p.peer = s.signer := by
  unfold authorizeCheckpoint at h
  split at h
  · contradiction
  · rename_i b signers verified
    obtain ⟨_, _, _, named, _, _, valid⟩ := verifyCheckpoint_spec verified
    obtain ⟨p, mem, ok⟩ := List.any_eq_true.mp h
    simp only [Bool.and_eq_true, beq_iff_eq, List.contains_iff_mem, named,
      List.mem_map] at ok
    obtain ⟨owner, s, signed, signer⟩ := ok
    exact ⟨s, signed, valid s signed, p, mem, owner, signer.symm⟩

end Spacewave.SObject
