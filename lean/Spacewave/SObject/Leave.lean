import Spacewave.SObject.Host

/-!
# Voluntary SharedObject departure

Mirrors `core/sobject/leave.go`. Each independently authenticated identity
consents only to its own removal; publication still requires a current owner.
Retained history, the retained configuration at the signed head, watched
snapshots and primitive signature/hash outcomes are provider inputs. The model computes membership, rebasing and retry decisions.

One attempt distinguishes failure, completion and retry. Finite traces may
remain pending: no eventual scheduling or quiescence guarantee is assumed.
Configuration hashes and request hashes are hexadecimal byte identities.
-/

namespace Spacewave.SObject

/-- LeaveRequest binds independently verified consent to an object and admission head. -/
structure LeaveRequest where
  object : String
  configHash : String
  signatures : List Sig
  deriving DecidableEq, Repr

/-- verifyLeave authenticates distinct, bounded identity proofs before any state access. -/
def verifyLeave (request : LeaveRequest) : Option (List String) :=
  let peers := request.signatures.map (·.signer)
  if request.object = "" || request.object.utf8ByteSize > 256 ||
      request.configHash.length != 64 || request.signatures.isEmpty ||
      request.signatures.length > 100 ||
      !request.signatures.all (fun sig => sig.signer != "" && sig.valid) ||
      !decide peers.Nodup then none
  else some peers

/-- buildLeave retains construction failures separately from receiver admission. -/
def buildLeave (request : LeaveRequest) (signOK : Bool) : Option LeaveRequest := do
  if !signOK then none
  else
    let _ ← verifyLeave request
    some request

/-- LeaveChange retains the signed configuration entry and its consent-request hash. -/
structure LeaveChange where
  entry : Entry
  requestHash : String
  deriving DecidableEq, Repr

/-- completedLeave returns only the portion ending at the first original removal. -/
def completedLeave (requestHash : String) : List LeaveChange → Option (List LeaveChange)
  | [] => none
  | change :: rest =>
    if change.requestHash = requestHash then some [change]
    else (completedLeave requestHash rest).map (change :: ·)

/-- leaveProofsRemainCurrent requires admission at the signed head and no interrupted membership. -/
def leaveProofsRemainCurrent (peers : List String) (signed : Config)
    (changes : List LeaveChange) : Bool :=
  !changes.isEmpty &&
    peers.all (fun peer => signed.participants.any (·.peer == peer)) &&
    changes.all (fun change => peers.all fun peer =>
      ((change.entry.config.map (·.participants)).getD []).any (·.peer == peer))

/-- leaveConfig filters exactly the identities authenticated by the request. -/
def leaveConfig (config : Config) (peers : List String) : Config :=
  {config with participants := config.participants.filter fun p => !peers.contains p.peer}

/-- leaveAllowed requires a real departure and prior ownership transfer if anyone remains. -/
def leaveAllowed (config : Config) (peers : List String) : Bool :=
  let next := leaveConfig config peers
  next.participants.length != config.participants.length &&
    (next.participants.isEmpty ||
      !config.participants.any (fun p => p.role == Role.owner && peers.contains p.peer))

/-- leaveCallback removes the consenting recipients' grants without changing other state. -/
def leaveCallback (peers : List String) (state : State) : Option State :=
  some {state with grants := state.grants.filter fun g => !peers.contains g.peer}

/-- leaveEntry binds a concrete filtered audience to the watched head and consent hash. -/
def leaveEntry (config : Config) (peers : List String) (sig : Sig)
    (hash requestHash : String) : LeaveChange :=
  ⟨⟨expectedSeqno config % seqnoLimit, some (leaveConfig config peers), some sig,
    config.hash, ChangeType.removeParticipant, hash⟩, requestHash⟩

/-- LeaveAttempt supplies one observed watch/lock/history/publication cycle. -/
structure LeaveAttempt where
  previous : State
  snapshot : Option Config
  history : Option (List LeaveChange)
  signed : Option Config
  latest : Option Config
  sig : Sig
  hash : String
  buildOK : Bool
  lockOK : Bool
  writeOK : Bool
  deriving DecidableEq, Repr

/-- LeaveResult distinguishes a retry from a completed response, including historical no-ops. -/
structure LeaveResult where
  retry : Bool
  changes : List LeaveChange
  outcome : HostResult
  deriving DecidableEq, Repr

/-- publishLeave checks the watched audience and publishes under the lock-held authority. -/
def publishLeave (attempt : LeaveAttempt) (snapshot : Config) (peers : List String)
    (requestHash : String) (history : List LeaveChange) : Option LeaveResult :=
  if !leaveAllowed snapshot peers || !attempt.buildOK then none
  else
    let change := leaveEntry snapshot peers attempt.sig attempt.hash requestHash
    match applyConfigChange attempt.previous (some change.entry) (leaveCallback peers)
        attempt.lockOK attempt.writeOK with
    | some outcome => some ⟨false, history ++ [change], outcome⟩
    | none => do
      let latest ← attempt.latest
      if latest.hash = snapshot.hash then none
      else some ⟨true, [], ⟨attempt.previous, false, false⟩⟩

/-- leaveAttempt mirrors one iteration, including idempotent history and optimistic retry. -/
def leaveAttempt (request : LeaveRequest) (hostId requestHash : String)
    (attempt : LeaveAttempt) : Option LeaveResult := do
  let peers ← verifyLeave request
  if request.object != hostId then none
  else
    let snapshot ← attempt.snapshot
    if snapshot.hash = request.configHash then publishLeave attempt snapshot peers requestHash []
    else
      let history ← attempt.history
      match completedLeave requestHash history with
      | some portion => some ⟨false, portion, ⟨attempt.previous, false, false⟩⟩
      | none =>
        let signed ← attempt.signed
        if leaveProofsRemainCurrent peers signed history then
          publishLeave attempt snapshot peers requestHash history
        else none

/-- LeaveTrace distinguishes a pending finite observation from a returned failure. -/
structure LeaveTrace where
  pending : Bool
  result : Option LeaveResult
  deriving DecidableEq, Repr

/-- leaveTrace continues only retries; each later attempt may observe external progress. -/
def leaveTrace (request : LeaveRequest) (hostId requestHash : String) :
    List LeaveAttempt → LeaveTrace
  | [] => ⟨true, none⟩
  | attempt :: rest =>
    match leaveAttempt request hostId requestHash attempt with
    | none => ⟨false, none⟩
    | some result =>
      if result.retry then leaveTrace request hostId requestHash rest
      else ⟨false, some result⟩

/-- Every departing identity has its own verified, nonempty signature and appears once. -/
theorem verifyLeave_consent {request : LeaveRequest} {peers : List String}
    (h : verifyLeave request = some peers) :
    peers = request.signatures.map (·.signer) ∧ peers.Nodup ∧
      ∀ peer ∈ peers, ∃ sig ∈ request.signatures,
        sig.signer = peer ∧ sig.signer ≠ "" ∧ sig.valid = true := by
  unfold verifyLeave at h
  dsimp only at h
  split at h
  · contradiction
  · rename_i valid
    cases h
    simp only [Bool.or_eq_true, Bool.not_eq_true', decide_eq_true_eq, not_or] at valid
    have unique : (request.signatures.map (·.signer)).Nodup := by
      simpa using valid.2
    have signed : request.signatures.all (fun sig => sig.signer != "" && sig.valid) = true := by
      simpa using valid.1.2
    refine ⟨rfl, unique, ?_⟩
    intro peer present
    obtain ⟨sig, member, equal⟩ := List.mem_map.mp present
    have checked := List.all_eq_true.mp signed sig member
    exact ⟨sig, member, equal, by simpa only [Bool.and_eq_true, bne_iff_ne] using checked⟩

/-- A historical retry reveals exactly a portion, ending at the first matching consent hash. -/
theorem completedLeave_prefix {requestHash : String} {history portion : List LeaveChange}
    (h : completedLeave requestHash history = some portion) :
    ∃ rest last, portion ++ rest = history ∧ portion.getLast? = some last ∧
      last.requestHash = requestHash := by
  induction history generalizing portion with
  | nil => simp [completedLeave] at h
  | cons change history ih =>
    simp only [completedLeave] at h
    split at h
    · rename_i matching
      cases h
      exact ⟨history, change, rfl, rfl, matching⟩
    · cases found : completedLeave requestHash history with
      | none => simp [found] at h
      | some tail =>
        simp [found] at h
        subst portion
        obtain ⟨rest, last, concat, ended, matching⟩ := ih found
        refine ⟨rest, last, by simp [concat], ?_, matching⟩
        cases tail with
        | nil => simp at ended
        | cons head tail => simpa using ended

/-- Rebasing requires every consenting peer at the signed head and in every later entry. -/
theorem leaveProofsRemainCurrent_members {peers : List String} {signed : Config}
    {history : List LeaveChange}
    (h : leaveProofsRemainCurrent peers signed history = true) :
    (∀ peer ∈ peers, ∃ p ∈ signed.participants, p.peer = peer) ∧
      ∀ change ∈ history, ∀ peer ∈ peers,
        ∃ p ∈ ((change.entry.config.map (·.participants)).getD []), p.peer = peer := by
  simp only [leaveProofsRemainCurrent, Bool.and_eq_true] at h
  obtain ⟨⟨_, admitted⟩, retained⟩ := h
  refine ⟨fun peer present => ?_, fun change member peer present => ?_⟩
  · simpa using List.all_eq_true.mp admitted peer present
  · simpa using List.all_eq_true.mp (List.all_eq_true.mp retained change member) peer present

/-- A departure preserves every nonconsenting participant with all its fields unchanged. -/
theorem leaveConfig_members {config : Config} {peers : List String} {p : Participant} :
    p ∈ (leaveConfig config peers).participants ↔ p ∈ config.participants ∧ p.peer ∉ peers := by
  simp [leaveConfig]

/-- Grant removal preserves the host callback's verified configuration contract. -/
theorem leaveCallback_config {peers : List String} {state next : State}
    (h : leaveCallback peers state = some next) : next.config = state.config := by
  cases h
  rfl

/-- A successful publication authenticates the filtered configuration under the held owner. -/
theorem publishLeave_authorized {attempt : LeaveAttempt} {snapshot : Config}
    {peers : List String} {requestHash : String} {history : List LeaveChange} {out : LeaveResult}
    (h : publishLeave attempt snapshot peers requestHash history = some out)
    (wrote : out.outcome.wrote = true) :
    verifyChange attempt.previous.config
      (leaveEntry snapshot peers attempt.sig attempt.hash requestHash).entry =
        some out.outcome.state.config := by
  unfold publishLeave at h
  split at h
  · contradiction
  · dsimp only at h
    split at h
    · rename_i result published
      cases h
      obtain ⟨e, equal, authorized⟩ := applyConfigChange_authorized
        (fun _ _ => leaveCallback_config) published
      cases equal
      exact authorized
    · cases latest : attempt.latest with
      | none => simp [latest] at h
      | some config =>
        simp only [latest, Option.bind_eq_bind, Option.bind_some] at h
        split at h
        · contradiction
        · cases h; contradiction

/-- Every successful attempt, including historical no-ops, authenticates the same consent. -/
theorem leaveAttempt_consent {request : LeaveRequest} {hostId requestHash : String}
    {attempt : LeaveAttempt} {out : LeaveResult}
    (h : leaveAttempt request hostId requestHash attempt = some out) :
    ∃ peers, verifyLeave request = some peers ∧ request.object = hostId := by
  unfold leaveAttempt at h
  cases consent : verifyLeave request with
  | none => simp [consent] at h
  | some peers =>
    simp only [consent, Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · contradiction
    · rename_i addressed
      exact ⟨peers, rfl, by simpa using addressed⟩

/-- Any actual publication comes from the concrete leave callback after authenticated consent. -/
theorem leaveAttempt_published {request : LeaveRequest} {hostId requestHash : String}
    {attempt : LeaveAttempt} {out : LeaveResult}
    (h : leaveAttempt request hostId requestHash attempt = some out)
    (wrote : out.outcome.wrote = true) :
    ∃ peers snapshot history, verifyLeave request = some peers ∧
      attempt.snapshot = some snapshot ∧
      publishLeave attempt snapshot peers requestHash history = some out := by
  unfold leaveAttempt at h
  cases consent : verifyLeave request with
  | none => simp [consent] at h
  | some peers =>
    simp only [consent, Option.bind_eq_bind, Option.bind_some] at h
    split at h
    · contradiction
    · cases watched : attempt.snapshot with
      | none => simp [watched] at h
      | some snapshot =>
        simp only [watched, Option.bind_some] at h
        split at h
        · exact ⟨peers, snapshot, [], rfl, rfl, h⟩
        · cases historyEq : attempt.history with
          | none => simp [historyEq] at h
          | some history =>
            simp only [historyEq, Option.bind_some] at h
            split at h
            · cases h; contradiction
            · cases signedEq : attempt.signed with
              | none => simp [signedEq] at h
              | some signed =>
                simp only [signedEq, Option.bind_some] at h
                split at h
                · exact ⟨peers, snapshot, history, rfl, rfl, h⟩
                · contradiction

/-- Published membership is filtered only by authenticated consenting peers and signed by an owner. -/
theorem leaveAttempt_authorized {request : LeaveRequest} {hostId requestHash : String}
    {attempt : LeaveAttempt} {out : LeaveResult}
    (h : leaveAttempt request hostId requestHash attempt = some out)
    (wrote : out.outcome.wrote = true) :
    attempt.sig.valid = true ∧ isOwner attempt.previous.config attempt.sig.signer = true ∧
      ∃ peers snapshot, verifyLeave request = some peers ∧ attempt.snapshot = some snapshot ∧
        out.outcome.state.config.participants = (leaveConfig snapshot peers).participants := by
  obtain ⟨peers, snapshot, history, consent, watched, published⟩ := leaveAttempt_published h wrote
  have verified := publishLeave_authorized published wrote
  obtain ⟨sig, present, valid, owner⟩ := verifyChange_authorized verified
  simp only [leaveEntry, Option.some.injEq] at present
  subst sig
  obtain ⟨config, present, shape, _⟩ := verifyChange_spec verified
  simp only [leaveEntry, Option.some.injEq] at present
  subst config
  exact ⟨valid,
    owner (by simp [leaveEntry, ChangeType.removeParticipant, ChangeType.selfEnrollPeer]),
    peers, snapshot, consent, watched, by simp [shape, Config.withHead]⟩

/-- A completed finite trace returns the result of one authenticated attempt. -/
theorem leaveTrace_completed {request : LeaveRequest} {hostId requestHash : String}
    {attempts : List LeaveAttempt} {out : LeaveResult}
    (h : leaveTrace request hostId requestHash attempts = ⟨false, some out⟩) :
    ∃ attempt ∈ attempts, leaveAttempt request hostId requestHash attempt = some out ∧
      out.retry = false := by
  induction attempts with
  | nil => simp [leaveTrace] at h
  | cons attempt rest ih =>
    simp only [leaveTrace] at h
    cases tried : leaveAttempt request hostId requestHash attempt with
    | none => simp [tried] at h
    | some result =>
      simp only [tried] at h
      split at h
      · obtain ⟨found, member, accepted, finished⟩ := ih h
        exact ⟨found, by simp [member], accepted, finished⟩
      · rename_i finished
        cases h
        exact ⟨attempt, by simp, tried, by simpa using finished⟩

/-- A departing owner cannot leave any other participant dependent on its proofs. -/
theorem leaveAllowed_owners {config : Config} {peers : List String} {p : Participant}
    (h : leaveAllowed config peers = true)
    (remaining : (leaveConfig config peers).participants.isEmpty = false)
    (member : p ∈ config.participants) (owner : p.role = Role.owner) : p.peer ∉ peers := by
  intro departed
  have found : config.participants.any
      (fun p => p.role == Role.owner && peers.contains p.peer) = true :=
    List.any_eq_true.mpr ⟨p, member, by simp [owner, departed]⟩
  unfold leaveAllowed at h
  dsimp only at h
  rw [remaining, found] at h
  simp at h

/-- Published leave uses the concrete grant-filtering callback exactly once. -/
theorem publishLeave_applied {attempt : LeaveAttempt} {snapshot : Config}
    {peers : List String} {requestHash : String} {history : List LeaveChange} {out : LeaveResult}
    (h : publishLeave attempt snapshot peers requestHash history = some out)
    (wrote : out.outcome.wrote = true) :
    applyConfigChange attempt.previous
      (some (leaveEntry snapshot peers attempt.sig attempt.hash requestHash).entry)
      (leaveCallback peers) attempt.lockOK attempt.writeOK = some out.outcome := by
  unfold publishLeave at h
  split at h
  · contradiction
  · dsimp only at h
    split at h
    · rename_i result published
      cases h
      exact published
    · cases latest : attempt.latest with
      | none => simp [latest] at h
      | some config =>
        simp only [latest, Option.bind_eq_bind, Option.bind_some] at h
        split at h
        · contradiction
        · cases h; contradiction

/-- Published leave removes only consenting grants and preserves every root and operation byte. -/
theorem leaveAttempt_state {request : LeaveRequest} {hostId requestHash : String}
    {attempt : LeaveAttempt} {out : LeaveResult}
    (h : leaveAttempt request hostId requestHash attempt = some out)
    (wrote : out.outcome.wrote = true) :
    out.outcome.state.root = attempt.previous.root ∧
      out.outcome.state.ops = attempt.previous.ops ∧
      out.outcome.state.queued = attempt.previous.queued ∧
      out.outcome.state.rejections = attempt.previous.rejections ∧
      out.outcome.state.invites = attempt.previous.invites ∧
      ∃ peers, verifyLeave request = some peers ∧
        out.outcome.state.grants = attempt.previous.grants.filter (fun g => !peers.contains g.peer) := by
  obtain ⟨peers, _, _, consent, _, published⟩ := leaveAttempt_published h wrote
  have applied := publishLeave_applied published wrote
  obtain ⟨_, _, _, _, called, _, _⟩ := applyConfigChange_spec applied
  simp only [leaveCallback, Option.some.injEq] at called
  rw [← called]
  exact ⟨rfl, rfl, rfl, rfl, rfl, peers, consent, rfl⟩

/-- A retry neither publishes a state nor occurs without observing a different chain head. -/
theorem publishLeave_retry {attempt : LeaveAttempt} {snapshot : Config}
    {peers : List String} {requestHash : String} {history : List LeaveChange} {out : LeaveResult}
    (h : publishLeave attempt snapshot peers requestHash history = some out)
    (retry : out.retry = true) :
    out.outcome = ⟨attempt.previous, false, false⟩ ∧
      ∃ latest, attempt.latest = some latest ∧ latest.hash ≠ snapshot.hash := by
  unfold publishLeave at h
  split at h
  · contradiction
  · dsimp only at h
    split at h
    · cases h; contradiction
    · cases latest : attempt.latest with
      | none => simp [latest] at h
      | some config =>
        simp only [latest, Option.bind_eq_bind, Option.bind_some] at h
        split at h
        · contradiction
        · rename_i changed
          cases h
          exact ⟨rfl, config, rfl, changed⟩

end Spacewave.SObject
