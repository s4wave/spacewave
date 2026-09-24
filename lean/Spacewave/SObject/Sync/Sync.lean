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
serialized writer's current-authority checks. Watcher and shutdown traces remain
separate lifetime obligations.
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

end Spacewave.SObject.Sync
