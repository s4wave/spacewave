import Spacewave.SObject.State

/-!
# SharedObject host import

Mirrors `SOHost.ImportPeerSnapshot` in `core/sobject/host.go`. Ordinary
failure is `none`; committed revocation is a successful `HostResult` with
`revoked = true`, matching Go's write followed by ErrParticipantRevoked.

Byte sizes are the real serialized input sizes. Lock, access callback and
persistence outcomes are explicit inputs. Access validation is assumed
read-only. The provider must publish nothing on a failed write.

`merged` is the observed result of merging the candidate's checkpoint, key
epochs and operations into the held state under the candidate configuration:
`AdoptCheckpoint`, `MergeKeyEpochs`, `AddOperation` and `Validate`, or none
when any of them fails. The model owns the authority decisions around it: the
configuration suffix, revocation, local invitations, access and publication.
-/

namespace Spacewave.SObject

/-- HostResult separates committed state, revocation notification and publication. -/
structure HostResult where
  state : State
  revoked : Bool
  wrote : Bool
  deriving DecidableEq, Repr

/-- readableBy mirrors the `CanReadState` loop over the participants. -/
def readableBy (config : Config) (peer : String) : Bool :=
  config.participants.any fun p => p.peer == peer &&
    p.role ∈ [Role.reader, Role.writer, Role.owner]

/-- publishHost represents atomic provider publication. -/
def publishHost (next : State) (revoked writeOK : Bool) : Option HostResult :=
  if writeOK then some ⟨next, revoked, true⟩ else none

/-- visibleHost makes unchanged live state explicit for an ordinary rejection. -/
def visibleHost (previous : State) (result : Option HostResult) : HostResult :=
  result.getD ⟨previous, false, false⟩

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

/--
importPeerSnapshot mirrors bounds, chain authority, revocation, the merge,
access and the write.
-/
def importPeerSnapshot (previous candidate : State) (entries : List Entry)
    (localPeer : String) (candidateBytes historyBytes : Nat) (merged : Option State)
    (lockOK accessOK writeOK : Bool) : Option HostResult := do
  if candidateBytes > 10 * 1024 * 1024 || entries.length > 4096 ||
      historyBytes > 8 * 1024 * 1024 || !lockOK then none
  else if !verifySuffix previous.config candidate.config
      (unappliedEntries previous.config.hash entries) then none
  else if !readableBy candidate.config localPeer then
    if (unappliedEntries previous.config.hash entries).isEmpty then some ⟨previous, true, false⟩
    else publishHost {previous with config := candidate.config, epochs := [], ops := []} true writeOK
  else
    let merged ← merged
    let next := {merged with config := candidate.config, invites := previous.invites}
    if !accessOK then none
    else if next = previous then some ⟨previous, false, false⟩
    else publishHost next false writeOK

/-! ## Publication -/

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

/-! ## Peer import -/

/-- Every accepted import was authorized by the configuration suffix from the held head. -/
theorem importPeerSnapshot_authority {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} {merged : Option State} {lockOK accessOK writeOK : Bool}
    {out : HostResult}
    (h : importPeerSnapshot previous candidate entries peer bytes history merged
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

/--
An import keeps the held state, or commits the candidate configuration with the
local invitations.
-/
theorem importPeerSnapshot_target {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} {merged : Option State} {lockOK accessOK writeOK : Bool}
    {out : HostResult}
    (h : importPeerSnapshot previous candidate entries peer bytes history merged
      lockOK accessOK writeOK = some out) :
    out.state = previous ∨
      (out.state.config = candidate.config ∧ out.state.invites = previous.invites) := by
  unfold importPeerSnapshot at h
  split at h
  · contradiction
  · split at h
    · contradiction
    · split at h
      · split at h
        · cases h; exact Or.inl rfl
        · obtain ⟨state, _, _⟩ := publishHost_spec h
          exact Or.inr ⟨by rw [state], by rw [state]⟩
      · cases m : merged with
        | none => simp [m] at h
        | some m' =>
          simp only [m, Option.bind_eq_bind, Option.bind_some] at h
          split at h
          · contradiction
          · split at h
            · cases h; exact Or.inl rfl
            · obtain ⟨state, _, _⟩ := publishHost_spec h
              exact Or.inr ⟨by rw [state], by rw [state]⟩

/-- No accepted peer import can roll back the configuration sequence. -/
theorem importPeerSnapshot_config_monotone {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} {merged : Option State} {lockOK accessOK writeOK : Bool}
    {out : HostResult}
    (hashes : ∀ e ∈ entries, e.hash ≠ "")
    (h : importPeerSnapshot previous candidate entries peer bytes history merged
      lockOK accessOK writeOK = some out) :
    previous.config.seqno ≤ out.state.config.seqno := by
  have seq := verifySuffix_seqno (fun e mem => hashes e (mem_of_mem_unappliedEntries mem))
    (importPeerSnapshot_authority h)
  rcases importPeerSnapshot_target h with unchanged | ⟨advanced, _⟩
  · simp [unchanged]
  · rw [advanced, seq]; omega

/-- A proved removal commits without the merge or the access callback. -/
theorem importPeerSnapshot_revoked {previous candidate : State} {entries : List Entry}
    {peer : String} {bytes history : Nat} (merged : Option State) (accessOK : Bool)
    (bounded : bytes ≤ 10 * 1024 * 1024 ∧ entries.length ≤ 4096 ∧
      history ≤ 8 * 1024 * 1024)
    (chain : verifySuffix previous.config candidate.config entries = true)
    (removed : readableBy candidate.config peer = false) (nonempty : entries ≠ []) :
    importPeerSnapshot previous candidate entries peer bytes history merged true accessOK true =
      some ⟨{previous with config := candidate.config, epochs := [], ops := []}, true, true⟩ := by
  simp [importPeerSnapshot, bounded.1, bounded.2.1, bounded.2.2,
    unappliedEntries_of_verifySuffix chain, chain, removed, nonempty, publishHost]

/-- A readable import publishes the merge with the candidate configuration and local invitations. -/
theorem importPeerSnapshot_merged {previous candidate merged : State} {entries : List Entry}
    {peer : String} {bytes history : Nat}
    (bounded : bytes ≤ 10 * 1024 * 1024 ∧ entries.length ≤ 4096 ∧
      history ≤ 8 * 1024 * 1024)
    (chain : verifySuffix previous.config candidate.config entries = true)
    (readable : readableBy candidate.config peer = true)
    (changed : {merged with config := candidate.config, invites := previous.invites} ≠ previous) :
    importPeerSnapshot previous candidate entries peer bytes history (some merged) true true true =
      some ⟨{merged with config := candidate.config, invites := previous.invites}, false, true⟩ := by
  simp [importPeerSnapshot, bounded.1, bounded.2.1, bounded.2.2,
    unappliedEntries_of_verifySuffix chain, chain, readable, changed, publishHost]

end Spacewave.SObject
