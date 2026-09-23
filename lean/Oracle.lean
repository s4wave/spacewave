import Lean.Data.Json
import Spacewave.SObject.ConfigChain

/-!
# Conformance oracle

Reads one JSON request per line from standard input and writes one JSON result
per line. A request names a model function in `op` and carries its inputs:

- `verifyChange`: `current`, `entry`; result `{"ok", "config"}`.
- `verifySuffix`: `current`, `candidate`, `entries`; result `{"ok"}`.
- `verifyChain`: `entries`; result `{"ok"}`.

A request the oracle cannot parse yields `{"error"}`. Field names match the
model structures and the projection in `core/sobject/lean-conformance_test.go`.
-/

open Lean Spacewave.SObject

deriving instance ToJson, FromJson for Participant, Config, Sig, Entry

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
