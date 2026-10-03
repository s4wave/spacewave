import Lean.Data.Json
import Spacewave.SObject.Envelope
import Spacewave.SObject.Host
import Spacewave.SObject.Order
import Spacewave.SObject.Recovery
import Spacewave.SObject.Sync.Auth
import Spacewave.SObject.Sync.Catchup
import Spacewave.SObject.Sync.Sync

/-!
# Conformance oracle

Reads one JSON request per line from standard input and writes one JSON result
per line. A request names a model function in `op` and carries its inputs:

- `verifyChange`: `object`, `current`, `entry`; result `{"ok", "config"}`.
- `verifySuffix`: `object`, `current`, `candidate`, `entries`; result `{"ok"}`.
- `verifyChain`: `object`, `entries`; result `{"ok"}`.
- `authorizeOperation`: `object`, `participants`, `operation`; result
  `{"ok", "result": {"authorized"}}`, where `ok` is `verifyOperation`.
- `authorizeCheckpoint`: `object`, `participants`, `checkpoint`; result
  `{"ok", "result": {"signers", "authorized"}}`, where `ok` is `verifyCheckpoint`.
- `orderOperations`: held `ops`, `checkpoint` author heads, `roster`; result
  `{"ok", "result": {"order", "stable"}}`.
- `importPeerSnapshot`: `object`, `previous`, `candidate`, `entries`, `localPeer`,
  `candidateBytes`, `historyBytes`, nullable `merged`, `lockOK`, `accessOK`, `writeOK`.
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
- `buildSelfEnroll`: `object` and proposal inputs; result `{"ok", "entry"}`.
- `enrollRecovery`: proposal inputs and current admission; result `{"ok", "config"}`.
- `validateRecoveryGrant`: `grant`, `config`; result `{"ok"}`.
- `buildSelfEnrollGrant`: grant inputs and primitive encrypted byte identities.

Host operations return `{"ok", "outcome"}`. Outcome contains visible `state`,
`revoked`, and `wrote`, including unchanged state on rejection.

A request the oracle cannot parse yields `{"error"}`. Field names match the
model structures and the projections in the Go conformance tests.
-/

open Lean Spacewave.SObject

deriving instance ToJson, FromJson for Participant, Config, Sig, Entry
deriving instance ToJson, FromJson for Grant, State, HostResult

deriving instance ToJson, FromJson for Order.Pos, Order.Op

deriving instance ToJson, FromJson for Position, OpBody, Operation, CheckpointBody, Checkpoint

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
    let host := (result.imported.map (·.host)).getD (visibleHost input.acceptance.previous none)
    return json% {ok: $(result.ok), exchange: $result, host: $host}
  | "joinSyncWorkers" =>
    let joining := Sync.joinWorkers (← req.getObjValAs? (List Sync.WorkerJoin) "workers")
    return json% {joining: $joining}
  | "advanceSyncExchange" =>
    let input ← req.getObjValAs? Sync.LoopInput "loop"
    let some result := Sync.advanceExchange (← req.getObjValAs? Sync.Exchange "before")
      (← req.getObjValAs? String "local") (← req.getObjValAs? String "remote") input
      (← req.getObjValAs? Sync.ExchangeFrame "frame") | throw "impossible selected event"
    let host := (result.exchange.imported.map (·.host)).getD
      (visibleHost input.reception.acceptance.previous none)
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
    let result := buildSelfEnroll (← req.getObjValAs? String "object")
      (← req.getObjValAs? (Option Config) "current")
      (← req.getObjValAs? (Option Sig) "sig") (← req.getObjValAs? String "peer")
      (← req.getObjValAs? String "entity") (← req.getObjValAs? Int "role")
      (← req.getObjValAs? String "hash") (← req.getObjValAs? Bool "signOK")
    return json% {ok: $(result.isSome), entry: $result}
  | "enrollRecovery" =>
    let result := enrollRecovery (← req.getObjValAs? String "object")
      (← req.getObjValAs? Config "current")
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
  | "importPeerSnapshot" =>
    let previous ← req.getObjValAs? State "previous"
    let result := importPeerSnapshot (← req.getObjValAs? String "object") previous
      (← req.getObjValAs? State "candidate")
      (← req.getObjValAs? (List Entry) "entries") (← req.getObjValAs? String "localPeer")
      (← req.getObjValAs? Nat "candidateBytes") (← req.getObjValAs? Nat "historyBytes")
      (← req.getObjValAs? (Option State) "merged") (← req.getObjValAs? Bool "lockOK") (← req.getObjValAs? Bool "accessOK")
      (← req.getObjValAs? Bool "writeOK")
    return json% {ok: $(result.isSome), outcome: $(visibleHost previous result)}
  | "verifyChange" =>
    let result := verifyChange (← req.getObjValAs? String "object")
      (← req.getObjValAs? Config "current")
      (← req.getObjValAs? Entry "entry")
    return json% {ok: $(result.isSome), config: $(result)}
  | "verifySuffix" =>
    let ok := verifySuffix (← req.getObjValAs? String "object")
      (← req.getObjValAs? Config "current")
      (← req.getObjValAs? Config "candidate") (← req.getObjValAs? (List Entry) "entries")
    return json% {ok: $ok}
  | "verifyChain" =>
    let ok := (verifyChain (← req.getObjValAs? String "object")
      (← req.getObjValAs? (List Entry) "entries")).isSome
    return json% {ok: $ok}
  | "authorizeOperation" =>
    let object ← req.getObjValAs? String "object"
    let participants ← req.getObjValAs? (List Participant) "participants"
    let operation ← req.getObjValAs? Operation "operation"
    return json% {ok: $((verifyOperation object operation).isSome),
      result: {authorized: $(authorizeOperation object participants operation)}}
  | "authorizeCheckpoint" =>
    let object ← req.getObjValAs? String "object"
    let participants ← req.getObjValAs? (List Participant) "participants"
    let checkpoint ← req.getObjValAs? Checkpoint "checkpoint"
    let verified := verifyCheckpoint object checkpoint
    return json% {ok: $(verified.isSome), result: {signers: $((verified.map (·.2)).getD []),
      authorized: $(authorizeCheckpoint object participants checkpoint)}}
  | "orderOperations" =>
    let ops ← req.getObjValAs? (List Order.Op) "ops"
    let checkpoint ← req.getObjValAs? (List Order.Pos) "checkpoint"
    let roster ← req.getObjValAs? (List String) "roster"
    let order := Order.order ops checkpoint
    let stable := Order.stablePoint ops checkpoint roster
    return json% {ok: true, result: {order: $order, stable: $stable}}
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
