import Spacewave.SObject.JournalReducer

/-!
# Journal frame storage and checkpoint representation

The compact snapshot portion mirrors `marshalCompactJournalCheckpoint`,
`unmarshalCompactJournalCheckpoint` and `validateCheckpointAttempt` in
`core/sobject/journal-frame.go` at Spacewave `8ccfce160`. Serialized protobuf
identities, encryption and hashes are primitive boundaries. Compact snapshots
omit the redundant lookup history; hydration reconstructs its current entry.
-/

namespace Spacewave.SObject.Journal

/-- validCheckpointAttempt mirrors the complete retained-attempt validation contract. -/
def validCheckpointAttempt (a : Attempt) : Bool :=
  a.key.isSome && a.lineage.isSome && a.version.isSome && validLineage a.key a.lineage &&
  (1 ≤ a.state && a.state ≤ 6) && (1 ≤ a.readiness && a.readiness ≤ 4) &&
  a.lookupHistory.length ≤ 1 && (a.lookup.isSome || a.lookupHistory.isEmpty) &&
  (a.lookup.isNone || (validLookup a.lookup a.key a.lineage a.version &&
    (a.lookupHistory.isEmpty || a.lookupHistory.head?.join.map (·.data) == a.lookup.map (·.data)))) &&
  validPayload a.intent && a.intentSequence != 0 &&
  (if a.envelope.isNone then a.envelopeSequence == 0 && a.envelopeDigest == ""
   else validPayload a.envelope && a.envelopeSequence != 0 &&
     a.intentSequence < a.envelopeSequence && a.envelopeDigest.length == digestLength) &&
  (a.receipt.isNone || (a.envelope.isSome &&
    validReceipt a.receipt a.key a.lineage a.version a.envelopeDigest)) &&
  (a.acknowledgement.isNone || (a.receipt.isSome && validAcknowledgement a.acknowledgement a.key &&
    (a.acknowledgement.getD default).receiptDigest == (a.receipt.getD default).terminalDigest)) &&
  (a.projection.isNone || (a.receipt.isSome && validProjection a.projection a.key &&
    (a.projection.getD default).receiptDigest == (a.receipt.getD default).terminalDigest &&
    (a.projection.getD default).rootSeqno == (a.receipt.getD default).rootSeqno &&
    (a.projection.getD default).rootDigest == (a.receipt.getD default).rootDigest)) &&
  (a.receipt.isSome || (a.acknowledgement.isNone && a.projection.isNone)) &&
  (!(a.lookup.isSome && ((a.lookup.getD default).state == 3 || (a.lookup.getD default).state == 4)) ||
    (a.receipt.isSome && a.receipt.map (·.data) == (a.lookup.getD default).receipt.map (·.data))) &&
  (!a.resendAuthorized || (a.state == 3 && a.sendAttempted && a.lookup.isSome &&
    (a.lookup.getD default).state == 1)) &&
  (!a.lineageRecoveryBlocked || a.state == 5) &&
  (if a.state == 1 then
    a.envelope.isNone && a.receipt.isNone && a.acknowledgement.isNone && a.projection.isNone &&
      a.lookup.isNone && !a.sendAttempted && !a.resendAuthorized && !a.lineageRecoveryBlocked
  else if a.state == 2 then
    a.readiness == 1 && a.envelope.isSome && a.receipt.isNone && a.acknowledgement.isNone &&
      a.projection.isNone && a.lookup.isNone && !a.sendAttempted && !a.resendAuthorized && !a.lineageRecoveryBlocked
  else if a.state == 3 then
    a.readiness == 1 && a.envelope.isSome && a.sendAttempted && a.receipt.isNone &&
      a.acknowledgement.isNone && a.projection.isNone &&
      (a.lookup.isNone || (a.lookup.getD default).state == 2 || (a.lookup.getD default).state == 1)
  else if a.state == 4 then
    a.readiness == 1 && a.envelope.isSome && a.sendAttempted && a.receipt.isSome && !a.resendAuthorized &&
      (a.lookup.isNone || ((a.lookup.getD default).state ≥ 1 && (a.lookup.getD default).state ≤ 4))
  else a.receipt.isNone && a.acknowledgement.isNone && a.projection.isNone && !a.resendAuthorized &&
    (!a.sendAttempted || lookupComplete a.lookup)) &&
  (a.checkpointEligible == (a.receipt.isSome && a.projection.isSome))

/-- compactAttempt removes only the redundant history field omitted by the wire snapshot. -/
def compactAttempt (a : Attempt) : Attempt := {a with lookupHistory := []}

/-- restoreAttempt reconstructs the bounded history from the current lookup. -/
def restoreAttempt (a : Attempt) : Attempt := a.withLookup a.lookup

/-- CompactCheckpoint retains the metadata checked before attempt hydration. -/
structure CompactCheckpoint where
  identity : String
  generation : Nat
  nextSequence : Nat
  attempts : List Attempt
  deriving DecidableEq, Repr, Inhabited

/-- buildCheckpoint validates the exact compact evidence before serialization/encryption. -/
def buildCheckpoint (identity : String) (generation nextSequence : Nat)
    (state : Option State) : Option CompactCheckpoint := do
  if identity.length != digestLength || generation == 0 || nextSequence == 0 then none
  else
    let state ← state
    let attempts := state.map compactAttempt
    if !attempts.all validCheckpointAttempt then none
    else some ⟨identity, generation, nextSequence, attempts⟩

/-- hydrateAttempts rejects invalid or duplicate entries, matching fresh-reducer hydration. -/
def hydrateAttempts (state : State) : List Attempt → Option State
  | [] => some state
  | attempt :: attempts =>
    if !validCheckpointAttempt attempt || (findAttempt state (attemptDigest attempt)).isSome then none
    else hydrateAttempts (putAttempt state attempt) attempts

/-- readCheckpoint checks metadata, restores history and validates every retained attempt. -/
def readCheckpoint (checkpoint : CompactCheckpoint) (identity : String)
    (generation nextSequence : Nat) : Option State :=
  if checkpoint.identity != identity || checkpoint.generation != generation ||
      checkpoint.nextSequence != nextSequence then none
  else hydrateAttempts [] (checkpoint.attempts.map restoreAttempt)

/-- Restoring a compacted canonical attempt preserves every observable field. -/
theorem restore_compact {attempt : Attempt} (h : lookupConsistent attempt) :
    restoreAttempt (compactAttempt attempt) = attempt := by
  cases attempt
  simp_all [lookupConsistent, restoreAttempt, compactAttempt, Attempt.withLookup]

/-- Compacting and restoring a reducer snapshot preserves all evidence from full replay. -/
theorem replay_checkpoint_fields {records : List (Option Record)} {state : State}
    (h : reduceJournal records = some state) :
    (state.map compactAttempt).map restoreAttempt = state := by
  rw [List.map_map]
  calc
    state.map (restoreAttempt ∘ compactAttempt) = state.map id :=
      List.map_congr_left (fun attempt member => restore_compact (reduceJournal_lookup h attempt member))
    _ = state := List.map_id state

/-- Restored checkpoint evidence followed by a suffix agrees with full replay. -/
theorem checkpoint_suffix {records suffix : List (Option Record)} {state : State}
    (h : reduceJournal records = some state) :
    replayFrom ((state.map compactAttempt).map restoreAttempt)
        (advanceSequence 1 records.length) suffix = reduceJournal (records ++ suffix) := by
  rw [replay_checkpoint_fields h]
  unfold reduceJournal
  rw [replayFrom_append]
  simp only [reduceJournal] at h
  simp [h]

/-- Hydrating a previously absent digest slot retains every old attempt exactly once. -/
theorem putAttempt_absent_perm {state : State} {attempt : Attempt}
    (absent : findAttempt state (attemptDigest attempt) = none) :
    (putAttempt state attempt).Perm (attempt :: state) := by
  have filtered : state.filter (fun a => attemptDigest a != attemptDigest attempt) = state := by
    apply List.filter_eq_self.mpr
    intro a member
    have unequal := List.find?_eq_none.mp absent a member
    simpa only [bne_iff_ne, beq_iff_eq] using unequal
  unfold putAttempt
  rw [filtered]
  exact List.mergeSort_perm _ _

/-- Successful hydration reorders entries but neither drops nor invents retained evidence. -/
theorem hydrateAttempts_perm {attempts state out : State}
    (h : hydrateAttempts state attempts = some out) : out.Perm (attempts ++ state) := by
  induction attempts generalizing state with
  | nil =>
    simp only [hydrateAttempts, Option.some.injEq] at h
    cases h
    simp
  | cons attempt attempts ih =>
    simp only [hydrateAttempts] at h
    split at h
    · contradiction
    · rename_i allowed
      have absent : findAttempt state (attemptDigest attempt) = none := by
        cases found : findAttempt state (attemptDigest attempt) with
        | none => rfl
        | some a => simp [found] at allowed
      have perm := ih h
      exact (perm.trans ((List.Perm.refl attempts).append (putAttempt_absent_perm absent))).trans
        List.perm_middle

/-- Fresh hydration exports the same sorted order as a live reducer. -/
theorem hydrateAttempts_sorted {attempts state out : State}
    (sorted : stateSorted state) (h : hydrateAttempts state attempts = some out) : stateSorted out := by
  induction attempts generalizing state with
  | nil =>
    simp only [hydrateAttempts, Option.some.injEq] at h
    cases h
    exact sorted
  | cons attempt attempts ih =>
    simp only [hydrateAttempts] at h
    split at h
    · contradiction
    · exact ih (putAttempt_sorted state attempt) h

/-- A successfully written and hydrated checkpoint equals the complete replay snapshot. -/
theorem checkpoint_roundtrip {records : List (Option Record)} {state restored : State}
    {identity : String} {generation nextSequence : Nat} {checkpoint : CompactCheckpoint}
    (replayed : reduceJournal records = some state)
    (built : buildCheckpoint identity generation nextSequence (some state) = some checkpoint)
    (read : readCheckpoint checkpoint identity generation nextSequence = some restored) :
    restored = state := by
  unfold buildCheckpoint at built
  split at built
  · contradiction
  · simp only [Option.bind_eq_bind, Option.bind_some] at built
    split at built
    · contradiction
    · cases built
      simp only [readCheckpoint, bne_self_eq_false, Bool.false_or, Bool.false_eq_true, ↓reduceIte] at read
      rw [replay_checkpoint_fields replayed] at read
      have perm := hydrateAttempts_perm read
      simp only [List.append_nil] at perm
      obtain ⟨sorted, unique⟩ := reduceJournal_indexed replayed
      apply List.Perm.eq_of_pairwise (le := fun a b => attemptDigest a ≤ attemptDigest b)
      · intro a b am bm ab ba
        exact unique a (perm.mem_iff.mp am) b bm (Std.le_antisymm ab ba)
      · exact hydrateAttempts_sorted (by simp [stateSorted]) read
      · exact sorted
      · exact perm

/-- Actual accepted checkpoint hydration followed by a suffix equals full record replay. -/
theorem hydrated_checkpoint_suffix {records suffix : List (Option Record)} {state restored : State}
    {identity : String} {generation nextSequence : Nat} {checkpoint : CompactCheckpoint}
    (replayed : reduceJournal records = some state)
    (built : buildCheckpoint identity generation nextSequence (some state) = some checkpoint)
    (read : readCheckpoint checkpoint identity generation nextSequence = some restored)
    (sequence : nextSequence = advanceSequence 1 records.length) :
    replayFrom restored nextSequence suffix = reduceJournal (records ++ suffix) := by
  rw [sequence, checkpoint_roundtrip replayed built read]
  unfold reduceJournal
  rw [replayFrom_append]
  simp only [reduceJournal] at replayed
  simp [replayed]

end Spacewave.SObject.Journal
