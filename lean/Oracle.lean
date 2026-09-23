import Lean.Data.Json
import Spacewave.SObject.Host
import Spacewave.SObject.KeyRotation
import Spacewave.SObject.Invite

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
deriving instance ToJson, FromJson for HostResult
deriving instance ToJson, FromJson for RotationPeer, KeyGrant, KeyEpoch, Rotation

/-- respond evaluates one request against the model. -/
def respond (req : Json) : Except String Json := do
  match ← req.getObjValAs? String "op" with
  | "validateInviteUsable" =>
    return json% {ok: $(validateInviteUsable (← req.getObjValAs? Invite "invite")
      (← req.getObjValAs? Bool "expired"))}
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
