import Spacewave.SObject.JournalFrame

/-!
# Mutation journal pipeline authentication

Mirrors retained record authentication in `core/sobject/journal-pipeline.go`.
Authenticated decryption, protobuf decoding and SHA-256 are primitive boundaries.
The projection supplies their outputs for the prepared writer-owned sequence;
key, lineage, version and envelope bindings remain model decisions.
-/

namespace Spacewave.SObject.Journal

/-- IntentContent retains all fields compared after authenticated intent decoding. -/
structure IntentContent where
  key : Option Key
  lineage : Option Lineage
  version : Option Version
  deriving Repr, Inhabited

/-- Authentication contains primitive outputs, including failure as None. -/
structure Authentication where
  crypto : Bool
  intent : Option IntentContent
  envelopeHash : Option String
  deriving Repr, Inhabited

/-- authenticateRecord mirrors both retained-stage authentication functions. -/
def authenticateRecord (record : Record) (auth : Authentication) : Bool :=
  if record.kind == 1 then
    auth.crypto && match auth.intent with
    | none => false
    | some intent =>
      sameKey intent.key record.key &&
      sameKey (intent.lineage.getD default).root (record.lineage.getD default).root &&
      sameKey (intent.lineage.getD default).supersedes (record.lineage.getD default).supersedes &&
      intent.version.map (·.data) == record.version.map (·.data)
  else if record.kind == 2 then auth.crypto && auth.envelopeHash == some record.envelopeDigest
  else true

/-- An authenticated intent retains the exact key, predecessor and serialized version. -/
theorem authenticated_intent {record : Record} {auth : Authentication}
    (kind : record.kind = 1) (h : authenticateRecord record auth = true) :
    ∃ intent, auth.crypto = true ∧ auth.intent = some intent ∧
      sameKey intent.key record.key = true ∧
      sameKey (intent.lineage.getD default).root (record.lineage.getD default).root = true ∧
      sameKey (intent.lineage.getD default).supersedes (record.lineage.getD default).supersedes = true ∧
      intent.version.map (·.data) = record.version.map (·.data) := by
  cases found : auth.intent with
  | none => simp [authenticateRecord, kind, found] at h
  | some intent =>
    simp only [authenticateRecord, kind, beq_self_eq_true, ↓reduceIte, found,
      Bool.and_eq_true, beq_iff_eq] at h
    exact ⟨intent, h.1, rfl, h.2.1.1.1, h.2.1.1.2, h.2.1.2, h.2.2⟩

/-- An authenticated envelope must hash to the immutable retained digest. -/
theorem authenticated_envelope {record : Record} {auth : Authentication}
    (kind : record.kind = 2) (h : authenticateRecord record auth = true) :
    auth.crypto = true ∧ auth.envelopeHash = some record.envelopeDigest := by
  simpa [authenticateRecord, kind] using h

end Spacewave.SObject.Journal
