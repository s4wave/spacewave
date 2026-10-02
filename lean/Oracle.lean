import Lean.Data.Json
import Spacewave.SObject.Host
import Spacewave.SObject.Process
import Spacewave.SObject.KeyRotation
import Spacewave.SObject.Invite
import Spacewave.SObject.RemoveParticipant
import Spacewave.SObject.Reencrypt
import Spacewave.SObject.Leave
import Spacewave.SObject.Recovery
import Spacewave.SObject.Sync.Auth
import Spacewave.SObject.Sync.Catchup
import Spacewave.SObject.Sync.Sync

/-!
# Conformance oracle

Reads one JSON request per line from standard input and writes one JSON result
per line. A request names a model function in `op` and carries its inputs:

- `verifyChange`: `current`, `entry`; result `{"ok", "config"}`.
- `verifySuffix`: `current`, `candidate`, `entries`; result `{"ok"}`.
- `verifyChain`: `entries`; result `{"ok"}`.
- `validateState`: `state`; result `{"ok"}`.
- `validateNextRootState`: `state`, `root`, `enforce`; result `{"ok"}`.
- `getNextAccountNonce`: `state`, `peer`; result `{"ok", "nonce"}`.
- `queueOperation`: `state`, `operation`; result `{"ok", "state"}`.
- `updateRootState`: `state`, `root`, `enforce`, `rejected`, `accepted`;
  result `{"ok", "state"}`.
- `clearOperationResult`: `state`, `peer`, `localId`, `format`, `sig`;
  result `{"ok", "state"}`.
- `importPeerSnapshot`: `previous`, `candidate`, `entries`, `localPeer`,
  `candidateBytes`, `historyBytes`, `lockOK`, `accessOK`, `writeOK`.
- `applyConfigChange`: `previous`, `entry`, `callbackOK`, `replacement`,
  `lockOK`, `writeOK`.
- `installInviteSnapshot`: `previous`, `candidate`, `checkpoint`, `lockOK`, `writeOK`.
- `hostUpdateRootState`: `previous`, `root`, `enforce`, `rejected`, `accepted`,
  `lockOK`, `writeOK`.
- `hostQueueOperation`: `previous`, nullable `operation`, `validator`, nullable
  `processed`, `lockOK`, `writeOK`.
- `processOperations`: `state`, `input`; result `{"ok", "processed"}`.
- `advanceSyncExchange`: held-state reads and one selected production loop event.
- `joinSyncWorkers`: optional routine exit channels and observed body acknowledgments.
- `watchSyncAuthority`: host retention, current state reads and transport deadline observations.
- `prepareSyncOutgoing`, `receiveSyncExchange`: sole-owner protocol state and primitive
  observations.
- `writeSyncFrames`: endpoint identities and current-authority/transport trace; result
  `{"ok", "writer"}`.
- `startSyncStream`: authentication inputs and deadline observations; result `{"ok", "started"}`.
- `authenticateSync`: wire and primitive authentication observations; result
  `{"ok", "authentication"}`.
- `authorizeSync`: participants, held hash and endpoint identities; result `{"ok"}`.
- `verifySyncProof`: exact-transcript proof observation; result `{"ok", "remote"}`.
- `receiveSyncPages`: initial receive buffer and complete page sequence; result
  `{"ok", "received"}`.
- `acceptSyncResponse`: complete pinned response, decoded bytes and host observations; result
  `{"ok", "accepted"}`.
- `prepareSyncResponse`: pinned state, history read and encoding observations; result
  `{"ok", "response"}`.
- `syncStateHash`: stripped-state encoding and digest observations; result `{"ok", "digest"}`.
- `syncResponseObsolete`: observed state and pinned head; result `{"ok"}`.
- `appendSyncPage`: receive buffer and raw page projection; result `{"ok", "received"}`.
- `nextSyncMessage`: response buffer and measured page-prefix sizes; result `{"ok", "message"}`.
- `buildRecoveryEnvelope`: envelope inputs and primitive `cryptoOK`/`encoded`.
- `unlockRecovery`: `keys`, `envelope`, primitive `decoded`; result `{"ok", "material"}`.
- `resolveRecovery`: provider outcomes, `entity`, `envelope`, `material`.
- `buildSelfEnroll`: proposal inputs; result `{"ok", "entry"}`.
- `enrollRecovery`: proposal inputs and current admission; result `{"ok", "config"}`.
- `validateRecoveryGrant`: `grant`, `config`; result `{"ok"}`.
- `buildSelfEnrollGrant`: grant inputs and primitive encrypted byte identities.
- `buildLeave`: `request`, `signOK`; result `{"ok", "request"}`.
- `verifyLeave`: `request`; result `{"ok", "peers"}`.
- `leaveProofsRemainCurrent`: `peers`, `signed`, `changes`; result `{"ok"}`.
- `completedLeave`: `requestHash`, `changes`; result `{"ok", "changes"}`.
- `leaveTrace`: `request`, `hostId`, `requestHash`, `attempts`; result `{"ok", "leave"}`.
- `reencryptState`: `input`; result `{"ok", "reencrypted"}`.
- `removeParticipants`: `previous`, `snapshot`, `targets`, `sig`, `hash`, `crypto`,
  `watchOK`, `buildOK`, `lockOK`, `writeOK`; result `{"ok", "removal"}`.
- `mutateInvite`: `previous`, `snapshot`, `kind`, `invite`, `id`, `expired`,
  `sig`, `hash`, `buildOK`, `lockOK`, `writeOK`; host outcome.
- `validateInviteUsable`: `invite`, `expired`; result `{"ok"}`.
- `findInvite`: `invites`, `id`; result `{"ok", "invite"}`.
- `rotateTransformKey`: `participants`, `epoch`, `seqno`, `key`, `cryptoOK`;
  result `{"ok", "rotation"}`.
- `findCoveringEpoch`: `epochs`, `seqno`; result `{"ok", "epoch"}`.
- `currentEpochNumber`: `epochs`; result `{"ok", "epoch"}`.

Host operations return `{"ok", "outcome"}`. Outcome contains visible `state`,
`revoked`, and `wrote`, including unchanged state on rejection.

A request the oracle cannot parse yields `{"error"}`. Field names match the
model structures and the projection in `core/sobject/lean-conformance_test.go`.
-/

open Lean Spacewave.SObject

deriving instance ToJson, FromJson for Participant, Config, Sig, Entry
deriving instance ToJson, FromJson for AccountNonce, Operation, Rejections, Grant, Root
deriving instance ToJson, FromJson for Invite, State
deriving instance ToJson, FromJson for HostResult, ProcessedRoot
deriving instance ToJson, FromJson for ProcessOp, OpResult, RejectionRef, ProcessInput, Processed
deriving instance ToJson, FromJson for RewrapInput, RemovalCrypto, RemovalResult
deriving instance ToJson, FromJson for RotationPeer, KeyGrant, KeyEpoch, Rotation
deriving instance ToJson, FromJson for PlainRoot, ReencryptInput, Reencrypted

deriving instance ToJson, FromJson for LeaveRequest, LeaveChange, LeaveAttempt, LeaveResult
deriving instance ToJson, FromJson for LeaveTrace

deriving instance ToJson, FromJson for RecoveryMaterial, RecoveryEnvelope, RecoveryGrant

deriving instance ToJson, FromJson for Sync.AuthenticationInput, Sync.AuthenticationResult
deriving instance ToJson, FromJson for Sync.StreamStart
deriving instance ToJson, FromJson for Sync.AuthorityRead, Sync.AuthorityWatch
deriving instance ToJson, FromJson for Sync.WorkerJoin, Sync.JoinResult, Sync.StreamFinish
deriving instance ToJson, FromJson for Sync.Head, Sync.HistoryChange, Sync.Receive, Sync.HistoryPage
deriving instance ToJson, FromJson for Sync.ReceiveResult
deriving instance ToJson, FromJson for Sync.Snapshot, Sync.Response, Sync.NextMessage
deriving instance ToJson, FromJson for Sync.Request, Sync.AcceptanceInput, Sync.AcceptanceResult
deriving instance ToJson, FromJson for Sync.WriterAttempt, Sync.WriterResult
deriving instance ToJson, FromJson for Sync.ExchangeFrame, Sync.Exchange, Sync.ExchangePrimitives
deriving instance ToJson, FromJson for Sync.ExchangeResult
deriving instance ToJson, FromJson for Sync.LoopInput, Sync.LoopResult, Sync.LoopObservation

/-- respond evaluates one request against the model. -/
def respond (req : Json) : Except String Json := do
  match ← req.getObjValAs? String "op" with
  | "receiveSyncPages" =>
    let result := Sync.receivePages (← req.getObjValAs? Sync.Receive "before")
      (← req.getObjValAs? (List Sync.HistoryPage) "pages")
    return json% {ok: $(result.isSome), received: $result}
  | "acceptSyncResponse" =>
    let result := Sync.acceptResponse (← req.getObjValAs? Sync.AcceptanceInput "input")
    return json% {ok: $(result.ok), accepted: $result}
  | "prepareSyncResponse" =>
    let state ← req.getObjValAs? State "state"
    let request ← req.getObjValAs? Sync.Request "request"
    let history ← req.getObjValAs? (Option (List Sync.HistoryChange)) "history"
    let encoded ← req.getObjValAs? (Option String) "encoded"
    let bytes ← req.getObjValAs? Nat "bytes"
    let result := Sync.prepareResponse state request history (fun _ => encoded) (fun _ => bytes)
    return json% {ok: $(result.isSome), response: $result}
  | "syncStateHash" =>
    let state ← req.getObjValAs? State "state"
    let bytes ← req.getObjValAs? Nat "bytes"
    let encoded ← req.getObjValAs? (Option String) "encoded"
    let digest ← req.getObjValAs? String "digest"
    let result := Sync.syncStateHash state (fun _ => bytes) (fun _ => encoded) (fun _ => digest)
    return json% {ok: $(result.isSome), digest: $result}
  | "syncResponseObsolete" =>
    let result := Sync.responseObsolete (← req.getObjValAs? (Option State) "current")
      (← req.getObjValAs? Sync.Head "head")
    return json% {ok: $result}
  | "appendSyncPage" =>
    let result := Sync.appendPage (← req.getObjValAs? Sync.Receive "before")
      (← req.getObjValAs? (Option Sync.HistoryPage) "page")
    return json% {ok: $(result.ok), received: $result}
  | "nextSyncMessage" =>
    let result := Sync.nextMessage (← req.getObjValAs? Sync.Response "before")
      (← req.getObjValAs? (List Nat) "sizes")
    return json% {ok: $(result.ok), message: $result}
  | "prepareSyncOutgoing" =>
    let result := Sync.prepareOutgoing (← req.getObjValAs? Sync.Exchange "before")
      (← req.getObjValAs? (Option State) "current")
      (← req.getObjValAs? Sync.ExchangePrimitives "input")
    return json% {ok: $(result.ok), exchange: $result}
  | "receiveSyncExchange" =>
    let input ← req.getObjValAs? Sync.ExchangePrimitives "input"
    let result := Sync.receiveExchange (← req.getObjValAs? Sync.Exchange "before")
      (← req.getObjValAs? State "current") (← req.getObjValAs? Sync.ExchangeFrame "frame") input
    let host := if result.operation then none else
      some ((result.imported.map (·.host)).getD (visibleHost input.acceptance.previous none))
    return json% {ok: $(result.ok), exchange: $result, host: $host}
  | "joinSyncWorkers" =>
    let joining := Sync.joinWorkers (← req.getObjValAs? (List Sync.WorkerJoin) "workers")
    return json% {joining: $joining}
  | "advanceSyncExchange" =>
    let input ← req.getObjValAs? Sync.LoopInput "loop"
    let some result := Sync.advanceExchange (← req.getObjValAs? Sync.Exchange "before")
      (← req.getObjValAs? String "local") (← req.getObjValAs? String "remote") input
      (← req.getObjValAs? Sync.ExchangeFrame "frame") | throw "impossible selected event"
    let host := if result.exchange.operation then none else
      some ((result.exchange.imported.map (·.host)).getD
        (visibleHost input.reception.acceptance.previous none))
    return json% {ok: $(result.exchange.ok), exchange: $(result.exchange), host: $host,
      denial: $(result.denial), handed: $(result.handed)}
  | "watchSyncAuthority" =>
    let watcher := Sync.watchStreamAuthority (← req.getObjValAs? String "local")
      (← req.getObjValAs? String "remote")
      (← req.getObjValAs? Int "retainError") (← req.getObjValAs? Bool "deadlineOK")
      (← req.getObjValAs? (List Sync.AuthorityRead) "reads")
    return json% {watcher: $watcher}
  | "writeSyncFrames" =>
    let writer := Sync.writeFrames (← req.getObjValAs? String "local")
      (← req.getObjValAs? String "remote")
      (← req.getObjValAs? (List Sync.WriterAttempt) "attempts")
    return json% {ok: $(writer.waiting), writer: $writer}
  | "startSyncStream" =>
    let started := Sync.startStream (← req.getObjValAs? Sync.AuthenticationInput "input")
      (← req.getObjValAs? Bool "deadlineOK") (← req.getObjValAs? Bool "resetOK")
    return Json.mkObj [("ok", toJson started.remote.isSome), ("started", toJson started)]
  | "authenticateSync" =>
    let result := Sync.authenticate (← req.getObjValAs? Sync.AuthenticationInput "input")
    return json% {ok: $(result.ok), authentication: $result}
  | "authorizeSync" =>
    let result := Sync.authorizeParticipants
      (← req.getObjValAs? (List Participant) "participants")
      (← req.getObjValAs? String "hash") (← req.getObjValAs? String "local")
      (← req.getObjValAs? String "remote")
    return json% {ok: $result}
  | "verifySyncProof" =>
    let result := Sync.verifyParticipantProof (← req.getObjValAs? (Option Sig) "proof")
    return json% {ok: $(result.isSome), remote: $result}
  | "buildRecoveryEnvelope" =>
    let result := buildRecoveryEnvelope (← req.getObjValAs? String "entity")
      (← req.getObjValAs? Nat "epoch") (← req.getObjValAs? (Option Config) "config")
      (← req.getObjValAs? (Option RecoveryMaterial) "material")
      (← req.getObjValAs? (List String) "recipients") (← req.getObjValAs? Bool "cryptoOK")
      (← req.getObjValAs? String "encoded")
    return json% {ok: $(result.isSome), envelope: $result}
  | "unlockRecovery" =>
    let result := unlockRecovery (← req.getObjValAs? (List String) "keys")
      (← req.getObjValAs? (Option RecoveryEnvelope) "envelope")
      (← req.getObjValAs? (Option RecoveryMaterial) "decoded")
    return json% {ok: $(result.isSome), material: $result}
  | "resolveRecovery" =>
    let result := resolveRecovery (← req.getObjValAs? Bool "featureOK")
      (← req.getObjValAs? (Option String) "entity") (← req.getObjValAs? Bool "readOK")
      (← req.getObjValAs? (Option RecoveryEnvelope) "envelope")
      (← req.getObjValAs? Bool "decoderOK") (← req.getObjValAs? Bool "decodeOK")
      (← req.getObjValAs? (Option RecoveryMaterial) "material")
    return json% {ok: $(result.isSome), material: $result}
  | "buildSelfEnroll" =>
    let result := buildSelfEnroll (← req.getObjValAs? (Option Config) "current")
      (← req.getObjValAs? (Option Sig) "sig") (← req.getObjValAs? String "peer")
      (← req.getObjValAs? String "entity") (← req.getObjValAs? Int "role")
      (← req.getObjValAs? String "hash") (← req.getObjValAs? Bool "signOK")
    return json% {ok: $(result.isSome), entry: $result}
  | "enrollRecovery" =>
    let result := enrollRecovery (← req.getObjValAs? Config "current")
      (← req.getObjValAs? Sig "sig") (← req.getObjValAs? String "peer")
      (← req.getObjValAs? String "entity") (← req.getObjValAs? Int "role")
      (← req.getObjValAs? String "hash") (← req.getObjValAs? Bool "signOK")
    return json% {ok: $(result.isSome), config: $result}
  | "validateRecoveryGrant" =>
    let grant ← req.getObjValAs? Grant "grant"
    let config ← req.getObjValAs? Config "config"
    return json% {ok: $(grant.valid config.participants)}
  | "buildSelfEnrollGrant" =>
    let result := buildSelfEnrollGrant (← req.getObjValAs? String "signer")
      (← req.getObjValAs? String "peer") (← req.getObjValAs? String "object")
      (← req.getObjValAs? (Option RecoveryMaterial) "material")
      (← req.getObjValAs? Bool "publicKey") (← req.getObjValAs? Bool "cryptoOK")
      (← req.getObjValAs? String "encoded") (← req.getObjValAs? String "inner")
    return json% {ok: $(result.isSome), grant: $result}
  | "buildLeave" =>
    let result := buildLeave (← req.getObjValAs? LeaveRequest "request")
      (← req.getObjValAs? Bool "signOK")
    return json% {ok: $(result.isSome), request: $result}
  | "verifyLeave" =>
    let result := verifyLeave (← req.getObjValAs? LeaveRequest "request")
    return json% {ok: $(result.isSome), peers: $result}
  | "leaveProofsRemainCurrent" =>
    return json% {ok: $(leaveProofsRemainCurrent (← req.getObjValAs? (List String) "peers")
      (← req.getObjValAs? Config "signed") (← req.getObjValAs? (List LeaveChange) "changes"))}
  | "completedLeave" =>
    let result := completedLeave (← req.getObjValAs? String "requestHash")
      (← req.getObjValAs? (List LeaveChange) "changes")
    return json% {ok: $(result.isSome), changes: $result}
  | "leaveTrace" =>
    let result := leaveTrace (← req.getObjValAs? LeaveRequest "request")
      (← req.getObjValAs? String "hostId") (← req.getObjValAs? String "requestHash")
      (← req.getObjValAs? (List LeaveAttempt) "attempts")
    return json% {ok: $(!result.pending && result.result.isSome), leave: $result}
  | "reencryptState" =>
    let result := reencryptState (← req.getObjValAs? ReencryptInput "input")
    return json% {ok: $(result.isSome), reencrypted: $result}
  | "removeParticipants" =>
    let previous ← req.getObjValAs? State "previous"
    let result := removeParticipants previous (← req.getObjValAs? (Option Config) "snapshot")
      (← req.getObjValAs? (List String) "targets") (← req.getObjValAs? Sig "sig")
      (← req.getObjValAs? String "hash") (← req.getObjValAs? RemovalCrypto "crypto")
      (← req.getObjValAs? Bool "watchOK") (← req.getObjValAs? Bool "buildOK")
      (← req.getObjValAs? Bool "lockOK") (← req.getObjValAs? Bool "writeOK")
    let outcome := visibleHost previous (result.map (·.outcome))
    return json% {ok: $(result.isSome), removal: {removed: $((result.map (·.removed)).getD []),
      outcome: $outcome}}
  | "validateInviteUsable" =>
    return json% {ok: $(validateInviteUsable (← req.getObjValAs? Invite "invite")
      (← req.getObjValAs? Bool "expired"))}
  | "redeemInvite" =>
    let result := redeemInvite (← req.getObjValAs? Bool "approvalRequired")
      (← req.getObjValAs? (Option (List String)) "participantOf")
      (← req.getObjValAs? (List (String × List String)) "held")
      (← req.getObjValAs? String "peer")
    return json% {ok: true, redemption: $(result.code)}
  | "findInvite" =>
    let result := findInvite (← req.getObjValAs? (List Invite) "invites")
      (← req.getObjValAs? String "id")
    return json% {ok: $(result.isSome), invite: $result}
  | "mutateInvite" =>
    let previous ← req.getObjValAs? State "previous"
    let result := mutateInvite previous (← req.getObjValAs? Config "snapshot")
      (← req.getObjValAs? Int "kind") (← req.getObjValAs? Invite "invite")
      (← req.getObjValAs? String "id") (← req.getObjValAs? Bool "expired")
      (← req.getObjValAs? Sig "sig") (← req.getObjValAs? String "hash")
      (← req.getObjValAs? Bool "buildOK") (← req.getObjValAs? Bool "lockOK")
      (← req.getObjValAs? Bool "writeOK")
    return json% {ok: $(result.isSome), outcome: $(visibleHost previous result)}
  | "rotateTransformKey" =>
    let result := rotateTransformKey (← req.getObjValAs? (List RotationPeer) "participants")
      (← req.getObjValAs? Nat "epoch") (← req.getObjValAs? Nat "seqno")
      (← req.getObjValAs? String "key") (← req.getObjValAs? Bool "cryptoOK")
    return json% {ok: $(result.isSome), rotation: $result}
  | "findCoveringEpoch" =>
    let result := findCoveringEpoch (← req.getObjValAs? (List (Option KeyEpoch)) "epochs")
      (← req.getObjValAs? Nat "seqno")
    return json% {ok: $(result.isSome), epoch: $result}
  | "currentEpochNumber" =>
    let result := currentEpochNumber (← req.getObjValAs? (List (Option KeyEpoch)) "epochs")
    return json% {ok: true, epoch: $result}
  | "importPeerSnapshot" =>
    let previous ← req.getObjValAs? State "previous"
    let result := importPeerSnapshot previous (← req.getObjValAs? State "candidate")
      (← req.getObjValAs? (List Entry) "entries") (← req.getObjValAs? String "localPeer")
      (← req.getObjValAs? Nat "candidateBytes") (← req.getObjValAs? Nat "historyBytes")
      (← req.getObjValAs? Bool "lockOK") (← req.getObjValAs? Bool "accessOK")
      (← req.getObjValAs? Bool "writeOK")
    return json% {ok: $(result.isSome), outcome: $(visibleHost previous result)}
  | "applyConfigChange" =>
    let previous ← req.getObjValAs? State "previous"
    let callbackOK ← req.getObjValAs? Bool "callbackOK"
    let replacement ← req.getObjValAs? (Option State) "replacement"
    let callback := fun s => if callbackOK then some (replacement.getD s) else none
    let result := applyConfigChange previous (← req.getObjValAs? (Option Entry) "entry")
      callback (← req.getObjValAs? Bool "lockOK") (← req.getObjValAs? Bool "writeOK")
    return json% {ok: $(result.isSome), outcome: $(visibleHost previous result)}
  | "installInviteSnapshot" =>
    let previous ← req.getObjValAs? State "previous"
    let result := installInviteSnapshot (← req.getObjValAs? State "candidate")
      (← req.getObjValAs? Bool "checkpoint") (← req.getObjValAs? Bool "lockOK")
      (← req.getObjValAs? Bool "writeOK")
    return json% {ok: $(result.isSome), outcome: $(visibleHost previous result)}
  | "hostUpdateRootState" =>
    let previous ← req.getObjValAs? State "previous"
    let result := hostUpdateRootState previous (← req.getObjValAs? Root "root")
      (← req.getObjValAs? String "enforce") (← req.getObjValAs? (List Operation) "rejected")
      (← req.getObjValAs? (List Operation) "accepted") (← req.getObjValAs? Bool "lockOK")
      (← req.getObjValAs? Bool "writeOK")
    return json% {ok: $(result.isSome), outcome: $(visibleHost previous result)}
  | "hostQueueOperation" =>
    let previous ← req.getObjValAs? State "previous"
    let result := hostQueueOperation previous (← req.getObjValAs? (Option Operation) "operation")
      (← req.getObjValAs? String "validator")
      (← req.getObjValAs? (Option ProcessedRoot) "processed") (← req.getObjValAs? Bool "lockOK")
      (← req.getObjValAs? Bool "writeOK")
    return json% {ok: $(result.isSome), outcome: $(visibleHost previous result)}
  | "verifyChange" =>
    let result := verifyChange (← req.getObjValAs? Config "current")
      (← req.getObjValAs? Entry "entry")
    return json% {ok: $(result.isSome), config: $(result)}
  | "verifySuffix" =>
    let ok := verifySuffix (← req.getObjValAs? Config "current")
      (← req.getObjValAs? Config "candidate") (← req.getObjValAs? (List Entry) "entries")
    return json% {ok: $ok}
  | "verifyChain" =>
    let ok := (verifyChain (← req.getObjValAs? (List Entry) "entries")).isSome
    return json% {ok: $ok}
  | "validateState" =>
    return json% {ok: $((← req.getObjValAs? State "state").validate)}
  | "validateNextRootState" =>
    let ok := validateNextRootState (← req.getObjValAs? State "state")
      (← req.getObjValAs? Root "root") (← req.getObjValAs? String "enforce")
    return json% {ok: $ok}
  | "getNextAccountNonce" =>
    let nonce := getNextAccountNonce (← req.getObjValAs? State "state")
      (← req.getObjValAs? String "peer")
    return json% {ok: true, nonce: $nonce}
  | "queueOperation" =>
    let result := queueOperation (← req.getObjValAs? State "state")
      (← req.getObjValAs? Operation "operation")
    return json% {ok: $(result.isSome), state: $result}
  | "updateRootState" =>
    let result := updateRootState (← req.getObjValAs? State "state")
      (← req.getObjValAs? Root "root") (← req.getObjValAs? String "enforce")
      (← req.getObjValAs? (List Operation) "rejected")
      (← req.getObjValAs? (List Operation) "accepted")
    return json% {ok: $(result.isSome), state: $result}
  | "processOperations" =>
    let result := processOperations (← req.getObjValAs? State "state")
      (← req.getObjValAs? ProcessInput "input")
    return json% {ok: $(result.isSome), processed: $result}
  | "clearOperationResult" =>
    let result := clearOperationResult (← req.getObjValAs? State "state")
      (← req.getObjValAs? String "peer") (← req.getObjValAs? String "localId")
      (← req.getObjValAs? Bool "format") (← req.getObjValAs? Sig "sig")
    return json% {ok: $(result.isSome), state: $result}
  | op => throw s!"unknown op {op}"

/-- serve answers requests until standard input closes. -/
partial def serve (stdin : IO.FS.Stream) (stdout : IO.FS.Stream) : IO Unit := do
  let line ← stdin.getLine
  if line.isEmpty then
    return

  let result := match Json.parse line.trimAscii.toString >>= respond with
    | .ok json => json
    | .error msg => json% {error: $msg}
  stdout.putStrLn result.compress
  serve stdin stdout

def main : IO Unit := do
  let stdout ← IO.getStdout
  serve (← IO.getStdin) stdout
  stdout.flush
