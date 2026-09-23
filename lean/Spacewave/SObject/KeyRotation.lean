import Spacewave.SObject.Crypto

/-!
# SharedObject key epochs

Mirrors `RotateTransformKey`, `FindCoveringEpoch`, and `CurrentEpochNumber` in
`core/sobject/key-rotation.go` at `757c38aec` with counter-exhaustion rejection
and unambiguous epoch selection. A grant is a recipient and an opaque transform
key identity. The conformance projection decrypts real generated grants and
hashes their transform material to obtain that identity. Fresh random material
and successful encryption are explicit cryptographic boundary inputs. Public
key extraction is projected separately from the recipient's role. Rotation
does not itself check owner authority; that is its caller's contract.
-/

namespace Spacewave.SObject

/-- RotationPeer retains a participant and whether its ID exposes an encryption key. -/
structure RotationPeer where
  participant : Participant
  publicKey : Bool
  deriving DecidableEq, Repr

/-- KeyGrant is the capability to decrypt one epoch's transform material. -/
structure KeyGrant where
  peer : String
  key : String
  deriving DecidableEq, Repr

/-- KeyEpoch retains all SOKeyEpoch fields after decrypting its grants. -/
structure KeyEpoch where
  epoch : Nat
  start : Nat
  finish : Nat
  grants : List KeyGrant
  deriving DecidableEq, Repr

/-- Rotation returns one transform identity and its new open epoch. -/
structure Rotation where
  key : String
  grants : List KeyGrant
  epoch : KeyEpoch
  deriving DecidableEq, Repr

/-- rotationReaders mirrors the CanReadState filter in RotateTransformKey. -/
def rotationReaders (ps : List RotationPeer) : List RotationPeer :=
  ps.filter (fun p => p.participant.role ∈ [Role.reader, Role.writer, Role.validator, Role.owner])

/-- rotateTransformKey issues the fresh key to exactly the readable audience. -/
def rotateTransformKey (ps : List RotationPeer) (currentEpoch currentSeqno : Nat)
    (key : String) (cryptoOK : Bool) : Option Rotation :=
  if currentEpoch + 1 ≥ seqnoLimit || currentSeqno + 1 ≥ seqnoLimit then none
  else if cryptoOK && (rotationReaders ps).all (·.publicKey) then
    let grants := (rotationReaders ps).map (fun p => KeyGrant.mk p.participant.peer key)
    some ⟨key, grants, ⟨currentEpoch + 1, currentSeqno + 1, 0, grants⟩⟩
  else none

/-- KeyEpoch.covers treats zero end as an open range and both endpoints as inclusive. -/
def KeyEpoch.covers (e : KeyEpoch) (seqno : Nat) : Bool :=
  e.start ≤ seqno && (e.finish == 0 || seqno ≤ e.finish)

/-- coveringEpochs ignores absent entries and retains every range containing the sequence. -/
def coveringEpochs (epochs : List (Option KeyEpoch)) (seqno : Nat) : List KeyEpoch :=
  epochs.filterMap id |>.filter (·.covers seqno)

/-- findCoveringEpoch returns a result only when exactly one entry covers the sequence. -/
def findCoveringEpoch (epochs : List (Option KeyEpoch)) (seqno : Nat) : Option KeyEpoch :=
  match coveringEpochs epochs seqno with
  | [epoch] => some epoch
  | _ => none

/-- currentEpochNumber is the maximum numeric epoch, with zero for absent entries. -/
def currentEpochNumber (epochs : List (Option KeyEpoch)) : Nat :=
  epochs.foldr (fun e current => max ((e.map (·.epoch)).getD 0) current) 0

/-! ## Rotation and access -/

/-- A successful rotation advances both counters and shares one key with exactly the readers. -/
theorem rotateTransformKey_spec {ps : List RotationPeer} {epoch seqno : Nat}
    {key : String} {cryptoOK : Bool} {out : Rotation}
    (h : rotateTransformKey ps epoch seqno key cryptoOK = some out) :
    out.key = key ∧ out.epoch.epoch = epoch + 1 ∧ out.epoch.start = seqno + 1 ∧
      out.epoch.finish = 0 ∧ out.epoch.grants = out.grants ∧
      out.grants = (rotationReaders ps).map (fun p => KeyGrant.mk p.participant.peer key) ∧
      epoch + 1 < seqnoLimit ∧ seqno + 1 < seqnoLimit := by
  unfold rotateTransformKey at h
  split at h
  · contradiction
  · rename_i bounds
    split at h
    · cases h
      simp only [Bool.or_eq_true, decide_eq_true_eq, not_or] at bounds
      exact ⟨rfl, rfl, rfl, rfl, rfl, rfl, by omega, by omega⟩
    · contradiction

/-- Every remaining reader gets the new key, and no other recipient gets a grant. -/
theorem rotateTransformKey_coverage {ps : List RotationPeer} {epoch seqno : Nat}
    {key : String} {cryptoOK : Bool} {out : Rotation}
    (h : rotateTransformKey ps epoch seqno key cryptoOK = some out) (peer : String) :
    KeyGrant.mk peer key ∈ out.grants ↔
      ∃ p ∈ ps, p.participant.peer = peer ∧
        p.participant.role ∈ [Role.reader, Role.writer, Role.validator, Role.owner] := by
  obtain ⟨_, _, _, _, _, grants, _, _⟩ := rotateTransformKey_spec h
  simp only [grants, rotationReaders, List.mem_map, List.mem_filter, decide_eq_true_eq,
    KeyGrant.mk.injEq, and_true]
  constructor
  · rintro ⟨p, ⟨member, readable⟩, same⟩
    exact ⟨p, member, same, readable⟩
  · rintro ⟨p, member, same, readable⟩
    exact ⟨p, ⟨member, readable⟩, same⟩

/-- A removed peer has no grant for this rotation, even if it retains every older key. -/
theorem rotateTransformKey_excludes {ps : List RotationPeer} {epoch seqno : Nat}
    {key : String} {cryptoOK : Bool} {out : Rotation} {peer : String}
    (removed : ∀ p ∈ ps, p.participant.peer ≠ peer)
    (h : rotateTransformKey ps epoch seqno key cryptoOK = some out) :
    ∀ grant ∈ out.grants, grant.peer ≠ peer := by
  obtain ⟨_, _, _, _, _, grants, _, _⟩ := rotateTransformKey_spec h
  intro grant member
  simp only [grants, rotationReaders, List.mem_map, List.mem_filter] at member
  obtain ⟨p, ⟨member, _⟩, shape⟩ := member
  exact shape ▸ removed p member

/-- Exhaustion fails rather than reusing epoch zero or an earlier root range. -/
theorem rotateTransformKey_exhausted (ps : List RotationPeer) (epoch seqno : Nat)
    (key : String) (cryptoOK : Bool)
    (exhausted : epoch + 1 ≥ seqnoLimit ∨ seqno + 1 ≥ seqnoLimit) :
    rotateTransformKey ps epoch seqno key cryptoOK = none := by
  simp [rotateTransformKey, exhausted]

/-! ## Lookup -/

/-- The current epoch number bounds every retained epoch, regardless of history order. -/
theorem currentEpochNumber_member {epochs : List (Option KeyEpoch)} {epoch : KeyEpoch}
    (member : some epoch ∈ epochs) : epoch.epoch ≤ currentEpochNumber epochs := by
  induction epochs with
  | nil => simp at member
  | cons first rest ih =>
    change epoch.epoch ≤ max ((first.map (·.epoch)).getD 0) (currentEpochNumber rest)
    rcases List.mem_cons.mp member with same | tail
    · rw [← same]
      exact Nat.le_max_left ..
    · exact Nat.le_trans (ih tail) (Nat.le_max_right ..)

/-- Rotating the maximum retained epoch gives a strictly newer generation than every old entry. -/
theorem rotateTransformKey_freshEpoch {ps : List RotationPeer} {history : List (Option KeyEpoch)}
    {seqno : Nat} {key : String} {cryptoOK : Bool} {out : Rotation}
    (h : rotateTransformKey ps (currentEpochNumber history) seqno key cryptoOK = some out)
    {old : KeyEpoch} (member : some old ∈ history) : old.epoch < out.epoch.epoch := by
  have bound := currentEpochNumber_member member
  have step := (rotateTransformKey_spec h).2.1
  rw [step]
  omega

/-- A returned epoch is the sole covering entry, not a first-match guess. -/
theorem findCoveringEpoch_unique {epochs : List (Option KeyEpoch)} {seqno : Nat}
    {epoch : KeyEpoch} (h : findCoveringEpoch epochs seqno = some epoch) :
    coveringEpochs epochs seqno = [epoch] := by
  unfold findCoveringEpoch at h
  split at h
  · rename_i found shape
    cases h
    exact shape
  · contradiction

/-- A returned epoch actually covers the requested sequence and came from the supplied history. -/
theorem findCoveringEpoch_covers {epochs : List (Option KeyEpoch)} {seqno : Nat}
    {epoch : KeyEpoch} (h : findCoveringEpoch epochs seqno = some epoch) :
    some epoch ∈ epochs ∧ epoch.covers seqno = true := by
  have member : epoch ∈ coveringEpochs epochs seqno := by
    rw [findCoveringEpoch_unique h]
    simp
  simpa [coveringEpochs] using member

/-- A single covering entry is found regardless of unrelated or absent history entries. -/
theorem findCoveringEpoch_complete {epochs : List (Option KeyEpoch)} {seqno : Nat}
    {epoch : KeyEpoch} (unique : coveringEpochs epochs seqno = [epoch]) :
    findCoveringEpoch epochs seqno = some epoch := by
  simp [findCoveringEpoch, unique]

end Spacewave.SObject
