import Spacewave.SObject.JournalReducer

/-!
# Journal frame storage and checkpoint representation

Mirrors checkpoint hydration, frame scanning, durable append, generation windows
and memory storage in `core/sobject/journal-frame.go`. Serialized protobuf identities,
CRC computation, encryption and hashes are primitive boundaries. Frame
observations carry exact raw byte slices and independently computed CRCs;
parsing, canonical frame-version-2 escaping, bounds, sequence and record admission
remain model decisions. Encoded payloads exclude the trailer marker byte.
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

/-- escapePayload removes zero/marker ambiguity while preserving every protobuf byte. -/
def escapePayload : List Nat → List Nat
  | [] => []
  | byte :: bytes =>
    if byte == 0 then 0 :: 0 :: escapePayload bytes
    else if byte == 69 then 0 :: 1 :: escapePayload bytes
    else byte :: escapePayload bytes

/-- unescapePayload decodes only canonical frame-version-2 payloads. -/
def unescapePayload : List Nat → Option (List Nat)
  | [] => some []
  | byte :: bytes =>
    if byte == 69 then none
    else if byte == 0 then
      match bytes with
      | [] => none
      | escaped :: rest =>
        if escaped == 0 then (unescapePayload rest).map (0 :: ·)
        else if escaped == 1 then (unescapePayload rest).map (69 :: ·)
        else none
    else (unescapePayload bytes).map (byte :: ·)

/-- Encoding and decoding preserve every input byte, including all opaque protobuf content. -/
theorem payload_roundtrip (bytes : List Nat) : unescapePayload (escapePayload bytes) = some bytes := by
  induction bytes with
  | nil => rfl
  | cons byte bytes ih =>
    by_cases zero : byte = 0
    · simp [escapePayload, unescapePayload, zero, ih]
    · by_cases marker : byte = 69
      · simp [escapePayload, unescapePayload, marker, ih]
      · simp only [escapePayload, beq_iff_eq, zero, marker, ↓reduceIte]
        unfold unescapePayload
        simp_all

/-- No encoded payload byte can begin a committed frame trailer. -/
theorem payload_excludes_marker (bytes : List Nat) : 69 ∉ escapePayload bytes := by
  induction bytes with
  | nil => simp [escapePayload]
  | cons byte bytes ih =>
    by_cases zero : byte = 0
    · simp [escapePayload, zero, ih]
    · by_cases marker : byte = 69
      · simp [escapePayload, marker, ih]
      · simp [escapePayload, zero, marker, ih, Ne.symm marker]

/-- Encoded size is bounded by twice the raw payload size. -/
theorem payload_size_bound (bytes : List Nat) : (escapePayload bytes).length ≤ 2 * bytes.length := by
  induction bytes with
  | nil => simp [escapePayload]
  | cons byte bytes ih =>
    simp only [escapePayload]
    split
    · simp_all <;> omega
    · split <;> simp_all <;> omega

/-- A trailer-shaped subsequence cannot occur anywhere inside a correctly encoded payload. -/
theorem payload_has_no_internal_trailer (bytes before after : List Nat) :
    escapePayload bytes ≠ before ++ [69, 78, 68, 33] ++ after := by
  intro h
  have absent := payload_excludes_marker bytes
  rw [h] at absent
  simp at absent

/-- A torn payload, even with a partial real trailer, cannot mimic an earlier committed trailer. -/
theorem torn_payload_no_false_trailer (bytes trailerPart : List Nat) (cut : Nat)
    (short : trailerPart.length < 8)
    (enough : 8 ≤ ((escapePayload bytes).take cut).length + trailerPart.length) :
    ((((escapePayload bytes).take cut ++ trailerPart).drop
      (((escapePayload bytes).take cut ++ trailerPart).length - 8)).head?) ≠ some 69 := by
  intro h
  have index : ((escapePayload bytes).take cut ++ trailerPart).length - 8 <
      ((escapePayload bytes).take cut).length := by
    simp only [List.length_append]
    omega
  rw [List.head?_drop, List.getElem?_append_left index] at h
  exact payload_excludes_marker bytes (List.mem_of_mem_take (List.mem_of_getElem? h))

/-- FrameObservation supplies raw bytes and independently computed CRC primitives. -/
structure FrameObservation where
  header : List Nat
  remaining : Nat
  headerCRC : Nat
  frameCRC : Nat
  trailer : List Nat
  payload : List Nat
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
  (header.length < 6 || headerField header 4 2 == 2) &&
  (header.length < 7 || header[6]! == 0) &&
  (header.length < 8 || (1 ≤ headerField header 6 2 && headerField header 6 2 ≤ 11)) &&
  (expected == 0 || (header.drop 8).take 8 == (beBytes 8 expected).take (header.length - 8)) &&
  (header.length < 17 ||
    readBE ((header.drop 16).take 4 ++ List.replicate (4 - min (header.length - 16) 4) 0) ≤ 8388608) &&
  (header.length < 21 || (header.drop 20).take 4 == (beBytes 4 crc).take (header.length - 20))

/-- parseHeader preserves the order and bounds of the complete Go header parser. -/
def parseHeader (frame : FrameObservation) : Option (Int × Nat × Nat) :=
  let kind := headerField frame.header 6 2
  let length := headerField frame.header 16 4
  if frame.header.take 4 != [83, 87, 74, 49] || headerField frame.header 4 2 != 2 ||
      !(1 ≤ kind && kind ≤ 11) || headerField frame.header 20 4 != frame.headerCRC ||
      length > 8388608 then none
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
  let decoded ← match unescapePayload frame.payload with
    | none => .error 1
    | some value => .ok value
  if decoded.length > 4194304 then throw 1
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

/-- prepareRecord assigns only the writer-owned sequence and current format. -/
def prepareRecord (sequence : Nat) (record : Option Record) : Option Record := do
  let record ← record
  if record.sequence != 0 && record.sequence != sequence then none
  else some {record with format := 1, sequence := sequence}

/-- FrameEncoding contains protobuf and checksum primitive results for the prepared record. -/
structure FrameEncoding where
  payload : Option (List Nat)
  headerCRC : Nat
  frameCRC : Nat
  deriving Repr, Inhabited

/-- encodeFrame constructs the complete frame, including the canonical escaped payload. -/
def encodeFrame (record : Record) (encoding : FrameEncoding) : Option (List Nat) := do
  let raw ← encoding.payload
  if !(1 ≤ record.kind && record.kind ≤ 11) || raw.length > 4194304 then none
  else
    let payload := escapePayload raw
    let header := [83, 87, 74, 49] ++ beBytes 2 2 ++ beBytes 2 record.kind.toNat ++
      beBytes 8 record.sequence ++ beBytes 4 payload.length ++
      beBytes 4 encoding.headerCRC ++ beBytes 4 encoding.frameCRC
    some (header ++ payload ++ [69, 78, 68, 33] ++ beBytes 4 payload.length)

/-- GenerationMarker is the complete retained segment/checkpoint binding. -/
structure GenerationMarker where
  identity : String
  generation : Nat
  nextSequence : Nat
  snapshotLength : Nat
  snapshotDigest : String
  retiredLength : Nat
  retiredDigest : String
  deriving DecidableEq, Repr, Inhabited

/-- MarkerObservation exposes raw field decoding and a separately computed checksum. -/
structure MarkerObservation where
  size : Nat
  magic : String
  format : Nat
  crc : Nat
  computedCRC : Nat
  marker : GenerationMarker
  deriving Repr, Inhabited

/-- readMarker mirrors unmarshalJournalGenerationMarker's complete admission. -/
def readMarker (identity : String) (input : MarkerObservation) : Option GenerationMarker :=
  if input.size != 144 || input.magic != "53574731" || input.format != 1 ||
      identity.length != digestLength || input.marker.identity != identity || input.crc != input.computedCRC ||
      input.marker.generation == 0 || input.marker.nextSequence == 0 || input.marker.snapshotLength == 0 then none
  else some input.marker

/-- PendingActivation retains the marker and floor captured before authority verification. -/
structure PendingActivation where
  marker : GenerationMarker
  floor : Nat
  deriving DecidableEq, Repr, Inhabited

/-- WriterState is the state protected by journalWriter.mu during Append. -/
structure WriterState where
  bytes : MemoryBytes
  sequence : Nat
  offset : Nat
  records : List Record
  state : State
  poisoned : Bool
  pending : Option PendingActivation
  identity : String
  generation : Nat
  deriving Repr, Inhabited

/-- AppendEffects supplies concrete WriteAt and Sync outcomes for the existing memory storage. -/
structure AppendEffects where
  encoding : FrameEncoding
  writeFail : Bool
  writeLimit : Int
  syncOK : Bool
  deriving Repr, Inhabited

/-- AppendResult retains storage side effects when no acknowledgement is returned. -/
structure AppendResult where
  ok : Bool
  result : WriterState
  deriving Repr, Inhabited

/-- persistRecord writes, syncs, then exposes the already validated reducer transition.
The writer lock keeps validation and Apply on the same reducer state. -/
def persistRecord (before : WriterState) (record : Record) (frame : List Nat)
    (state : State) (effects : AppendEffects) : AppendResult :=
  let written := memoryWrite before.bytes before.offset frame effects.writeFail effects.writeLimit
  if effects.writeFail then ⟨false, {before with bytes := written, poisoned := true}⟩
  else
    let synced := syncBytes written effects.syncOK
    if !effects.syncOK then ⟨false, {before with bytes := synced, poisoned := true}⟩
    else ⟨true, {before with
      bytes := synced
      state := state
      offset := before.offset + frame.length
      sequence := (before.sequence + 1) % seqnoLimit
      records := before.records ++ [record]}⟩

/-- appendWriter mirrors Append's preparation, staged authentication, validation and ordered effects.
Authentication is the journal-pipeline dependency; encoding supplies only serialization/CRC primitives. -/
def appendWriter (before : WriterState) (record : Option Record)
    (authenticate : Record → Bool) (effects : AppendEffects) : AppendResult :=
  if before.poisoned || before.pending.isSome then ⟨false, before⟩
  else match prepareRecord before.sequence record with
  | none => ⟨false, before⟩
  | some record =>
    if !validRecord (some record) || ((record.kind == 1 || record.kind == 2) && !authenticate record) then
      ⟨false, before⟩
    else match encodeFrame record effects.encoding with
    | none => ⟨false, before⟩
    | some frame =>
      match applyRecord before.state (some record) with
      | none => ⟨false, before⟩
      | some state => persistRecord before record frame state effects

/-- Every acknowledgement follows a complete write and a successful Sync. -/
theorem persist_acknowledged {before : WriterState} {record : Record} {frame : List Nat}
    {state : State} {effects : AppendEffects}
    (h : (persistRecord before record frame state effects).ok = true) :
    effects.writeFail = false ∧ effects.syncOK = true ∧
    (persistRecord before record frame state effects).result.bytes.durable =
      (writeBytes before.bytes before.offset frame).data ∧
    (persistRecord before record frame state effects).result.state = state ∧
    (persistRecord before record frame state effects).result.sequence = (before.sequence + 1) % seqnoLimit ∧
    (persistRecord before record frame state effects).result.records = before.records ++ [record] := by
  unfold persistRecord at *
  cases write : effects.writeFail <;> cases sync : effects.syncOK <;> simp_all [memoryWrite, syncBytes]

/-- A failed write or Sync cannot discard an earlier synced byte. -/
theorem persist_failed {before : WriterState} {record : Record} {frame : List Nat}
    {state : State} {effects : AppendEffects}
    (h : (persistRecord before record frame state effects).ok = false) :
    (persistRecord before record frame state effects).result.bytes.durable = before.bytes.durable ∧
    (persistRecord before record frame state effects).result.poisoned = true ∧
    (persistRecord before record frame state effects).result.state = before.state := by
  unfold persistRecord at *
  cases write : effects.writeFail <;> cases sync : effects.syncOK <;> simp_all [memoryWrite_preserves_durable, syncBytes]

/-- Appending at the exact durable end preserves the entire acknowledged prefix. -/
theorem persist_extends_prefix {before : WriterState} {record : Record} {frame : List Nat}
    {state : State} {effects : AppendEffects}
    (synced : before.bytes.data = before.bytes.durable) (endOffset : before.offset = before.bytes.data.length)
    (h : (persistRecord before record frame state effects).ok = true) :
    (persistRecord before record frame state effects).result.bytes.durable = before.bytes.durable ++ frame := by
  rw [(persist_acknowledged h).2.2.1]
  simp [writeBytes, endOffset, synced]

/-- Successful Append exposes the exact prepared record, reducer transition and synced frame. -/
theorem append_acknowledged {before : WriterState} {record : Option Record}
    {authenticate : Record → Bool} {effects : AppendEffects}
    (h : (appendWriter before record authenticate effects).ok = true) :
    ∃ prepared frame state,
      prepareRecord before.sequence record = some prepared ∧
      validRecord (some prepared) = true ∧
      ((prepared.kind = 1 ∨ prepared.kind = 2) → authenticate prepared = true) ∧
      encodeFrame prepared effects.encoding = some frame ∧
      applyRecord before.state (some prepared) = some state ∧
      appendWriter before record authenticate effects = persistRecord before prepared frame state effects ∧
      (persistRecord before prepared frame state effects).ok = true := by
  unfold appendWriter at h
  split at h
  · contradiction
  · rename_i unfenced
    split at h
    · contradiction
    · rename_i prepared preparation
      split at h
      · contradiction
      · rename_i admitted
        split at h
        · contradiction
        · rename_i frame encoded
          split at h
          · contradiction
          · rename_i state applied
            have valid : validRecord (some prepared) = true := by
              simp_all
            have authenticated : (prepared.kind = 1 ∨ prepared.kind = 2) → authenticate prepared = true := by
              simp_all
            exact ⟨prepared, frame, state, preparation, valid, authenticated, encoded, applied,
              by simp [appendWriter, unfenced, preparation, admitted, encoded, applied], h⟩

/-- An acknowledged append both extends durable bytes and records the validated reducer result. -/
theorem append_extends_prefix {before : WriterState} {record : Option Record}
    {authenticate : Record → Bool} {effects : AppendEffects}
    (synced : before.bytes.data = before.bytes.durable) (endOffset : before.offset = before.bytes.data.length)
    (h : (appendWriter before record authenticate effects).ok = true) :
    ∃ prepared frame state,
      prepareRecord before.sequence record = some prepared ∧
      encodeFrame prepared effects.encoding = some frame ∧
      applyRecord before.state (some prepared) = some state ∧
      effects.writeFail = false ∧ effects.syncOK = true ∧
      (appendWriter before record authenticate effects).result.bytes.durable = before.bytes.durable ++ frame ∧
      (appendWriter before record authenticate effects).result.state = state ∧
      (appendWriter before record authenticate effects).result.records = before.records ++ [prepared] := by
  obtain ⟨prepared, frame, state, preparation, _, _, encoded, applied, result, ack⟩ := append_acknowledged h
  obtain ⟨written, sync, _, reduced, _, records⟩ := persist_acknowledged ack
  refine ⟨prepared, frame, state, preparation, encoded, applied, written, sync, ?_⟩
  rw [result]
  exact ⟨persist_extends_prefix synced endOffset ack, reduced, records⟩

/-- Preparation cannot borrow another sequence, including at the uint64 boundary. -/
theorem prepared_sequence {sequence : Nat} {record : Option Record} {prepared : Record}
    (h : prepareRecord sequence record = some prepared) : prepared.sequence = sequence := by
  cases record <;> simp only [prepareRecord, Option.bind_eq_bind, Option.bind_none, Option.bind_some] at h
  · contradiction
  · split at h
    · contradiction
    · cases h
      rfl

/-- An acknowledged append agrees with replay from the retained checkpoint and prior records. -/
theorem append_replay {before : WriterState} {record : Option Record}
    {authenticate : Record → Bool} {effects : AppendEffects} {base : State} {initial : Nat}
    (replayed : replayFrom base initial (before.records.map some) = some before.state)
    (sequence : before.sequence = advanceSequence initial before.records.length)
    (h : (appendWriter before record authenticate effects).ok = true) :
    replayFrom base initial ((appendWriter before record authenticate effects).result.records.map some) =
      some (appendWriter before record authenticate effects).result.state := by
  obtain ⟨prepared, frame, state, preparation, _, _, _, applied, result, ack⟩ := append_acknowledged h
  obtain ⟨_, _, _, reduced, _, records⟩ := persist_acknowledged ack
  rw [result, records, reduced, List.map_append, replayFrom_append, replayed]
  simp [replayFrom, prepared_sequence preparation, sequence, applied]

/-- Rejection at any Append stage preserves the complete previously synced journal. -/
theorem append_failure_preserves_durable (before : WriterState) (record : Option Record)
    (authenticate : Record → Bool) (effects : AppendEffects)
    (h : (appendWriter before record authenticate effects).ok = false) :
    (appendWriter before record authenticate effects).result.bytes.durable = before.bytes.durable := by
  unfold appendWriter at *
  repeat' first | split at * | simp_all | exact (persist_failed h).1

/-- A fenced or unactivated writer cannot change storage or acknowledge another append. -/
theorem append_fenced (before : WriterState) (record : Option Record)
    (authenticate : Record → Bool) (effects : AppendEffects)
    (h : before.poisoned = true ∨ before.pending.isSome = true) :
    appendWriter before record authenticate effects = ⟨false, before⟩ := by
  rcases h with h | h <;> simp [appendWriter, h]

/-- Exhaustion remains terminal: no sequence-zero record can reach storage. -/
theorem append_exhausted (before : WriterState) (record : Option Record)
    (authenticate : Record → Bool) (effects : AppendEffects) (h : before.sequence = 0) :
    appendWriter before record authenticate effects = ⟨false, before⟩ := by
  cases record with
  | none => simp [appendWriter, prepareRecord]
  | some record =>
    by_cases zero : record.sequence = 0 <;> simp [appendWriter, prepareRecord, h, zero, validRecord]

/-- ActivationInput supplies storage reads and injected publication effects.
Faults are 1/2 floor failure before/after write, 3/4 truncate failure before/after
change, and 5 failed Sync. Other values complete normally. -/
structure ActivationInput where
  generationSupported : Bool
  floorSupported : Bool
  floor : Nat
  floorReadOK : Bool
  marker : Option MarkerObservation
  retiredReadOK : Bool
  retiredDigest : String
  fault : Int
  deriving Repr, Inhabited

/-- ActivationResult retains partial storage effects on failure. -/
structure ActivationResult where
  ok : Bool
  result : WriterState
  floor : Nat
  deriving Repr, Inhabited

/-- activateWriter rechecks the observed generation before retiring its exact segment. -/
def activateWriter (before : WriterState) (input : ActivationInput) : ActivationResult :=
  match before.pending with
  | none => ⟨true, before, input.floor⟩
  | some pending =>
    if before.poisoned || !input.generationSupported || !input.floorSupported ||
        !input.floorReadOK || input.floor != pending.floor then ⟨false, before, input.floor⟩
    else match input.marker.bind (readMarker before.identity) with
    | none => ⟨false, before, input.floor⟩
    | some marker =>
      if marker.generation != pending.marker.generation || marker.nextSequence != pending.marker.nextSequence ||
          marker.snapshotLength != pending.marker.snapshotLength || marker.snapshotDigest != pending.marker.snapshotDigest ||
          marker.retiredLength != pending.marker.retiredLength || marker.retiredDigest != pending.marker.retiredDigest ||
          (marker.generation != input.floor && marker.generation != (input.floor + 1) % seqnoLimit) ||
          !input.retiredReadOK || before.bytes.data.length != marker.retiredLength || input.retiredDigest != marker.retiredDigest then
        ⟨false, before, input.floor⟩
      else
        let advance := marker.generation == (input.floor + 1) % seqnoLimit
        if advance && input.fault == 1 then ⟨false, before, input.floor⟩
        else
          let floor := if advance then max input.floor marker.generation else input.floor
          if advance && input.fault == 2 then ⟨false, before, floor⟩
          else if input.fault == 3 then ⟨false, {before with poisoned := true}, floor⟩
          else
            let truncated := {before with bytes := {before.bytes with data := []}}
            if input.fault == 4 then ⟨false, {truncated with poisoned := true}, floor⟩
            else if input.fault == 5 then
              ⟨false, {truncated with bytes := syncBytes truncated.bytes false, poisoned := true}, floor⟩
            else ⟨true, {truncated with
              bytes := syncBytes truncated.bytes true
              offset := 0
              sequence := marker.nextSequence
              records := []
              generation := marker.generation
              pending := none}, floor⟩

/-- Activation never lowers the durable generation floor, even after a partial failure. -/
theorem activation_floor_monotone (before : WriterState) (input : ActivationInput) :
    input.floor ≤ (activateWriter before input).floor := by
  unfold activateWriter
  dsimp only
  repeat' first | split | (simp_all <;> omega)

/-- Retirement preserves the authenticated reducer snapshot already held by the writer. -/
theorem activation_preserves_state (before : WriterState) (input : ActivationInput) :
    (activateWriter before input).result.state = before.state := by
  unfold activateWriter
  dsimp only
  repeat' first | split | simp_all

/-- A failed activation remains blocked from append by pending activation or poison. -/
theorem activation_failure_fenced {before : WriterState} {input : ActivationInput}
    (h : (activateWriter before input).ok = false) :
    (activateWriter before input).result.pending = before.pending ∧
    ((activateWriter before input).result.poisoned = true ∨
      (activateWriter before input).result.pending.isSome = true) := by
  unfold activateWriter at *
  repeat' first | split at * | simp_all

/-- Successful pending activation retires bytes only after checking the captured binding. -/
theorem activation_success {before : WriterState} {input : ActivationInput} {pending : PendingActivation}
    (waiting : before.pending = some pending) (h : (activateWriter before input).ok = true) :
    input.floor = pending.floor ∧
    input.retiredDigest = pending.marker.retiredDigest ∧
    before.bytes.data.length = pending.marker.retiredLength ∧
    (activateWriter before input).result.pending = none ∧
    (activateWriter before input).result.bytes.durable = [] ∧
    (activateWriter before input).result.sequence = pending.marker.nextSequence ∧
    (activateWriter before input).result.generation = pending.marker.generation ∧
    (activateWriter before input).result.records = [] := by
  unfold activateWriter at *
  rw [waiting] at *
  repeat' first | split at * | simp_all [syncBytes]

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
