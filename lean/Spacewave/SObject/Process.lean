import Spacewave.SObject.State

/-!
# Validator operation processing

Mirrors `SOStateParticipantHandle.ProcessOperations` in
`core/sobject/participant.go` at Spacewave `c9eaf16de`. A validator decodes each
queued operation, rejects those whose data does not decode, passes the rest to
the application callback and builds the next root from its results. Rejections
from decoding advance the root so the rejected operations leave the queue.

Transform lookup, data decoding, root decoding, root encoding and signing
outcomes are inputs, as are the application's state change and its
per-operation results. A rejection is identified by the submitter, nonce and
local ID its signature binds; conformance compares those identities.
-/

namespace Spacewave.SObject

/-- ProcessOp is one queued operation with the outcome of decoding its data. -/
structure ProcessOp where
  op : Operation
  decodeOK : Bool
  deriving DecidableEq, Repr

/-- OpResult is one application result: absent, malformed, or a reference with its outcome. -/
structure OpResult where
  present : Bool
  refValid : Bool
  peer : String
  nonce : Nat
  accepted : Bool
  deriving DecidableEq, Repr

/-- RejectionRef is the operation identity a signed rejection binds. -/
structure RejectionRef where
  peer : String
  nonce : Nat
  localId : String
  deriving DecidableEq, Repr

/-- ProcessInput supplies one validator pass: identity, queue, primitives and application output. -/
structure ProcessInput where
  validator : String
  transformOK : Bool
  ops : List ProcessOp
  rootOK : Bool
  callbackOK : Bool
  changed : Bool
  results : List OpResult
  signOK : Bool
  deriving DecidableEq, Repr

/-- Processed is the root decision: whether it advances, its results and its account nonces. -/
structure Processed where
  advance : Bool
  rejected : List RejectionRef
  accepted : List Operation
  nonces : List AccountNonce
  deriving DecidableEq, Repr

/-- decodeRejections rejects, in queue order, operations whose data does not decode. -/
def decodeRejections (ops : List ProcessOp) : List RejectionRef :=
  (ops.filter (!·.decodeOK)).map fun p => ⟨p.op.peer, p.op.nonce, p.op.localId⟩

/-- matchOperation returns the first decoded operation for a submitter and nonce. -/
def matchOperation (ops : List ProcessOp) (peer : String) (nonce : Nat) : Option Operation :=
  (ops.find? fun p => p.decodeOK && p.op.peer == peer && p.op.nonce == nonce).map (·.op)

/-- applyResults mirrors the result loop; accepted operations raise the root's account nonces. -/
def applyResults (ops : List ProcessOp) : List OpResult → Processed → Option Processed
  | [], p => some p
  | r :: rest, p =>
    if !r.present then applyResults ops rest p
    else if !r.refValid then none
    else
      match matchOperation ops r.peer r.nonce with
      | none => applyResults ops rest p
      | some o =>
        if r.accepted then
          applyResults ops rest {p with
            accepted := p.accepted ++ [o]
            nonces := updateQueuedAccountNonce p.nonces r.peer r.nonce}
        else
          applyResults ops rest {p with rejected := p.rejected ++ [⟨r.peer, r.nonce, o.localId⟩]}

/-- processOperations builds the next root decision, or keeps the current root when nothing
happened. -/
def processOperations (s : State) (input : ProcessInput) : Option Processed := do
  let participant ← s.config.participants.find? (·.peer == input.validator)
  if !decide (participant.role ∈ [Role.validator, Role.owner]) || !input.transformOK ||
      !input.ops.all (·.op.innerValid) || !input.rootOK || !input.callbackOK then none
  else
    let rejected := decodeRejections input.ops
    if !input.changed && input.results.isEmpty && rejected.isEmpty then
      some ⟨false, [], [], s.root.nonces⟩
    else
      let p ← applyResults input.ops input.results ⟨true, rejected, [], s.root.nonces⟩
      if input.signOK then some p else none

/-! ## Result accounting -/

/-- ResultsInvariant relates a partial result to its starting point and the decoded queue. -/
def ResultsInvariant (ops : List ProcessOp) (start p : Processed) : Prop :=
  p.advance = start.advance ∧ start.rejected <+: p.rejected ∧
    (∀ peer, nonceMaximum peer start.nonces ≤ nonceMaximum peer p.nonces) ∧
    (∀ o ∈ p.accepted,
      o ∈ start.accepted ∨ ∃ po ∈ ops, po.op = o ∧ po.decodeOK = true) ∧
    ((∀ o ∈ start.accepted, o.nonce ≤ nonceMaximum o.peer start.nonces) →
      ∀ o ∈ p.accepted, o.nonce ≤ nonceMaximum o.peer p.nonces)

/-- A matched operation was decoded and carries the referenced submitter and nonce. -/
theorem matchOperation_spec {ops : List ProcessOp} {peer : String} {nonce : Nat} {o : Operation}
    (h : matchOperation ops peer nonce = some o) :
    (∃ po ∈ ops, po.op = o ∧ po.decodeOK = true) ∧ o.peer = peer ∧ o.nonce = nonce := by
  unfold matchOperation at h
  cases found : ops.find? (fun p => p.decodeOK && p.op.peer == peer && p.op.nonce == nonce) with
  | none => simp [found] at h
  | some po =>
    simp only [found, Option.map_some, Option.some.injEq] at h
    subst o
    have member := List.mem_of_find?_eq_some found
    have matched := List.find?_some found
    simp only [Bool.and_eq_true, beq_iff_eq] at matched
    exact ⟨⟨po, member, rfl, matched.1.1⟩, matched.1.2, matched.2⟩

/-- The result loop only appends decisions and never lowers an account nonce. -/
theorem applyResults_invariant {ops : List ProcessOp} {results : List OpResult}
    {start out : Processed} (h : applyResults ops results start = some out) :
    ResultsInvariant ops start out := by
  induction results generalizing start with
  | nil =>
    cases h
    exact ⟨rfl, List.prefix_refl _, fun _ => Nat.le_refl _, fun o m => .inl m, id⟩
  | cons r rest ih =>
    unfold applyResults at h
    split at h
    · exact ih h
    · split at h
      · contradiction
      · split at h
        · exact ih h
        · rename_i o matched
          obtain ⟨decoded, samePeer, sameNonce⟩ := matchOperation_spec matched
          split at h
          · obtain ⟨advance, rejected, nonces, accepted, recorded⟩ := ih h
            refine ⟨advance, rejected,
              fun peer => Nat.le_trans (updateQueuedAccountNonce_monotone _ peer _ _) (nonces peer),
              fun o' m => ?_, fun before => recorded ?_⟩
            · rcases accepted o' m with old | fresh
              · simp only [List.mem_append, List.mem_singleton] at old
                rcases old with old | rfl
                · exact .inl old
                · exact .inr decoded
              · exact .inr fresh
            · intro o' m
              simp only [List.mem_append, List.mem_singleton] at m
              rcases m with old | rfl
              · exact Nat.le_trans (before o' old) (updateQueuedAccountNonce_monotone _ _ _ _)
              · rw [samePeer, sameNonce]
                exact updateQueuedAccountNonce_records _ _ _
          · obtain ⟨advance, rejected, nonces, accepted, recorded⟩ := ih h
            exact ⟨advance, (List.prefix_append _ _).trans rejected, nonces, accepted, recorded⟩

/-- A pass that keeps the current root reports no results and keeps its nonces. -/
theorem processOperations_noop {s : State} {input : ProcessInput} {p : Processed}
    (h : processOperations s input = some p) (kept : p.advance = false) :
    p.rejected = [] ∧ p.accepted = [] ∧ p.nonces = s.root.nonces := by
  unfold processOperations at h
  cases found : s.config.participants.find? (·.peer == input.validator) with
  | none => simp [found] at h
  | some participant =>
    simp only [found, Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · contradiction
    · split at h
      · cases h; exact ⟨rfl, rfl, rfl⟩
      · cases applied : applyResults input.ops input.results
            ⟨true, decodeRejections input.ops, [], s.root.nonces⟩ with
        | none => simp [applied] at h
        | some q =>
          simp only [applied, Option.bind_some] at h
          split at h
          · cases h
            have := (applyResults_invariant applied).1
            simp_all
          · contradiction

/-- Operations rejected at decode always advance the root, so they leave the queue. -/
theorem processOperations_decode_advance {s : State} {input : ProcessInput} {p : Processed}
    (h : processOperations s input = some p) (failed : ∃ po ∈ input.ops, po.decodeOK = false) :
    p.advance = true ∧ decodeRejections input.ops <+: p.rejected := by
  have nonempty : decodeRejections input.ops ≠ [] := by
    obtain ⟨po, member, bad⟩ := failed
    intro empty
    have rejected : po ∈ input.ops.filter (!·.decodeOK) :=
      List.mem_filter.mpr ⟨member, by simp [bad]⟩
    rw [List.map_eq_nil_iff.mp empty] at rejected
    simp at rejected
  unfold processOperations at h
  cases found : s.config.participants.find? (·.peer == input.validator) with
  | none => simp [found] at h
  | some participant =>
    simp only [found, Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · contradiction
    · split at h
      · rename_i unchanged
        simp [List.isEmpty_iff, nonempty] at unchanged
      · cases applied : applyResults input.ops input.results
            ⟨true, decodeRejections input.ops, [], s.root.nonces⟩ with
        | none => simp [applied] at h
        | some q =>
          simp only [applied, Option.bind_some] at h
          split at h
          · cases h
            obtain ⟨advance, rejected, _⟩ := applyResults_invariant applied
            exact ⟨advance, rejected⟩
          · contradiction

/-- An advancing root keeps every prior account nonce, accepts only decoded queued operations,
and records each accepted operation's nonce, so root admission resolves it. -/
theorem processOperations_accepted {s : State} {input : ProcessInput} {p : Processed}
    (h : processOperations s input = some p) :
    (∀ peer, nonceMaximum peer s.root.nonces ≤ nonceMaximum peer p.nonces) ∧
      (∀ o ∈ p.accepted, ∃ po ∈ input.ops, po.op = o ∧ po.decodeOK = true) ∧
      ∀ o ∈ p.accepted, o.nonce ≤ nonceMaximum o.peer p.nonces := by
  unfold processOperations at h
  cases found : s.config.participants.find? (·.peer == input.validator) with
  | none => simp [found] at h
  | some participant =>
    simp only [found, Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · contradiction
    · split at h
      · cases h
        exact ⟨fun _ => Nat.le_refl _, by simp, by simp⟩
      · cases applied : applyResults input.ops input.results
            ⟨true, decodeRejections input.ops, [], s.root.nonces⟩ with
        | none => simp [applied] at h
        | some q =>
          simp only [applied, Option.bind_some] at h
          split at h
          · cases h
            obtain ⟨_, _, nonces, accepted, recorded⟩ := applyResults_invariant applied
            refine ⟨nonces, fun o m => ?_, recorded (by simp)⟩
            rcases accepted o m with old | fresh
            · simp at old
            · exact fresh
          · contradiction

end Spacewave.SObject
