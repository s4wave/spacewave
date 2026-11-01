import Spacewave.SObject.Sync.Auth

/-!
# Authenticated data entry

Retains the security composition of runStream's authentication and deadline
checks before the first data frame. The Go startup conformance corpus exercises
this boundary. Worker ownership, transport shutdown, authority watches and
page exchange are temporal mechanisms modeled in colocated TLA+ modules.
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

end Spacewave.SObject.Sync
