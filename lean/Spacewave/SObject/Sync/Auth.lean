import Spacewave.SObject.ConfigChain

/-!
# SharedObject peer authentication

Mirrors `core/sobject/sync/auth.go` at `dde93d9b7`: `verifyParticipantProof`,
`authorizeParticipants` and `authenticate`. The result retains a proved peer
on local admission failure, as Go does, and reports the optional observer call.

Transport and participant IDs are opaque strings. A proof's `valid` observation
combines public-key parsing, verification over the exact directional transcript
with the synchronization context, and signer-ID derivation. The Go projection
evaluates these primitives over real signed bytes, never participant admission.
Challenge size and equality are primitive byte observations. Entropy, encoding,
signing, transport and host-read outcomes are explicit inputs. The allocation
of the local 32-byte nonce and cryptographic unforgeability remain contracts.

Authorization scans for any readable occurrence, faithfully matching the loop
even on duplicate participants. It is not a last-wins map lookup. Valid held
configurations have unique participants by the configuration-chain proof.
-/

namespace Spacewave.SObject.Sync

/-- readableRole mirrors the four `CanReadState` cases, including unknown enum codes. -/
def readableRole (role : Int) : Bool :=
  role == Role.reader || role == Role.writer || role == Role.validator || role == Role.owner

/-- readableParticipant mirrors the loop's accumulated readable occurrence. -/
def readableParticipant (participants : List Participant) (peer : String) : Bool :=
  participants.any fun p => readableRole p.role && p.peer == peer

/-- authorizeParticipants checks both held roles before requiring configuration history. -/
def authorizeParticipants (participants : List Participant) (hash localID remote : String) : Bool :=
  readableParticipant participants localID && readableParticipant participants remote && hash != ""

/-- verifyParticipantProof exposes a signer only after exact-transcript primitive verification. -/
def verifyParticipantProof (proof : Option Sig) : Option String := do
  let proof ← proof
  if proof.valid then some proof.signer else none

/-- AuthenticationInput contains only wire observations and primitive operation outcomes. -/
structure AuthenticationInput where
  localTransport : String
  remoteTransport : String
  localPeer : String
  keyPeer : Option String
  nonceOK : Bool
  challengeOK : Bool
  remoteNonceSize : Nat
  noncesDistinct : Bool
  signOK : Bool
  proofExchangeOK : Bool
  proof : Option Sig
  stateOK : Bool
  participants : List Participant
  hash : String
  authorizationOK : Bool
  remoteAuthorization : Option Bool
  observer : Bool
  deriving Repr

/-- AuthenticationResult preserves the return identity and optional admission notification. -/
structure AuthenticationResult where
  ok : Bool
  remote : String
  admission : Option Bool
  deriving DecidableEq, Repr

/-- authenticate follows challenge, proof, held admission and explicit remote acknowledgment in order. -/
def authenticate (input : AuthenticationInput) : AuthenticationResult := Id.run do
  let rejected : AuthenticationResult := ⟨false, "", none⟩
  if input.localTransport == "" || input.remoteTransport == "" || input.localTransport == input.remoteTransport then
    return rejected
  if input.keyPeer != some input.localPeer || !input.nonceOK || !input.challengeOK then
    return rejected
  if input.remoteNonceSize != 32 || !input.noncesDistinct || !input.signOK || !input.proofExchangeOK then
    return rejected
  let some remote := verifyParticipantProof input.proof | return rejected
  if !input.stateOK then
    return rejected
  let accepted := authorizeParticipants input.participants input.hash input.localPeer remote
  if !input.authorizationOK then
    return rejected
  let some remoteAccepted := input.remoteAuthorization | return rejected
  let admission := if accepted && input.observer then some remoteAccepted else none
  if !accepted then
    return ⟨false, remote, admission⟩
  if !remoteAccepted then
    return ⟨false, "", admission⟩
  return ⟨true, remote, admission⟩

/-- Admission requires readable occurrences for both endpoints and held history. -/
theorem authorizeParticipants_spec {participants : List Participant} {hash localID remote : String}
    (accepted : authorizeParticipants participants hash localID remote = true) :
    readableParticipant participants localID = true ∧ readableParticipant participants remote = true ∧ hash ≠ "" := by
  simpa only [authorizeParticipants, Bool.and_eq_true, bne_iff_ne, and_assoc] using accepted

/-- A verified participant identity comes from a valid proof over the requested transcript. -/
theorem verifyParticipantProof_valid {proof : Option Sig} {remote : String}
    (accepted : verifyParticipantProof proof = some remote) :
    ∃ signature, proof = some signature ∧ signature.valid = true ∧ signature.signer = remote := by
  cases proof with
  | none => simp [verifyParticipantProof] at accepted
  | some signature =>
    change (if signature.valid then some signature.signer else none) = some remote at accepted
    split at accepted
    · exact ⟨signature, rfl, ‹signature.valid = true›, Option.some.inj accepted⟩
    · contradiction

/-- A successful handshake has a verified signer, held authority and explicit remote admission. -/
theorem authenticate_admitted {input : AuthenticationInput}
    (accepted : (authenticate input).ok = true) :
    verifyParticipantProof input.proof = some (authenticate input).remote ∧
    authorizeParticipants input.participants input.hash input.localPeer (authenticate input).remote = true ∧
    input.remoteAuthorization = some true := by
  unfold authenticate at *
  simp only [Id.run, pure] at *
  repeat' first | split at * | simp_all

/-- Invalid participant proofs cannot reach successful authentication. -/
theorem authenticate_invalid_proof {input : AuthenticationInput}
    (invalid : verifyParticipantProof input.proof = none) : (authenticate input).ok = false := by
  cases result : (authenticate input).ok
  · rfl
  · have admitted := authenticate_admitted result
    rw [invalid] at admitted
    cases admitted.1

/-- Authentication never succeeds after either endpoint loses every readable occurrence. -/
theorem authenticate_readable {input : AuthenticationInput}
    (accepted : (authenticate input).ok = true) :
    readableParticipant input.participants input.localPeer = true ∧
    readableParticipant input.participants (authenticate input).remote = true ∧ input.hash ≠ "" :=
  authorizeParticipants_spec (authenticate_admitted accepted).2.1

/-- Successful authentication binds distinct transport endpoints and the local participant key. -/
theorem authenticate_transport {input : AuthenticationInput} (accepted : (authenticate input).ok = true) :
    input.localTransport ≠ "" ∧ input.remoteTransport ≠ "" ∧ input.localTransport ≠ input.remoteTransport ∧
    input.keyPeer = some input.localPeer ∧ input.remoteNonceSize = 32 ∧ input.noncesDistinct = true := by
  unfold authenticate at accepted
  simp only [Id.run, pure] at accepted
  repeat' first | split at accepted | simp_all

/-- Healthy mutually admitted endpoints complete authentication, including the optional observer call. -/
theorem authenticate_succeeds {input : AuthenticationInput} {remote : String}
    (localTransport : input.localTransport ≠ "") (remoteTransport : input.remoteTransport ≠ "")
    (distinct : input.localTransport ≠ input.remoteTransport) (key : input.keyPeer = some input.localPeer)
    (primitives : input.nonceOK && input.challengeOK && input.signOK && input.proofExchangeOK &&
      input.stateOK && input.authorizationOK = true)
    (nonceSize : input.remoteNonceSize = 32) (nonceDistinct : input.noncesDistinct = true)
    (proof : verifyParticipantProof input.proof = some remote)
    (held : authorizeParticipants input.participants input.hash input.localPeer remote = true)
    (acknowledged : input.remoteAuthorization = some true) :
    authenticate input = ⟨true, remote, if input.observer then some true else none⟩ := by
  simp only [Bool.and_eq_true] at primitives
  simp_all [authenticate]

end Spacewave.SObject.Sync
