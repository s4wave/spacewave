import Spacewave.SObject.JournalKey

/-!
# Mutation journal reducer

Mirrors `JournalReducer.apply`, `validateJournalRecord` and `ReduceJournal` in
`core/sobject/journal-reducer.go` at Spacewave `8ccfce160`, with the resend
lookup-history correction. Byte fields are lowercase hex; enum codes remain Int.
Hash annotations contain primitive SHA-256 results, not aggregate validation
outcomes. Serialized message identities preserve protobuf unknown fields where
Go uses EqualVT. Envelope encryption and receipt/lookup authentication belong to the
pipeline boundary. This reducer checks their structural bindings itself.
-/

namespace Spacewave.SObject.Journal

/-- Version retains every immutable journal version observation. -/
structure Version where
  data : String
  localVersion : Nat
  remoteVersion : Nat
  transformEpoch : Nat
  configDigest : String
  deriving DecidableEq, Repr, Inhabited

/-- Payload retains the authenticated-encryption nonce and ciphertext. -/
structure Payload where
  nonce : String
  ciphertext : String
  deriving DecidableEq, Repr, Inhabited

/-- Receipt retains terminal evidence and the independently computed terminal hash. -/
structure Receipt where
  data : String
  key : Option Key
  supersedes : Option Key
  envelopeDigest : String
  outcome : Int
  terminal : String
  terminalDigest : String
  terminalHash : String
  rootSeqno : Nat
  rootDigest : String
  configDigest : String
  terminalTime : Nat
  deriving DecidableEq, Repr, Inhabited

/-- Lookup retains the latest authoritative response and its primitive hash. -/
structure Lookup where
  data : String
  key : Option Key
  state : Int
  receipt : Option Receipt
  response : String
  responseDigest : String
  responseHash : String
  configDigest : String
  deriving DecidableEq, Repr, Inhabited

/-- Acknowledgement binds a durable acknowledgement timestamp to its exact receipt. -/
structure Acknowledgement where
  data : String
  key : Option Key
  receiptDigest : String
  time : Nat
  deriving DecidableEq, Repr, Inhabited

/-- Projection binds body progress to the exact terminal receipt root. -/
structure Projection where
  data : String
  key : Option Key
  receiptDigest : String
  rootSeqno : Nat
  rootDigest : String
  deriving DecidableEq, Repr, Inhabited

/-- Record retains all fields read by structural validation and reducer transitions. -/
structure Record where
  format : Nat
  sequence : Nat
  kind : Int
  key : Option Key
  lineage : Option Lineage
  version : Option Version
  state : Int
  readiness : Int
  intent : Option Payload
  envelope : Option Payload
  envelopeDigest : String
  receipt : Option Receipt
  acknowledgement : Option Acknowledgement
  projection : Option Projection
  lookup : Option Lookup
  recoveryReason : Int
  deriving DecidableEq, Repr, Inhabited

/-- Attempt is every observable field of JournalAttemptSnapshot. -/
structure Attempt where
  key : Option Key
  lineage : Option Lineage
  version : Option Version
  state : Int
  readiness : Int
  intentSequence : Nat
  envelopeSequence : Nat
  intent : Option Payload
  envelope : Option Payload
  envelopeDigest : String
  receipt : Option Receipt
  acknowledgement : Option Acknowledgement
  projection : Option Projection
  lookup : Option Lookup
  lookupHistory : List (Option Lookup)
  sendAttempted : Bool
  resendAuthorized : Bool
  lineageRecoveryBlocked : Bool
  checkpointEligible : Bool
  deriving DecidableEq, Repr, Inhabited

/-- validPayload mirrors the twelve-byte nonce and minimum authentication-tag length. -/
def validPayload (payload : Option Payload) : Bool :=
  match payload with
  | none => false
  | some p => p.nonce.length == 24 && p.ciphertext.length ≥ 32

/-- validReceipt checks exact identity, digest sizes, terminal hashing and optional bindings. -/
def validReceipt (receipt : Option Receipt) (key : Option Key) (lineage : Option Lineage)
    (version : Option Version) (envelopeDigest : String) : Bool :=
  match receipt with
  | none => false
  | some r => sameKey r.key key && sameKey r.supersedes (lineage.getD default).supersedes &&
    (r.outcome == 1 || r.outcome == 2) && r.envelopeDigest.length == digestLength &&
    r.terminal != "" && r.terminalDigest.length == digestLength &&
    r.rootDigest.length == digestLength && r.configDigest.length == digestLength &&
    r.terminalHash == r.terminalDigest &&
    (envelopeDigest == "" || envelopeDigest == r.envelopeDigest) &&
    (version.isNone || (version.getD default).configDigest == r.configDigest)

/-- validLookup verifies response integrity and terminal receipt consistency. -/
def validLookup (lookup : Option Lookup) (key : Option Key) (lineage : Option Lineage)
    (version : Option Version) : Bool :=
  match lookup with
  | none => false
  | some l => sameKey l.key key && (1 ≤ l.state && l.state ≤ 4) &&
    l.response != "" && l.responseDigest.length == digestLength &&
    l.configDigest.length == digestLength && l.responseHash == l.responseDigest &&
    (version.isNone || (version.getD default).configDigest == l.configDigest) &&
    (if l.state == 3 || l.state == 4 then
      validReceipt l.receipt key lineage version "" &&
        (l.receipt.getD default).outcome == (if l.state == 3 then 1 else 2)
    else l.receipt.isNone)

/-- validAcknowledgement checks identity and the full receipt digest. -/
def validAcknowledgement (ack : Option Acknowledgement) (key : Option Key) : Bool :=
  match ack with
  | none => false
  | some a => sameKey a.key key && a.receiptDigest.length == digestLength

/-- validProjection checks identity and full receipt/root digests. -/
def validProjection (projection : Option Projection) (key : Option Key) : Bool :=
  match projection with
  | none => false
  | some p => sameKey p.key key && p.receiptDigest.length == digestLength &&
    p.rootDigest.length == digestLength

/-- lookupComplete distinguishes an authoritative resolution from absent or pending evidence. -/
def lookupComplete (lookup : Option Lookup) : Bool :=
  lookup.isSome && (lookup.getD default).state != 0 && (lookup.getD default).state != 2

/-- readinessMatchesRecovery preserves the body owner's specific recovery-stop reason. -/
def readinessMatchesRecovery (readiness reason : Int) : Bool :=
  if readiness == 2 then reason == 4
  else if readiness == 3 then reason == 5
  else if readiness == 4 then reason == 6
  else reason == 2 || reason == 3

/-- validRecord mirrors all kind-specific record admission before state is consulted. -/
def validRecord (record : Option Record) : Bool :=
  match record with
  | none => false
  | some r => r.format == 1 && r.sequence != 0 && (1 ≤ r.kind && r.kind ≤ 11) &&
    validLineage r.key r.lineage && r.version.isSome &&
    (if r.kind == 1 then r.state == 1 && (1 ≤ r.readiness && r.readiness ≤ 4) && validPayload r.intent
    else if r.kind == 2 then r.state == 2 && validPayload r.envelope && r.envelopeDigest.length == digestLength
    else if r.kind == 3 then r.state == 3
    else if r.kind == 4 then r.state == 4 && validReceipt r.receipt r.key r.lineage none ""
    else if r.kind == 10 then
      r.state == (if (r.lookup.getD default).state == 3 || (r.lookup.getD default).state == 4 then 4 else 3) &&
        validLookup r.lookup r.key r.lineage r.version
    else if r.kind == 11 then r.state == 3
    else if r.kind == 5 then r.state == 4 && validAcknowledgement r.acknowledgement r.key
    else if r.kind == 6 then r.state == 4 && validProjection r.projection r.key
    else if r.kind == 7 then r.state == 5 && r.recoveryReason == 1
    else if r.kind == 8 then r.state == 6 && (1 ≤ r.recoveryReason && r.recoveryReason ≤ 6)
    else r.state == 5 && (1 ≤ r.recoveryReason && r.recoveryReason ≤ 6) && r.recoveryReason != 1)

/-- Attempt.withLookup keeps the bounded history equal to its current lookup. -/
def Attempt.withLookup (attempt : Attempt) (lookup : Option Lookup) : Attempt :=
  {attempt with lookup, lookupHistory := lookup.toList.map some}

/-- advanceEnvelope mirrors record-kind 2 in the reducer transition switch. -/
def advanceEnvelope (a : Attempt) (r : Record) : Option Attempt := do
  if a.state != 1 || a.readiness != 1 || a.envelope.isSome || r.envelopeDigest.length != digestLength then none
  else some {a with
    envelope := r.envelope
    envelopeSequence := r.sequence
    envelopeDigest := r.envelopeDigest
    state := r.state}

/-- advanceSent mirrors record-kind 3 in the reducer transition switch. -/
def advanceSent (a : Attempt) (r : Record) : Option Attempt := do
  if a.state != 2 && (a.state != 3 || !a.resendAuthorized) then none
  else
    let a := if a.resendAuthorized then a.withLookup none else a
    some {a with sendAttempted := true, resendAuthorized := false, state := r.state}

/-- advanceReceipt mirrors record-kind 4 in the reducer transition switch. -/
def advanceReceipt (a : Attempt) (r : Record) : Option Attempt := do
  if a.state != 3 || !a.sendAttempted || a.receipt.isSome ||
      !validReceipt r.receipt r.key r.lineage a.version a.envelopeDigest then none
  else some {a with receipt := r.receipt, resendAuthorized := false, state := r.state}

/-- advanceLookup mirrors record-kind 10 in the reducer transition switch. -/
def advanceLookup (a : Attempt) (r : Record) : Option Attempt := do
  if a.state != 3 || !a.sendAttempted || a.receipt.isSome ||
      (a.lookup.isSome && (a.lookup.getD default).state != 2) then none
  else
    let lookup := r.lookup.getD default
    if !sameKey lookup.key r.key ||
        (a.lookup.isSome && (a.lookup.getD default).state == 2 && lookup.state == 1) then none
    else
      let a := a.withLookup (some lookup)
      if lookup.state == 3 || lookup.state == 4 then
        if !validReceipt lookup.receipt r.key r.lineage a.version a.envelopeDigest then none
        else some {a with receipt := lookup.receipt, resendAuthorized := false, state := 4}
      else some a

/-- advanceResend mirrors record-kind 11 in the reducer transition switch. -/
def advanceResend (a : Attempt) (_r : Record) : Option Attempt := do
  if a.state != 3 || a.lookup.isNone || (a.lookup.getD default).state != 1 || a.resendAuthorized then none
  else some {a with resendAuthorized := true}

/-- advanceAcknowledgement mirrors record-kind 5 in the reducer transition switch. -/
def advanceAcknowledgement (a : Attempt) (r : Record) : Option Attempt := do
  if a.receipt.isNone || (a.receipt.getD default).terminalDigest !=
      (r.acknowledgement.getD default).receiptDigest ||
      (a.acknowledgement.isSome && (a.acknowledgement.map (·.data)) != (r.acknowledgement.map (·.data))) then none
  else some {a with acknowledgement := r.acknowledgement}

/-- advanceProjection mirrors record-kind 6 in the reducer transition switch. -/
def advanceProjection (a : Attempt) (r : Record) : Option Attempt := do
  if a.receipt.isNone || (a.receipt.getD default).terminalDigest !=
      (r.projection.getD default).receiptDigest ||
      (a.receipt.getD default).rootSeqno != (r.projection.getD default).rootSeqno ||
      (a.receipt.getD default).rootDigest != (r.projection.getD default).rootDigest ||
      (a.projection.isSome && (a.projection.map (·.data)) != (r.projection.map (·.data))) then none
  else some {a with projection := r.projection}

/-- advanceStale mirrors record-kind 7 in the reducer transition switch. -/
def advanceStale (a : Attempt) (r : Record) : Option Attempt := do
  if (a.state != 1 && a.state != 2 && a.state != 3) ||
      (a.state == 3 && !lookupComplete a.lookup) then none
  else some {a with resendAuthorized := false, state := r.state}

/-- advanceBlocked mirrors record-kind 8 in the reducer transition switch. -/
def advanceBlocked (a : Attempt) (r : Record) : Option Attempt := do
  if a.state == 4 || a.state == 5 || a.state == 6 ||
      (a.state == 3 && !lookupComplete a.lookup) ||
      !readinessMatchesRecovery a.readiness r.recoveryReason then none
  else some {a with resendAuthorized := false, state := r.state}

/-- advanceLineageBlocked mirrors record-kind 9 in the reducer transition switch. -/
def advanceLineageBlocked (a : Attempt) (_r : Record) : Option Attempt := do
  if a.state != 5 || a.lineageRecoveryBlocked then none
  else some {a with lineageRecoveryBlocked := true}

/-- advanceAttempt dispatches the same integer record kinds as the Go reducer. -/
def advanceAttempt (a : Attempt) (r : Record) : Option Attempt :=
  if r.kind == 2 then advanceEnvelope a r
  else if r.kind == 3 then advanceSent a r
  else if r.kind == 4 then advanceReceipt a r
  else if r.kind == 10 then advanceLookup a r
  else if r.kind == 11 then advanceResend a r
  else if r.kind == 5 then advanceAcknowledgement a r
  else if r.kind == 6 then advanceProjection a r
  else if r.kind == 7 then advanceStale a r
  else if r.kind == 8 then advanceBlocked a r
  else if r.kind == 9 then advanceLineageBlocked a r
  else none

/-- State is a digest-indexed map serialized in ascending digest order like Snapshot. -/
abbrev State := List Attempt

/-- attemptDigest retrieves the primitive index key of an attempt. -/
def attemptDigest (a : Attempt) : String := (a.key.getD default).digest

/-- findAttempt reads the existing unique digest slot. -/
def findAttempt (state : State) (digest : String) : Option Attempt :=
  state.find? (fun a => attemptDigest a == digest)

/-- putAttempt replaces a digest slot and keeps snapshot ordering deterministic. -/
def putAttempt (state : State) (attempt : Attempt) : State :=
  (attempt :: state.filter (fun a => attemptDigest a != attemptDigest attempt)).mergeSort
    (fun a b => attemptDigest a ≤ attemptDigest b)

/-- applyRecord returns a committed candidate or rejection, leaving the input map unchanged. -/
def applyRecord (state : State) (record : Option Record) : Option State := do
  if !validRecord record then none
  else
    let r ← record
    let digest := (r.key.getD default).digest
    match findAttempt state digest with
    | none =>
      if r.kind != 1 then none
      else
        let previous := (r.lineage.getD default).supersedes
        let predecessor := findAttempt state (previous.getD default).digest
        if previous.isSome && (predecessor.isNone ||
            ((predecessor.getD default).state != 5 && (predecessor.getD default).state != 6)) then none
        else some (putAttempt state {(default : Attempt) with
          key := r.key
          lineage := r.lineage
          version := r.version
          state := r.state
          readiness := r.readiness
          intent := r.intent
          intentSequence := r.sequence})
    | some a =>
      if !sameKey a.key r.key ||
          !sameKey (a.lineage.getD default).root (r.lineage.getD default).root ||
          !sameKey (a.lineage.getD default).supersedes (r.lineage.getD default).supersedes ||
          (a.version.map (·.data)) != (r.version.map (·.data)) then none
      else
        let next ← advanceAttempt a r
        some (putAttempt state {next with checkpointEligible := next.receipt.isSome && next.projection.isSome})

/-- replayFrom enforces contiguous uint64 sequences and applies each accepted record. -/
def replayFrom (state : State) (sequence : Nat) : List (Option Record) → Option State
  | [] => some state
  | record :: records => do
    let r ← record
    if r.sequence != sequence then none
    else
      let next ← applyRecord state record
      replayFrom next ((sequence + 1) % seqnoLimit) records

/-- reduceJournal starts the durable sequence at one with no prior attempts. -/
def reduceJournal (records : List (Option Record)) : Option State := replayFrom [] 1 records

/-- advanceSequence applies the same uint64 increment for a prefix of the given length. -/
def advanceSequence (sequence : Nat) : Nat → Nat
  | 0 => sequence
  | count + 1 => advanceSequence ((sequence + 1) % seqnoLimit) count

/-- Replaying a suffix after an accepted prefix is exactly replaying the concatenation. -/
theorem replayFrom_append (left right : List (Option Record)) (state : State) (sequence : Nat) :
    replayFrom state sequence (left ++ right) =
      (replayFrom state sequence left).bind
        (fun checkpoint => replayFrom checkpoint (advanceSequence sequence left.length) right) := by
  induction left generalizing state sequence with
  | nil => simp [replayFrom, advanceSequence]
  | cons record records ih =>
    cases record with
    | none => simp [replayFrom]
    | some record =>
      simp only [List.cons_append, replayFrom, Option.bind_eq_bind, Option.bind_some]
      split
      · simp
      · cases applied : applyRecord state (some record) with
        | none => simp
        | some next => simpa [applied, advanceSequence] using ih next ((sequence + 1) % seqnoLimit)

/-- Two successful replays of the same records and starting checkpoint return the same state. -/
theorem replayFrom_deterministic {state first second : State} {sequence : Nat}
    {records : List (Option Record)}
    (left : replayFrom state sequence records = some first)
    (right : replayFrom state sequence records = some second) : first = second :=
  Option.some.inj (left.symm.trans right)

/-- lookupConsistent states the bounded current-lookup representation required by checkpoints. -/
def lookupConsistent (attempt : Attempt) : Prop := attempt.lookupHistory = attempt.lookup.toList.map some

/-- Every accepted transition preserves the current lookup and its bounded history together. -/
theorem advanceAttempt_lookup {a out : Attempt} {record : Record}
    (consistent : lookupConsistent a) (h : advanceAttempt a record = some out) :
    lookupConsistent out := by
  unfold advanceAttempt at h
  unfold lookupConsistent at consistent ⊢
  repeat' first
    | dsimp only at h
    | split at h
    | contradiction
    | unfold advanceEnvelope at h
    | unfold advanceSent at h
    | unfold advanceReceipt at h
    | unfold advanceLookup at h
    | unfold advanceResend at h
    | unfold advanceAcknowledgement at h
    | unfold advanceProjection at h
    | unfold advanceStale at h
    | unfold advanceBlocked at h
    | unfold advanceLineageBlocked at h
    | (cases h; simp_all [Attempt.withLookup])

/-- stateConsistent requires every indexed attempt to carry canonical lookup evidence. -/
def stateConsistent (state : State) : Prop := ∀ a ∈ state, lookupConsistent a

/-- Replacing a digest slot preserves canonical evidence in every retained attempt. -/
theorem putAttempt_lookup {state : State} {attempt : Attempt}
    (consistent : stateConsistent state) (added : lookupConsistent attempt) :
    stateConsistent (putAttempt state attempt) := by
  intro a member
  simp only [putAttempt, List.mem_mergeSort, List.mem_cons, List.mem_filter] at member
  rcases member with equal | retained
  · subst a; exact added
  · exact consistent a retained.1

/-- Accepted application preserves the exact history shape reconstructed by checkpoints. -/
theorem applyRecord_lookup {state out : State} {record : Option Record}
    (consistent : stateConsistent state) (h : applyRecord state record = some out) :
    stateConsistent out := by
  unfold applyRecord at h
  split at h
  · contradiction
  · cases record with
    | none => simp at h
    | some record =>
      simp only [Option.bind_eq_bind, Option.bind_some] at h
      cases found : findAttempt state (record.key.getD default).digest with
      | none =>
        simp only [found] at h
        split at h
        · contradiction
        · split at h
          · contradiction
          · cases h
            exact putAttempt_lookup consistent rfl
      | some attempt =>
        simp only [found] at h
        split at h
        · contradiction
        · cases advanced : advanceAttempt attempt record with
          | none => simp [advanced] at h
          | some next =>
            simp only [advanced, Option.bind_some] at h
            cases h
            apply putAttempt_lookup consistent
            have previous := consistent attempt (List.mem_of_find?_eq_some found)
            change lookupConsistent next
            exact advanceAttempt_lookup previous advanced

/-- A replay from canonical evidence remains canonical at every accepted prefix. -/
theorem replayFrom_lookup {records : List (Option Record)} {state out : State} {sequence : Nat}
    (consistent : stateConsistent state) (h : replayFrom state sequence records = some out) :
    stateConsistent out := by
  induction records generalizing state sequence with
  | nil =>
    simp only [replayFrom, Option.some.injEq] at h
    cases h
    exact consistent
  | cons record records ih =>
    cases record with
    | none => simp [replayFrom] at h
    | some record =>
      simp only [replayFrom, Option.bind_eq_bind, Option.bind_some] at h
      split at h
      · contradiction
      · cases applied : applyRecord state (some record) with
        | none => simp [applied] at h
        | some next =>
          simp only [applied, Option.bind_some] at h
          exact ih (applyRecord_lookup consistent applied) h

/-- Full replay constructs canonical lookup evidence without a checkpoint assumption. -/
theorem reduceJournal_lookup {records : List (Option Record)} {out : State}
    (h : reduceJournal records = some out) : stateConsistent out :=
  replayFrom_lookup (by intro a member; contradiction) h

/-- stateSorted records Snapshot's ascending digest ordering. -/
def stateSorted (state : State) : Prop :=
  state.Pairwise (fun a b => attemptDigest a ≤ attemptDigest b)

/-- stateUnique rules out different attempts in the same digest slot. -/
def stateUnique (state : State) : Prop :=
  ∀ a ∈ state, ∀ b ∈ state, attemptDigest a = attemptDigest b → a = b

/-- Digest-slot replacement always produces Snapshot ordering. -/
theorem putAttempt_sorted (state : State) (attempt : Attempt) :
    stateSorted (putAttempt state attempt) := by
  unfold stateSorted putAttempt
  apply List.Pairwise.imp (R := fun a b => decide (attemptDigest a ≤ attemptDigest b) = true)
    (by intro a b h; simpa only [decide_eq_true_eq] using h)
  apply List.pairwise_mergeSort
  · intro a b c hab hbc
    simp only [decide_eq_true_eq] at hab hbc ⊢
    exact Std.le_trans hab hbc
  · intro a b
    simp only [Bool.or_eq_true, decide_eq_true_eq]
    exact Std.le_total

/-- Digest-slot replacement preserves the map's unique value for each digest. -/
theorem putAttempt_unique {state : State} (unique : stateUnique state) (attempt : Attempt) :
    stateUnique (putAttempt state attempt) := by
  intro a am b bm eq
  simp only [putAttempt, List.mem_mergeSort, List.mem_cons, List.mem_filter, bne_iff_ne] at am bm
  rcases am with rfl | ⟨am, aneq⟩
  · rcases bm with rfl | ⟨bm, bneq⟩
    · rfl
    · exact False.elim (bneq eq.symm)
  · rcases bm with rfl | ⟨bm, _⟩
    · exact False.elim (aneq eq)
    · exact unique a am b bm eq

/-- Every accepted reducer transition replaces one digest slot in its input map. -/
theorem applyRecord_isPut {state out : State} {record : Option Record}
    (h : applyRecord state record = some out) : ∃ a, out = putAttempt state a := by
  unfold applyRecord at h
  split at h
  · contradiction
  · cases record with
    | none => simp at h
    | some record =>
      simp only [Option.bind_eq_bind, Option.bind_some] at h
      cases found : findAttempt state (record.key.getD default).digest with
      | none =>
        simp only [found] at h
        split at h
        · contradiction
        · split at h
          · contradiction
          · cases h; exact ⟨_, rfl⟩
      | some attempt =>
        simp only [found] at h
        split at h
        · contradiction
        · cases advanced : advanceAttempt attempt record with
          | none => simp [advanced] at h
          | some next =>
            simp only [advanced, Option.bind_some] at h
            cases h; exact ⟨_, rfl⟩

/-- Accepted replay preserves sorted, unique digest slots. -/
theorem replayFrom_indexed {records : List (Option Record)} {state out : State} {sequence : Nat}
    (sorted : stateSorted state) (unique : stateUnique state)
    (h : replayFrom state sequence records = some out) : stateSorted out ∧ stateUnique out := by
  induction records generalizing state sequence with
  | nil =>
    simp only [replayFrom, Option.some.injEq] at h
    cases h
    exact ⟨sorted, unique⟩
  | cons record records ih =>
    cases record with
    | none => simp [replayFrom] at h
    | some record =>
      simp only [replayFrom, Option.bind_eq_bind, Option.bind_some] at h
      split at h
      · contradiction
      · cases applied : applyRecord state (some record) with
        | none => simp [applied] at h
        | some next =>
          simp only [applied, Option.bind_some] at h
          obtain ⟨a, shape⟩ := applyRecord_isPut applied
          subst next
          exact ih (putAttempt_sorted state a) (putAttempt_unique unique a) h

/-- Full replay produces the unique sorted representation exported by Snapshot. -/
theorem reduceJournal_indexed {records : List (Option Record)} {out : State}
    (h : reduceJournal records = some out) : stateSorted out ∧ stateUnique out :=
  replayFrom_indexed (by simp [stateSorted]) (by intro a member; contradiction) h

/-- stateDistinct prohibits duplicate digest entries, including identical attempts. -/
def stateDistinct (state : State) : Prop :=
  state.Pairwise (fun a b => attemptDigest a ≠ attemptDigest b)

/-- Replacing a digest slot preserves one list entry per map key. -/
theorem putAttempt_distinct {state : State} (distinct : stateDistinct state) (attempt : Attempt) :
    stateDistinct (putAttempt state attempt) := by
  unfold stateDistinct putAttempt
  apply (List.mergeSort_perm _ _).symm.pairwise _ (fun h => Ne.symm h)
  apply List.pairwise_cons.mpr
  constructor
  · intro a member
    simp only [List.mem_filter, bne_iff_ne] at member
    exact Ne.symm member.2
  · exact distinct.filter _

/-- Replay preserves the absence of duplicate digest entries. -/
theorem replayFrom_distinct {records : List (Option Record)} {state out : State} {sequence : Nat}
    (distinct : stateDistinct state) (h : replayFrom state sequence records = some out) : stateDistinct out := by
  induction records generalizing state sequence with
  | nil =>
    simp only [replayFrom, Option.some.injEq] at h
    cases h
    exact distinct
  | cons record records ih =>
    cases record with
    | none => simp [replayFrom] at h
    | some record =>
      simp only [replayFrom, Option.bind_eq_bind, Option.bind_some] at h
      split at h
      · contradiction
      · cases applied : applyRecord state (some record) with
        | none => simp [applied] at h
        | some next =>
          simp only [applied, Option.bind_some] at h
          obtain ⟨a, shape⟩ := applyRecord_isPut applied
          subst next
          exact ih (putAttempt_distinct distinct a) h

/-- A fresh reducer snapshot has exactly one entry for each retained digest. -/
theorem reduceJournal_distinct {records : List (Option Record)} {out : State}
    (h : reduceJournal records = some out) : stateDistinct out :=
  replayFrom_distinct (by simp [stateDistinct]) h

end Spacewave.SObject.Journal
