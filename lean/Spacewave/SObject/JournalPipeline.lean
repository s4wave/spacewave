import Spacewave.SObject.JournalFrame

/-!
# Mutation journal pipeline authentication

Mirrors public opening and retained record authentication in `core/sobject/journal-pipeline.go`.
Authenticated decryption, protobuf decoding and SHA-256 are primitive boundaries.
The projection supplies their outputs for the prepared writer-owned sequence;
key, lineage, version and envelope bindings remain model decisions.
Go behavior checked at revision `cf8d8f69a`.
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

/-- With recovery, authority and activation established, the public constructor returns the activated writer. -/
theorem public_pipeline_completes {input : OpenInput} {activation : ActivationInput} {writer : WriterState}
    {auth : Record → Authentication} {receipt : Receipt → Option Version → Bool}
    {lookup : Option Lookup → Option Version → Bool}
    (storage : input.storageAvailable = true) (crypto : input.crypto = true)
    (recovered : (openWriter input (fun r => authenticateRecord r (auth r))
      (fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)).writer = some writer)
    (authority : pipelineAuthority writer input.crypto true true auth receipt lookup = true)
    (active : (activateWriter writer activation).ok = true) :
    (openPipeline input activation true true auth receipt lookup).writer = some (activateWriter writer activation).result := by
  simp only [openPipeline, storage, crypto, Bool.not_true, Bool.false_or, Bool.false_eq_true, ↓reduceIte]
  rw [crypto] at recovered authority
  simp [recovered, finishPipelineOpen, authority, active]

/-- Retained checkpoint authority follows from its primitive authentication and receipt/lookup results. -/
theorem checkpoint_state_authority {writer : WriterState} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool}
    (records : writer.records = [])
    (authenticated : authenticateSnapshots true writer.identity (writer.state.map some) auth = true)
    (verified : writer.state.all (fun a => a.receipt.all (fun value => receipt value a.version) &&
      (a.lookup.isNone || lookup a.lookup a.version)) = true) :
    pipelineAuthority writer true true true auth receipt lookup = true := by
  simp only [pipelineAuthority, records, List.all_nil, Bool.true_and, Bool.and_true, Bool.and_eq_true]
  exact ⟨authenticated, verified⟩

/-- A completed checkpoint publicly reopens under healthy primitive reads and retained authority. -/
theorem public_checkpoint_empty {input : OpenInput} {activation : ActivationInput}
    {auth : Record → Authentication} {receipt : Receipt → Option Version → Bool}
    {lookup : Option Lookup → Option Version → Bool}
    {marker : GenerationMarker} {observation : MarkerObservation} {crc : Nat}
    {checkpoint : CompactCheckpoint} {state : State}
    (readable : OpenReadable input) (crypto : input.crypto = true)
    (identity : input.identity = marker.identity) (present : input.marker = some observation)
    (encoded : encodeMarker marker crc = some observation) (floor : input.floor = marker.generation)
    (candidate : input.checkpoint = some checkpoint)
    (hydrated : readCheckpoint checkpoint input.identity marker.generation marker.nextSequence = some state)
    (authenticated : authenticateSnapshots true input.identity (state.map some) auth = true)
    (verified : state.all (fun a => a.receipt.all (fun value => receipt value a.version) &&
      (a.lookup.isNone || lookup a.lookup a.version)) = true)
    (empty : input.bytes.data = []) (tailSize : input.tailSizeOK = true) (sync : input.fault ≠ 3) :
    (openPipeline input activation true true auth receipt lookup).writer.map (·.state) = some state := by
  have opened := open_checkpoint_empty (authenticate := fun r => authenticateRecord r (auth r))
    (authenticateState := fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)
    readable crypto identity present encoded floor candidate hydrated (by simpa [crypto] using authenticated) empty tailSize sync
  have authority := checkpoint_state_authority
    (writer := ⟨syncBytes input.bytes true, marker.nextSequence % seqnoLimit, 0, [], state, false, none, input.identity, marker.generation⟩)
    rfl authenticated verified
  have ready := public_pipeline_ready readable.1 crypto opened rfl
    (by simpa only [crypto] using authority) (activation := activation)
  simp only [ready, Option.map_some]

/-- A recovered checkpoint awaiting retirement becomes public after its captured reads and retained authority succeed. -/
theorem public_pending_checkpoint {input : OpenInput} {activation : ActivationInput}
    {writer : WriterState} {pending : PendingActivation} {observation : MarkerObservation} {crc : Nat}
    {auth : Record → Authentication} {receipt : Receipt → Option Version → Bool}
    {lookup : Option Lookup → Option Version → Bool}
    (storage : input.storageAvailable = true) (crypto : input.crypto = true)
    (recovered : (openWriter input (fun r => authenticateRecord r (auth r))
      (fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)).writer = some writer)
    (records : writer.records = []) (waiting : writer.pending = some pending) (usable : writer.poisoned = false)
    (authenticated : authenticateSnapshots true writer.identity (writer.state.map some) auth = true)
    (verified : writer.state.all (fun a => a.receipt.all (fun value => receipt value a.version) &&
      (a.lookup.isNone || lookup a.lookup a.version)) = true)
    (generationSupported : activation.generationSupported = true) (floorSupported : activation.floorSupported = true)
    (floorRead : activation.floorReadOK = true) (floor : activation.floor = pending.floor)
    (identity : writer.identity = pending.marker.identity) (present : activation.marker = some observation)
    (encoded : encodeMarker pending.marker crc = some observation)
    (window : activation.floor = pending.marker.generation ∨ activation.floor + 1 = pending.marker.generation)
    (bounded : pending.marker.generation < seqnoLimit) (retiredRead : activation.retiredReadOK = true)
    (retiredLength : writer.bytes.data.length = pending.marker.retiredLength)
    (retiredDigest : activation.retiredDigest = pending.marker.retiredDigest) (fault : activation.fault = 0) :
    (openPipeline input activation true true auth receipt lookup).writer.map (·.state) = some writer.state := by
  have authority := checkpoint_state_authority records authenticated verified
  have active := activation_completes waiting usable generationSupported floorSupported floorRead floor identity present
    encoded window bounded retiredRead retiredLength retiredDigest fault
  have opened := public_pipeline_completes storage crypto recovered (by simpa only [crypto] using authority) active
  simp only [opened, Option.map_some, activation_preserves_state]

/-- A retained replacement checkpoint publicly recovers after every permitted retired-segment observation. -/
theorem prepared_checkpoint_public_recovery {before : PublicationState} {preparation : CheckpointInput}
    {prepared : PreparedCheckpoint} {history : List (Option Record)} {initial : Nat}
    {primitives : List FramePrimitives} {records : List Record} {input : OpenInput}
    {observation : MarkerObservation} {crc : Nat} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool}
    (historyReplay : reduceJournal history = some before.state) (available : preparation.reducerAvailable = true)
    (ready : prepareCheckpoint before preparation = some prepared)
    (emitted : EmittedPrefix initial before.bytes.data primitives records before.sequence)
    (readable : OpenReadable input) (crypto : input.crypto = true) (identity : input.identity = preparation.identity)
    (present : input.marker = some observation) (encoded : encodeMarker prepared.marker crc = some observation)
    (candidate : input.checkpoint = some prepared.checkpoint)
    (window : input.floor = prepared.marker.generation ∨ input.floor + 1 = prepared.marker.generation)
    (bounded : prepared.marker.generation < seqnoLimit)
    (bytes : input.bytes.data = before.bytes.data ∨ (input.bytes.data = [] ∧ input.floor = prepared.marker.generation))
    (decoded : input.bytes.data = before.bytes.data → input.frames = primitives)
    (digest : input.bytes.data = before.bytes.data → input.retiredDigest = preparation.retiredDigest)
    (retiredRead : input.retiredReadOK = true) (tailSize : input.tailSizeOK = true) (fault : input.fault = 0)
    (authenticated : authenticateSnapshots true input.identity (before.state.map some) auth = true)
    (verified : before.state.all (fun a => a.receipt.all (fun value => receipt value a.version) &&
      (a.lookup.isNone || lookup a.lookup a.version)) = true) :
    (openPipeline input ⟨true, true, input.floor, true, some observation, true, input.retiredDigest, 0⟩
      true true auth receipt lookup).writer.map (·.state) = some before.state := by
  have binding := prepared_checkpoint_binding ready
  have actualIdentity : input.identity = prepared.marker.identity := identity.trans binding.2.2.1.symm
  have hydrated : readCheckpoint prepared.checkpoint input.identity prepared.marker.generation
      prepared.marker.nextSequence = some before.state := by
    simpa only [actualIdentity] using prepared_checkpoint_reopens historyReplay available ready
  by_cases completed : input.bytes.data = [] ∧ input.floor = prepared.marker.generation
  · exact public_checkpoint_empty readable crypto actualIdentity present encoded completed.2 candidate hydrated
      authenticated verified completed.1 tailSize (by simp [fault])
  · have observed : input.bytes.data = before.bytes.data := bytes.resolve_right completed
    have length : input.bytes.data.length = prepared.marker.retiredLength := by
      simp only [observed, binding.2.2.2.2.1]
    have retiredDigest : input.retiredDigest = prepared.marker.retiredDigest :=
      (digest observed).trans binding.2.2.2.2.2.1.symm
    have completePending
        (opened : (openWriter input (fun r => authenticateRecord r (auth r))
          (fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)).writer =
          some ⟨input.bytes, prepared.marker.nextSequence % seqnoLimit, 0, [], before.state, false,
            some ⟨prepared.marker, input.floor⟩, input.identity, prepared.marker.generation⟩) :
        (openPipeline input ⟨true, true, input.floor, true, some observation, true, input.retiredDigest, 0⟩
          true true auth receipt lookup).writer.map (·.state) = some before.state := by
      exact public_pending_checkpoint readable.1 crypto opened rfl rfl rfl authenticated verified
        rfl rfl rfl rfl actualIdentity rfl encoded window bounded rfl length retiredDigest rfl
    apply completePending
    by_cases noRecords : records = []
    · have emptyBefore : before.bytes.data = [] := emitted_prefix_empty (by simpa only [noRecords] using emitted)
      have empty : input.bytes.data = [] := observed.trans emptyBefore
      have forward : input.floor + 1 = prepared.marker.generation := by
        rcases window with same | forward
        · exact False.elim (completed ⟨empty, same⟩)
        · exact forward
      exact open_checkpoint_pending_empty readable crypto actualIdentity present encoded forward bounded candidate hydrated
        (by simpa only [crypto] using authenticated) empty retiredRead (by simpa [empty] using length.symm) retiredDigest
    · have usable := (built_checkpoint_metadata binding.2.1).2.2
      have retired := emitted_prefix_retired_at_next emitted noRecords usable
      exact open_checkpoint_retired readable crypto actualIdentity present encoded window candidate hydrated
        (by simpa only [crypto] using authenticated)
        (by simpa only [observed, decoded observed, binding.2.2.2.1] using retired) retiredRead length retiredDigest

/-- CheckpointRecoveryResult preserves both the publication outcome and subsequent public recovery. -/
structure CheckpointRecoveryResult where
  checkpoint : CheckpointResult
  pipeline : PipelineOpenResult
  deriving Repr, Inhabited

/-- checkpointRecoveryInput derives the physical recovery observation from modeled publication and an optional crash. -/
def checkpointRecoveryInput (checkpoint : CheckpointResult) (preparation : CheckpointInput)
    (input : OpenInput) (crash : Bool) (markerCRC : Nat) : OpenInput :=
  let bytes := if crash then syncBytes checkpoint.result.bytes false else checkpoint.result.bytes
  let marker := match checkpoint.prepared with
    | none => preparation.marker
    | some prepared =>
      if checkpoint.result.markerGeneration == prepared.marker.generation then encodeMarker prepared.marker markerCRC
      else preparation.marker
  {input with bytes := bytes, marker := marker, floor := checkpoint.result.floor}

/-- checkpointAndOpen follows publication with an optional crash and public recovery.
Bytes, marker selection and floor come from modeled publication; decoding, hashes,
CRC, authentication and read outcomes remain primitive observations. -/
def checkpointAndOpen (before : PublicationState) (preparation : CheckpointInput)
    (input : OpenInput) (activation : ActivationInput) (crash : Bool) (markerCRC : Nat)
    (receiptAvailable lookupAvailable : Bool) (auth : Record → Authentication)
    (receipt : Receipt → Option Version → Bool) (lookup : Option Lookup → Option Version → Bool) : CheckpointRecoveryResult :=
  let checkpoint := checkpointWriter before preparation
  let opened := checkpointRecoveryInput checkpoint preparation input crash markerCRC
  let activate := {activation with marker := opened.marker, floor := opened.floor}
  ⟨checkpoint, openPipeline opened activate receiptAvailable lookupAvailable auth receipt lookup⟩

/-- healthyActivation supplies successful storage reads of the unchanged recovery observation. -/
def healthyActivation (input : OpenInput) : ActivationInput :=
  ⟨true, true, input.floor, true, input.marker, true, input.retiredDigest, 0⟩

/-- The empty initialized journal establishes the public recoverability base case. -/
theorem empty_journal_public_recovery {input : OpenInput} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool}
    (readable : OpenReadable input) (crypto : input.crypto = true) (marker : input.marker = none) (floor : input.floor = 0)
    (empty : input.bytes.data = []) (tailSize : input.tailSizeOK = true) (fault : input.fault = 0) :
    (openPipeline input (healthyActivation input) true true auth receipt lookup).writer.map (·.state) = some [] := by
  let writer : WriterState := ⟨syncBytes input.bytes true, 1 % seqnoLimit, 0, [], [], false, none, input.identity, 0⟩
  have scanned : scanBytes 1 input.bytes.data input.frames = .ok ⟨[], 0⟩ := by
    cases input.frames <;> simp [scanBytes, scanBytesFrom, empty]
  have prepared := prepareOpen_plain (records := []) (state := [])
    (authenticate := fun r => authenticateRecord r (auth r))
    (authenticateState := fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)
    readable marker floor scanned rfl rfl
  have opened : (openWriter input (fun r => authenticateRecord r (auth r))
      (fun state => authenticateSnapshots input.crypto input.identity (state.map some) auth)).writer = some writer := by
    simp [openWriter, prepared, finishOpen, empty, tailSize, fault, writer]
  have authority := checkpoint_state_authority (writer := writer) (auth := auth) (receipt := receipt) (lookup := lookup)
    rfl (by simp [writer, authenticateSnapshots, readable.2.2.2.1]) rfl
  have ready := public_pipeline_ready readable.1 crypto opened rfl
    (by simpa only [crypto] using authority) (activation := healthyActivation input)
  simp only [ready, Option.map_some]
  rfl

/-- Every checkpoint publication cut publicly recovers the acknowledged state under stable primitive observations.
Old storage uses its prior recoverability invariant; replacement storage derives admission
from the built snapshot and emitted retired bytes. No replacement opener or activation result is assumed. -/
theorem checkpoint_public_recovery {before : PublicationState} {preparation : CheckpointInput}
    {input oldInput : OpenInput} {history : List (Option Record)} {initial : Nat}
    {primitives : List FramePrimitives} {records : List Record} {crash : Bool} {markerCRC : Nat}
    {auth : Record → Authentication} {receipt : Receipt → Option Version → Bool}
    {lookup : Option Lookup → Option Version → Bool}
    (synced : before.bytes.data = before.bytes.durable) (held : before.generation ≤ before.floor)
    (markerHeld : before.markerGeneration ≤ before.floor) (bounded : before.floor < seqnoLimit)
    (retained : ∀ observation ∈ preparation.marker, observation.marker.generation ≤ before.floor)
    (historyReplay : reduceJournal history = some before.state) (available : preparation.reducerAvailable = true)
    (emitted : EmittedPrefix initial before.bytes.data primitives records before.sequence)
    (readable : OpenReadable input) (crypto : input.crypto = true) (identity : input.identity = preparation.identity)
    (retiredRead : input.retiredReadOK = true) (tailSize : input.tailSizeOK = true) (fault : input.fault = 0)
    (oldRecovery : (openPipeline oldInput (healthyActivation oldInput) true true auth receipt lookup).writer.map (·.state) = some before.state)
    (oldReads : (checkpointWriter before preparation).result.markerGeneration = before.markerGeneration →
      {input with bytes := before.bytes, marker := preparation.marker, floor := before.floor} = oldInput)
    (newCheckpoint : ∀ prepared, prepareCheckpoint before preparation = some prepared →
      (checkpointWriter before preparation).result.markerGeneration = prepared.marker.generation →
      input.checkpoint = some prepared.checkpoint)
    (decoded : (checkpointRecoveryInput (checkpointWriter before preparation) preparation input crash markerCRC).bytes.data =
      before.bytes.data → input.frames = primitives)
    (digest : (checkpointRecoveryInput (checkpointWriter before preparation) preparation input crash markerCRC).bytes.data =
      before.bytes.data → input.retiredDigest = preparation.retiredDigest)
    (authenticated : authenticateSnapshots true input.identity (before.state.map some) auth = true)
    (verified : before.state.all (fun a => a.receipt.all (fun value => receipt value a.version) &&
      (a.lookup.isNone || lookup a.lookup a.version)) = true) :
    (checkpointAndOpen before preparation input (healthyActivation input) crash markerCRC
      true true auth receipt lookup).pipeline.writer.map (·.state) = some before.state := by
  let observed := checkpointRecoveryInput (checkpointWriter before preparation) preparation input crash markerCRC
  change (openPipeline observed (healthyActivation observed) true true auth receipt lookup).writer.map (·.state) = some before.state
  have oldReopens
      (marker : (checkpointWriter before preparation).result.markerGeneration = before.markerGeneration)
      (floor : (checkpointWriter before preparation).result.floor = before.floor)
      (bytes : observed.bytes = before.bytes) (selected : observed.marker = preparation.marker) :
      (openPipeline observed (healthyActivation observed) true true auth receipt lookup).writer.map (·.state) = some before.state := by
    have agreement : observed = oldInput := by
      rw [← oldReads marker]
      change {input with bytes := observed.bytes, marker := observed.marker, floor := observed.floor} = _
      rw [bytes, selected, show observed.floor = before.floor from floor]
    rw [agreement]
    exact oldRecovery
  cases ready : prepareCheckpoint before preparation with
  | none =>
    have unchanged : checkpointWriter before preparation = ⟨false, before, none⟩ := by simp [checkpointWriter, ready]
    have stable : syncBytes before.bytes false = before.bytes := by cases bytes : before.bytes <;> simp_all [syncBytes]
    apply oldReopens (by simp [unchanged]) (by simp [unchanged])
    · simp [observed, checkpointRecoveryInput, unchanged, stable]
    · simp [observed, checkpointRecoveryInput, unchanged]
  | some prepared =>
    have next := prepared_checkpoint_successor held bounded retained ready
    have advanced := prepared_checkpoint_advances (Nat.lt_of_le_of_lt held bounded) bounded
      (fun observation member => Nat.lt_of_le_of_lt (retained observation member) bounded) ready
    have publication : (checkpointWriter before preparation).result =
        (publishCheckpoint before prepared.marker.generation preparation.fault (validOutgoingMarker prepared.marker)).result := by
      simp [checkpointWriter, ready]
    have candidate : (checkpointWriter before preparation).prepared = some prepared := by simp [checkpointWriter, ready]
    have different : prepared.marker.generation ≠ before.markerGeneration := by omega
    have shape := publication_crash_recovery_shape before prepared.marker.generation preparation.fault
      (validOutgoingMarker prepared.marker) crash synced next
    dsimp only at shape
    rw [← publication] at shape
    rcases shape with old | replacement
    · apply oldReopens old.1 old.2.1 old.2.2
      simp [observed, checkpointRecoveryInput, candidate, old.1, Ne.symm different]
    · have published : (publishCheckpoint before prepared.marker.generation preparation.fault
          (validOutgoingMarker prepared.marker)).result.markerGeneration = prepared.marker.generation := by
        rw [← publication]
        exact replacement.1
      have valid := publication_new_marker_valid before prepared.marker.generation preparation.fault
        (validOutgoingMarker prepared.marker) different published
      obtain ⟨observation, encoded⟩ : ∃ observation, encodeMarker prepared.marker markerCRC = some observation := by
        simp [encodeMarker, valid]
      have present : observed.marker = some observation := by
        simp [observed, checkpointRecoveryInput, candidate, replacement.1, encoded]
      change (openPipeline observed ⟨true, true, observed.floor, true, observed.marker, true, observed.retiredDigest, 0⟩
        true true auth receipt lookup).writer.map (·.state) = some before.state
      rw [present]
      exact prepared_checkpoint_public_recovery historyReplay available ready emitted readable crypto identity present encoded
        (newCheckpoint prepared ready replacement.1) replacement.2.2.1 advanced.2.2 replacement.2.2.2
        decoded digest retiredRead tailSize fault authenticated verified

/-- A trace that returns a writable pipeline has reestablished durable bytes after every modeled publication cut. -/
theorem checkpoint_recovery_synced {before : PublicationState} {preparation : CheckpointInput}
    {input : OpenInput} {activation : ActivationInput} {crash : Bool} {markerCRC : Nat}
    {receiptAvailable lookupAvailable : Bool} {auth : Record → Authentication}
    {receipt : Receipt → Option Version → Bool} {lookup : Option Lookup → Option Version → Bool} {writer : WriterState}
    (accepted : (checkpointAndOpen before preparation input activation crash markerCRC
      receiptAvailable lookupAvailable auth receipt lookup).pipeline.writer = some writer) :
    writer.bytes.data = writer.bytes.durable := by
  exact public_pipeline_synced accepted

/-- Publication, an intervening crash and public recovery cannot lower the captured durable floor. -/
theorem checkpoint_recovery_floor (before : PublicationState) (preparation : CheckpointInput)
    (input : OpenInput) (activation : ActivationInput) (crash : Bool) (markerCRC : Nat)
    (receiptAvailable lookupAvailable : Bool) (auth : Record → Authentication)
    (receipt : Receipt → Option Version → Bool) (lookup : Option Lookup → Option Version → Bool) :
    before.floor ≤ (checkpointAndOpen before preparation input activation crash markerCRC
      receiptAvailable lookupAvailable auth receipt lookup).pipeline.floor := by
  let checkpoint := checkpointWriter before preparation
  let bytes := if crash then syncBytes checkpoint.result.bytes false else checkpoint.result.bytes
  let marker := match checkpoint.prepared with
    | none => preparation.marker
    | some prepared =>
      if checkpoint.result.markerGeneration == prepared.marker.generation then encodeMarker prepared.marker markerCRC
      else preparation.marker
  have recovery := public_pipeline_floor
    {input with bytes := bytes, marker := marker, floor := checkpoint.result.floor}
    {activation with marker := marker, floor := checkpoint.result.floor}
    receiptAvailable lookupAvailable auth receipt lookup (Nat.le_refl _)
  exact Nat.le_trans (checkpoint_floor_monotone before preparation) recovery

end Spacewave.SObject.Journal
