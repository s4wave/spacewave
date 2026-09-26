import Spacewave.SObject.State

/-!
# SharedObject host acceptance

Mirrors `core/sobject/host.go` with historical-root retention and snapshot
nonce-progress corrections. `importPeerSnapshot`, `applyConfigChange`,
`installInviteSnapshot`, and `hostUpdateRootState` clone before admission.
Ordinary failure is `none`; committed revocation is a successful `HostResult`
with `revoked = true`, matching Go's write followed by ErrParticipantRevoked.

Cryptographic and encoding boundaries are inherited from State. Byte sizes are
the real serialized input sizes. Lock, access callback and persistence outcomes
are explicit inputs. Access validation is assumed read-only. The provider must
publish nothing on a failed write; Phase 5 checks the persistent implementation.
Configuration callbacks are trusted local code, represented as State → Option
State; theorems state their preservation contracts as hypotheses. Invitation
installation requires externally authenticated owner consent and intentionally
replaces, rather than extends, the old checkpoint.
-/

namespace Spacewave.SObject

/-- HostResult separates committed state, revocation notification and publication. -/
structure HostResult where
  state : State
  revoked : Bool
  wrote : Bool
  deriving DecidableEq, Repr

/-- Root.sameContent is Go's equality check for an equal-sequence root. -/
def Root.sameContent (a b : Root) : Bool :=
  a.content == b.content && a.nonces == b.nonces

/-- rootAuthorized checks all signatures and consensus against one configuration. -/
def rootAuthorized (c : Config) (r : Root) : Bool :=
  match validateSignatures r.hasInner r.nonces r.sigs c.participants with
  | none => false
  | some count => checkConsensusAcceptance c.mode count

/-- lastAccountNonce mirrors the map built from a candidate root, including duplicates. -/
def lastAccountNonce (peer : String) (ns : List AccountNonce) : Nat :=
  ((ns.reverse.find? (·.peer == peer)).map (·.nonce)).getD 0

/-- importRoot retains historical proofs and authenticates strictly newer content. -/
def importRoot (previous candidate : Root) (config : Config) : Option Root :=
  if candidate.seqno < previous.seqno then none
  else if candidate.seqno = previous.seqno then
    if candidate.sameContent previous then some previous else none
  else if !candidate.hasInner then some candidate
  else if previous.nonces.all (fun n => n.nonce ≤ lastAccountNonce n.peer candidate.nonces) &&
      rootAuthorized config candidate then some candidate
  else none

/-- replaySnapshotOps rebuilds local reservations using ordinary queue admission. -/
def replaySnapshotOps (s : State) (ops : List Operation) : Option State :=
  ops.foldlM queueOperation s

/-- mergeLocalOperation retains one local operation precisely when queueing admits it. -/
def mergeLocalOperation (s : State) (o : Operation) : Option State := do
  if !o.innerValid then none
  else if ← getOperationStatus s o.peer o.localId then some s
  else some ((queueOperation s o).getD s)

/-- mergeLocalOps visits pending local operations in their original order. -/
def mergeLocalOps (s : State) (ops : List Operation) : Option State :=
  ops.foldlM mergeLocalOperation s

/-- readableBy uses Go's any-match known-role check. -/
def readableBy (config : Config) (peer : String) : Bool :=
  config.participants.any fun p => p.peer == peer &&
    p.role ∈ [Role.reader, Role.writer, Role.validator, Role.owner]

/-- publishHost represents atomic provider publication. -/
def publishHost (next : State) (revoked writeOK : Bool) : Option HostResult :=
  if writeOK then some ⟨next, revoked, true⟩ else none

/-- visibleHost makes unchanged live state explicit for an ordinary rejection. -/
def visibleHost (previous : State) (result : Option HostResult) : HostResult :=
  result.getD ⟨previous, false, false⟩

/-- prepareReadableSnapshot authenticates and rebuilds a readable candidate before publication. -/
def prepareReadableSnapshot (previous candidate : State) (accessOK : Bool) : Option State := do
  let root ← importRoot previous.root candidate.root candidate.config
  let next := {candidate with root}
  if !next.validate || !accessOK then none
  else
    let replayed ← replaySnapshotOps
      {next with invites := previous.invites, ops := [], queued := []} next.ops
    mergeLocalOps replayed previous.ops

/--
unappliedEntries drops the proof prefix through the held checkpoint's head when
the proof does not already link to it.
-/
def unappliedEntries (head : String) (entries : List Entry) : List Entry :=
  match entries with
  | [] => []
  | e :: _ =>
    if e.prev = head then entries
    else match entries.findIdx? (·.hash == head) with
      | some i => entries.drop (i + 1)
      | none => entries

/-- A proof that already verifies from the checkpoint is used unchanged. -/
theorem unappliedEntries_of_verifySuffix {cur cand : Config} {entries : List Entry}
    (h : verifySuffix cur cand entries = true) : unappliedEntries cur.hash entries = entries := by
  cases entries with
  | nil => rfl
  | cons e rest =>
    by_cases linked : e.prev = cur.hash
    · simp [unappliedEntries, linked]
    · have step : suffixStep cur e = none := by
        unfold suffixStep verifyChange
        split <;> (try split) <;> simp [linked]
      simp [verifySuffix, applySuffix, List.foldlM, step] at h

/-- Trimming keeps only entries from the original proof. -/
theorem mem_of_mem_unappliedEntries {head : String} {entries : List Entry} {e : Entry}
    (h : e ∈ unappliedEntries head entries) : e ∈ entries := by
  unfold unappliedEntries at h
  split at h
  · simp at h
  · split at h
    · exact h
    · split at h
      · exact List.mem_of_mem_drop h
      · exact h

/-- importPeerSnapshot mirrors bounds, chain/root/access checks, replay and write. -/
def importPeerSnapshot (previous candidate : State) (entries : List Entry)
    (localPeer : String) (candidateBytes historyBytes : Nat)
    (lockOK accessOK writeOK : Bool) : Option HostResult := do
  if candidateBytes > 10 * 1024 * 1024 || entries.length > 4096 ||
      historyBytes > 8 * 1024 * 1024 || !lockOK then none
  else if !verifySuffix previous.config candidate.config
      (unappliedEntries previous.config.hash entries) then none
  else if !readableBy candidate.config localPeer then
    if (unappliedEntries previous.config.hash entries).isEmpty then some ⟨previous, true, false⟩
    else publishHost {previous with
      config := candidate.config
      grants := []
      ops := []
      queued := []
      rejections := []} true writeOK
  else
    let merged ← prepareReadableSnapshot previous candidate accessOK
    if merged = previous then some ⟨previous, false, false⟩
    else publishHost merged false writeOK

/-- applyConfigChange includes the optional local callback in the same atomic write. -/
def applyConfigChange (previous : State) (entry : Option Entry)
    (callback : State → Option State) (lockOK writeOK : Bool) : Option HostResult := do
  let entry ← entry
  if !lockOK then none
  else
    let config ← verifyChange previous.config entry
    let next ← callback {previous with config}
    publishHost next false writeOK

/-- installInviteSnapshot validates an externally authenticated replacement checkpoint. -/
def installInviteSnapshot (candidate : State) (checkpoint lockOK writeOK : Bool) :
    Option HostResult :=
  if checkpoint && candidate.config.hash != "" && candidate.validate &&
      rootAuthorized candidate.config candidate.root && lockOK then
    publishHost candidate false writeOK
  else none

/-- hostUpdateRootState discards a failed working copy before provider publication. -/
def hostUpdateRootState (previous : State) (root : Root) (enforce : String)
    (rejected accepted : List Operation) (lockOK writeOK : Bool) : Option HostResult := do
  if !lockOK then none
  else
    let next ← updateRootState previous root enforce rejected accepted
    publishHost next false writeOK

/-- Queue reconstruction cannot edit authority or local invitation capabilities. -/
def State.hostFrame (s : State) : Config × Root × List Invite :=
  (s.config, s.root, s.invites)

/-- sameCheckpoint compares consensus content, independently of historical signature bytes. -/
def sameCheckpoint (a b : State) : Bool :=
  a.config.same b.config && a.root.seqno == b.root.seqno && a.root.sameContent b.root

/-! ## Admission and publication -/

/-- Ordinary rejection never publishes a working copy. -/
theorem visibleHost_rejected (previous : State) :
    visibleHost previous none = ⟨previous, false, false⟩ := rfl

/-- Provider failure cannot publish an accepted working copy under its atomic contract. -/
theorem publishHost_failure (next : State) (revoked : Bool) :
    publishHost next revoked false = none := rfl

/-- Publication retains exactly the prepared state and notification. -/
theorem publishHost_spec {next : State} {revoked writeOK : Bool} {out : HostResult}
    (h : publishHost next revoked writeOK = some out) :
    out.state = next ∧ out.revoked = revoked ∧ out.wrote = true := by
  unfold publishHost at h
  split at h
  · cases h; exact ⟨rfl, rfl, rfl⟩
  · contradiction

/-- Root import cannot decrease the sequence and either retains proof or authenticates progress. -/
theorem importRoot_spec {previous candidate accepted : Root} {config : Config}
    (h : importRoot previous candidate config = some accepted) :
    previous.seqno ≤ accepted.seqno ∧
    accepted.seqno = candidate.seqno ∧
    accepted.sameContent candidate = true ∧
    (accepted = previous ∨ (accepted = candidate ∧
      (candidate.hasInner = true → rootAuthorized config candidate = true ∧
        previous.nonces.all
          (fun n => n.nonce ≤ lastAccountNonce n.peer candidate.nonces) = true))) := by
  unfold importRoot at h
  split at h
  · contradiction
  · rename_i ordered
    split at h
    · rename_i equalSeq
      split at h
      · rename_i content
        cases h
        simp only [Root.sameContent, Bool.and_eq_true, beq_iff_eq] at content ⊢
        exact ⟨by omega, equalSeq.symm, ⟨content.1.symm, content.2.symm⟩, Or.inl trivial⟩
      · contradiction
    · split at h
      · rename_i empty
        cases h
        refine ⟨by omega, rfl, by simp [Root.sameContent], Or.inr ⟨rfl, ?_⟩⟩
        intro present
        simp [present] at empty
      · split at h
        · rename_i authority
          cases h
          simp only [Bool.and_eq_true] at authority
          exact ⟨by omega, rfl, by simp [Root.sameContent],
            Or.inr ⟨rfl, fun _ => ⟨authority.2, authority.1⟩⟩⟩
        · contradiction

/-- Equal-sequence different content is always rejected on the readable path. -/
theorem importRoot_conflict {previous candidate : Root} (config : Config)
    (seq : candidate.seqno = previous.seqno) (different : candidate.sameContent previous = false) :
    importRoot previous candidate config = none := by
  simp [importRoot, seq, different]

/-- A retained historical root carries any authority witness its checkpoint supplied. -/
theorem importRoot_historical {previous candidate : Root} (oldConfig newConfig : Config)
    (seq : candidate.seqno = previous.seqno) (content : candidate.sameContent previous = true)
    (authority : rootAuthorized oldConfig previous = true) :
    importRoot previous candidate newConfig = some previous ∧
      rootAuthorized oldConfig previous = true := by
  exact ⟨by simp [importRoot, seq, content], authority⟩

/-! ## Pending operation reconstruction -/

/-- Successful queueing changes only pending operations and their nonce reservations. -/
theorem queueOperation_hostFrame {s next : State} {o : Operation}
    (h : queueOperation s o = some next) : next.hostFrame = s.hostFrame := by
  obtain ⟨_, _, _, shape⟩ := queueOperation_spec h
  simp [shape, State.hostFrame]

/-- One local step either skips a known/inadmissible operation or appends the admitted one. -/
theorem mergeLocalOperation_spec {s next : State} {o : Operation}
    (h : mergeLocalOperation s o = some next) :
    o.innerValid = true ∧
      ((next = s ∧ (getOperationStatus s o.peer o.localId = some true ∨
        (getOperationStatus s o.peer o.localId = some false ∧ queueOperation s o = none))) ∨
      (getOperationStatus s o.peer o.localId = some false ∧
        queueOperation s o = some next ∧ next.ops = s.ops ++ [o])) := by
  unfold mergeLocalOperation at h
  split at h
  · contradiction
  · rename_i valid
    have inner : o.innerValid = true := by simpa using valid
    refine ⟨inner, ?_⟩
    cases status : getOperationStatus s o.peer o.localId with
    | none => simp [status] at h
    | some found =>
      cases found
      · simp only [status] at h
        cases queued : queueOperation s o with
        | none =>
          simp [queued] at h
          exact Or.inl ⟨h.symm, Or.inr ⟨rfl, rfl⟩⟩
        | some accepted =>
          simp [queued] at h
          subst next
          obtain ⟨_, _, _, shape⟩ := queueOperation_spec queued
          exact Or.inr ⟨rfl, rfl, by simp [shape]⟩
      · simp [status] at h
        exact Or.inl ⟨h.symm, Or.inl rfl⟩

/-- Decoded local operations cannot fail import through a failed queue attempt. -/
theorem mergeLocalOperation_admissible {s : State} {o : Operation}
    (valid : o.innerValid = true) (fresh : getOperationStatus s o.peer o.localId = some false) :
    mergeLocalOperation s o = some ((queueOperation s o).getD s) := by
  simp [mergeLocalOperation, valid, fresh]

/-- Local replay preserves the selected root, configuration and invitations. -/
theorem mergeLocalOperation_hostFrame {s next : State} {o : Operation}
    (h : mergeLocalOperation s o = some next) : next.hostFrame = s.hostFrame := by
  obtain ⟨_, skipped | admitted⟩ := mergeLocalOperation_spec h
  · rw [skipped.1]
  · exact queueOperation_hostFrame admitted.2.1

/-- A fold preserves a projection when every accepted step preserves it. -/
theorem foldlM_preserves {α β γ : Type} (f : α → β → Option α) (projection : α → γ)
    (step : ∀ s x next, f s x = some next → projection next = projection s)
    (xs : List β) {s next : α} (h : xs.foldlM f s = some next) :
    projection next = projection s := by
  induction xs generalizing s with
  | nil => simpa using congrArg (Option.map projection) h.symm
  | cons x xs ih =>
    simp only [List.foldlM_cons] at h
    cases first : f s x with
    | none => simp [first] at h
    | some mid =>
      simp only [first, Option.bind_eq_bind, Option.bind_some] at h
      exact (ih h).trans (step s x mid first)

/-- Rebuilding the remote queue preserves its admitted authority. -/
theorem replaySnapshotOps_hostFrame {s next : State} {ops : List Operation}
    (h : replaySnapshotOps s ops = some next) : next.hostFrame = s.hostFrame :=
  foldlM_preserves queueOperation State.hostFrame
    (fun _ _ _ h => queueOperation_hostFrame h) ops h

/-- Merging the entire local queue preserves its admitted authority. -/
theorem mergeLocalOps_hostFrame {s next : State} {ops : List Operation}
    (h : mergeLocalOps s ops = some next) : next.hostFrame = s.hostFrame :=
  foldlM_preserves mergeLocalOperation State.hostFrame
    (fun _ _ _ h => mergeLocalOperation_hostFrame h) ops h

/-- Later local replay never removes an operation that was already admitted. -/
theorem mergeLocalOps_retains {s next : State} {ops : List Operation}
    (h : mergeLocalOps s ops = some next) : s.ops.Sublist next.ops := by
  induction ops generalizing s with
  | nil => simp [mergeLocalOps] at h; subst next; exact List.Sublist.refl _
  | cons o ops ih =>
    simp only [mergeLocalOps, List.foldlM_cons] at h
    cases first : mergeLocalOperation s o with
    | none => simp [first] at h
    | some mid =>
      simp only [first, Option.bind_eq_bind, Option.bind_some] at h
      have tail := ih h
      obtain ⟨_, skipped | admitted⟩ := mergeLocalOperation_spec first
      · simpa [skipped.1] using tail
      · have retained : s.ops.Sublist mid.ops := by
          rw [admitted.2.2]
          exact List.sublist_append_left ..
        exact retained.trans tail

/-- An admissible local operation survives the entire remaining import replay. -/
theorem mergeLocalOps_admitted_survives {s admitted next : State} {o : Operation}
    {rest : List Operation} (queued : queueOperation s o = some admitted)
    (tail : mergeLocalOps admitted rest = some next) : o ∈ next.ops := by
  obtain ⟨_, _, _, shape⟩ := queueOperation_spec queued
  apply (mergeLocalOps_retains tail).subset
  simp [shape]

/-! ## Configuration and root progress -/

/-- Every accepted suffix is also a sequence of individually verified changes. -/
theorem applySuffix_verified {es : List Entry} {cur next : Config}
    (h : applySuffix cur es = some next) : es.foldlM verifyChange cur = some next := by
  induction es generalizing cur with
  | nil => exact h
  | cons e es ih =>
    simp only [applySuffix, List.foldlM_cons] at h ⊢
    cases first : suffixStep cur e with
    | none => simp [first] at h
    | some mid =>
      simp only [first, Option.bind_eq_bind, Option.bind_some] at h
      have checked : verifyChange cur e = some mid := by
        unfold suffixStep at first
        split at first
        · exact first
        · contradiction
      simp only [checked, Option.bind_eq_bind, Option.bind_some]
      exact ih h

/-- Nonempty entry hashes make a verified suffix advance exactly its length. -/
theorem verifySuffix_seqno {previous candidate : Config} {entries : List Entry}
    (hashes : ∀ e ∈ entries, e.hash ≠ "")
    (h : verifySuffix previous candidate entries = true) :
    candidate.seqno = previous.seqno + entries.length := by
  simp only [verifySuffix, Bool.and_eq_true] at h
  obtain ⟨⟨checkpoint, _⟩, suffix⟩ := h
  have checkpoint' : previous.hash ≠ "" := by simpa using checkpoint
  cases applied : applySuffix previous entries with
  | none => simp [applied] at suffix
  | some next =>
    simp only [applied] at suffix
    have progressed := foldlM_verifyChange_seqno checkpoint' hashes (applySuffix_verified applied)
    simp only [Config.same, Bool.and_eq_true, beq_iff_eq, List.isPerm_iff] at suffix
    exact suffix.1.2 ▸ progressed.1

/-- Readable preparation preserves candidate authority and locally administered invitations. -/
theorem prepareReadableSnapshot_spec {previous candidate next : State} {accessOK : Bool}
    (h : prepareReadableSnapshot previous candidate accessOK = some next) :
    ∃ root, importRoot previous.root candidate.root candidate.config = some root ∧
      next.config = candidate.config ∧ next.root = root ∧ next.invites = previous.invites := by
  unfold prepareReadableSnapshot at h
  cases rootResult : importRoot previous.root candidate.root candidate.config with
  | none => simp [rootResult] at h
  | some root =>
    simp only [rootResult, Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · contradiction
    · cases replay : replaySnapshotOps
        {candidate with root, invites := previous.invites, ops := [], queued := []}
        candidate.ops with
      | none => simp [replay] at h
      | some rebuilt =>
        simp only [replay, Option.bind_some] at h
        have frame := (mergeLocalOps_hostFrame h).trans (replaySnapshotOps_hostFrame replay)
        simp only [State.hostFrame, Prod.mk.injEq] at frame
        exact ⟨root, rfl, frame.1, frame.2.1, frame.2.2⟩

/-- Host imports only configurations authenticated by the held checkpoint. -/
theorem importPeerSnapshot_authority {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} {lockOK accessOK writeOK : Bool} {out : HostResult}
    (h : importPeerSnapshot previous candidate entries peer bytes history
      lockOK accessOK writeOK = some out) :
    verifySuffix previous.config candidate.config
      (unappliedEntries previous.config.hash entries) = true := by
  unfold importPeerSnapshot at h
  split at h
  · contradiction
  · split at h
    · contradiction
    · rename_i authorized
      simpa using authorized

/-- Imports retain state or commit the candidate configuration without root rollback. -/
theorem importPeerSnapshot_progress {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} {lockOK accessOK writeOK : Bool} {out : HostResult}
    (h : importPeerSnapshot previous candidate entries peer bytes history
      lockOK accessOK writeOK = some out) :
    previous.root.seqno ≤ out.state.root.seqno ∧
      (out.state = previous ∨ out.state.config = candidate.config) := by
  unfold importPeerSnapshot at h
  split at h
  · contradiction
  · split at h
    · contradiction
    · split at h
      · split at h
        · cases h; exact ⟨Nat.le_refl _, Or.inl rfl⟩
        · obtain ⟨state, _, _⟩ := publishHost_spec h
          simp only [state]
          exact ⟨Nat.le_refl _, Or.inr trivial⟩
      · cases prepared : prepareReadableSnapshot previous candidate accessOK with
        | none => simp [prepared] at h
        | some next =>
          simp only [prepared, Option.bind_eq_bind, Option.bind_some] at h
          obtain ⟨root, accepted, config, rootEq, _⟩ := prepareReadableSnapshot_spec prepared
          have progress := (importRoot_spec accepted).1
          split at h
          · cases h; exact ⟨Nat.le_refl _, Or.inl rfl⟩
          · obtain ⟨state, _, _⟩ := publishHost_spec h
            exact ⟨by simpa [state, rootEq] using progress, Or.inr (state ▸ config)⟩

/-- No accepted peer import can roll back the configuration sequence. -/
theorem importPeerSnapshot_config_monotone {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} {lockOK accessOK writeOK : Bool} {out : HostResult}
    (hashes : ∀ e ∈ entries, e.hash ≠ "")
    (h : importPeerSnapshot previous candidate entries peer bytes history
      lockOK accessOK writeOK = some out) :
    previous.config.seqno ≤ out.state.config.seqno := by
  have seq := verifySuffix_seqno (fun e mem => hashes e (mem_of_mem_unappliedEntries mem))
    (importPeerSnapshot_authority h)
  obtain ⟨_, unchanged | advanced⟩ := importPeerSnapshot_progress h
  · simp [unchanged]
  · rw [advanced, seq]; omega

/-- A proved removal commits without examining the root or access callback. -/
theorem importPeerSnapshot_revoked {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} (accessOK : Bool)
    (bounded : bytes ≤ 10 * 1024 * 1024 ∧ entries.length ≤ 4096 ∧ history ≤ 8 * 1024 * 1024)
    (chain : verifySuffix previous.config candidate.config entries = true)
    (removed : readableBy candidate.config peer = false) (nonempty : entries ≠ []) :
    importPeerSnapshot previous candidate entries peer bytes history true accessOK true =
      some ⟨{previous with
        config := candidate.config, grants := [], ops := [], queued := [], rejections := []},
        true, true⟩ := by
  simp [importPeerSnapshot, bounded.1, bounded.2.1, bounded.2.2,
    unappliedEntries_of_verifySuffix chain, chain, removed, nonempty, publishHost]

/-- Readable imports reach the candidate checkpoint, retaining local invitation capabilities. -/
theorem importPeerSnapshot_target {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} {lockOK accessOK writeOK : Bool} {out : HostResult}
    (readable : readableBy candidate.config peer = true)
    (h : importPeerSnapshot previous candidate entries peer bytes history
      lockOK accessOK writeOK = some out) :
    out.state.config = candidate.config ∧ out.state.root.seqno = candidate.root.seqno ∧
      out.state.root.sameContent candidate.root = true ∧ out.state.invites = previous.invites := by
  unfold importPeerSnapshot at h
  split at h
  · contradiction
  · split at h
    · contradiction
    · simp only [readable, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at h
      cases prepared : prepareReadableSnapshot previous candidate accessOK with
      | none => simp [prepared] at h
      | some next =>
        simp only [prepared, Option.bind_eq_bind, Option.bind_some] at h
        obtain ⟨root, accepted, config, rootEq, invites⟩ := prepareReadableSnapshot_spec prepared
        obtain ⟨_, seq, content, _⟩ := importRoot_spec accepted
        have reached : next.config = candidate.config ∧ next.root.seqno = candidate.root.seqno ∧
            next.root.sameContent candidate.root = true ∧ next.invites = previous.invites := by
          exact ⟨config, by simpa [rootEq] using seq, by simpa [rootEq] using content, invites⟩
        split at h
        · rename_i unchanged
          cases h
          simpa [unchanged] using reached
        · obtain ⟨state, _, _⟩ := publishHost_spec h
          simpa [state] using reached

/-- A conflicting equal-sequence root cannot be installed for a retained reader. -/
theorem importPeerSnapshot_conflict {previous candidate : State} {entries : List Entry}
    {peer : String} (bytes history : Nat) (lockOK accessOK writeOK : Bool)
    (readable : readableBy candidate.config peer = true)
    (seq : candidate.root.seqno = previous.root.seqno)
    (different : candidate.root.sameContent previous.root = false) :
    importPeerSnapshot previous candidate entries peer bytes history
      lockOK accessOK writeOK = none := by
  have rejected : prepareReadableSnapshot previous candidate accessOK = none := by
    simp [prepareReadableSnapshot, importRoot_conflict candidate.config seq different]
  simp [importPeerSnapshot, readable, rejected]

/-- Older root proposals leave a readable receiver unchanged, including its configuration. -/
theorem importPeerSnapshot_stale {previous candidate : State} {entries : List Entry}
    {peer : String} (bytes history : Nat) (lockOK accessOK writeOK : Bool)
    (readable : readableBy candidate.config peer = true)
    (older : candidate.root.seqno < previous.root.seqno) :
    importPeerSnapshot previous candidate entries peer bytes history
      lockOK accessOK writeOK = none := by
  have rejected : prepareReadableSnapshot previous candidate accessOK = none := by
    simp [prepareReadableSnapshot, importRoot, older]
  simp [importPeerSnapshot, readable, rejected]

/-- Accepted exchanges to one compatible target converge despite distinct local pending state. -/
theorem importPeerSnapshot_converges {a b target : State} {ea eb : List Entry}
    {pa pb : String} {bytes ha hb : Nat} {la aa wa lb ab wb : Bool} {oa ob : HostResult}
    (ra : readableBy target.config pa = true) (rb : readableBy target.config pb = true)
    (acceptedA : importPeerSnapshot a target ea pa bytes ha la aa wa = some oa)
    (acceptedB : importPeerSnapshot b target eb pb bytes hb lb ab wb = some ob) :
    sameCheckpoint oa.state ob.state = true := by
  obtain ⟨ca, sa, contentA, _⟩ := importPeerSnapshot_target ra acceptedA
  obtain ⟨cb, sb, contentB, _⟩ := importPeerSnapshot_target rb acceptedB
  simp only [Root.sameContent, Bool.and_eq_true, beq_iff_eq] at contentA contentB
  simp [sameCheckpoint, Config.same, List.isPerm_iff, ca, cb, sa, sb, Root.sameContent,
    contentA.1, contentB.1, contentA.2, contentB.2]

/-- Clean valid checkpoints can be prepared without assuming import success. -/
theorem prepareReadableSnapshot_clean {previous candidate : State}
    (root : importRoot previous.root candidate.root candidate.config = some candidate.root)
    (valid : candidate.validate = true) (localEmpty : previous.ops = [])
    (remoteEmpty : candidate.ops = []) :
    prepareReadableSnapshot previous candidate true =
      some {candidate with invites := previous.invites, ops := [], queued := []} := by
  simp only [prepareReadableSnapshot, root, Option.bind_eq_bind, Option.bind_some]
  change (if !candidate.validate || !true then none else _) = _
  simp [valid, localEmpty, remoteEmpty, replaySnapshotOps, mergeLocalOps]

/-- A newer compatible clean checkpoint is installed without assuming import success. -/
theorem cleanExchange_converges {older newer : State} {entries : List Entry} {peer : String}
    {bytes history : Nat}
    (bounded : bytes ≤ 10 * 1024 * 1024 ∧ entries.length ≤ 4096 ∧ history ≤ 8 * 1024 * 1024)
    (chain : verifySuffix older.config newer.config entries = true)
    (readable : readableBy newer.config peer = true)
    (root : importRoot older.root newer.root newer.config = some newer.root)
    (progress : older.root.seqno < newer.root.seqno)
    (valid : newer.validate = true) (localEmpty : older.ops = []) (remoteEmpty : newer.ops = []) :
    ∃ out, importPeerSnapshot older newer entries peer bytes history true true true = some out ∧
      sameCheckpoint out.state newer = true := by
  let next := {newer with invites := older.invites, ops := [], queued := []}
  have changed : next ≠ older := by
    intro eq
    have equalSeq := congrArg (fun s : State => s.root.seqno) eq
    dsimp [next] at equalSeq
    omega
  have accepted : importPeerSnapshot older newer entries peer bytes history true true true =
      some ⟨next, false, true⟩ := by
    simp [importPeerSnapshot, bounded.1, bounded.2.1, bounded.2.2,
      unappliedEntries_of_verifySuffix chain, chain, readable,
      prepareReadableSnapshot_clean root valid localEmpty remoteEmpty, publishHost]
    split
    · rename_i same
      exact False.elim (changed same)
    · rfl
  refine ⟨⟨next, false, true⟩, accepted, ?_⟩
  simp [next, sameCheckpoint, Config.same, List.isPerm_iff, Root.sameContent]

/-! ## Local changes and explicit checkpoints -/

/-- A successful configuration call verifies against the held state before invoking its callback. -/
theorem applyConfigChange_spec {previous : State} {entry : Option Entry}
    {callback : State → Option State} {lockOK writeOK : Bool} {out : HostResult}
    (h : applyConfigChange previous entry callback lockOK writeOK = some out) :
    ∃ e config, entry = some e ∧ verifyChange previous.config e = some config ∧
      callback {previous with config} = some out.state ∧
      out.revoked = false ∧ out.wrote = true := by
  unfold applyConfigChange at h
  cases entry with
  | none => simp at h
  | some e =>
    simp only [Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · contradiction
    · cases checked : verifyChange previous.config e with
      | none => simp [checked] at h
      | some config =>
        simp only [checked, Option.bind_some] at h
        cases called : callback {previous with config} with
        | none => simp [called] at h
        | some next =>
          simp only [called, Option.bind_some] at h
          obtain ⟨state, revoked, wrote⟩ := publishHost_spec h
          exact ⟨e, config, rfl, checked, by simpa [state] using called, revoked, wrote⟩

/-- Concrete membership callbacks must preserve the verified configuration they receive. -/
theorem applyConfigChange_authorized {previous : State} {entry : Option Entry}
    {callback : State → Option State} {lockOK writeOK : Bool} {out : HostResult}
    (preserves : ∀ s next, callback s = some next → next.config = s.config)
    (h : applyConfigChange previous entry callback lockOK writeOK = some out) :
    ∃ e, entry = some e ∧ verifyChange previous.config e = some out.state.config := by
  obtain ⟨e, config, entryEq, checked, called, _, _⟩ := applyConfigChange_spec h
  exact ⟨e, entryEq, by simpa [preserves _ _ called] using checked⟩

/-- A configuration-preserving callback cannot roll back or fork a held chain head. -/
theorem applyConfigChange_progress {previous : State} {entry : Option Entry}
    {callback : State → Option State} {lockOK writeOK : Bool} {out : HostResult}
    (checkpoint : previous.config.hash ≠ "")
    (preserves : ∀ s next, callback s = some next → next.config = s.config)
    (h : applyConfigChange previous entry callback lockOK writeOK = some out) :
    ∃ e, entry = some e ∧ e.prev = previous.config.hash ∧
      out.state.config.seqno = previous.config.seqno + 1 ∧
      verifySignature e previous.config = true := by
  obtain ⟨e, present, verified⟩ := applyConfigChange_authorized preserves h
  obtain ⟨config, _, shape, linked, _, seq, signed, _⟩ := verifyChange_spec verified
  refine ⟨e, present, linked, ?_, signed⟩
  simpa [shape, Config.withHead, expectedSeqno, checkpoint] using seq

/-- Explicit invitation installation still requires a valid state and current root consensus. -/
theorem installInviteSnapshot_spec {candidate : State} {checkpoint lockOK writeOK : Bool}
    {out : HostResult} (h : installInviteSnapshot candidate checkpoint lockOK writeOK = some out) :
    out.state = candidate ∧ candidate.validate = true ∧
      rootAuthorized candidate.config candidate.root = true ∧ checkpoint = true := by
  unfold installInviteSnapshot at h
  split at h
  · rename_i checks
    simp only [Bool.and_eq_true] at checks
    exact ⟨(publishHost_spec h).1, checks.1.1.2, checks.1.2, checks.1.1.1.1⟩
  · contradiction

/-- Host root admission publishes only a successful state-level transition. -/
theorem hostUpdateRootState_spec {previous : State} {root : Root} {enforce : String}
    {rejected accepted : List Operation} {lockOK writeOK : Bool} {out : HostResult}
    (h : hostUpdateRootState previous root enforce rejected accepted lockOK writeOK = some out) :
    updateRootState previous root enforce rejected accepted = some out.state := by
  unfold hostUpdateRootState at h
  split at h
  · contradiction
  · cases updated : updateRootState previous root enforce rejected accepted with
    | none => simp [updated] at h
    | some next =>
      simp only [updated, Option.bind_eq_bind, Option.bind_some] at h
      exact congrArg some (publishHost_spec h).1.symm

end Spacewave.SObject
