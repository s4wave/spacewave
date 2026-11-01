import Lean.Data.Json
import Spacewave.SObject.Envelope
import Spacewave.SObject.Order
import Spacewave.SObject.Recovery
import Spacewave.SObject.Sync.Auth
import Spacewave.SObject.Sync.Sync

/-!
# Mathematical and security conformance oracle

Reads one JSON request per line and returns one result. Checks configuration
chains, signed envelopes, recovery grants, ordering, peer authentication and
authenticated data entry. Temporal host and exchange models live beside Go
as TLA+ specifications. Crypto primitive outcomes remain explicit observations.
-/

open Lean Spacewave.SObject

deriving instance ToJson, FromJson for Participant, Config, Sig

mutual
/-- entryToJson encodes an entry with its nested merge, which deriving cannot. -/
partial def entryToJson (e : Entry) : Json :=
  json% {object: $e.object, seqno: $e.seqno, config: $e.config, sigs: $e.sigs, prev: $e.prev,
    kind: $e.kind, hash: $e.hash, merge: $(mergeToJson e.merge)}

/-- mergeToJson encodes a merge as null or `{"fork", "branch"}`. -/
partial def mergeToJson : Merge → Json
  | .none => .null
  | .some fork branch => json% {fork: $(entryToJson fork), branch: $(branchToJson branch)}

/-- branchToJson encodes a branch as its entries, oldest first. -/
partial def branchToJson : Branch → List Json
  | .nil => []
  | .cons head tail => entryToJson head :: branchToJson tail
end

mutual
/-- entryFromJson decodes an entry encoded by entryToJson. -/
partial def entryFromJson (j : Json) : Except String Entry := do
  return {
    object := ← j.getObjValAs? String "object", seqno := ← j.getObjValAs? Nat "seqno"
    config := ← j.getObjValAs? (Option Config) "config", sigs := ← j.getObjValAs? (List Sig) "sigs"
    prev := ← j.getObjValAs? String "prev", kind := ← j.getObjValAs? ChangeType "kind"
    hash := ← j.getObjValAs? String "hash"
    merge := ← mergeFromJson ((j.getObjVal? "merge").toOption.getD .null) }

/-- mergeFromJson decodes a merge encoded by mergeToJson. -/
partial def mergeFromJson : Json → Except String Merge
  | .null => return .none
  | j => do
    let branch ← (← (← j.getObjVal? "branch").getArr?).toList.mapM entryFromJson
    return .some (← entryFromJson (← j.getObjVal? "fork")) (branch.foldr .cons .nil)
end

instance : ToJson Entry := ⟨entryToJson⟩
instance : FromJson Entry := ⟨entryFromJson⟩
deriving instance ToJson, FromJson for Grant

deriving instance ToJson, FromJson for Order.Pos, Order.Op

deriving instance ToJson, FromJson for Position, OpBody, Operation, CheckpointBody, Checkpoint

deriving instance ToJson, FromJson for RecoveryMaterial, RecoveryEnvelope, RecoveryGrant

deriving instance ToJson, FromJson for Sync.AuthenticationInput, Sync.AuthenticationResult
deriving instance ToJson, FromJson for Sync.StreamStart
/-- respond evaluates one request against the model. -/
def respond (req : Json) : Except String Json := do
  match ← req.getObjValAs? String "op" with
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
