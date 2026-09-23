import Lean.Data.Json
import Spacewave.SObject.State

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

A request the oracle cannot parse yields `{"error"}`. Field names match the
model structures and the projection in `core/sobject/lean-conformance_test.go`.
-/

open Lean Spacewave.SObject

deriving instance ToJson, FromJson for Participant, Config, Sig, Entry
deriving instance ToJson, FromJson for AccountNonce, Operation, Rejections, Root, State

/-- respond evaluates one request against the model. -/
def respond (req : Json) : Except String Json := do
  match ← req.getObjValAs? String "op" with
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
