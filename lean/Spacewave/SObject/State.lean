import Spacewave.SObject.ResolvedOps

/-!
# SharedObject state

Mirrors `core/sobject/state.go` at Spacewave `f98a55b07` plus nonce and operation
identity corrections. Encodings, size limits, and signature structure are
projected as `format`; cryptographic verification is `Sig.valid`. These boundary
facts do not include authorization, nonce selection, ordering, or uniqueness.
`grantsValid` is the unchanged grant-validation result under this configuration;
membership transitions and the grant algorithm belong to the membership models.
All counters retain Go's uint64 arithmetic, including zero on nonce exhaustion.
Failure discards the working copy, as callers of SOState must do in Go.
-/

namespace Spacewave.SObject

/-- Root retains the authority and account nonce projection of a signed root. -/
structure Root where
  seqno : Nat
  digest : String
  format : Bool
  hasInner : Bool
  nonces : List AccountNonce
  sigs : List Sig
  deriving DecidableEq, Repr

/-- State contains the fields state.go reads or changes. -/
structure State where
  config : Config
  root : Root
  grantsValid : Bool
  ops : List Operation
  queued : List AccountNonce
  rejections : List Rejections
  deriving DecidableEq, Repr

/-- Root.validate mirrors root envelope validation, including canonical nonces. -/
def Root.validate (r : Root) : Bool :=
  r.seqno > 0 && r.format && r.hasInner && !r.sigs.isEmpty && r.sigs.length ≤ 100 &&
    r.nonces.all (·.peer != "") && decide (r.nonces.Pairwise fun a b => a.peer < b.peer)

/-- Operation.signedBy checks the crypto verifier's any-match role lookup. -/
def Operation.signedBy (o : Operation) (ps : List Participant) (roles : List Role) : Bool :=
  o.sig.valid && o.sig.signer != "" &&
    ps.any (fun p => p.peer == o.sig.signer && p.role ∈ roles)

/-- Operation.valid is a formatted operation from a writer, validator, or owner. -/
def Operation.valid (o : Operation) (c : Config) : Bool :=
  o.format && o.innerValid && o.nonce > 0 && o.nonce < seqnoLimit &&
    o.peer != "" && o.localId != "" &&
    (o.signedBy c.participants [Role.writer, Role.validator, Role.owner] && o.peer == o.sig.signer)

/-- Rejections.valid checks the group and each distinct signed rejection. -/
def Rejections.valid (g : Rejections) (c : Config) : Bool :=
  g.peer != "" && g.entries.all (fun r => r.format && r.innerValid && r.peer == g.peer &&
    r.signedBy c.participants [Role.validator, Role.owner]) &&
    decide (g.entries.map (·.nonce)).Nodup && decide (g.entries.map (·.localId)).Nodup

/-- State.validate mirrors the state validator, including queue/result identity. -/
def State.validate (s : State) : Bool :=
  s.config.validate && s.root.validate && s.grantsValid &&
    s.ops.all (·.valid s.config) &&
    decide (s.ops.Pairwise fun a b => a.peer ≠ b.peer ∨ a.nonce < b.nonce) &&
    decide (s.ops.map (fun o => (o.peer, o.localId))).Nodup &&
    s.queued.all (·.peer != "") && decide (s.queued.Pairwise fun a b => a.peer < b.peer) &&
    decide (s.rejections.Pairwise fun a b => a.peer < b.peer) &&
    s.rejections.all (fun g => g.valid s.config && g.entries.all (fun r =>
      s.ops.all (fun o => o.peer != g.peer || (o.nonce != r.nonce && o.localId != r.localId))))

/-- State.nonces collects every nonce considered by GetNextAccountNonce. -/
def State.nonces (s : State) : List AccountNonce :=
  s.queued ++ s.root.nonces ++
    (s.ops.filter (·.innerValid)).map (fun o => ⟨o.peer, o.nonce⟩) ++
    s.rejections.flatMap (fun g =>
      (g.entries.filter (·.innerValid)).map (fun r => ⟨g.peer, r.nonce⟩))

/-- getNextAccountNonce returns zero when the uint64 nonce space is exhausted. -/
def getNextAccountNonce (s : State) (peer : String) : Nat :=
  (nonceMaximum peer s.nonces + 1) % seqnoLimit

/-- validateNextRootState checks progression, nonce retention, and current authority. -/
def validateNextRootState (s : State) (next : Root) (enforce : String) : Bool :=
  next.seqno == (s.root.seqno + 1) % seqnoLimit && next.validate &&
    s.root.nonces.all (fun n => n.nonce ≤ nonceMaximum n.peer next.nonces) &&
    match validateSignatures next.hasInner next.nonces next.sigs s.config.participants with
    | none => false
    | some count => checkConsensusAcceptance s.config.mode count &&
      (enforce == "" || next.sigs.any (·.signer == enforce))

/-- raiseAccountNonce is the existing-entry loop in updateQueuedAccountNonce. -/
def raiseAccountNonce (peer : String) (nonce : Nat) : List AccountNonce → List AccountNonce
  | [] => []
  | n :: rest =>
    if n.peer == peer then {n with nonce := max n.nonce nonce} :: rest
    else n :: raiseAccountNonce peer nonce rest

/-- updateQueuedAccountNonce raises the first matching reservation or inserts it in order. -/
def updateQueuedAccountNonce (ns : List AccountNonce) (peer : String) (nonce : Nat) :
    List AccountNonce :=
  if ns.any (·.peer == peer) then
    raiseAccountNonce peer nonce ns
  else
    (ns ++ [AccountNonce.mk peer nonce]).mergeSort (fun a b => a.peer ≤ b.peer)

/-- recordRejection appends to the first submitter group unless that identity is recorded. -/
def recordRejection (gs : List Rejections) (r : Operation) : List Rejections :=
  match gs with
  | [] => [Rejections.mk r.peer [r]]
  | g :: rest =>
    if g.peer == r.peer then
      if g.entries.any (fun e => e.innerValid && e.nonce == r.nonce && e.localId == r.localId)
      then g :: rest else {g with entries := g.entries ++ [r]} :: rest
    else g :: recordRejection rest r

/-- updateRootState performs root admission, rejection recording, and pending-state pruning. -/
def updateRootState (s : State) (root : Root) (enforce : String)
    (rejected accepted : List Operation) : Option State :=
  if validateNextRootState s root enforce && rejected.all (·.innerValid) &&
      accepted.all (·.innerValid) then
    let rejections := (rejected.foldl recordRejection s.rejections).mergeSort
      (fun a b => a.peer ≤ b.peer)
    let reserved := accepted.foldl
      (fun ns o => updateQueuedAccountNonce ns o.peer o.nonce) s.queued
    let next := { s with
      root := root
      rejections := rejections
      ops := filterResolvedOperations s.ops root.nonces accepted rejections
      queued := reserved.filter (fun q =>
        !root.nonces.any (fun n => n.peer == q.peer && q.nonce ≤ n.nonce))}
    if next.validate then some next else none
  else none

/-- findOperation models parsing up to the first matching local operation ID. -/
def findOperation (peer localId : String) : List Operation → Option Bool
  | [] => some false
  | o :: rest =>
    if !o.innerValid then none
    else if o.peer == peer && o.localId == localId then some true
    else findOperation peer localId rest

/-- findRejection checks the local ID within the already selected submitter group. -/
def findRejection (localId : String) : List Operation → Option Bool
  | [] => some false
  | r :: rest =>
    if !r.innerValid then none
    else if r.localId == localId then some true
    else findRejection localId rest

/-- getOperationStatus reports whether a queued or rejected local ID is already present. -/
def getOperationStatus (s : State) (peer localId : String) : Option Bool := do
  if ← findOperation peer localId s.ops then return true
  match s.rejections.find? (·.peer == peer) with
  | none => return false
  | some g => return ← findRejection localId g.entries

/-- queueOperation authorizes a fresh next-nonce operation and reserves its nonce. -/
def queueOperation (s : State) (o : Operation) : Option State := do
  if s.ops.length ≥ 1000 || !o.valid s.config ||
      o.nonce != getNextAccountNonce s o.peer then none
  else if ← getOperationStatus s o.peer o.localId then none
  else some { s with
    ops := s.ops ++ [o]
    queued := updateQueuedAccountNonce s.queued o.peer o.nonce}

/-- clearRejection removes the first local ID, preserving parsing failures before it. -/
def clearRejection (localId : String) : List Operation → Option (List Operation × Option Nat)
  | [] => some ([], none)
  | r :: rest =>
    if !r.innerValid then none
    else if r.localId == localId then some (rest, some r.nonce)
    else do
      let (tail, nonce) ← clearRejection localId rest
      return (r :: tail, nonce)

/-- clearRejectionGroups edits only the first matching peer's group. -/
def clearRejectionGroups (peer localId : String) :
    List Rejections → Option (List Rejections × Option Nat)
  | [] => some ([], none)
  | g :: rest => do
    if g.peer == peer then
      let (entries, removed) ← clearRejection localId g.entries
      match removed with
      | none => some (g :: rest, none)
      | some nonce =>
        some ((if entries.isEmpty then rest else {g with entries} :: rest), some nonce)
    else
      let (groups, removed) ← clearRejectionGroups peer localId rest
      some (g :: groups, removed)

/-- clearOperationResult verifies the submitter's request and retains the consumed nonce. -/
def clearOperationResult (s : State) (peer localId : String) (format : Bool) (sig : Sig) :
    Option State := do
  if !format || sig.signer != peer || !sig.valid then none
  else
    let (rejections, removed) ← clearRejectionGroups peer localId s.rejections
    match removed with
    | none => some s
    | some nonce =>
      some {s with rejections, queued := updateQueuedAccountNonce s.queued peer nonce}

/-! ## Accepted root transitions -/

/-- Replaying a decoded rejection records it only once, including within one batch. -/
theorem recordRejection_idempotent (gs : List Rejections) (r : Operation)
    (valid : r.innerValid = true) :
    recordRejection (recordRejection gs r) r = recordRejection gs r := by
  induction gs with
  | nil => simp [recordRejection, valid]
  | cons g rest ih =>
    by_cases peer : g.peer == r.peer
    · by_cases recorded : g.entries.any (fun e =>
        e.innerValid && e.nonce == r.nonce && e.localId == r.localId)
      · simp [recordRejection, peer, recorded]
      · simp [recordRejection, peer, recorded, valid]
    · simp [recordRejection, peer, ih]

/-- Reservation updates keep existing peer identities and their order. -/
theorem raiseAccountNonce_peers (ns : List AccountNonce) (peer : String) (nonce : Nat) :
    (raiseAccountNonce peer nonce ns).map (·.peer) = ns.map (·.peer) := by
  induction ns with
  | nil => rfl
  | cons n rest ih =>
    by_cases h : n.peer == peer <;> simp [raiseAccountNonce, h, ih]

/-- Raising or inserting a reservation preserves strictly ordered unique peer IDs. -/
theorem updateQueuedAccountNonce_sorted {ns : List AccountNonce}
    (hs : ns.Pairwise fun a b => a.peer < b.peer) (peer : String) (nonce : Nat) :
    (updateQueuedAccountNonce ns peer nonce).Pairwise fun a b => a.peer < b.peer := by
  unfold updateQueuedAccountNonce
  split
  · have hmap : (ns.map (·.peer)).Pairwise (· < ·) := List.pairwise_map.mpr hs
    apply List.pairwise_map.mp
    rwa [raiseAccountNonce_peers]
  · rename_i absent
    let xs := ns ++ [AccountNonce.mk peer nonce]
    have nd : (xs.map (·.peer)).Nodup := by
      have old : (ns.map (·.peer)).Nodup :=
        List.pairwise_map.mpr (hs.imp (fun h => String.ne_of_lt h))
      simp only [List.any_eq_true, beq_iff_eq, not_exists, not_and] at absent
      simpa [xs, List.map_append, List.nodup_append, old, List.mem_map] using absent
    have perm := List.mergeSort_perm xs (fun a b => a.peer ≤ b.peer)
    have nd' : ((xs.mergeSort (fun a b => a.peer ≤ b.peer)).map (·.peer)).Nodup :=
      (perm.map (·.peer)).nodup_iff.mpr nd
    have sorted : (xs.mergeSort (fun a b => a.peer ≤ b.peer)).Pairwise
        (fun a b => a.peer ≤ b.peer) := by
      have h := List.pairwise_mergeSort (le := fun (a b : AccountNonce) => decide (a.peer ≤ b.peer))
        (fun a b c hab hbc => by simpa using String.le_trans (by simpa using hab) (by simpa using hbc))
        (fun a b => by simpa using String.le_total a.peer b.peer) xs
      simpa using h
    exact sorted.imp₂ (fun a b hab hne => by
      apply String.not_le.mp
      intro hba
      exact hne (String.le_antisymm hab hba)) (List.pairwise_map.mp nd')

/-- Reservation updates cannot introduce an empty peer ID. -/
theorem updateQueuedAccountNonce_nonempty {ns : List AccountNonce}
    (hs : ∀ n ∈ ns, n.peer ≠ "") {peer : String} (hp : peer ≠ "") (nonce : Nat) :
    ∀ n ∈ updateQueuedAccountNonce ns peer nonce, n.peer ≠ "" := by
  intro n hn
  unfold updateQueuedAccountNonce at hn
  split at hn
  · have hm : n.peer ∈ (raiseAccountNonce peer nonce ns).map (·.peer) := List.mem_map.mpr ⟨n, hn, rfl⟩
    rw [raiseAccountNonce_peers] at hm
    obtain ⟨old, hold, heq⟩ := List.mem_map.mp hm
    exact heq ▸ hs old hold
  · have hm := (List.mergeSort_perm (ns ++ [AccountNonce.mk peer nonce])
        (fun a b => a.peer ≤ b.peer)).mem_iff.mp hn
    simp only [List.mem_append, List.mem_singleton] at hm
    rcases hm with old | rfl
    · exact hs n old
    · exact hp

/-- An absent status means no parsed pending operation has this local identity. -/
theorem findOperation_false {ops : List Operation} {peer localId : String}
    (h : findOperation peer localId ops = some false) :
    ∀ o ∈ ops, o.innerValid = true ∧ (o.peer ≠ peer ∨ o.localId ≠ localId) := by
  induction ops with
  | nil => simp
  | cons o rest ih =>
    unfold findOperation at h
    split at h
    · contradiction
    · rename_i valid
      split at h
      · contradiction
      · rename_i different
        have hv : o.innerValid = true := by simpa using valid
        have hd : o.peer ≠ peer ∨ o.localId ≠ localId := by
          by_cases hp : o.peer = peer
          · right
            intro hl
            simp [hp, hl] at different
          · exact Or.inl hp
        simp only [List.mem_cons, forall_eq_or_imp]
        exact ⟨⟨hv, hd⟩, ih h⟩

/-- Successful queueing appends exactly the operation and reserves its selected nonce. -/
theorem queueOperation_spec {s next : State} {o : Operation}
    (h : queueOperation s o = some next) :
    o.valid s.config = true ∧ o.nonce = getNextAccountNonce s o.peer ∧
      getOperationStatus s o.peer o.localId = some false ∧
      next = { s with
        ops := s.ops ++ [o]
        queued := updateQueuedAccountNonce s.queued o.peer o.nonce } := by
  unfold queueOperation at h
  split at h
  · contradiction
  · rename_i checks
    have hc : s.ops.length < 1000 ∧ o.valid s.config = true ∧
        o.nonce = getNextAccountNonce s o.peer := by
      simpa [Bool.or_eq_true, and_assoc] using checks
    cases status : getOperationStatus s o.peer o.localId with
    | none => simp [status] at h
    | some found =>
      cases found
      · simp [status] at h
        exact ⟨hc.2.1, hc.2.2, rfl, h.symm⟩
      · simp [status] at h

/-- Sorted rejection groups have exactly one first match for each member peer. -/
theorem findRejectionGroup {gs : List Rejections}
    (sorted : gs.Pairwise fun a b => a.peer < b.peer) {g : Rejections} (member : g ∈ gs) :
    gs.find? (·.peer == g.peer) = some g := by
  induction gs with
  | nil => simp at member
  | cons a rest ih =>
    simp only [List.pairwise_cons] at sorted
    rcases List.mem_cons.mp member with rfl | hrest
    · simp
    · have different : a.peer ≠ g.peer := String.ne_of_lt (sorted.1 g hrest)
      have hd : (a.peer == g.peer) = false := by simpa using different
      simp [List.find?, hd, ih sorted.2 hrest]

/-- An absent rejection status excludes its local ID from the selected group. -/
theorem findRejection_false {rs : List Operation} {localId : String}
    (h : findRejection localId rs = some false) : ∀ r ∈ rs, r.localId ≠ localId := by
  induction rs with
  | nil => simp
  | cons r rest ih =>
    unfold findRejection at h
    split at h
    · contradiction
    · split at h
      · contradiction
      · rename_i different
        simp only [List.mem_cons, forall_eq_or_imp]
        exact ⟨by simpa using different, ih h⟩

/-- A fresh local ID is absent from both the pending queue and every rejection group. -/
theorem getOperationStatus_fresh {s : State} {peer localId : String}
    (sorted : s.rejections.Pairwise fun a b => a.peer < b.peer)
    (h : getOperationStatus s peer localId = some false) :
    (∀ o ∈ s.ops, o.peer ≠ peer ∨ o.localId ≠ localId) ∧
      (∀ g ∈ s.rejections, g.peer = peer → ∀ r ∈ g.entries, r.localId ≠ localId) := by
  unfold getOperationStatus at h
  cases pending : findOperation peer localId s.ops with
  | none => simp [pending] at h
  | some found =>
    cases found
    · simp [pending] at h
      refine ⟨fun o ho => (findOperation_false pending o ho).2, ?_⟩
      intro g hg hp
      have selected := findRejectionGroup sorted hg
      rw [hp] at selected
      rw [selected] at h
      exact findRejection_false h
    · simp [pending] at h

/-- Every successful root transition returns a validated state. -/
theorem updateRootState_valid {s next : State} {root : Root} {enforce : String}
    {rejected accepted : List Operation}
    (h : updateRootState s root enforce rejected accepted = some next) :
    next.validate = true := by
  unfold updateRootState at h
  split at h
  · dsimp only at h
    split at h
    · rename_i hv
      cases h
      exact hv
    · contradiction
  · contradiction

/-- Successful root transitions pass current-configuration root admission. -/
theorem updateRootState_admitted {s next : State} {root : Root} {enforce : String}
    {rejected accepted : List Operation}
    (h : updateRootState s root enforce rejected accepted = some next) :
    next.root = root ∧ validateNextRootState s root enforce = true := by
  unfold updateRootState at h
  split at h
  · rename_i checks
    dsimp only at h
    split at h
    · cases h
      simp only [Bool.and_eq_true] at checks
      exact ⟨rfl, checks.1.1⟩
    · contradiction
  · contradiction

/-! ## Root admission -/

/-- An accepted root has current-configuration consensus and retains committed nonces. -/
theorem validateNextRootState_spec {s : State} {next : Root} {enforce : String}
    (h : validateNextRootState s next enforce = true) :
    next.seqno = (s.root.seqno + 1) % seqnoLimit ∧ next.validate = true ∧
      (∀ n ∈ s.root.nonces, n.nonce ≤ nonceMaximum n.peer next.nonces) ∧
      ∃ count, validateSignatures next.hasInner next.nonces next.sigs
        s.config.participants = some count ∧ checkConsensusAcceptance s.config.mode count = true := by
  simp only [validateNextRootState, Bool.and_eq_true, beq_iff_eq,
    List.all_eq_true, decide_eq_true_eq] at h
  obtain ⟨⟨⟨hs, hv⟩, hn⟩, hc⟩ := h
  refine ⟨hs, hv, hn, ?_⟩
  split at hc
  · contradiction
  · rename_i count heq
    simp only [Bool.and_eq_true] at hc
    exact ⟨count, heq, hc.1⟩

/-- Root sequence advances exactly once; wrapping to zero is structurally invalid. -/
theorem validateNextRootState_seqno {s : State} {next : Root} {enforce : String}
    (bound : s.root.seqno < seqnoLimit)
    (h : validateNextRootState s next enforce = true) : next.seqno = s.root.seqno + 1 := by
  obtain ⟨hs, hv, _⟩ := validateNextRootState_spec h
  have positive : 0 < next.seqno := by
    simp only [Root.validate, Bool.and_eq_true, decide_eq_true_eq] at hv
    exact hv.1.1.1.1.1.1
  by_cases hn : s.root.seqno + 1 < seqnoLimit
  · simpa [Nat.mod_eq_of_lt hn] using hs
  · have heq : s.root.seqno + 1 = seqnoLimit := by omega
    simp [heq] at hs
    omega

/-- Root admission supplies a nonempty, distinct set of authorized valid signatures. -/
theorem validateNextRootState_consensus {s : State} {next : Root} {enforce : String}
    (h : validateNextRootState s next enforce = true) :
    next.sigs ≠ [] ∧ (next.sigs.map (·.signer)).Nodup ∧
      ∀ sig ∈ next.sigs, sig.valid = true ∧ ∃ p,
        s.config.participants.find? (·.peer == sig.signer) = some p ∧
        (p.role = Role.validator ∨ p.role = Role.owner) := by
  obtain ⟨_, _, _, count, hs, hc⟩ := validateNextRootState_spec h
  obtain ⟨hlen, hdistinct, hauth⟩ := validateSignatures_authorized hs
  have positive := (checkConsensusAcceptance_spec hc).2
  refine ⟨?_, hdistinct, hauth⟩
  intro hn
  simp [hn] at hlen
  omega

/-! ## Nonce selection -/

/-- Every matching observed nonce is bounded by the selected maximum. -/
theorem nonceMaximum_member {ns : List AccountNonce} {n : AccountNonce} (h : n ∈ ns) :
    n.nonce ≤ nonceMaximum n.peer ns := by
  induction ns with
  | nil => simp at h
  | cons a rest ih =>
    simp only [List.mem_cons] at h
    rcases h with heq | hrest
    · subst a
      simp only [nonceMaximum, beq_self_eq_true, ↓reduceIte]
      exact Nat.le_max_left ..
    · exact Nat.le_trans (ih hrest) (Nat.le_max_right ..)

/-- Before exhaustion, the next nonce strictly exceeds every observed nonce. -/
theorem getNextAccountNonce_fresh {s : State} {n : AccountNonce} (h : n ∈ s.nonces)
    (bound : nonceMaximum n.peer s.nonces + 1 < seqnoLimit) :
    n.nonce < getNextAccountNonce s n.peer := by
  have hm := nonceMaximum_member h
  simp only [getNextAccountNonce, Nat.mod_eq_of_lt bound]
  omega

/-- A bounded uint64 input list has a bounded nonce maximum. -/
theorem nonceMaximum_bounded {ns : List AccountNonce} (peer : String)
    (bounded : ∀ n ∈ ns, n.nonce < seqnoLimit) : nonceMaximum peer ns < seqnoLimit := by
  induction ns with
  | nil => exact (by decide : 0 < seqnoLimit)
  | cons n rest ih =>
    have hn := bounded n (List.mem_cons_self ..)
    have hr := ih (fun a ha => bounded a (List.mem_cons_of_mem n ha))
    simp only [nonceMaximum]
    split
    · omega
    · have hp : 0 < seqnoLimit := by decide
      omega

/-- Every decoded pending operation contributes to the next nonce. -/
theorem State.op_nonce {s : State} {o : Operation} (member : o ∈ s.ops)
    (valid : o.innerValid = true) : AccountNonce.mk o.peer o.nonce ∈ s.nonces := by
  unfold State.nonces
  apply List.mem_append_left
  apply List.mem_append_right
  exact List.mem_map.mpr ⟨o, List.mem_filter.mpr ⟨member, valid⟩, rfl⟩

/-- Every decoded rejection contributes its nonce under the group's peer. -/
theorem State.rejection_nonce {s : State} {g : Rejections} {r : Operation}
    (group : g ∈ s.rejections) (member : r ∈ g.entries) (valid : r.innerValid = true) :
    AccountNonce.mk g.peer r.nonce ∈ s.nonces := by
  unfold State.nonces
  apply List.mem_append_right
  exact List.mem_flatMap.mpr ⟨g, group,
    List.mem_map.mpr ⟨r, List.mem_filter.mpr ⟨member, valid⟩, rfl⟩⟩

/-- An accepted operation cannot wrap the nonce counter: zero is structurally invalid. -/
theorem queueOperation_fresh_nonce {s next : State} {o : Operation}
    (bounded : ∀ n ∈ s.nonces, n.nonce < seqnoLimit)
    (h : queueOperation s o = some next) : nonceMaximum o.peer s.nonces < o.nonce := by
  obtain ⟨hv, hn, _, _⟩ := queueOperation_spec h
  have positive : 0 < o.nonce := by
    simp only [Operation.valid, Bool.and_eq_true, decide_eq_true_eq] at hv
    exact hv.1.1.1.1.2
  have hb := nonceMaximum_bounded o.peer bounded
  unfold getNextAccountNonce at hn
  by_cases hnext : nonceMaximum o.peer s.nonces + 1 < seqnoLimit
  · rw [Nat.mod_eq_of_lt hnext] at hn
    omega
  · have heq : nonceMaximum o.peer s.nonces + 1 = seqnoLimit := by omega
    simp [heq] at hn
    omega

/-- Queueing preserves every validated state invariant on bounded Go counters. -/
theorem queueOperation_valid {s next : State} {o : Operation}
    (valid : s.validate = true)
    (bounded : ∀ n ∈ s.nonces, n.nonce < seqnoLimit)
    (accepted : queueOperation s o = some next) : next.validate = true := by
  have fresh := queueOperation_fresh_nonce bounded accepted
  obtain ⟨hv, _, status, rfl⟩ := queueOperation_spec accepted
  simp only [State.validate, Bool.and_eq_true, List.all_eq_true, decide_eq_true_eq,
    bne_iff_ne, and_assoc] at valid ⊢
  obtain ⟨hc, hr, hg, hops, horder, hids, hq, hqs, hgs, hgroups⟩ := valid
  obtain ⟨pendingFresh, rejectedFresh⟩ := getOperationStatus_fresh hgs status
  have peerNonempty : o.peer ≠ "" := by
    simp only [Operation.valid, Bool.and_eq_true, bne_iff_ne] at hv
    exact hv.1.1.2
  refine ⟨hc, hr, hg, ?_, ?_, ?_,
    updateQueuedAccountNonce_nonempty hq peerNonempty o.nonce,
    updateQueuedAccountNonce_sorted hqs o.peer o.nonce, hgs, ?_⟩
  · intro a ha
    rcases List.mem_append.mp ha with old | added
    · exact hops a old
    · simpa only [List.mem_singleton.mp added] using hv
  · rw [List.pairwise_append]
    refine ⟨horder, by simp, ?_⟩
    intro a ha b hb
    have heq := List.mem_singleton.mp hb
    subst b
    by_cases same : a.peer = o.peer
    · right
      have haValid : a.innerValid = true := by
        have h := hops a ha
        simp only [Operation.valid, Bool.and_eq_true] at h
        exact h.1.1.1.1.1.2
      have hm := nonceMaximum_member (State.op_nonce ha haValid)
      simp only at hm
      rw [same] at hm
      omega
    · exact Or.inl same
  · simp only [List.map_append, List.map_cons, List.map_nil, List.nodup_append,
      List.nodup_cons, List.not_mem_nil, not_false_eq_true, List.nodup_nil,
      and_self]
    refine ⟨hids, trivial, ?_⟩
    intro pair member added addedMember heq
    have addedEq := List.mem_singleton.mp addedMember
    rw [addedEq] at heq
    obtain ⟨a, ha, pairEq⟩ := List.mem_map.mp member
    rw [← pairEq] at heq
    have hf := pendingFresh a ha
    have peers := congrArg Prod.fst heq
    have ids := congrArg Prod.snd heq
    simp only at peers ids
    rw [peers, ids] at hf
    simp at hf
  · intro g member
    obtain ⟨groupValid, separated⟩ := hgroups g member
    refine ⟨groupValid, ?_⟩
    intro r hr
    intro a ha
    rcases List.mem_append.mp ha with old | added
    · exact separated r hr a old
    have addedEq := List.mem_singleton.mp added
    subst a
    by_cases same : o.peer = g.peer
    · have localFresh := rejectedFresh g member same.symm r hr
      have rValid : r.innerValid = true := by
        simp only [Rejections.valid, Bool.and_eq_true, List.all_eq_true] at groupValid
        exact (groupValid.1.1.2 r hr).1.1.2
      have hm := nonceMaximum_member (State.rejection_nonce member hr rValid)
      simp only at hm
      rw [← same] at hm
      have nonceFresh : o.nonce ≠ r.nonce := by omega
      simp [same, nonceFresh, Ne.symm localFresh]
    · simp [same]

/-- Clearing a result only deletes entries from its submitter's list. -/
theorem clearRejection_sublist {entries remaining : List Operation} {localId : String}
    {removed : Option Nat} (h : clearRejection localId entries = some (remaining, removed)) :
    remaining.Sublist entries := by
  induction entries generalizing remaining removed with
  | nil =>
    simp only [clearRejection, Option.some.injEq, Prod.mk.injEq] at h
    simp [← h.1]
  | cons r rest ih =>
    simp only [clearRejection] at h
    split at h
    · contradiction
    · split at h
      · cases h
        exact List.Sublist.cons _ (List.Sublist.refl _)
      · cases tail : clearRejection localId rest with
        | none => simp [tail] at h
        | some result =>
          obtain ⟨next, nonce⟩ := result
          simp only [tail] at h
          obtain ⟨rfl, rfl⟩ := h
          exact List.Sublist.cons_cons _ (ih tail)

/-- A cleared group keeps its peer and a subset of its original entries. -/
def Rejections.Subset (next previous : Rejections) : Prop :=
  next.peer = previous.peer ∧ next.entries.Sublist previous.entries

/-- Clearing groups keeps their order, their original entries, and the removed peer. -/
theorem clearRejectionGroups_spec {groups remaining : List Rejections} {peer localId : String}
    {removed : Option Nat}
    (h : clearRejectionGroups peer localId groups = some (remaining, removed)) :
    (remaining.map (·.peer)).Sublist (groups.map (·.peer)) ∧
    (∀ g ∈ remaining, ∃ old ∈ groups, g.Subset old) ∧
    (removed.isSome = true → ∃ g ∈ groups, g.peer = peer) := by
  induction groups generalizing remaining removed with
  | nil =>
    simp only [clearRejectionGroups, Option.some.injEq, Prod.mk.injEq] at h
    obtain ⟨rfl, rfl⟩ := h
    simp
  | cons g rest ih =>
    simp only [clearRejectionGroups] at h
    split at h
    · rename_i same
      cases cleared : clearRejection localId g.entries with
      | none => simp [cleared] at h
      | some result =>
        obtain ⟨entries, nonce⟩ := result
        have subset := clearRejection_sublist cleared
        simp only [cleared] at h
        cases nonce with
        | none =>
          cases h
          exact ⟨List.Sublist.refl _, fun a ha => ⟨a, ha, rfl, List.Sublist.refl _⟩,
            by simp⟩
        | some nonce =>
          change some ((if entries.isEmpty then rest else {g with entries} :: rest),
            some nonce) = some (remaining, removed) at h
          split at h
          · cases h
            exact ⟨List.Sublist.cons _ (List.Sublist.refl _),
              fun a ha => ⟨a, List.mem_cons_of_mem _ ha, rfl, List.Sublist.refl _⟩,
              fun _ => ⟨g, List.mem_cons_self .., by simpa using same⟩⟩
          · cases h
            refine ⟨List.Sublist.refl _, ?_,
              fun _ => ⟨g, List.mem_cons_self .., by simpa using same⟩⟩
            intro a ha
            rcases List.mem_cons.mp ha with rfl | old
            · exact ⟨g, List.mem_cons_self .., rfl, subset⟩
            · exact ⟨a, List.mem_cons_of_mem _ old, rfl, List.Sublist.refl _⟩
    · cases cleared : clearRejectionGroups peer localId rest with
      | none => simp [cleared] at h
      | some result =>
        obtain ⟨next, nonce⟩ := result
        obtain ⟨peers, members, removedPeer⟩ := ih cleared
        simp only [cleared] at h
        obtain ⟨rfl, rfl⟩ := h
        refine ⟨List.Sublist.cons_cons _ peers, ?_, ?_⟩
        · intro a ha
          rcases List.mem_cons.mp ha with same | old
          · subst a
            exact ⟨g, List.mem_cons_self .., rfl, List.Sublist.refl _⟩
          · obtain ⟨b, hb, subset⟩ := members a old
            exact ⟨b, List.mem_cons_of_mem _ hb, subset⟩
        · intro someNonce
          obtain ⟨a, ha, same⟩ := removedPeer someNonce
          exact ⟨a, List.mem_cons_of_mem _ ha, same⟩

/-- Removing entries preserves group validation and nonce/local-ID uniqueness. -/
theorem Rejections.Subset.valid {next old : Rejections} {c : Config}
    (subset : next.Subset old) (valid : old.valid c = true) : next.valid c = true := by
  obtain ⟨peers, entries⟩ := subset
  simp only [Rejections.valid, Bool.and_eq_true, List.all_eq_true, decide_eq_true_eq] at valid ⊢
  obtain ⟨⟨⟨nonempty, members⟩, nonces⟩, ids⟩ := valid
  refine ⟨⟨⟨?_, ?_⟩, (entries.map (·.nonce)).nodup nonces⟩,
    (entries.map (·.localId)).nodup ids⟩
  · simpa only [peers] using nonempty
  · intro r hr
    simpa only [peers] using members r (entries.subset hr)

/-- Clearing a signed result preserves every validated state invariant. -/
theorem clearOperationResult_valid {s next : State} {peer localId : String}
    {format : Bool} {sig : Sig} (valid : s.validate = true)
    (accepted : clearOperationResult s peer localId format sig = some next) :
    next.validate = true := by
  unfold clearOperationResult at accepted
  split at accepted
  · contradiction
  · cases cleared : clearRejectionGroups peer localId s.rejections with
    | none => simp [cleared] at accepted
    | some result =>
      obtain ⟨groups, removed⟩ := result
      simp only [cleared] at accepted
      cases removed with
      | none => cases accepted; exact valid
      | some nonce =>
        cases accepted
        obtain ⟨peers, members, removedPeer⟩ := clearRejectionGroups_spec cleared
        simp only [State.validate, Bool.and_eq_true, List.all_eq_true, decide_eq_true_eq,
          bne_iff_ne, and_assoc] at valid ⊢
        obtain ⟨hc, hr, hg, hops, horder, hids, hq, hqs, hgs, hgroups⟩ := valid
        have nonempty : peer ≠ "" := by
          obtain ⟨g, member, same⟩ := removedPeer rfl
          have groupValid := (hgroups g member).1
          simp only [Rejections.valid, Bool.and_eq_true, bne_iff_ne] at groupValid
          simpa only [same] using groupValid.1.1.1
        refine ⟨hc, hr, hg, hops, horder, hids,
          updateQueuedAccountNonce_nonempty hq nonempty nonce,
          updateQueuedAccountNonce_sorted hqs peer nonce, ?_, ?_⟩
        · exact List.pairwise_map.mp ((List.pairwise_map.mpr hgs).sublist peers)
        · intro g member
          obtain ⟨old, oldMember, subset⟩ := members g member
          obtain ⟨groupValid, separated⟩ := hgroups old oldMember
          refine ⟨subset.valid groupValid, ?_⟩
          intro r hr o ho
          simpa only [subset.1] using separated r (subset.2.subset hr) o ho

/-- Accepted operations are bound to their signing account. -/
theorem queueOperation_signer {s next : State} {o : Operation}
    (accepted : queueOperation s o = some next) : o.peer = o.sig.signer := by
  have valid := (queueOperation_spec accepted).1
  simp only [Operation.valid, Bool.and_eq_true, beq_iff_eq] at valid
  exact valid.2.2

/-- A maximum is below a bound when every matching account entry is below it. -/
theorem nonceMaximum_le {ns : List AccountNonce} {peer : String} {bound : Nat}
    (h : ∀ n ∈ ns, n.peer = peer → n.nonce ≤ bound) : nonceMaximum peer ns ≤ bound := by
  induction ns with
  | nil => exact Nat.zero_le _
  | cons n rest ih =>
    have tail := ih (fun a ha => h a (List.mem_cons_of_mem _ ha))
    simp only [nonceMaximum]
    split
    · rename_i same
      have head := h n (List.mem_cons_self ..) (by simpa using same)
      omega
    · omega

/-- Combining observations takes the maximum of their account floors. -/
theorem nonceMaximum_append (peer : String) (xs ys : List AccountNonce) :
    nonceMaximum peer (xs ++ ys) = max (nonceMaximum peer xs) (nonceMaximum peer ys) := by
  induction xs with
  | nil => simp [nonceMaximum]
  | cons n rest ih => simp [nonceMaximum, ih, Nat.max_assoc]

/-- Reordering nonce entries does not change the selected maximum. -/
theorem nonceMaximum_perm {xs ys : List AccountNonce} (h : xs.Perm ys) (peer : String) :
    nonceMaximum peer xs = nonceMaximum peer ys := by
  apply Nat.le_antisymm
  · apply nonceMaximum_le
    intro n hn same
    have bound := nonceMaximum_member (h.mem_iff.mp hn)
    simpa only [same] using bound
  · apply nonceMaximum_le
    intro n hn same
    have bound := nonceMaximum_member (h.mem_iff.mpr hn)
    simpa only [same] using bound

/-- Raising one reservation cannot lower any account's observed maximum. -/
theorem raiseAccountNonce_monotone (ns : List AccountNonce) (peer target : String)
    (nonce : Nat) :
    nonceMaximum peer ns ≤ nonceMaximum peer (raiseAccountNonce target nonce ns) := by
  induction ns with
  | nil => exact Nat.le_refl _
  | cons n rest ih =>
    simp only [raiseAccountNonce]
    split
    · simp only [nonceMaximum]
      split <;> omega
    · simp only [nonceMaximum]
      omega

/-- Updating or inserting a reservation cannot lower any account's maximum. -/
theorem updateQueuedAccountNonce_monotone (ns : List AccountNonce) (peer target : String)
    (nonce : Nat) :
    nonceMaximum peer ns ≤ nonceMaximum peer (updateQueuedAccountNonce ns target nonce) := by
  unfold updateQueuedAccountNonce
  split
  · exact raiseAccountNonce_monotone ns peer target nonce
  · rw [nonceMaximum_perm (List.mergeSort_perm ..), nonceMaximum_append]
    exact Nat.le_max_left ..

/-- Raising the first matching reservation records the new nonce. -/
theorem raiseAccountNonce_records {ns : List AccountNonce} {peer : String} (nonce : Nat)
    (found : ns.any (·.peer == peer) = true) :
    nonce ≤ nonceMaximum peer (raiseAccountNonce peer nonce ns) := by
  induction ns with
  | nil => simp at found
  | cons n rest ih =>
    simp only [raiseAccountNonce]
    split
    · rename_i same
      simp only [nonceMaximum, same, ↓reduceIte]
      omega
    · rename_i different
      have absent : (n.peer == peer) = false := by simpa using different
      have tail : rest.any (·.peer == peer) = true := by simpa [absent] using found
      have bound := ih tail
      simp only [nonceMaximum]
      omega

/-- Every reservation update records at least its supplied nonce. -/
theorem updateQueuedAccountNonce_records (ns : List AccountNonce) (peer : String)
    (nonce : Nat) : nonce ≤ nonceMaximum peer (updateQueuedAccountNonce ns peer nonce) := by
  unfold updateQueuedAccountNonce
  split
  · rename_i found
    exact raiseAccountNonce_records nonce found
  · rw [nonceMaximum_perm (List.mergeSort_perm ..), nonceMaximum_append]
    simp [nonceMaximum]
    exact Nat.le_max_right ..

/-- Queueing never lowers any account's consumed nonce floor. -/
theorem queueOperation_nonce_monotone {s next : State} {o : Operation}
    (accepted : queueOperation s o = some next) (peer : String) :
    nonceMaximum peer s.nonces ≤ nonceMaximum peer next.nonces := by
  obtain ⟨_, _, _, rfl⟩ := queueOperation_spec accepted
  have queued := updateQueuedAccountNonce_monotone s.queued peer o.peer o.nonce
  simp only [State.nonces, List.filter_append, List.map_append, nonceMaximum_append]
  omega

/-- A cleared list retains each original entry or reports its removed nonce. -/
theorem clearRejection_entries {entries remaining : List Operation} {localId : String}
    {removed : Option Nat} (h : clearRejection localId entries = some (remaining, removed)) :
    ∀ r ∈ entries, r ∈ remaining ∨ removed = some r.nonce := by
  induction entries generalizing remaining removed with
  | nil => simp
  | cons head rest ih =>
    simp only [clearRejection] at h
    split at h
    · contradiction
    · split at h
      · cases h
        intro r member
        rcases List.mem_cons.mp member with rfl | tail
        · exact Or.inr rfl
        · exact Or.inl tail
      · cases cleared : clearRejection localId rest with
        | none => simp [cleared] at h
        | some result =>
          obtain ⟨next, nonce⟩ := result
          simp only [cleared] at h
          obtain ⟨rfl, rfl⟩ := h
          intro r member
          rcases List.mem_cons.mp member with same | tail
          · exact Or.inl (List.mem_cons.mpr (Or.inl same))
          · rcases ih cleared r tail with retained | removed
            · exact Or.inl (List.mem_cons_of_mem _ retained)
            · exact Or.inr removed

/-- Every rejection survives clearing unless its nonce is returned for reservation. -/
theorem clearRejectionGroups_entries {groups remaining : List Rejections} {peer localId : String}
    {removed : Option Nat}
    (h : clearRejectionGroups peer localId groups = some (remaining, removed)) :
    ∀ g ∈ groups, ∀ r ∈ g.entries,
      (∃ next ∈ remaining, next.peer = g.peer ∧ r ∈ next.entries) ∨
        (g.peer = peer ∧ removed = some r.nonce) := by
  induction groups generalizing remaining removed with
  | nil => simp
  | cons head rest ih =>
    simp only [clearRejectionGroups] at h
    split at h
    · rename_i same
      cases cleared : clearRejection localId head.entries with
      | none => simp [cleared] at h
      | some result =>
        obtain ⟨entries, nonce⟩ := result
        simp only [cleared] at h
        cases nonce with
        | none =>
          cases h
          exact fun g hg r hr => Or.inl ⟨g, hg, rfl, hr⟩
        | some nonce =>
          change some ((if entries.isEmpty then rest else {head with entries} :: rest),
            some nonce) = some (remaining, removed) at h
          split at h
          · rename_i empty
            cases h
            intro g hg r hr
            rcases List.mem_cons.mp hg with eq | tail
            · subst g
              rcases clearRejection_entries cleared r hr with retained | removed
              · have nilEntries : entries = [] := List.isEmpty_iff.mp empty
                simp [nilEntries] at retained
              · exact Or.inr ⟨by simpa using same, removed⟩
            · exact Or.inl ⟨g, tail, rfl, hr⟩
          · cases h
            intro g hg r hr
            rcases List.mem_cons.mp hg with eq | tail
            · subst g
              rcases clearRejection_entries cleared r hr with retained | removed
              · exact Or.inl ⟨{head with entries}, List.mem_cons_self .., rfl, retained⟩
              · exact Or.inr ⟨by simpa using same, removed⟩
            · exact Or.inl ⟨g, List.mem_cons_of_mem _ tail, rfl, hr⟩
    · cases cleared : clearRejectionGroups peer localId rest with
      | none => simp [cleared] at h
      | some result =>
        obtain ⟨next, nonce⟩ := result
        simp only [cleared] at h
        obtain ⟨rfl, rfl⟩ := h
        intro g hg r hr
        rcases List.mem_cons.mp hg with eq | tail
        · subst g
          exact Or.inl ⟨head, List.mem_cons_self .., rfl, hr⟩
        · rcases ih cleared g tail r hr with retained | removed
          · obtain ⟨group, member, peerEq, entry⟩ := retained
            exact Or.inl ⟨group, List.mem_cons_of_mem _ member, peerEq, entry⟩
          · exact Or.inr removed

/-- A state's consumed floor includes each queued reservation. -/
theorem State.queued_nonce {s : State} {n : AccountNonce} (member : n ∈ s.queued) :
    n.nonce ≤ nonceMaximum n.peer s.nonces := by
  apply nonceMaximum_member
  simp only [State.nonces, List.mem_append]
  exact Or.inl (Or.inl (Or.inl member))

/-- Clearing a result never lowers any account's consumed nonce floor. -/
theorem clearOperationResult_nonce_monotone {s next : State} {peer localId : String}
    {format : Bool} {sig : Sig}
    (accepted : clearOperationResult s peer localId format sig = some next) (target : String) :
    nonceMaximum target s.nonces ≤ nonceMaximum target next.nonces := by
  unfold clearOperationResult at accepted
  split at accepted
  · contradiction
  · cases cleared : clearRejectionGroups peer localId s.rejections with
    | none => simp [cleared] at accepted
    | some result =>
      obtain ⟨groups, removed⟩ := result
      simp only [cleared] at accepted
      cases removed with
      | none => cases accepted; exact Nat.le_refl _
      | some nonce =>
        cases accepted
        have queued := updateQueuedAccountNonce_monotone s.queued target peer nonce
        have recorded := updateQueuedAccountNonce_records s.queued peer nonce
        let next := {s with
          rejections := groups
          queued := updateQueuedAccountNonce s.queued peer nonce}
        change nonceMaximum target s.nonces ≤ nonceMaximum target next.nonces
        have queuedBound : nonceMaximum target next.queued ≤ nonceMaximum target next.nonces := by
          apply nonceMaximum_le
          intro n member same
          simpa only [same] using (State.queued_nonce (s := next) member)
        apply nonceMaximum_le
        intro n member same
        simp only [State.nonces, List.mem_append] at member
        rcases member with ((reservation | root) | op) | rejection
        · have bound := nonceMaximum_member reservation
          rw [same] at bound
          exact Nat.le_trans (Nat.le_trans bound queued) queuedBound
        · have present : n ∈ next.nonces := by
            simp only [State.nonces, next, List.mem_append]
            exact Or.inl (Or.inl (Or.inr root))
          simpa only [same] using nonceMaximum_member present
        · have present : n ∈ next.nonces := by
            simp only [State.nonces, next, List.mem_append]
            exact Or.inl (Or.inr op)
          simpa only [same] using nonceMaximum_member present
        · obtain ⟨g, group, mappedMember⟩ := List.mem_flatMap.mp rejection
          obtain ⟨r, decodedMember, identity⟩ := List.mem_map.mp mappedMember
          obtain ⟨entryMember, valid⟩ := List.mem_filter.mp decodedMember
          rw [← identity] at same ⊢
          simp only at same ⊢
          rcases clearRejectionGroups_entries cleared g group r entryMember with retained | removed
          · obtain ⟨g', member, peerEq, entry⟩ := retained
            have bound := nonceMaximum_member (State.rejection_nonce (s := next) member entry valid)
            simpa only [peerEq, same] using bound
          · obtain ⟨peerEq, nonceEq⟩ := removed
            have nonceEq := Option.some.inj nonceEq
            have peerTarget : peer = target := peerEq.symm.trans same
            have bound : r.nonce ≤ nonceMaximum target next.queued := by
              simpa only [next, peerTarget, nonceEq] using recorded
            exact Nat.le_trans bound queuedBound

/-- Recording rejections preserves every already recorded operation. -/
theorem recordRejection_retains (groups : List Rejections) (r : Operation)
    {g : Rejections} (member : g ∈ groups) :
    ∃ next ∈ recordRejection groups r, next.peer = g.peer ∧ g.entries.Sublist next.entries := by
  induction groups with
  | nil => simp at member
  | cons head rest ih =>
    simp only [recordRejection]
    split
    · split
      · exact ⟨g, member, rfl, List.Sublist.refl _⟩
      · rcases List.mem_cons.mp member with same | tail
        · subst g
          exact ⟨{head with entries := head.entries ++ [r]}, List.mem_cons_self ..,
            rfl, List.sublist_append_left ..⟩
        · exact ⟨g, List.mem_cons_of_mem _ tail, rfl, List.Sublist.refl _⟩
    · rcases List.mem_cons.mp member with same | tail
      · subst g
        exact ⟨head, List.mem_cons_self .., rfl, List.Sublist.refl _⟩
      · obtain ⟨next, hn, peer, entries⟩ := ih tail
        exact ⟨next, List.mem_cons_of_mem _ hn, peer, entries⟩

/-- A rejection batch preserves every previously recorded operation. -/
theorem recordRejections_retains (rejected : List Operation) (groups : List Rejections)
    {g : Rejections} (member : g ∈ groups) :
    ∃ next ∈ rejected.foldl recordRejection groups,
      next.peer = g.peer ∧ g.entries.Sublist next.entries := by
  induction rejected generalizing groups g with
  | nil => exact ⟨g, member, rfl, List.Sublist.refl _⟩
  | cons r rest ih =>
    obtain ⟨next, hn, peer, entries⟩ := recordRejection_retains groups r member
    obtain ⟨final, hf, peer', entries'⟩ := ih _ hn
    exact ⟨final, hf, peer'.trans peer, entries.trans entries'⟩

/-- Reserving an accepted batch never lowers an existing reservation. -/
theorem reserveOperations_monotone (ops : List Operation) (ns : List AccountNonce)
    (peer : String) : nonceMaximum peer ns ≤ nonceMaximum peer
      (ops.foldl (fun ns o => updateQueuedAccountNonce ns o.peer o.nonce) ns) := by
  induction ops generalizing ns with
  | nil => exact Nat.le_refl _
  | cons o rest ih =>
    exact Nat.le_trans (updateQueuedAccountNonce_monotone ns peer o.peer o.nonce) (ih _)

/-- Each explicitly accepted operation is represented in the reserved floor. -/
theorem reserveOperations_records {ops : List Operation} (ns : List AccountNonce)
    {o : Operation} (member : o ∈ ops) : o.nonce ≤ nonceMaximum o.peer
      (ops.foldl (fun ns o => updateQueuedAccountNonce ns o.peer o.nonce) ns) := by
  induction ops generalizing ns with
  | nil => simp at member
  | cons head rest ih =>
    rcases List.mem_cons.mp member with same | tail
    · subst head
      exact Nat.le_trans (updateQueuedAccountNonce_records ns o.peer o.nonce)
        (reserveOperations_monotone rest _ o.peer)
    · exact ih _ tail

/-- A pruned reservation is still covered by the committed root. -/
theorem pruneReservations_retains (ns committed : List AccountNonce) (peer : String) :
    nonceMaximum peer ns ≤ max
      (nonceMaximum peer (ns.filter (fun q =>
        !committed.any (fun n => n.peer == q.peer && q.nonce ≤ n.nonce))))
      (nonceMaximum peer committed) := by
  apply nonceMaximum_le
  intro n member same
  by_cases covered : committed.any (fun c => c.peer == n.peer && n.nonce ≤ c.nonce) = true
  · simp only [List.any_eq_true, Bool.and_eq_true, beq_iff_eq, decide_eq_true_eq] at covered
    obtain ⟨c, hc, peerEq, bound⟩ := covered
    have floor := nonceMaximum_member hc
    rw [peerEq, same] at floor
    omega
  · have absent : committed.any (fun c => c.peer == n.peer && n.nonce ≤ c.nonce) = false :=
      by simpa using covered
    have present : n ∈ ns.filter (fun q =>
        !committed.any (fun c => c.peer == q.peer && q.nonce ≤ c.nonce)) := by
      exact List.mem_filter.mpr ⟨member, by simp only [absent, Bool.not_false]⟩
    have floor := nonceMaximum_member present
    rw [same] at floor
    omega

/-- A removed pending operation is covered by a root, accepted batch, or rejection. -/
theorem filterResolvedOperations_resolution {ops : List Operation} {ns : List AccountNonce}
    {accepted : List Operation} {rejected : List Rejections} {o : Operation}
    (member : o ∈ ops) :
    o ∈ filterResolvedOperations ops ns accepted rejected ∨
      (∃ n ∈ ns, n.peer = o.peer ∧ o.nonce ≤ n.nonce) ∨
      (∃ a ∈ accepted, a.peer = o.peer ∧ o.nonce ≤ a.nonce) ∨
      (∃ g ∈ rejected, g.peer = o.peer ∧
        ∃ r ∈ g.entries, r.innerValid = true ∧ r.nonce = o.nonce) := by
  let committed := ns.filter (·.peer != "") ++
    (accepted.filter (·.peer != "")).map (fun a => AccountNonce.mk a.peer a.nonce)
  by_cases covered : committed.any (fun n => n.peer == o.peer && o.nonce ≤ n.nonce) = true
  · simp only [List.any_eq_true, Bool.and_eq_true, beq_iff_eq, decide_eq_true_eq] at covered
    obtain ⟨n, hn, peerEq, nonce⟩ := covered
    rcases List.mem_append.mp hn with root | batch
    · exact Or.inr (Or.inl ⟨n, (List.mem_filter.mp root).1, peerEq, nonce⟩)
    · obtain ⟨a, ha, identity⟩ := List.mem_map.mp batch
      rw [← identity] at peerEq nonce
      exact Or.inr (Or.inr (Or.inl ⟨a, (List.mem_filter.mp ha).1, peerEq, nonce⟩))
  · by_cases rejectedNonce : rejected.any (fun g => g.peer != "" && g.peer == o.peer &&
        g.entries.any (fun r => r.innerValid && r.nonce == o.nonce)) = true
    · simp only [List.any_eq_true, Bool.and_eq_true, bne_iff_ne, beq_iff_eq] at rejectedNonce
      obtain ⟨g, hg, ⟨_, peerEq⟩, r, hr, valid, nonce⟩ := rejectedNonce
      exact Or.inr (Or.inr (Or.inr ⟨g, hg, peerEq, r, hr, valid, nonce⟩))
    · have notCovered : committed.any (fun n => n.peer == o.peer && o.nonce ≤ n.nonce) = false :=
        by simpa using covered
      have notRejected : rejected.any (fun g => g.peer != "" && g.peer == o.peer &&
          g.entries.any (fun r => r.innerValid && r.nonce == o.nonce)) = false :=
        by simpa using rejectedNonce
      left
      simp only [filterResolvedOperations, List.mem_filter]
      dsimp only [committed] at notCovered
      exact ⟨member, by simp only [notCovered, notRejected,
        Bool.not_false, Bool.and_true, Bool.or_true]⟩

/-- A state's consumed floor includes every committed root nonce. -/
theorem State.root_nonce {s : State} {n : AccountNonce} (member : n ∈ s.root.nonces) :
    n.nonce ≤ nonceMaximum n.peer s.nonces := by
  apply nonceMaximum_member
  simp only [State.nonces, List.mem_append]
  exact Or.inl (Or.inl (Or.inr member))

/-- The root and surviving reservations cover every reservation before pruning. -/
theorem State.reservations_cover {s : State} {reserved : List AccountNonce}
    (queued : s.queued = reserved.filter (fun q =>
      !s.root.nonces.any (fun n => n.peer == q.peer && q.nonce ≤ n.nonce))) (peer : String) :
    nonceMaximum peer reserved ≤ nonceMaximum peer s.nonces := by
  have retained := pruneReservations_retains reserved s.root.nonces peer
  rw [← queued] at retained
  have queuedBound : nonceMaximum peer s.queued ≤ nonceMaximum peer s.nonces := by
    apply nonceMaximum_le
    intro n hn same
    simpa only [same] using (State.queued_nonce (s := s) hn)
  have rootBound : nonceMaximum peer s.root.nonces ≤ nonceMaximum peer s.nonces := by
    apply nonceMaximum_le
    intro n hn same
    simpa only [same] using (State.root_nonce (s := s) hn)
  omega

/-- Accepted root updates never lower any account's consumed nonce floor. -/
theorem updateRootState_nonce_monotone {s next : State} {root : Root} {enforce : String}
    {rejected accepted : List Operation}
    (success : updateRootState s root enforce rejected accepted = some next) (peer : String) :
    nonceMaximum peer s.nonces ≤ nonceMaximum peer next.nonces := by
  have admitted := (updateRootState_admitted success).2
  have rootRetained := (validateNextRootState_spec admitted).2.2.1
  unfold updateRootState at success
  split at success
  · dsimp only at success
    split at success
    · cases success
      let groups := (rejected.foldl recordRejection s.rejections).mergeSort
        (fun a b => a.peer ≤ b.peer)
      let reserved := accepted.foldl
        (fun ns o => updateQueuedAccountNonce ns o.peer o.nonce) s.queued
      let next := {s with
        root := root
        rejections := groups
        ops := filterResolvedOperations s.ops root.nonces accepted groups
        queued := reserved.filter (fun q =>
          !root.nonces.any (fun n => n.peer == q.peer && q.nonce ≤ n.nonce))}
      change nonceMaximum peer s.nonces ≤ nonceMaximum peer next.nonces
      have reservations : ∀ p, nonceMaximum p reserved ≤ nonceMaximum p next.nonces :=
        fun p => State.reservations_cover (s := next) rfl p
      have rootBound : ∀ p, nonceMaximum p root.nonces ≤ nonceMaximum p next.nonces := by
        intro p
        apply nonceMaximum_le
        intro n member same
        simpa only [same] using (State.root_nonce (s := next) member)
      apply nonceMaximum_le
      intro n member same
      simp only [State.nonces, List.mem_append] at member
      rcases member with ((queued | committed) | pending) | rejection
      · have bound := nonceMaximum_member queued
        rw [same] at bound
        exact Nat.le_trans bound (Nat.le_trans (reserveOperations_monotone accepted _ peer)
          (reservations peer))
      · have bound := rootRetained n committed
        rw [same] at bound
        exact Nat.le_trans bound (rootBound peer)
      · obtain ⟨o, decodedMember, identity⟩ := List.mem_map.mp pending
        obtain ⟨opMember, valid⟩ := List.mem_filter.mp decodedMember
        rw [← identity] at same ⊢
        simp only at same ⊢
        rcases filterResolvedOperations_resolution (ns := root.nonces)
            (accepted := accepted) (rejected := groups) opMember with kept | committed | batch | rejectedOp
        · have bound := nonceMaximum_member (State.op_nonce (s := next) kept valid)
          simpa only [same] using bound
        · obtain ⟨c, hc, peerEq, nonce⟩ := committed
          have bound := nonceMaximum_member hc
          rw [peerEq, same] at bound
          exact Nat.le_trans nonce (Nat.le_trans bound (rootBound peer))
        · obtain ⟨a, ha, peerEq, nonce⟩ := batch
          have bound := reserveOperations_records s.queued ha
          rw [peerEq, same] at bound
          exact Nat.le_trans nonce (Nat.le_trans bound (reservations peer))
        · obtain ⟨g, hg, peerEq, r, hr, valid, nonce⟩ := rejectedOp
          have bound := nonceMaximum_member (State.rejection_nonce (s := next) hg hr valid)
          simpa only [peerEq, same, nonce] using bound
      · obtain ⟨g, hg, mappedMember⟩ := List.mem_flatMap.mp rejection
        obtain ⟨r, decodedMember, identity⟩ := List.mem_map.mp mappedMember
        obtain ⟨entryMember, valid⟩ := List.mem_filter.mp decodedMember
        rw [← identity] at same ⊢
        simp only at same ⊢
        obtain ⟨g', hg', peerEq, entries⟩ := recordRejections_retains rejected s.rejections hg
        have sortedMember : g' ∈ groups := (List.mergeSort_perm _ _).mem_iff.mpr hg'
        have bound := nonceMaximum_member
          (State.rejection_nonce (s := next) sortedMember (entries.subset entryMember) valid)
        simpa only [peerEq, same] using bound
    · contradiction
  · contradiction

/-- Zero is exhaustion, ordered after every usable uint64 nonce. -/
def nonceRank (nonce : Nat) : Nat := if nonce = 0 then seqnoLimit else nonce

/-- The ranked next nonce is exactly one above the consumed floor, even at exhaustion. -/
theorem getNextAccountNonce_rank {s : State} (peer : String)
    (bounded : ∀ n ∈ s.nonces, n.nonce < seqnoLimit) :
    nonceRank (getNextAccountNonce s peer) = nonceMaximum peer s.nonces + 1 := by
  have bound := nonceMaximum_bounded peer bounded
  by_cases usable : nonceMaximum peer s.nonces + 1 < seqnoLimit
  · simp only [getNextAccountNonce, Nat.mod_eq_of_lt usable, nonceRank, Nat.add_eq_zero_iff,
      Nat.one_ne_zero, and_false, ↓reduceIte]
  · have exhausted : nonceMaximum peer s.nonces + 1 = seqnoLimit := by omega
    simp [getNextAccountNonce, exhausted, nonceRank]

/-- Rank monotonicity expresses nonce progression without mistaking exhaustion for rollback. -/
theorem getNextAccountNonce_monotone {s next : State} (peer : String)
    (beforeBounded : ∀ n ∈ s.nonces, n.nonce < seqnoLimit)
    (afterBounded : ∀ n ∈ next.nonces, n.nonce < seqnoLimit)
    (retained : nonceMaximum peer s.nonces ≤ nonceMaximum peer next.nonces) :
    nonceRank (getNextAccountNonce s peer) ≤ nonceRank (getNextAccountNonce next peer) := by
  rw [getNextAccountNonce_rank peer beforeBounded, getNextAccountNonce_rank peer afterBounded]
  omega

/-- A preserved consumed floor cannot return from exhaustion to a usable nonce. -/
theorem getNextAccountNonce_exhaustion_retained {s next : State} (peer : String)
    (beforeBounded : ∀ n ∈ s.nonces, n.nonce < seqnoLimit)
    (afterBounded : ∀ n ∈ next.nonces, n.nonce < seqnoLimit)
    (retained : nonceMaximum peer s.nonces ≤ nonceMaximum peer next.nonces)
    (exhausted : getNextAccountNonce s peer = 0) : getNextAccountNonce next peer = 0 := by
  have monotone := getNextAccountNonce_monotone peer beforeBounded afterBounded retained
  rw [exhausted] at monotone
  have outputBound : getNextAccountNonce next peer < seqnoLimit :=
    Nat.mod_lt _ (by decide)
  by_cases exhaustedNext : getNextAccountNonce next peer = 0
  · exact exhaustedNext
  · simp only [nonceRank, exhaustedNext, ↓reduceIte] at monotone
    omega

/-- An exhausted account cannot queue any operation. -/
theorem queueOperation_exhausted {s : State} {o : Operation}
    (exhausted : getNextAccountNonce s o.peer = 0) : queueOperation s o = none := by
  cases result : queueOperation s o with
  | none => rfl
  | some next =>
    obtain ⟨valid, nonce, _, _⟩ := queueOperation_spec result
    simp only [Operation.valid, Bool.and_eq_true, decide_eq_true_eq] at valid
    have positive := valid.1.1.1.1.2
    rw [exhausted] at nonce
    omega

/-- StateStep is an accepted public state.go transition on a working copy. -/
inductive StateStep : State → State → Prop where
  | queue {s next : State} {o : Operation}
      (accepted : queueOperation s o = some next) : StateStep s next
  | root {s next : State} {r : Root} {enforce : String} {rejected accepted : List Operation}
      (admitted : updateRootState s r enforce rejected accepted = some next) : StateStep s next
  | clear {s next : State} {peer localId : String} {format : Bool} {sig : Sig}
      (accepted : clearOperationResult s peer localId format sig = some next) : StateStep s next

/-- Every accepted state transition retains each account's consumed floor. -/
theorem StateStep.nonce_monotone {s next : State} (step : StateStep s next) (peer : String) :
    nonceMaximum peer s.nonces ≤ nonceMaximum peer next.nonces := by
  cases step with
  | queue accepted => exact queueOperation_nonce_monotone accepted peer
  | root admitted => exact updateRootState_nonce_monotone admitted peer
  | clear accepted => exact clearOperationResult_nonce_monotone accepted peer

/-- Every accepted transition keeps exhausted accounts exhausted. -/
theorem StateStep.exhaustion_retained {s next : State} (step : StateStep s next) (peer : String)
    (beforeBounded : ∀ n ∈ s.nonces, n.nonce < seqnoLimit)
    (afterBounded : ∀ n ∈ next.nonces, n.nonce < seqnoLimit)
    (exhausted : getNextAccountNonce s peer = 0) : getNextAccountNonce next peer = 0 :=
  getNextAccountNonce_exhaustion_retained peer beforeBounded afterBounded
    (step.nonce_monotone peer) exhausted

/-- StateSteps composes finite sequences of accepted state transitions. -/
inductive StateSteps : State → State → Prop where
  | refl (s : State) : StateSteps s s
  | tail {first middle last : State} (steps : StateSteps first middle)
      (step : StateStep middle last) : StateSteps first last

/-- Consumed nonce floors persist across every finite accepted trace. -/
theorem StateSteps.nonce_monotone {first last : State} (steps : StateSteps first last)
    (peer : String) : nonceMaximum peer first.nonces ≤ nonceMaximum peer last.nonces := by
  induction steps with
  | refl => exact Nat.le_refl _
  | tail _ step ih => exact Nat.le_trans ih (step.nonce_monotone peer)

/-- Queueing records the admitted operation's nonce in the resulting state. -/
theorem queueOperation_consumes {s next : State} {o : Operation}
    (accepted : queueOperation s o = some next) : o.nonce ≤ nonceMaximum o.peer next.nonces := by
  obtain ⟨_, _, _, rfl⟩ := queueOperation_spec accepted
  have recorded := updateQueuedAccountNonce_records s.queued o.peer o.nonce
  simp only [State.nonces, nonceMaximum_append]
  omega

/-- A later accepted operation from the same account cannot reuse an earlier nonce. -/
theorem queueOperation_no_reuse {initial queued later final : State} {first second : Operation}
    (firstAccepted : queueOperation initial first = some queued)
    (steps : StateSteps queued later)
    (bounded : ∀ n ∈ later.nonces, n.nonce < seqnoLimit)
    (secondAccepted : queueOperation later second = some final)
    (same : first.peer = second.peer) : first.nonce < second.nonce := by
  have consumed := queueOperation_consumes firstAccepted
  have retained := steps.nonce_monotone first.peer
  have fresh := queueOperation_fresh_nonce bounded secondAccepted
  rw [same] at consumed retained
  omega

end Spacewave.SObject
