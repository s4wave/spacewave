import Spacewave.SObject.JournalReducer

/-!
# Journal frame storage and checkpoint representation

Mirrors checkpoint hydration, frame scanning, generation windows and memory
storage in `core/sobject/journal-frame.go`. Serialized protobuf identities,
CRC computation, encryption and hashes are primitive boundaries. Frame
observations carry exact raw byte slices and independently computed CRCs;
parsing, bounds, sequence and record admission remain model decisions.
Compact snapshots omit redundant lookup history; hydration reconstructs it.
The failed-Sync rollback is the existing memory storage contract. Filesystem
durability requires the corresponding successful-sync assumption.
-/

namespace Spacewave.SObject.Journal

/-- validCheckpointAttempt mirrors the complete retained-attempt validation contract. -/
def validCheckpointAttempt (a : Attempt) : Bool :=
  a.key.isSome && a.lineage.isSome && a.version.isSome && validLineage a.key a.lineage &&
  (1 ≤ a.state && a.state ≤ 6) && (1 ≤ a.readiness && a.readiness ≤ 4) &&
  a.lookupHistory.length ≤ 1 && (a.lookup.isSome || a.lookupHistory.isEmpty) &&
  (a.lookup.isNone || (validLookup a.lookup a.key a.lineage a.version &&
    (a.lookupHistory.isEmpty || a.lookupHistory.head?.join.map (·.data) == a.lookup.map (·.data)))) &&
  validPayload a.intent && a.intentSequence != 0 &&
  (if a.envelope.isNone then a.envelopeSequence == 0 && a.envelopeDigest == ""
   else validPayload a.envelope && a.envelopeSequence != 0 &&
     a.intentSequence < a.envelopeSequence && a.envelopeDigest.length == digestLength) &&
  (a.receipt.isNone || (a.envelope.isSome &&
    validReceipt a.receipt a.key a.lineage a.version a.envelopeDigest)) &&
  (a.acknowledgement.isNone || (a.receipt.isSome && validAcknowledgement a.acknowledgement a.key &&
    (a.acknowledgement.getD default).receiptDigest == (a.receipt.getD default).terminalDigest)) &&
  (a.projection.isNone || (a.receipt.isSome && validProjection a.projection a.key &&
    (a.projection.getD default).receiptDigest == (a.receipt.getD default).terminalDigest &&
    (a.projection.getD default).rootSeqno == (a.receipt.getD default).rootSeqno &&
    (a.projection.getD default).rootDigest == (a.receipt.getD default).rootDigest)) &&
  (a.receipt.isSome || (a.acknowledgement.isNone && a.projection.isNone)) &&
  (!(a.lookup.isSome && ((a.lookup.getD default).state == 3 || (a.lookup.getD default).state == 4)) ||
    (a.receipt.isSome && a.receipt.map (·.data) == (a.lookup.getD default).receipt.map (·.data))) &&
  (!a.resendAuthorized || (a.state == 3 && a.sendAttempted && a.lookup.isSome &&
    (a.lookup.getD default).state == 1)) &&
  (!a.lineageRecoveryBlocked || a.state == 5) &&
  (if a.state == 1 then
    a.envelope.isNone && a.receipt.isNone && a.acknowledgement.isNone && a.projection.isNone &&
      a.lookup.isNone && !a.sendAttempted && !a.resendAuthorized && !a.lineageRecoveryBlocked
  else if a.state == 2 then
    a.readiness == 1 && a.envelope.isSome && a.receipt.isNone && a.acknowledgement.isNone &&
      a.projection.isNone && a.lookup.isNone && !a.sendAttempted && !a.resendAuthorized && !a.lineageRecoveryBlocked
  else if a.state == 3 then
    a.readiness == 1 && a.envelope.isSome && a.sendAttempted && a.receipt.isNone &&
      a.acknowledgement.isNone && a.projection.isNone &&
      (a.lookup.isNone || (a.lookup.getD default).state == 2 || (a.lookup.getD default).state == 1)
  else if a.state == 4 then
    a.readiness == 1 && a.envelope.isSome && a.sendAttempted && a.receipt.isSome && !a.resendAuthorized &&
      (a.lookup.isNone || ((a.lookup.getD default).state ≥ 1 && (a.lookup.getD default).state ≤ 4))
  else a.receipt.isNone && a.acknowledgement.isNone && a.projection.isNone && !a.resendAuthorized &&
    (!a.sendAttempted || lookupComplete a.lookup)) &&
  (a.checkpointEligible == (a.receipt.isSome && a.projection.isSome))

/-- compactAttempt removes only the redundant history field omitted by the wire snapshot. -/
def compactAttempt (a : Attempt) : Attempt := {a with lookupHistory := []}

/-- restoreAttempt reconstructs the bounded history from the current lookup. -/
def restoreAttempt (a : Attempt) : Attempt := a.withLookup a.lookup

/-- CompactCheckpoint retains the metadata checked before attempt hydration. -/
structure CompactCheckpoint where
  identity : String
  generation : Nat
  nextSequence : Nat
  attempts : List Attempt
  deriving DecidableEq, Repr, Inhabited

/-- buildCheckpoint validates the exact compact evidence before serialization/encryption. -/
def buildCheckpoint (identity : String) (generation nextSequence : Nat)
    (state : Option State) : Option CompactCheckpoint := do
  if identity.length != digestLength || generation == 0 || nextSequence == 0 then none
  else
    let state ← state
    let attempts := state.map compactAttempt
    if !attempts.all validCheckpointAttempt then none
    else some ⟨identity, generation, nextSequence, attempts⟩

/-- hydrateAttempts rejects invalid or duplicate entries, matching fresh-reducer hydration. -/
def hydrateAttempts (state : State) : List Attempt → Option State
  | [] => some state
  | attempt :: attempts =>
    if !validCheckpointAttempt attempt || (findAttempt state (attemptDigest attempt)).isSome then none
    else hydrateAttempts (putAttempt state attempt) attempts

/-- readCheckpoint checks metadata, restores history and validates every retained attempt. -/
def readCheckpoint (checkpoint : CompactCheckpoint) (identity : String)
    (generation nextSequence : Nat) : Option State :=
  if checkpoint.identity != identity || checkpoint.generation != generation ||
      checkpoint.nextSequence != nextSequence then none
  else hydrateAttempts [] (checkpoint.attempts.map restoreAttempt)

/-- Restoring a compacted canonical attempt preserves every observable field. -/
theorem restore_compact {attempt : Attempt} (h : lookupConsistent attempt) :
    restoreAttempt (compactAttempt attempt) = attempt := by
  cases attempt
  simp_all [lookupConsistent, restoreAttempt, compactAttempt, Attempt.withLookup]

/-- Compacting and restoring a reducer snapshot preserves all evidence from full replay. -/
theorem replay_checkpoint_fields {records : List (Option Record)} {state : State}
    (h : reduceJournal records = some state) :
    (state.map compactAttempt).map restoreAttempt = state := by
  rw [List.map_map]
  calc
    state.map (restoreAttempt ∘ compactAttempt) = state.map id :=
      List.map_congr_left (fun attempt member => restore_compact (reduceJournal_lookup h attempt member))
    _ = state := List.map_id state

/-- Restored checkpoint evidence followed by a suffix agrees with full replay. -/
theorem checkpoint_suffix {records suffix : List (Option Record)} {state : State}
    (h : reduceJournal records = some state) :
    replayFrom ((state.map compactAttempt).map restoreAttempt)
        (advanceSequence 1 records.length) suffix = reduceJournal (records ++ suffix) := by
  rw [replay_checkpoint_fields h]
  unfold reduceJournal
  rw [replayFrom_append]
  simp only [reduceJournal] at h
  simp [h]

/-- Hydrating a previously absent digest slot retains every old attempt exactly once. -/
theorem putAttempt_absent_perm {state : State} {attempt : Attempt}
    (absent : findAttempt state (attemptDigest attempt) = none) :
    (putAttempt state attempt).Perm (attempt :: state) := by
  have filtered : state.filter (fun a => attemptDigest a != attemptDigest attempt) = state := by
    apply List.filter_eq_self.mpr
    intro a member
    have unequal := List.find?_eq_none.mp absent a member
    simpa only [bne_iff_ne, beq_iff_eq] using unequal
  unfold putAttempt
  rw [filtered]
  exact List.mergeSort_perm _ _

/-- Successful hydration reorders entries but neither drops nor invents retained evidence. -/
theorem hydrateAttempts_perm {attempts state out : State}
    (h : hydrateAttempts state attempts = some out) : out.Perm (attempts ++ state) := by
  induction attempts generalizing state with
  | nil =>
    simp only [hydrateAttempts, Option.some.injEq] at h
    cases h
    simp
  | cons attempt attempts ih =>
    simp only [hydrateAttempts] at h
    split at h
    · contradiction
    · rename_i allowed
      have absent : findAttempt state (attemptDigest attempt) = none := by
        cases found : findAttempt state (attemptDigest attempt) with
        | none => rfl
        | some a => simp [found] at allowed
      have perm := ih h
      exact (perm.trans ((List.Perm.refl attempts).append (putAttempt_absent_perm absent))).trans
        List.perm_middle

/-- Fresh hydration exports the same sorted order as a live reducer. -/
theorem hydrateAttempts_sorted {attempts state out : State}
    (sorted : stateSorted state) (h : hydrateAttempts state attempts = some out) : stateSorted out := by
  induction attempts generalizing state with
  | nil =>
    simp only [hydrateAttempts, Option.some.injEq] at h
    cases h
    exact sorted
  | cons attempt attempts ih =>
    simp only [hydrateAttempts] at h
    split at h
    · contradiction
    · exact ih (putAttempt_sorted state attempt) h

/-- A successfully written and hydrated checkpoint equals the complete replay snapshot. -/
theorem checkpoint_roundtrip {records : List (Option Record)} {state restored : State}
    {identity : String} {generation nextSequence : Nat} {checkpoint : CompactCheckpoint}
    (replayed : reduceJournal records = some state)
    (built : buildCheckpoint identity generation nextSequence (some state) = some checkpoint)
    (read : readCheckpoint checkpoint identity generation nextSequence = some restored) :
    restored = state := by
  unfold buildCheckpoint at built
  split at built
  · contradiction
  · simp only [Option.bind_eq_bind, Option.bind_some] at built
    split at built
    · contradiction
    · cases built
      simp only [readCheckpoint, bne_self_eq_false, Bool.false_or, Bool.false_eq_true, ↓reduceIte] at read
      rw [replay_checkpoint_fields replayed] at read
      have perm := hydrateAttempts_perm read
      simp only [List.append_nil] at perm
      obtain ⟨sorted, unique⟩ := reduceJournal_indexed replayed
      apply List.Perm.eq_of_pairwise (le := fun a b => attemptDigest a ≤ attemptDigest b)
      · intro a b am bm ab ba
        exact unique a (perm.mem_iff.mp am) b bm (Std.le_antisymm ab ba)
      · exact hydrateAttempts_sorted (by simp [stateSorted]) read
      · exact sorted
      · exact perm

/-- Actual accepted checkpoint hydration followed by a suffix equals full record replay. -/
theorem hydrated_checkpoint_suffix {records suffix : List (Option Record)} {state restored : State}
    {identity : String} {generation nextSequence : Nat} {checkpoint : CompactCheckpoint}
    (replayed : reduceJournal records = some state)
    (built : buildCheckpoint identity generation nextSequence (some state) = some checkpoint)
    (read : readCheckpoint checkpoint identity generation nextSequence = some restored)
    (sequence : nextSequence = advanceSequence 1 records.length) :
    replayFrom restored nextSequence suffix = reduceJournal (records ++ suffix) := by
  rw [sequence, checkpoint_roundtrip replayed built read]
  unfold reduceJournal
  rw [replayFrom_append]
  simp only [reduceJournal] at replayed
  simp [replayed]

/-- The publication window admits the current floor or one forward generation. -/
def generationWindow (floor generation : Nat) : Bool :=
  generation != 0 && !(generation < floor) &&
    !(generation > floor && generation - floor > 1)

/-- advanceFloor mirrors the monotone memory/file generation-floor update. -/
def advanceFloor (floor generation : Nat) : Option Nat :=
  if generation == 0 then none else some (max floor generation)

/-- A published marker never precedes the retained floor. -/
theorem generationWindow_not_rollback {floor generation : Nat}
    (h : generationWindow floor generation = true) : floor ≤ generation := by
  simp [generationWindow] at h
  omega

/-- Admitted publication is exactly the floor or its next mathematical generation. -/
theorem generationWindow_exact {floor generation : Nat}
    (h : generationWindow floor generation = true) :
    generation = floor ∨ generation = floor + 1 := by
  simp [generationWindow] at h
  omega

/-- Updating the durable floor cannot move it backward. -/
theorem advanceFloor_monotone {floor generation updated : Nat}
    (h : advanceFloor floor generation = some updated) : floor ≤ updated := by
  unfold advanceFloor at h
  split at h <;> simp_all <;> omega

/-- Counter exhaustion does not make the current final generation unreadable. -/
theorem final_generation_admitted : generationWindow (2^64 - 1) (2^64 - 1) = true := by
  decide

/-- readBE decodes the raw big-endian bytes observed by the framing parser. -/
def readBE (bytes : List Nat) : Nat := bytes.foldl (fun value byte => value * 256 + byte) 0

/-- beBytes selects the fixed-width big-endian representation used in headers. -/
def beBytes (width value : Nat) : List Nat :=
  (List.range width).map (fun index => value / 256^(width - 1 - index) % 256)

/-- FrameObservation supplies raw bytes and independently computed CRC primitives. -/
structure FrameObservation where
  header : List Nat
  remaining : Nat
  headerCRC : Nat
  frameCRC : Nat
  trailer : List Nat
  record : Option Record
  readOK : Bool := true
  deriving Repr, Inhabited

/-- Header fields are read directly, without Go admission results in the projection. -/
def headerField (header : List Nat) (start count : Nat) : Nat :=
  readBE ((header.drop start).take count)

/-- validHeaderPrefix mirrors the bytewise admissibility of an incomplete header. -/
def validHeaderPrefix (header : List Nat) (expected crc : Nat) : Bool :=
  !header.isEmpty && header.length < 28 &&
  header.take 4 == [83, 87, 74, 49].take header.length &&
  (header.length < 5 || header[4]! == 0) &&
  (header.length < 6 || headerField header 4 2 == 1) &&
  (header.length < 7 || header[6]! == 0) &&
  (header.length < 8 || (1 ≤ headerField header 6 2 && headerField header 6 2 ≤ 11)) &&
  (expected == 0 || (header.drop 8).take 8 == (beBytes 8 expected).take (header.length - 8)) &&
  (header.length < 17 ||
    readBE ((header.drop 16).take 4 ++ List.replicate (4 - min (header.length - 16) 4) 0) ≤ 4194304) &&
  (header.length < 21 || (header.drop 20).take 4 == (beBytes 4 crc).take (header.length - 20))

/-- parseHeader preserves the order and bounds of the complete Go header parser. -/
def parseHeader (frame : FrameObservation) : Option (Int × Nat × Nat) :=
  let kind := headerField frame.header 6 2
  let length := headerField frame.header 16 4
  if frame.header.take 4 != [83, 87, 74, 49] || headerField frame.header 4 2 != 1 ||
      !(1 ≤ kind && kind ≤ 11) || headerField frame.header 20 4 != frame.headerCRC ||
      length > 4194304 then none
  else some (Int.ofNat kind, headerField frame.header 8 8, length)

/-- scanFrame distinguishes a valid record, a truncatable torn tail and corruption.
Error 2 is the distinct first-frame sequence-base mismatch used during activation. -/
def scanFrame (initial expected : Nat) (first : Bool) (frame : FrameObservation) :
    Except Int (Option (Record × Nat)) := do
  if !frame.readOK then throw 1
  if frame.remaining < 28 then
    if validHeaderPrefix frame.header expected frame.headerCRC then return none else throw 1
  let (kind, sequence, length) ← match parseHeader frame with
    | none => .error 1
    | some value => .ok value
  if sequence != expected then
    if first && initial != 1 then throw 2 else throw 1
  if length + 36 > frame.remaining then
    if frame.remaining ≥ 36 && frame.trailer.take 4 == [69, 78, 68, 33] &&
        headerField frame.trailer 4 4 == frame.remaining - 36 then throw 1
    else return none
  if frame.trailer.take 4 != [69, 78, 68, 33] || headerField frame.trailer 4 4 != length ||
      headerField frame.header 24 4 != frame.frameCRC then throw 1
  let record ← match frame.record with
    | none => .error 1
    | some value => .ok value
  if record.format != 1 || record.sequence != sequence || record.kind != kind ||
      !validRecord (some record) then throw 1
  return some (record, length + 36)

/-- ScanResult retains the accepted records and exact byte offset to truncate to. -/
structure ScanResult where
  records : List Record
  offset : Nat
  deriving Repr, Inhabited

/-- scanFramesFrom mirrors scanJournalFrom's loop over primitive frame observations. -/
def scanFramesFrom (initial expected : Nat) (first : Bool) :
    List FrameObservation → Except Int ScanResult
  | [] => .ok ⟨[], 0⟩
  | frame :: frames =>
    match scanFrame initial expected first frame with
    | .error err => .error err
    | .ok none => .ok ⟨[], 0⟩
    | .ok (some (record, length)) =>
      (scanFramesFrom initial ((expected + 1) % (2^64)) false frames).map
        (fun tail => ⟨record :: tail.records, length + tail.offset⟩)

/-- scanFrames starts with the Go scanner's distinct first-record error behavior. -/
def scanFrames (initial : Nat) (frames : List FrameObservation) : Except Int ScanResult :=
  scanFramesFrom initial initial true frames

/-- A recognized torn tail contributes neither a record nor bytes to the replay prefix. -/
theorem scan_torn_tail {initial expected : Nat} {first : Bool} {frame : FrameObservation}
    (h : scanFrame initial expected first frame = .ok none) :
    scanFramesFrom initial expected first [frame] = .ok ⟨[], 0⟩ := by
  simp [scanFramesFrom, h]

/-- CompletePrefix describes the checked frame/sequence boundary of durable records. -/
inductive CompletePrefix (initial : Nat) : Nat → Bool → List FrameObservation → List Record → Nat → Nat → Prop
  | nil (expected first) : CompletePrefix initial expected first [] [] 0 expected
  | cons {expected first frame frames record records length bytes next}
      (head : scanFrame initial expected first frame = .ok (some (record, length)))
      (tail : CompletePrefix initial ((expected + 1) % (2^64)) false frames records bytes next) :
      CompletePrefix initial expected first (frame :: frames) (record :: records) (length + bytes) next

/-- Replay of complete frames followed by any tail composes at its exact sequence and byte boundary. -/
theorem scan_prefix {initial expected : Nat} {first : Bool} {frames : List FrameObservation}
    {records : List Record} {bytes next : Nat}
    (h : CompletePrefix initial expected first frames records bytes next)
    (suffix : List FrameObservation) :
    scanFramesFrom initial expected first (frames ++ suffix) =
      (scanFramesFrom initial next (if frames.isEmpty then first else false) suffix).map
        (fun tail => ⟨records ++ tail.records, bytes + tail.offset⟩) := by
  induction h with
  | nil expected first =>
    simp only [List.nil_append, List.isEmpty_nil, ↓reduceIte, List.nil_append, Nat.zero_add]
    cases scanFramesFrom initial expected first suffix <;> rfl
  | @cons expected first frame frames record records length bytes next head tail ih =>
    simp only [List.cons_append, scanFramesFrom, head, ih, List.isEmpty_cons, Bool.false_eq_true,
      ↓reduceIte, ite_self]
    cases scanFramesFrom initial next false suffix <;> simp [Except.map, Nat.add_assoc]

/-- The framing counter establishes the next-sequence hypothesis used by checkpoint replay. -/
theorem completePrefix_sequence {initial expected : Nat} {first : Bool} {frames : List FrameObservation}
    {records : List Record} {bytes next : Nat}
    (h : CompletePrefix initial expected first frames records bytes next) :
    next = advanceSequence expected records.length := by
  induction h with
  | nil => rfl
  | cons head tail ih => simpa [advanceSequence, seqnoLimit] using ih

/-- Appending a recognized torn final frame preserves every complete durable record and its offset. -/
theorem complete_prefix_torn_tail {initial expected : Nat} {first : Bool}
    {frames : List FrameObservation} {records : List Record} {bytes next : Nat}
    (h : CompletePrefix initial expected first frames records bytes next)
    {torn : FrameObservation}
    (tail : scanFrame initial next (if frames.isEmpty then first else false) torn = .ok none) :
    scanFramesFrom initial expected first (frames ++ [torn]) = .ok ⟨records, bytes⟩ := by
  rw [scan_prefix h, scan_torn_tail tail]
  simp [Except.map]

/-- MemoryBytes is the existing failure-injecting storage's visible and synced byte state. -/
structure MemoryBytes where
  data : List Nat
  durable : List Nat
  deriving DecidableEq, Repr, Inhabited

/-- writeBytes mirrors WriteAt, including a partial prefix before an injected error. -/
def writeBytes (storage : MemoryBytes) (offset : Nat) (bytes : List Nat) : MemoryBytes :=
  let before := storage.data.take offset ++ List.replicate (offset - storage.data.length) 0
  {storage with data := before ++ bytes ++ storage.data.drop (offset + bytes.length)}

/-- memoryWrite mirrors the injected WriteAt failure, including failures before any copy. -/
def memoryWrite (storage : MemoryBytes) (offset : Nat) (bytes : List Nat)
    (fail : Bool) (limit : Int) : MemoryBytes :=
  if fail then
    if 0 ≤ limit && limit < Int.ofNat bytes.length then
      writeBytes storage offset (bytes.take limit.toNat)
    else storage
  else writeBytes storage offset bytes

/-- Every WriteAt outcome preserves the previously synced bytes. -/
theorem memoryWrite_preserves_durable (storage : MemoryBytes) (offset : Nat) (bytes : List Nat)
    (fail : Bool) (limit : Int) :
    (memoryWrite storage offset bytes fail limit).durable = storage.durable := by
  unfold memoryWrite
  split
  · split <;> rfl
  · rfl

/-- syncBytes preserves the test storage's failed-Sync rollback contract. -/
def syncBytes (storage : MemoryBytes) (success : Bool) : MemoryBytes :=
  if success then {storage with durable := storage.data}
  else {storage with data := storage.durable}

/-- A crash discards only unsynced visible bytes under the declared storage contract. -/
def crashBytes (storage : MemoryBytes) : List Nat := storage.durable

/-- A write, including a torn write, never changes already-synced storage. -/
theorem write_preserves_durable (storage : MemoryBytes) (offset : Nat) (bytes : List Nat) :
    crashBytes (writeBytes storage offset bytes) = crashBytes storage := by rfl

/-- A failed Sync leaves the acknowledged durable prefix unchanged. -/
theorem failed_sync_preserves_durable (storage : MemoryBytes) :
    crashBytes (syncBytes storage false) = crashBytes storage := by rfl

/-- Successful Sync makes the exact visible bytes recoverable. -/
theorem successful_sync_recovers (storage : MemoryBytes) :
    crashBytes (syncBytes storage true) = storage.data := by rfl

/-- PublicationState retains the observable writer and storage state during checkpoint publication.
Checkpoint contents are handled by buildCheckpoint/readCheckpoint; this view tracks their durable slots. -/
structure PublicationState where
  bytes : MemoryBytes
  floor : Nat
  markerGeneration : Nat
  checkpointGenerations : List Nat
  generation : Nat
  sequence : Nat
  offset : Nat
  records : List Record
  state : State
  poisoned : Bool
  deriving Repr, Inhabited

/-- PublicationResult includes storage effects even when publication returns an error. -/
structure PublicationResult where
  ok : Bool
  result : PublicationState
  deriving Repr, Inhabited

/-- publishCheckpoint mirrors the ordered effects after candidate and marker construction.
Fault codes name injected API outcomes: 1/10 candidate before/after write; 2/3 marker
before/after write; 4/5 floor before/after write; 6/7 retired read/mismatch; 8/11
truncate before/after change; 9 failed Sync. Other codes complete successfully.
The candidate generation and compact content must first pass the preparation contract. -/
def publishCheckpoint (before : PublicationState) (generation : Nat) (fault : Int) : PublicationResult := Id.run do
  let mut result := before
  if fault == 1 then return ⟨false, result⟩
  result := {result with checkpointGenerations :=
    (generation :: result.checkpointGenerations.filter (· != generation)).mergeSort (· ≤ ·)}
  if fault == 10 then return ⟨false, result⟩
  result := {result with poisoned := true}
  if fault == 2 then return ⟨false, result⟩
  result := {result with markerGeneration := generation}
  if fault == 3 || fault == 4 then return ⟨false, result⟩
  if generation == 0 then return ⟨false, result⟩
  result := {result with floor := max result.floor generation}
  if fault == 5 || fault == 6 || fault == 7 || fault == 8 then return ⟨false, result⟩
  result := {result with bytes := {result.bytes with data := []}}
  if fault == 11 then return ⟨false, result⟩
  if fault == 9 then
    result := {result with bytes := syncBytes result.bytes false}
    return ⟨false, result⟩
  result := {result with bytes := syncBytes result.bytes true, offset := 0, generation := generation, records := [], poisoned := false}
  return ⟨true, result⟩

/-- Publication, successful or interrupted, never lowers the durable generation floor. -/
theorem publication_floor_monotone (before : PublicationState) (generation : Nat) (fault : Int) :
    before.floor ≤ (publishCheckpoint before generation fault).result.floor := by
  unfold publishCheckpoint
  simp only [Id.run, pure]
  repeat' first | split | (simp_all <;> omega)

/-- Every error once marker publication begins fences the old writer. -/
theorem publication_failure_fenced (before : PublicationState) (generation : Nat) (fault : Int)
    (candidate : fault ≠ 1 ∧ fault ≠ 10)
    (failed : (publishCheckpoint before generation fault).ok = false) :
    (publishCheckpoint before generation fault).result.poisoned = true := by
  unfold publishCheckpoint at *
  simp only [Id.run, pure] at *
  repeat' first | split at * | simp_all

/-- Complete publication commits the replacement before retiring the old bytes. -/
theorem publication_success (before : PublicationState) (generation : Nat) (fault : Int)
    (h : (publishCheckpoint before generation fault).ok = true) :
    (publishCheckpoint before generation fault).result.markerGeneration = generation ∧
    (publishCheckpoint before generation fault).result.floor = max before.floor generation ∧
    (publishCheckpoint before generation fault).result.bytes.durable = [] ∧
    (publishCheckpoint before generation fault).result.generation = generation ∧
    (publishCheckpoint before generation fault).result.offset = 0 ∧
    (publishCheckpoint before generation fault).result.poisoned = false := by
  unfold publishCheckpoint at *
  simp only [Id.run, pure] at *
  repeat' first | split at * | simp_all [syncBytes]

/-- Every outcome after a successful candidate write retains its generation slot. -/
theorem publication_candidate_retained (before : PublicationState) (generation : Nat) (fault : Int)
    (written : fault ≠ 1) :
    generation ∈ (publishCheckpoint before generation fault).result.checkpointGenerations := by
  unfold publishCheckpoint
  simp only [Id.run, pure]
  repeat' first | split | simp_all [List.mem_mergeSort]

/-- A checkpoint never changes the live reducer result or the next writer-owned sequence. -/
theorem publication_preserves_state (before : PublicationState) (generation : Nat) (fault : Int) :
    (publishCheckpoint before generation fault).result.state = before.state ∧
    (publishCheckpoint before generation fault).result.sequence = before.sequence := by
  unfold publishCheckpoint
  simp only [Id.run, pure]
  repeat' first | split | simp_all

end Spacewave.SObject.Journal
