import Spacewave.SObject.JournalFrame

/-!
# Mutation journal pipeline authentication

Mirrors public opening and retained record authentication in `core/sobject/journal-pipeline.go`.
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

/-- attemptIntentRecord names the exact encrypted stage authenticated during snapshot recovery. -/
def attemptIntentRecord (attempt : Attempt) : Record :=
  {(default : Record) with
    kind := 1
    sequence := attempt.intentSequence
    key := attempt.key
    lineage := attempt.lineage
    version := attempt.version
    intent := attempt.intent}

/-- attemptEnvelopeRecord names the immutable envelope stage in the retained snapshot. -/
def attemptEnvelopeRecord (attempt : Attempt) : Record :=
  {(default : Record) with
    kind := 2
    sequence := attempt.envelopeSequence
    key := attempt.key
    envelope := attempt.envelope
    envelopeDigest := attempt.envelopeDigest}

/-- authenticateSnapshots verifies every retained stage before a compact segment may be retired. -/
def authenticateSnapshots (crypto : Bool) (identity : String) (attempts : List (Option Attempt))
    (auth : Record → Authentication) : Bool :=
  crypto && identity.length == digestLength && attempts.all (fun attempt =>
    match attempt with
    | none => false
    | some attempt =>
      authenticateRecord (attemptIntentRecord attempt) (auth (attemptIntentRecord attempt)) &&
      (attempt.envelope.isNone ||
        authenticateRecord (attemptEnvelopeRecord attempt) (auth (attemptEnvelopeRecord attempt))))

/-- recordReceipt selects only receipts that the pipeline actually verifies. -/
def recordReceipt (record : Record) : Option Receipt :=
  if record.kind == 4 then record.receipt
  else if record.kind == 10 && ((record.lookup.getD default).state == 3 || (record.lookup.getD default).state == 4) then
    (record.lookup.getD default).receipt
  else none

/-- pipelineAuthority mirrors retained stage, terminal receipt and lookup verification order.
The receipt and lookup functions are external authority interfaces, not reducer admission results. -/
def pipelineAuthority (before : WriterState) (crypto receiptAvailable lookupAvailable : Bool)
    (auth : Record → Authentication) (receipt : Receipt → Option Version → Bool)
    (lookup : Option Lookup → Option Version → Bool) : Bool :=
  crypto && before.records.all (fun r => authenticateRecord r (auth r)) &&
  authenticateSnapshots crypto before.identity (before.state.map some) auth &&
  receiptAvailable && before.records.all (fun r => (recordReceipt r).all (fun value => receipt value r.version)) &&
  lookupAvailable && before.records.all (fun r => r.kind != 10 || lookup r.lookup r.version) &&
  before.state.all (fun a => a.receipt.all (fun value => receipt value a.version) &&
    (a.lookup.isNone || lookup a.lookup a.version))

/-- finishPipelineOpen activates only after all retained authority checks have succeeded. -/
def finishPipelineOpen (before : WriterState) (input : ActivationInput)
    (crypto receiptAvailable lookupAvailable : Bool) (auth : Record → Authentication)
    (receipt : Receipt → Option Version → Bool) (lookup : Option Lookup → Option Version → Bool) : ActivationResult :=
  if pipelineAuthority before crypto receiptAvailable lookupAvailable auth receipt lookup then
    activateWriter before input
  else ⟨false, before, input.floor⟩

/-- Failed authority checks cannot change bytes, the generation floor or writer state. -/
theorem failed_authority_preserves_storage (before : WriterState) (input : ActivationInput)
    (crypto receiptAvailable lookupAvailable : Bool) (auth : Record → Authentication)
    (receipt : Receipt → Option Version → Bool) (lookup : Option Lookup → Option Version → Bool)
    (h : pipelineAuthority before crypto receiptAvailable lookupAvailable auth receipt lookup = false) :
    finishPipelineOpen before input crypto receiptAvailable lookupAvailable auth receipt lookup =
      ⟨false, before, input.floor⟩ := by
  simp [finishPipelineOpen, h]

/-- A writable pipeline has authenticated every retained stage and external authority result. -/
theorem pipeline_open_authorized {before : WriterState} {input : ActivationInput}
    {crypto receiptAvailable lookupAvailable : Bool} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool}
    (h : (finishPipelineOpen before input crypto receiptAvailable lookupAvailable auth receipt lookup).ok = true) :
    pipelineAuthority before crypto receiptAvailable lookupAvailable auth receipt lookup = true ∧
    (finishPipelineOpen before input crypto receiptAvailable lookupAvailable auth receipt lookup).result.state = before.state := by
  unfold finishPipelineOpen at *
  split at h <;> simp_all [activation_preserves_state]

/-- Every retained terminal receipt in a writable pipeline passed its external authority interface. -/
theorem pipeline_receipts_verified {before : WriterState} {input : ActivationInput}
    {crypto receiptAvailable lookupAvailable : Bool} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool}
    (h : (finishPipelineOpen before input crypto receiptAvailable lookupAvailable auth receipt lookup).ok = true)
    {attempt : Attempt} (member : attempt ∈ before.state) {value : Receipt}
    (retained : attempt.receipt = some value) : receipt value attempt.version = true := by
  have authority := (pipeline_open_authorized h).1
  unfold pipelineAuthority at authority
  simp only [Bool.and_eq_true] at authority
  have checked := List.all_eq_true.mp authority.2 attempt member
  simp only [Bool.and_eq_true] at checked
  simpa [retained] using checked.1

/-- Pipeline activation combines successful authority checks with exact captured-segment retirement. -/
theorem pipeline_retirement {before : WriterState} {input : ActivationInput} {pending : PendingActivation}
    {crypto receiptAvailable lookupAvailable : Bool} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool}
    (waiting : before.pending = some pending)
    (h : (finishPipelineOpen before input crypto receiptAvailable lookupAvailable auth receipt lookup).ok = true) :
    input.retiredDigest = pending.marker.retiredDigest ∧
    before.bytes.data.length = pending.marker.retiredLength ∧
    (finishPipelineOpen before input crypto receiptAvailable lookupAvailable auth receipt lookup).result.bytes.durable = [] ∧
    (finishPipelineOpen before input crypto receiptAvailable lookupAvailable auth receipt lookup).result.state = before.state := by
  have authority := (pipeline_open_authorized h).1
  simp only [finishPipelineOpen, authority, ↓reduceIte] at h ⊢
  have checked := activation_success waiting h
  exact ⟨checked.2.1, checked.2.2.1, checked.2.2.2.2.1, activation_preserves_state before input⟩

/-- AuthenticationEntry carries a primitive result for one exact record API input. -/
structure AuthenticationEntry where
  record : Record
  auth : Authentication
  deriving Repr, Inhabited

/-- findAuthentication resolves projected primitive inputs without performing Go admission. -/
def findAuthentication (entries : List AuthenticationEntry) (record : Record) : Authentication :=
  ((entries.find? (fun entry => entry.record == record)).map (·.auth)).getD default

/-- PipelineOpenResult retains storage effects even when the public constructor returns no pipeline. -/
structure PipelineOpenResult where
  writer : Option WriterState
  bytes : MemoryBytes
  floor : Nat
  deriving Repr, Inhabited

/-- openPipeline mirrors public prerequisites, recovery, retained authority and pending activation.
Authentication primitives are stable for each exact retained-stage input during this call. -/
def openPipeline (input : OpenInput) (activation : ActivationInput) (receiptAvailable lookupAvailable : Bool)
    (auth : Record → Authentication) (receipt : Receipt → Option Version → Bool)
    (lookup : Option Lookup → Option Version → Bool) : PipelineOpenResult :=
  if !input.storageAvailable || !input.crypto || !receiptAvailable || !lookupAvailable then
    ⟨none, input.bytes, input.floor⟩
  else
    let opened := openWriter input (fun r => authenticateRecord r (auth r))
      (fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)
    match opened.writer with
    | none => ⟨none, opened.bytes, input.floor⟩
    | some writer =>
      let finished := finishPipelineOpen writer activation input.crypto receiptAvailable lookupAvailable auth receipt lookup
      ⟨if finished.ok then some finished.result else none, finished.result.bytes, finished.floor⟩

/-- Missing public capabilities reject before recovery can truncate or sync the byte stream. -/
theorem public_pipeline_prerequisites {input : OpenInput} {activation : ActivationInput}
    {receiptAvailable lookupAvailable : Bool} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool}
    (missing : input.storageAvailable = false ∨ input.crypto = false ∨
      receiptAvailable = false ∨ lookupAvailable = false) :
    openPipeline input activation receiptAvailable lookupAvailable auth receipt lookup = ⟨none, input.bytes, input.floor⟩ := by
  rcases missing with missing | missing | missing | missing <;> simp [openPipeline, missing]

/-- The public constructor cannot lower the floor supplied by monotone storage reads. -/
theorem public_pipeline_floor (input : OpenInput) (activation : ActivationInput)
    (receiptAvailable lookupAvailable : Bool) (auth : Record → Authentication)
    (receipt : Receipt → Option Version → Bool) (lookup : Option Lookup → Option Version → Bool)
    (monotoneRead : input.floor ≤ activation.floor) :
    input.floor ≤ (openPipeline input activation receiptAvailable lookupAvailable auth receipt lookup).floor := by
  unfold openPipeline
  split
  · exact Nat.le_refl _
  · dsimp only
    split
    · exact Nat.le_refl _
    · rename_i opened writer recovered
      unfold finishPipelineOpen
      split
      · exact Nat.le_trans monotoneRead (activation_floor_monotone writer activation)
      · exact monotoneRead

/-- Every returned public pipeline preserves the recovered state and passes retained authority. -/
theorem public_pipeline_authorized {input : OpenInput} {activation : ActivationInput}
    {receiptAvailable lookupAvailable : Bool} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool} {writer : WriterState}
    (accepted : (openPipeline input activation receiptAvailable lookupAvailable auth receipt lookup).writer = some writer) :
    ∃ recovered,
      (openWriter input (fun r => authenticateRecord r (auth r))
        (fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)).writer = some recovered ∧
      pipelineAuthority recovered input.crypto receiptAvailable lookupAvailable auth receipt lookup = true ∧
      writer.state = recovered.state := by
  unfold openPipeline at accepted
  split at accepted
  · contradiction
  · dsimp only at accepted
    split at accepted
    · contradiction
    · rename_i opened recovered present
      split at accepted
      · rename_i finished
        cases accepted
        have authority := pipeline_open_authorized finished
        exact ⟨recovered, present, authority.1, authority.2⟩
      · contradiction

/-- Every public writable pipeline has synced its accepted prefix before another checkpoint can publish. -/
theorem public_pipeline_synced {input : OpenInput} {activation : ActivationInput}
    {receiptAvailable lookupAvailable : Bool} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool} {writer : WriterState}
    (accepted : (openPipeline input activation receiptAvailable lookupAvailable auth receipt lookup).writer = some writer) :
    writer.bytes.data = writer.bytes.durable := by
  unfold openPipeline at accepted
  split at accepted
  · contradiction
  · dsimp only at accepted
    split at accepted
    · contradiction
    · rename_i opened recovered present
      split at accepted
      · rename_i finished
        cases accepted
        have authority := (pipeline_open_authorized finished).1
        simp only [finishPipelineOpen, authority, ↓reduceIte] at finished ⊢
        exact activation_synced (fun ready => openWriter_synced present ready) finished
      · contradiction

/-- A recovered writer with no pending retirement becomes usable after successful retained authority checks. -/
theorem public_pipeline_ready {input : OpenInput} {activation : ActivationInput}
    {auth : Record → Authentication} {receipt : Receipt → Option Version → Bool}
    {lookup : Option Lookup → Option Version → Bool} {writer : WriterState}
    (storage : input.storageAvailable = true) (crypto : input.crypto = true)
    (recovered : (openWriter input (fun r => authenticateRecord r (auth r))
      (fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)).writer = some writer)
    (ready : writer.pending = none)
    (authorized : pipelineAuthority writer input.crypto true true auth receipt lookup = true) :
    openPipeline input activation true true auth receipt lookup = ⟨some writer, writer.bytes, activation.floor⟩ := by
  simp only [openPipeline, storage, crypto, Bool.not_true, Bool.false_or, Bool.false_eq_true, ↓reduceIte]
  rw [crypto] at recovered authorized
  simp only [recovered, finishPipelineOpen, authorized, ↓reduceIte, activateWriter, ready]

end Spacewave.SObject.Journal
