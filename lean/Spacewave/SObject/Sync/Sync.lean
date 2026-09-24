import Spacewave.SObject.Sync.Auth
import Spacewave.SObject.Sync.Catchup

/-!
# SharedObject stream ownership

Mirrors `core/sobject/sync/sync.go`: the `runStream` startup path sets the
bounded authentication deadline, authenticates with a 64 KiB session, clears
the deadline and only then enters data synchronization. Deadline operations
are primitive transport observations. The authentication model derives the
participant identity and optional observer notification from the actual
challenge, signature, authority and explicit peer acknowledgment inputs.

`startStream` stops at entry to data synchronization. It does not assert that
synchronization succeeds, that authority remains unchanged, or that a later
transport write completes. `authenticatedFrames` composes that entry with the
serialized writer's current-authority checks. `watchStreamAuthority` mirrors the
separate authority watch, including the ignored denial-write result, retained
state release and unconditional owner cancellation and transport closure.
Transport close and successful deadlines must interrupt blocked I/O; state waits
must obey cancellation. `joinWorkers` retains partial exit waits; `finishStream`
requires the data workers and authority watcher to have exited before return,
then preserves the local access-denied cause and recovery notification.
-/

namespace Spacewave.SObject.Sync

/-- StreamStart preserves the observable startup branches and optional admission report. -/
structure StreamStart where
  remote : Option String
  admission : Option Bool
  authAttempted : Bool
  resetAttempted : Bool
  deriving DecidableEq, Repr

/-- startStream follows the deadline and authentication gates before entering synchronization. -/
def startStream (input : AuthenticationInput) (deadlineOK resetOK : Bool) : StreamStart :=
  if !deadlineOK then
    ⟨none, none, false, false⟩
  else
    let authenticated := authenticate input
    if !authenticated.ok then
      ⟨none, authenticated.admission, true, false⟩
    else
      ⟨if resetOK then some authenticated.remote else none, authenticated.admission, true, true⟩

/-- Data entry requires successful authentication and both transport deadline operations. -/
theorem startStream_gate {input : AuthenticationInput} {deadlineOK resetOK : Bool} {remote : String}
    (started : (startStream input deadlineOK resetOK).remote = some remote) :
    deadlineOK = true ∧ resetOK = true ∧ (authenticate input).ok = true ∧
      (authenticate input).remote = remote := by
  unfold startStream at started
  dsimp only at started
  repeat' first | split at started | simp_all

/-- Starting data retains the verified proof, held authority and explicit positive peer admission. -/
theorem startStream_authenticated {input : AuthenticationInput} {deadlineOK resetOK : Bool} {remote : String}
    (started : (startStream input deadlineOK resetOK).remote = some remote) :
    verifyParticipantProof input.proof = some remote ∧
    authorizeParticipants input.participants input.hash input.localPeer remote = true ∧
    input.remoteAuthorization = some true := by
  obtain ⟨_, _, admitted, identity⟩ := startStream_gate started
  simpa only [identity] using authenticate_admitted admitted

/-- Both readable participants and held configuration history are necessary for data entry. -/
theorem startStream_readable {input : AuthenticationInput} {deadlineOK resetOK : Bool} {remote : String}
    (started : (startStream input deadlineOK resetOK).remote = some remote) :
    readableParticipant input.participants input.localPeer = true ∧
    readableParticipant input.participants remote = true ∧ input.hash ≠ "" :=
  authorizeParticipants_spec (startStream_authenticated started).2.1

/-- No rejected authentication or failed deadline operation can enter data synchronization. -/
theorem startStream_rejected {input : AuthenticationInput} {deadlineOK resetOK : Bool}
    (rejected : deadlineOK = false ∨ resetOK = false ∨ (authenticate input).ok = false) :
    (startStream input deadlineOK resetOK).remote = none := by
  cases result : (startStream input deadlineOK resetOK).remote with
  | none => rfl
  | some remote =>
    obtain ⟨deadline, reset, authenticated, _⟩ := startStream_gate result
    rcases rejected with failure | failure | failure <;> simp_all

/-- Healthy mutual authentication reaches data entry, rather than only satisfying rejection properties. -/
theorem startStream_complete {input : AuthenticationInput} {remote : String}
    (localTransport : input.localTransport ≠ "") (remoteTransport : input.remoteTransport ≠ "")
    (distinct : input.localTransport ≠ input.remoteTransport) (key : input.keyPeer = some input.localPeer)
    (primitives : input.nonceOK && input.challengeOK && input.signOK && input.proofExchangeOK &&
      input.stateOK && input.authorizationOK = true)
    (nonceSize : input.remoteNonceSize = 32) (nonceDistinct : input.noncesDistinct = true)
    (proof : verifyParticipantProof input.proof = some remote)
    (held : authorizeParticipants input.participants input.hash input.localPeer remote = true)
    (acknowledged : input.remoteAuthorization = some true) :
    startStream input true true = ⟨some remote, if input.observer then some true else none, true, true⟩ := by
  have authenticated := authenticate_succeeds localTransport remoteTransport distinct key primitives nonceSize
    nonceDistinct proof held acknowledged
  simp [startStream, authenticated]

/-- authenticatedFrames starts the writer only at the authenticated data-entry boundary. -/
def authenticatedFrames (input : AuthenticationInput) (deadlineOK resetOK : Bool)
    (attempts : List WriterAttempt) : WriterResult :=
  match (startStream input deadlineOK resetOK).remote with
  | none => ⟨[], [], false⟩
  | some remote => writeFrames input.localPeer remote attempts

/-- Every data admission combines the established handshake with the writer's latest authority check. -/
theorem authenticatedFrames_authorized {input : AuthenticationInput} {deadlineOK resetOK : Bool}
    {attempts : List WriterAttempt} {kind : Int}
    (sent : kind ∈ (authenticatedFrames input deadlineOK resetOK attempts).frames) (dataFrame : kind ≠ 6) :
    ∃ remote, verifyParticipantProof input.proof = some remote ∧ input.remoteAuthorization = some true ∧
      authorizeParticipants input.participants input.hash input.localPeer remote = true ∧
      ∃ attempt ∈ attempts, attempt.selected = true ∧ attempt.kind = kind ∧
        authorizeParticipants attempt.participants attempt.hash input.localPeer remote = true := by
  unfold authenticatedFrames at sent
  cases started : (startStream input deadlineOK resetOK).remote with
  | none => simp [started] at sent
  | some remote =>
    simp only [started] at sent
    obtain ⟨proof, held, ack⟩ := startStream_authenticated started
    exact ⟨remote, proof, ack, held, writeFrames_authorized sent dataFrame⟩

/-- AuthorityRead is one primitive WaitValueChange result. Error codes are projections:
    zero is success, one cancellation, two access denial, three missing history and four other failure. -/
structure AuthorityRead where
  participants : List Participant
  hash : String
  error : Int
  deriving DecidableEq, Repr

/-- AuthorityWatch retains the observed prefix and the actual deferred cleanup calls.
    A finite healthy prefix is still running; only a terminal observation runs cleanup. -/
structure AuthorityWatch where
  reads : Nat
  stopped : Bool
  cause : Int
  deadline : Bool
  denial : Bool
  retained : Bool
  released : Bool
  canceled : Bool
  closed : Bool
  deriving DecidableEq, Repr

/-- watchAuthorityReads follows the state-change loop and bounded denial attempt.
    A successful deadline supplies the transport contract that even a blocked send terminates.
    Send failure is ignored by Go, so its result cannot change authority or cleanup. -/
def watchAuthorityReads (localID remote : String) (deadlineOK : Bool) : List AuthorityRead → AuthorityWatch
  | [] => ⟨0, false, 0, false, false, true, false, false, false⟩
  | read :: rest =>
    if read.error != 0 then
      ⟨1, true, read.error, false, false, true, true, true, true⟩
    else if !authorizeParticipants read.participants read.hash localID remote then
      let cause := if readableParticipant read.participants localID && readableParticipant read.participants remote then 3 else 2
      ⟨1, true, cause, true, deadlineOK, true, true, true, true⟩
    else
      let next := watchAuthorityReads localID remote deadlineOK rest
      {next with reads := next.reads + 1}

/-- watchStreamAuthority includes failed watch retention and the unconditional cancel/close defer. -/
def watchStreamAuthority (localID remote : String) (retainError : Int) (deadlineOK : Bool)
    (reads : List AuthorityRead) : AuthorityWatch :=
  if retainError != 0 then
    ⟨0, true, retainError, false, false, false, false, true, true⟩
  else watchAuthorityReads localID remote deadlineOK reads

/-- Every terminal watcher path cancels and closes, after releasing any successful retention. -/
theorem watchAuthorityReads_cleanup (localID remote : String) (deadlineOK : Bool) (reads : List AuthorityRead)
    (stopped : (watchAuthorityReads localID remote deadlineOK reads).stopped = true) :
    (watchAuthorityReads localID remote deadlineOK reads).released = true ∧
    (watchAuthorityReads localID remote deadlineOK reads).canceled = true ∧
    (watchAuthorityReads localID remote deadlineOK reads).closed = true := by
  induction reads with
  | nil => simp [watchAuthorityReads] at stopped
  | cons read rest ih =>
    unfold watchAuthorityReads at stopped ⊢
    split <;> simp_all
    split <;> simp_all

/-- No retention failure bypasses the owner's cancel and transport close. -/
theorem watchStreamAuthority_cleanup (localID remote : String) (retainError : Int) (deadlineOK : Bool)
    (reads : List AuthorityRead)
    (stopped : (watchStreamAuthority localID remote retainError deadlineOK reads).stopped = true) :
    (watchStreamAuthority localID remote retainError deadlineOK reads).canceled = true ∧
    (watchStreamAuthority localID remote retainError deadlineOK reads).closed = true ∧
    ((watchStreamAuthority localID remote retainError deadlineOK reads).retained = true →
      (watchStreamAuthority localID remote retainError deadlineOK reads).released = true) := by
  by_cases retained : retainError = 0
  · have terminal : (watchAuthorityReads localID remote deadlineOK reads).stopped = true := by
      simpa [watchStreamAuthority, retained] using stopped
    obtain ⟨released, canceled, closed⟩ := watchAuthorityReads_cleanup localID remote deadlineOK reads terminal
    simp [watchStreamAuthority, retained, released, canceled, closed]
  · simp [watchStreamAuthority, retained]

/-- A stopped watcher ignores every later observation, including a readable regrant. -/
theorem watchAuthorityReads_terminal (localID remote : String) (deadlineOK : Bool)
    (consumed suffix : List AuthorityRead)
    (stopped : (watchAuthorityReads localID remote deadlineOK consumed).stopped = true) :
    watchAuthorityReads localID remote deadlineOK (consumed ++ suffix) =
      watchAuthorityReads localID remote deadlineOK consumed := by
  induction consumed with
  | nil => simp [watchAuthorityReads] at stopped
  | cons read rest ih =>
    simp only [List.cons_append, watchAuthorityReads] at stopped ⊢
    split <;> simp_all
    split <;> simp_all

/-- A healthy state read with either participant excluded ends the watcher with access denial. -/
theorem watchAuthorityReads_revoked (localID remote : String) (deadlineOK : Bool)
    (read : AuthorityRead) (rest : List AuthorityRead) (available : read.error = 0)
    (revoked : readableParticipant read.participants localID = false ∨
      readableParticipant read.participants remote = false) :
    watchAuthorityReads localID remote deadlineOK (read :: rest) =
      ⟨1, true, 2, true, deadlineOK, true, true, true, true⟩ := by
  rcases revoked with localRevoked | remoteRevoked <;>
    simp [watchAuthorityReads, authorizeParticipants, *]

/-- Healthy observations remain live and never attempt denial or cleanup. -/
theorem watchAuthorityReads_healthy (localID remote : String) (deadlineOK : Bool) (reads : List AuthorityRead)
    (healthy : ∀ read ∈ reads, read.error = 0 ∧ authorizeParticipants read.participants read.hash localID remote = true) :
    watchAuthorityReads localID remote deadlineOK reads =
      ⟨reads.length, false, 0, false, false, true, false, false, false⟩ := by
  induction reads with
  | nil => rfl
  | cons read rest ih =>
    obtain ⟨available, admitted⟩ := healthy read (by simp)
    have tail := ih (fun item member => healthy item (by simp [member]))
    simp [watchAuthorityReads, available, admitted, tail]

/-- A denial attempt requires both a successful transport deadline and an actually rejected held state. -/
theorem watchAuthorityReads_denial (localID remote : String) (deadlineOK : Bool) (reads : List AuthorityRead)
    (denial : (watchAuthorityReads localID remote deadlineOK reads).denial = true) :
    deadlineOK = true ∧ ∃ read ∈ reads, read.error = 0 ∧
      authorizeParticipants read.participants read.hash localID remote = false := by
  induction reads with
  | nil => simp [watchAuthorityReads] at denial
  | cons read rest ih =>
    unfold watchAuthorityReads at denial
    split at denial
    · simp at denial
    · split at denial
      · simp_all
      · obtain ⟨deadline, item, member, available, rejected⟩ := ih denial
        exact ⟨deadline, item, by simp [member], available, rejected⟩

/-- Terminal prefix stability includes failed retention, which consumes no state observations. -/
theorem watchStreamAuthority_terminal (localID remote : String) (retainError : Int) (deadlineOK : Bool)
    (consumed suffix : List AuthorityRead)
    (stopped : (watchStreamAuthority localID remote retainError deadlineOK consumed).stopped = true) :
    watchStreamAuthority localID remote retainError deadlineOK (consumed ++ suffix) =
      watchStreamAuthority localID remote retainError deadlineOK consumed := by
  by_cases retained : retainError = 0
  · have terminal : (watchAuthorityReads localID remote deadlineOK consumed).stopped = true := by
      simpa [watchStreamAuthority, retained] using stopped
    simpa [watchStreamAuthority, retained] using watchAuthorityReads_terminal localID remote deadlineOK consumed suffix terminal
  · simp [watchStreamAuthority, retained]

/-- WorkerJoin records SetRoutine(nil)'s optional channel and its receive readiness.
    A missing channel means no body remains to join. A ready channel acknowledges body return.
    Container bookkeeping and transport-only AfterFunc callbacks are outside that body contract. -/
structure WorkerJoin where
  channel : Bool
  exited : Bool
  deriving DecidableEq, Repr

/-- workerFinished is the primitive routine-body exit contract. -/
def workerFinished (worker : WorkerJoin) : Bool := !worker.channel || worker.exited

/-- JoinResult preserves partial progress when an exit-channel receive would block. -/
structure JoinResult where
  stopped : Nat
  joined : Nat
  done : Bool
  deriving DecidableEq, Repr

/-- joinWorkers mirrors joinSyncWorkers's sequential remove-and-wait loop. -/
def joinWorkers : List WorkerJoin → JoinResult
  | [] => ⟨0, 0, true⟩
  | worker :: rest =>
    if !workerFinished worker then ⟨1, 0, false⟩
    else
      let next := joinWorkers rest
      ⟨next.stopped + 1, next.joined + 1, next.done⟩

/-- Joined return requires an exit acknowledgment or absence of a body for every worker. -/
theorem joinWorkers_finished (workers : List WorkerJoin) (done : (joinWorkers workers).done = true) :
    ∀ worker ∈ workers, workerFinished worker = true := by
  induction workers with
  | nil => simp
  | cons worker rest ih =>
    unfold joinWorkers at done
    split at done
    · contradiction
    · simp_all

/-- Healthy exit acknowledgments complete every join, with no missing or extra worker. -/
theorem joinWorkers_complete (workers : List WorkerJoin)
    (finished : ∀ worker ∈ workers, workerFinished worker = true) :
    joinWorkers workers = ⟨workers.length, workers.length, true⟩ := by
  induction workers with
  | nil => rfl
  | cons worker rest ih =>
    have head := finished worker (by simp)
    have tail := ih (fun item member => finished item (by simp [member]))
    simp [joinWorkers, head, tail]

/-- A pending worker receive cannot be skipped to declare joined return. -/
theorem joinWorkers_pending (workers : List WorkerJoin) (worker : WorkerJoin)
    (member : worker ∈ workers) (pending : worker.channel = true ∧ worker.exited = false) :
    (joinWorkers workers).done = false := by
  cases done : (joinWorkers workers).done
  · rfl
  · have finished := joinWorkers_finished workers done worker member
    simp [workerFinished, pending.1, pending.2] at finished

/-- StreamFinish is runStream's final error and recovery notification after its joins.
    Error codes use the watcher oracle projection; only access denial and history have special handling. -/
structure StreamFinish where
  error : Int
  recovery : Bool
  deriving DecidableEq, Repr

/-- finishStream follows synchronization cleanup then the authority-watcher join.
    Retention failure starts no sync workers; successful retention starts reader, writer and state watcher.
    Cancellation and transport closure precede joins, as required by their primitive liveness contracts.
    none means an incomplete or inconsistent exit trace, never a returned stream. -/
def finishStream (retained : Bool) (syncWorkers : List WorkerJoin) (authority : WorkerJoin)
    (syncError cause : Int) (remoteKnown observer : Bool) : Option StreamFinish := do
  if syncWorkers.length != (if retained then 3 else 0) then none
  else if !(joinWorkers syncWorkers).done || !workerFinished authority then none
  else
    let error := if cause == 2 then cause else syncError
    return ⟨error, remoteKnown && observer && error == 3⟩

/-- Returning from the stream requires all started sync bodies and the authority watcher to have exited. -/
theorem finishStream_joined {retained : Bool} {workers : List WorkerJoin} {authority : WorkerJoin}
    {syncError cause : Int} {remoteKnown observer : Bool} {result : StreamFinish}
    (returned : finishStream retained workers authority syncError cause remoteKnown observer = some result) :
    workers.length = (if retained then 3 else 0) ∧ workerFinished authority = true ∧
      ∀ worker ∈ workers, workerFinished worker = true := by
  have gates : workers.length = (if retained then 3 else 0) ∧
      (joinWorkers workers).done = true ∧ workerFinished authority = true := by
    unfold finishStream at returned
    simp only [pure] at returned
    repeat' first | split at returned | subst result | simp_all
  exact ⟨gates.1, gates.2.2, joinWorkers_finished workers gates.2.1⟩

/-- In particular, the writer body cannot admit a later frame after a joined stream return. -/
theorem finishStream_writer_exited {workers : List WorkerJoin} {authority writer : WorkerJoin}
    {syncError cause : Int} {remoteKnown observer : Bool} {result : StreamFinish}
    (returned : finishStream true workers authority syncError cause remoteKnown observer = some result)
    (writerSlot : workers[1]? = some writer) : workerFinished writer = true := by
  have joined := finishStream_joined returned
  exact joined.2.2 writer (List.mem_of_getElem? writerSlot)

/-- Access denial caused by local revocation survives transport errors encountered during closure. -/
theorem finishStream_denial {retained : Bool} {workers : List WorkerJoin} {authority : WorkerJoin}
    {syncError : Int} {remoteKnown observer : Bool} {result : StreamFinish}
    (returned : finishStream retained workers authority syncError 2 remoteKnown observer = some result) :
    result.error = 2 ∧ result.recovery = false := by
  unfold finishStream at returned
  simp only [pure] at returned
  repeat' first | split at returned | subst result | simp_all

/-- Authenticated loop entry retains the established participant identity across selected events. -/
def authenticatedExchange (authentication : AuthenticationInput) (deadlineOK resetOK : Bool)
    (before : Exchange) (input : LoopInput) (frame : ExchangeFrame) : Option LoopResult := do
  let remote ← (startStream authentication deadlineOK resetOK).remote
  advanceExchange before authentication.localPeer remote input frame

/-- Every owner import combines mutual authentication with both current held-authority checks. -/
theorem authenticatedExchange_import {authentication : AuthenticationInput} {deadlineOK resetOK : Bool}
    {before : Exchange} {input : LoopInput} {frame : ExchangeFrame} {result : LoopResult} {accepted : AcceptanceResult}
    (advanced : authenticatedExchange authentication deadlineOK resetOK before input frame = some result)
    (imported : result.exchange.imported = some accepted) :
    ∃ remote, verifyParticipantProof authentication.proof = some remote ∧
      authentication.remoteAuthorization = some true ∧
      authorizeParticipants authentication.participants authentication.hash authentication.localPeer remote = true ∧
      authorizedState input.current authentication.localPeer remote = true ∧
      authorizedState input.incomingCurrent authentication.localPeer remote = true := by
  unfold authenticatedExchange at advanced
  cases started : (startStream authentication deadlineOK resetOK).remote with
  | none => simp [started] at advanced
  | some remote =>
    simp [started] at advanced
    obtain ⟨proof, held, admitted⟩ := startStream_authenticated started
    obtain ⟨current, incoming, _⟩ := advanceExchange_import advanced imported
    exact ⟨remote, proof, admitted, held, current, incoming⟩

end Spacewave.SObject.Sync
