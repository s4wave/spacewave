import Spacewave.SObject.Host
import Spacewave.SObject.Sync.Auth

/-!
# SharedObject paginated catch-up

Mirrors paging, response preparation, acceptance and the serialized writer in
`core/sobject/sync/catchup.go`.
`appendPage` preserves partial receive-buffer mutations on failure; `nextMessage`
preserves the consumed response prefix even when hashing a later entry fails.
Neither paging operation has access to held host state. `acceptResponse` alone
composes pinned-response checks with the existing atomic host import model.

Entries retain the configuration-chain projection. Serialized entry and complete
frame sizes, and signature-free entry hashes, are primitive observations. The
response's prefix-size table measures each candidate page with its original
revision and cursor. Missing size observations exceed the frame budget. The Go
projection supplies every measured prefix; no admission decision is an input.
Byte counters use naturals under the nonnegative, nonoverflowing Go-int contract.
Hashed entries are nonnil: verified host history supplies sender entries, and a
receiver with a nonempty reachable cursor rejects nil entries before hashing.

Response serialization, decoding and SHA-256 observations refer to the exact
stripped or received bytes. Provider history is an optional raw return value,
with the host's equal-checkpoint shortcut modeled explicitly. Acceptance keeps
the preflight read, state held under the import lock, and post-error read
separate because local progress can occur between them. The result records this
call's publication; independently observed concurrent writes are not its effect.
Committed revocation remains an error unless the later read is already ahead.
`drainPages` is a finite composition of paging calls, not another protocol owner.
-/

namespace Spacewave.SObject.Sync

def maxHistoryPageBytes : Nat := 1024 * 1024
def maxHistoryPageEntries : Nat := 128
def maxSuffixBytes : Nat := 8 * 1024 * 1024
def maxSuffixEntries : Nat := 4096

/-- Head retains the exact advertisement binding used by catch-up. -/
structure Head where
  revision : Nat
  configHash : String
  configSeqno : Nat
  rootSeqno : Nat
  stateHash : String
  deriving DecidableEq, Repr

/-- HistoryChange retains a decoded entry and primitive encoding/hash observations. -/
structure HistoryChange where
  entry : Entry
  bytes : Nat
  hashOK : Bool
  deriving DecidableEq, Repr

/-- Receive owns only an untrusted suffix and its pinned advertisement. -/
structure Receive where
  head : Head
  base : String
  cursor : String
  changes : List HistoryChange
  bytes : Nat
  deriving DecidableEq, Repr

/-- HistoryPage includes the serialized size of its complete message envelope. -/
structure HistoryPage where
  revision : Nat
  cursor : String
  changes : List HistoryChange
  bytes : Nat
  deriving DecidableEq, Repr

/-- ReceiveResult retains Go's receive-buffer effects on either return path. -/
structure ReceiveResult where
  ok : Bool
  state : Receive
  recovery : Bool
  deriving DecidableEq, Repr

/-- appendChanges follows continuity, size increment, budget, hash and append order. -/
def appendChanges (before : Receive) (changes : List HistoryChange) : ReceiveResult :=
  match changes with
  | [] => ⟨true, before, false⟩
  | change :: rest =>
    if change.entry.prev != before.cursor then ⟨false, before, false⟩
    else
      let sized := {before with bytes := before.bytes + change.bytes}
      if sized.bytes > maxSuffixBytes || before.changes.length == maxSuffixEntries then ⟨false, sized, true⟩
      else if !change.hashOK then ⟨false, sized, false⟩
      else appendChanges {sized with cursor := change.entry.hash, changes := before.changes ++ [change]} rest

/-- appendPage checks its pinned revision, current cursor and per-page budget first. -/
def appendPage (before : Receive) (page : Option HistoryPage) : ReceiveResult :=
  let page := page.getD ⟨0, "", [], 0⟩
  if page.revision != before.head.revision || page.cursor != before.cursor || page.changes.isEmpty then ⟨false, before, false⟩
  else if page.bytes > maxHistoryPageBytes || page.changes.length > maxHistoryPageEntries then ⟨false, before, true⟩
  else appendChanges before page.changes

/-- historyBytes counts the serialized retained entries without their page envelope. -/
def historyBytes (changes : List HistoryChange) : Nat :=
  (changes.map (·.bytes)).sum

/-- historyCursor is the digest after the last entry, or the initial cursor for an empty suffix. -/
def historyCursor (initial : String) (changes : List HistoryChange) : String :=
  changes.foldl (fun _ change => change.entry.hash) initial

/-- follows records causal links and successful primitive hashing throughout a suffix. -/
def follows (cursor : String) (changes : List HistoryChange) : Bool :=
  match changes with
  | [] => true
  | change :: rest => change.entry.prev == cursor && change.hashOK && follows change.entry.hash rest

/-- Snapshot retains the serialized candidate and response binding without granting it authority. -/
structure Snapshot where
  data : String
  rootSeqno : Nat
  revision : Nat
  base : String
  deriving DecidableEq, Repr

/-- Response retains the advertised snapshot while draining its selected suffix. -/
structure Response where
  revision : Nat
  cursor : String
  changes : List HistoryChange
  snapshot : Option Snapshot
  deriving DecidableEq, Repr

/-- prefixBytes reads the complete encoded page size for a proposed nonempty prefix. -/
def prefixBytes (sizes : List Nat) (count : Nat) : Nat :=
  (sizes[count - 1]?).getD (maxHistoryPageBytes + 1)

/-- PageBuild retains the accepted prefix even when the next primitive hash fails. -/
structure PageBuild where
  ok : Bool
  state : Response
  selected : List HistoryChange
  deriving DecidableEq, Repr

/-- buildPage follows append, whole-frame size check, hash and response-prefix consumption. -/
def buildPage (before : Response) (selected : List HistoryChange) (sizes : List Nat) : PageBuild :=
  match _observed : before.changes with
  | [] => ⟨true, before, selected⟩
  | change :: rest =>
    if selected.length ≥ maxHistoryPageEntries || prefixBytes sizes (selected.length + 1) > maxHistoryPageBytes then
      ⟨true, before, selected⟩
    else if !change.hashOK then ⟨false, before, selected⟩
    else buildPage {before with cursor := change.entry.hash, changes := rest} (selected ++ [change]) sizes
termination_by before.changes.length
decreasing_by simp_all

/-- NextMessage reports the protobuf body tag, returned frame and all response mutations. -/
structure NextMessage where
  ok : Bool
  state : Response
  kind : Int
  page : Option HistoryPage
  snapshot : Option Snapshot
  deriving DecidableEq, Repr

/-- nextMessage emits a snapshot only after the selected suffix has drained. -/
def nextMessage (before : Response) (sizes : List Nat) : NextMessage :=
  if before.changes.isEmpty then ⟨true, before, 1, none, before.snapshot⟩
  else
    let built := buildPage before [] sizes
    if !built.ok || built.selected.isEmpty then ⟨false, built.state, 0, none, none⟩
    else ⟨true, built.state, 9,
      some ⟨before.revision, before.cursor, built.selected, prefixBytes sizes built.selected.length⟩, none⟩

/-- Every receive result preserves the pinned target and original trusted base. -/
theorem appendChanges_binding (before : Receive) (changes : List HistoryChange) :
    (appendChanges before changes).state.head = before.head ∧
    (appendChanges before changes).state.base = before.base := by
  induction changes generalizing before with
  | nil => simp [appendChanges]
  | cons change rest ih =>
    unfold appendChanges
    split
    · exact ⟨rfl, rfl⟩
    · dsimp only
      split
      · exact ⟨rfl, rfl⟩
      · split
        · exact ⟨rfl, rfl⟩
        · exact ih _

/-- A page cannot change the pinned advertisement or the trusted checkpoint. -/
theorem appendPage_binding (before : Receive) (page : Option HistoryPage) :
    (appendPage before page).state.head = before.head ∧ (appendPage before page).state.base = before.base := by
  unfold appendPage
  dsimp only
  split
  · exact ⟨rfl, rfl⟩
  · split
    · exact ⟨rfl, rfl⟩
    · exact appendChanges_binding _ _

/-- Successful accumulation retains every entry in order and its exact byte count and final cursor. -/
theorem appendChanges_exact {before : Receive} {changes : List HistoryChange}
    (accepted : (appendChanges before changes).ok = true) :
    (appendChanges before changes).state.changes = before.changes ++ changes ∧
    (appendChanges before changes).state.bytes = before.bytes + historyBytes changes ∧
    (appendChanges before changes).state.cursor = historyCursor before.cursor changes ∧ follows before.cursor changes = true := by
  induction changes generalizing before with
  | nil => simp [appendChanges, historyBytes, historyCursor, follows]
  | cons change rest ih =>
    unfold appendChanges at accepted ⊢
    split at accepted
    · contradiction
    · rename_i contiguous
      simp only [contiguous]
      dsimp only at accepted ⊢
      split at accepted
      · contradiction
      · rename_i bounded
        simp only [bounded]
        split at accepted
        · contradiction
        · rename_i hashed
          simp only [hashed]
          obtain ⟨ordered, sized, cursor, linked⟩ := ih accepted
          have same : change.entry.prev = before.cursor := by simpa using contiguous
          have hashing : change.hashOK = true := by simpa using hashed
          simpa [historyBytes, historyCursor, follows, same, hashing, List.append_assoc, Nat.add_assoc]
            using And.intro ordered (And.intro sized (And.intro cursor linked))

/-- Healthy contiguous input within both aggregate budgets is completely accumulated. -/
theorem appendChanges_complete (before : Receive) (changes : List HistoryChange)
    (linked : follows before.cursor changes = true)
    (sized : before.bytes + historyBytes changes ≤ maxSuffixBytes)
    (counted : before.changes.length + changes.length ≤ maxSuffixEntries) :
    (appendChanges before changes).ok = true := by
  induction changes generalizing before with
  | nil => rfl
  | cons change rest ih =>
    simp only [follows, Bool.and_eq_true, beq_iff_eq] at linked
    have sizeTail : before.bytes + change.bytes + historyBytes rest ≤ maxSuffixBytes := by
      simpa [historyBytes, Nat.add_assoc] using sized
    have sizeHead : before.bytes + change.bytes ≤ maxSuffixBytes := by omega
    have countHead : before.changes.length ≠ maxSuffixEntries := by simp only [List.length_cons] at counted; omega
    have countTail : (before.changes ++ [change]).length + rest.length ≤ maxSuffixEntries := by
      simp only [List.length_append, List.length_cons, List.length_nil] at counted ⊢
      omega
    simp only [appendChanges, linked.1.1, bne_self_eq_false]
    simp only [show ¬before.bytes + change.bytes > maxSuffixBytes from Nat.not_lt.mpr sizeHead,
      countHead, beq_iff_eq, Bool.or_eq_true, ↓reduceIte, linked.1.2, Bool.not_true, Bool.false_eq_true]
    exact ih _ linked.2 sizeTail countTail

/-- An admitted page is complete, causally ordered and bound to its original request. -/
theorem appendPage_exact {before : Receive} {page : HistoryPage}
    (accepted : (appendPage before (some page)).ok = true) :
    page.revision = before.head.revision ∧ page.cursor = before.cursor ∧ page.changes ≠ [] ∧
    page.bytes ≤ maxHistoryPageBytes ∧ page.changes.length ≤ maxHistoryPageEntries ∧
    (appendPage before (some page)).state.changes = before.changes ++ page.changes ∧
    (appendPage before (some page)).state.cursor = historyCursor before.cursor page.changes ∧ follows before.cursor page.changes = true := by
  unfold appendPage at accepted ⊢
  dsimp only at accepted ⊢
  split at accepted
  · contradiction
  · rename_i binding
    simp only [binding]
    split at accepted
    · contradiction
    · rename_i bounded
      simp only [bounded]
      have exact := appendChanges_exact accepted
      simp_all

/-- Healthy bounded pages advance the receive buffer without an assumed acceptance result. -/
theorem appendPage_complete (before : Receive) (page : HistoryPage)
    (revision : page.revision = before.head.revision) (cursor : page.cursor = before.cursor)
    (nonempty : page.changes ≠ []) (pageBytes : page.bytes ≤ maxHistoryPageBytes)
    (pageCount : page.changes.length ≤ maxHistoryPageEntries) (linked : follows before.cursor page.changes = true)
    (sized : before.bytes + historyBytes page.changes ≤ maxSuffixBytes)
    (counted : before.changes.length + page.changes.length ≤ maxSuffixEntries) :
    (appendPage before (some page)).ok = true := by
  simpa [appendPage, revision, cursor, nonempty, Nat.not_lt.mpr pageBytes, Nat.not_lt.mpr pageCount]
    using appendChanges_complete before page.changes linked sized counted

/-- Page construction consumes a prefix without dropping or reordering any entry, including on error. -/
theorem buildPage_partition (before : Response) (selected : List HistoryChange) (sizes : List Nat) :
    (buildPage before selected sizes).selected ++ (buildPage before selected sizes).state.changes =
      selected ++ before.changes := by
  fun_induction buildPage
  all_goals simp_all [List.append_assoc]

/-- Page construction retains the pinned revision and snapshot. -/
theorem buildPage_binding (before : Response) (selected : List HistoryChange) (sizes : List Nat) :
    (buildPage before selected sizes).state.revision = before.revision ∧
    (buildPage before selected sizes).state.snapshot = before.snapshot := by
  fun_induction buildPage
  all_goals simp_all

/-- The selected prefix never exceeds the entry cap. -/
theorem buildPage_count (before : Response) (selected : List HistoryChange) (sizes : List Nat)
    (bounded : selected.length ≤ maxHistoryPageEntries) :
    (buildPage before selected sizes).selected.length ≤ maxHistoryPageEntries := by
  fun_induction buildPage
  all_goals simp_all
  omega

/-- Every nonempty selected prefix has passed the complete-frame byte check. -/
theorem buildPage_bytes (before : Response) (selected : List HistoryChange) (sizes : List Nat)
    (bounded : selected = [] ∨ prefixBytes sizes selected.length ≤ maxHistoryPageBytes) :
    (buildPage before selected sizes).selected = [] ∨
      prefixBytes sizes (buildPage before selected sizes).selected.length ≤ maxHistoryPageBytes := by
  fun_induction buildPage
  all_goals simp_all

/-- Page construction preserves the digest reached by the selected prefix. -/
theorem buildPage_cursor (before : Response) (selected : List HistoryChange) (sizes : List Nat)
    (initial : String) (cursor : before.cursor = historyCursor initial selected) :
    (buildPage before selected sizes).state.cursor = historyCursor initial (buildPage before selected sizes).selected := by
  fun_induction buildPage
  all_goals simp_all [historyCursor, List.foldl_append]

/-- A successfully hashed suffix cannot cause a page-construction error. -/
theorem buildPage_healthy (before : Response) (selected : List HistoryChange) (sizes : List Nat)
    (hashed : before.changes.all (·.hashOK) = true) : (buildPage before selected sizes).ok = true := by
  fun_induction buildPage
  all_goals simp_all

/-- Once an entry is selected, no later outcome removes that prefix. -/
theorem buildPage_nonempty (before : Response) (selected : List HistoryChange) (sizes : List Nat)
    (nonempty : selected ≠ []) : (buildPage before selected sizes).selected ≠ [] := by
  fun_induction buildPage
  all_goals simp_all

/-- Healthy nonempty history with a fitting first entry can emit a page. -/
theorem nextMessage_available (before : Response) (sizes : List Nat)
    (nonempty : before.changes ≠ []) (hashed : before.changes.all (·.hashOK) = true)
    (fits : prefixBytes sizes 1 ≤ maxHistoryPageBytes) :
    (nextMessage before sizes).ok = true ∧ (nextMessage before sizes).page.isSome = true := by
  rcases before with ⟨revision, cursor, changes, snapshot⟩
  cases changes with
  | nil => contradiction
  | cons change rest =>
    have firstHash : change.hashOK = true := by
      simp only [List.all_cons, Bool.and_eq_true] at hashed
      exact hashed.1
    have healthy := buildPage_healthy ⟨revision, cursor, change :: rest, snapshot⟩ [] sizes hashed
    have selected : (buildPage ⟨revision, cursor, change :: rest, snapshot⟩ [] sizes).selected ≠ [] := by
      rw [buildPage]
      simp only [List.length_nil, maxHistoryPageEntries, Nat.zero_add,
        show ¬(0 : Nat) ≥ 128 from by decide, decide_false, Bool.false_or,
        show ¬prefixBytes sizes 1 > maxHistoryPageBytes from Nat.not_lt.mpr fits,
        ↓reduceIte, firstHash, Bool.not_true, Bool.false_eq_true, List.nil_append]
      exact buildPage_nonempty _ [change] sizes (by simp)
    simp [nextMessage, healthy, selected]

/-- A snapshot is emitted only after all response entries have been drained. -/
theorem nextMessage_snapshot {before : Response} {sizes : List Nat}
    (snapshot : (nextMessage before sizes).kind = 1) :
    before.changes = [] ∧ (nextMessage before sizes).snapshot = before.snapshot := by
  unfold nextMessage at snapshot ⊢
  split at snapshot
  · simp_all
  · dsimp only at snapshot ⊢
    split at snapshot <;> simp at snapshot

/-- A returned page is a nonempty bounded prefix with the pinned revision and initial cursor. -/
theorem nextMessage_page {before : Response} {sizes : List Nat} {page : HistoryPage}
    (returned : (nextMessage before sizes).page = some page) :
    page.revision = before.revision ∧ page.cursor = before.cursor ∧ page.changes ≠ [] ∧
    page.changes.length ≤ maxHistoryPageEntries ∧ page.bytes ≤ maxHistoryPageBytes ∧
    page.changes ++ (nextMessage before sizes).state.changes = before.changes := by
  have partition := buildPage_partition before [] sizes
  have count := buildPage_count before [] sizes (by simp [maxHistoryPageEntries])
  have bytes := buildPage_bytes before [] sizes (Or.inl rfl)
  unfold nextMessage at returned ⊢
  split at returned
  · contradiction
  · rename_i nonempty
    simp only [nonempty]
    dsimp only at returned ⊢
    split at returned
    · contradiction
    · rename_i emitted
      simp only [emitted]
      cases Option.some.inj returned
      simp_all

/-- Every emitted page advances the response cursor to its last entry. -/
theorem nextMessage_cursor {before : Response} {sizes : List Nat} {page : HistoryPage}
    (returned : (nextMessage before sizes).page = some page) :
    (nextMessage before sizes).state.cursor = historyCursor before.cursor page.changes := by
  have cursor := buildPage_cursor before [] sizes before.cursor rfl
  unfold nextMessage at returned ⊢
  split at returned
  · contradiction
  · rename_i nonempty
    simp only [nonempty]
    dsimp only at returned ⊢
    split at returned
    · contradiction
    · rename_i emitted
      simp only [emitted]
      cases Option.some.inj returned
      exact cursor

/-- Every emitted page strictly reduces the remaining suffix, providing the finite progress measure. -/
theorem nextMessage_progress {before : Response} {sizes : List Nat} {page : HistoryPage}
    (returned : (nextMessage before sizes).page = some page) :
    (nextMessage before sizes).state.changes.length < before.changes.length := by
  have exact := nextMessage_page returned
  have positive : 0 < page.changes.length := by
    cases observed : page.changes with
    | nil => exact False.elim (exact.2.2.1 observed)
    | cons change rest => simp
  have partition := congrArg List.length exact.2.2.2.2.2
  simp only [List.length_append] at partition
  omega

/-- wireState removes local capabilities and local nonce reservations before disclosure. -/
def wireState (state : State) : State :=
  {state with invites := [], queued := []}

/-- Request retains the exact requested base and advertised revision. -/
structure Request where
  revision : Nat
  base : String
  deriving DecidableEq, Repr

/-- retainedHistory mirrors the host's same-checkpoint shortcut before the provider read. -/
def retainedHistory (state : State) (request : Request) (history : Option (List HistoryChange)) :
    Option (List HistoryChange) :=
  if request.base == state.config.hash && request.base != "" then some [] else history

/-- prepareResponse bounds retained history and encodes the stripped advertised state. -/
def prepareResponse (state : State) (request : Request) (history : Option (List HistoryChange))
    (encode : State → Option String) (frameBytes : Snapshot → Nat) : Option Response := do
  let changes ← retainedHistory state request history
  if changes.any (fun change => change.bytes + 256 > maxHistoryPageBytes) then none
  else
    let data ← encode (wireState state)
    let snapshot : Snapshot := ⟨data, state.root.seqno, request.revision, request.base⟩
    if frameBytes snapshot > 10 * 1024 * 1024 then none
    else some ⟨request.revision, request.base, changes, some snapshot⟩

/-- syncStateHash hashes the same stripped state, after requiring a checkpoint and bounded encoding. -/
def syncStateHash (state : State) (bytes : State → Nat) (encode : State → Option String)
    (digest : String → String) : Option String := do
  if state.config.hash == "" then none
  else if bytes (wireState state) > 10 * 1024 * 1024 then none
  else return digest (← encode (wireState state))

/-- responseObsolete permits only coordinatewise progress with at least one strict increase. -/
def responseObsolete (current : Option State) (head : Head) : Bool :=
  match current with
  | none => false
  | some state => state.config.seqno ≥ head.configSeqno && state.root.seqno ≥ head.rootSeqno &&
      (state.config.seqno > head.configSeqno || state.root.seqno > head.rootSeqno)

/-- AcceptanceInput retains decoding, sizes and each independently timed host-read observation. -/
structure AcceptanceInput where
  previous : State
  receiving : Receive
  snapshot : Option Snapshot
  digest : String
  decoded : Option State
  candidateBytes : Nat
  beforeRead : Option State
  afterRead : Option State
  localPeer : String
  lockOK : Bool
  accessOK : Bool
  writeOK : Bool
  deriving Repr

/-- AcceptanceResult separates the Go error result from this call's atomic host publication. -/
structure AcceptanceResult where
  ok : Bool
  host : HostResult
  deriving DecidableEq, Repr

/-- acceptResponse checks the complete pinned response before invoking host authority. -/
def acceptResponse (input : AcceptanceInput) : AcceptanceResult :=
  let rejected : AcceptanceResult := ⟨false, visibleHost input.previous none⟩
  let snapshot := input.snapshot.getD ⟨"", 0, 0, ""⟩
  if snapshot.revision != input.receiving.head.revision || snapshot.base != input.receiving.base ||
      input.receiving.cursor != input.receiving.head.configHash then rejected
  else if input.digest != input.receiving.head.stateHash then rejected
  else if responseObsolete input.beforeRead input.receiving.head then
    ⟨true, visibleHost input.previous none⟩
  else
    match input.decoded with
    | none => rejected
    | some candidate =>
      if snapshot.rootSeqno != input.receiving.head.rootSeqno || candidate.root.seqno != snapshot.rootSeqno ||
          candidate.config.seqno != input.receiving.head.configSeqno || candidate.config.hash != input.receiving.head.configHash then rejected
      else
        let result := importPeerSnapshot input.previous candidate (input.receiving.changes.map (·.entry))
          input.localPeer input.candidateBytes (historyBytes input.receiving.changes)
          input.lockOK input.accessOK input.writeOK
        let host := visibleHost input.previous result
        let failed := result.isNone || host.revoked
        ⟨!failed || responseObsolete input.afterRead input.receiving.head, host⟩

/-- Serialization and hashing share a projection with no local invitations or reservations. -/
theorem wireState_private (state : State) :
    (wireState state).invites = [] ∧ (wireState state).queued = [] ∧
    (wireState state).config = state.config ∧ (wireState state).root = state.root := by
  exact ⟨rfl, rfl, rfl, rfl⟩

/-- Every prepared snapshot encodes the stripped pinned state under the exact request binding. -/
theorem prepareResponse_bound {state : State} {request : Request} {history : Option (List HistoryChange)}
    {encode : State → Option String} {frameBytes : Snapshot → Nat} {response : Response}
    (prepared : prepareResponse state request history encode frameBytes = some response) :
    response.revision = request.revision ∧ response.cursor = request.base ∧
    ∃ snapshot, response.snapshot = some snapshot ∧ snapshot.revision = request.revision ∧
      snapshot.base = request.base ∧ snapshot.rootSeqno = state.root.seqno ∧
      encode (wireState state) = some snapshot.data ∧ frameBytes snapshot ≤ 10 * 1024 * 1024 := by
  unfold prepareResponse at prepared
  cases selected : retainedHistory state request history with
  | none => simp [selected] at prepared
  | some changes =>
    simp only [selected, Option.bind_eq_bind, Option.bind_some] at prepared
    split at prepared
    · contradiction
    · cases encoded : encode (wireState state) with
      | none => simp [encoded] at prepared
      | some data =>
        simp only [encoded, Option.bind_some] at prepared
        split at prepared
        · contradiction
        · cases prepared
          exact ⟨rfl, rfl, _, rfl, rfl, rfl, rfl, rfl, by omega⟩

/-- Obsolescence cannot hide a conflicting equal-sequence advertisement. -/
theorem responseObsolete_equal (state : State) (head : Head)
    (config : state.config.seqno = head.configSeqno) (root : state.root.seqno = head.rootSeqno) :
    responseObsolete (some state) head = false := by
  simp [responseObsolete, config, root]

/-- Every write performed by catch-up is an invocation of the proved atomic host import. -/
theorem acceptResponse_publication {input : AcceptanceInput}
    (wrote : (acceptResponse input).host.wrote = true) :
    ∃ candidate, input.decoded = some candidate ∧
      importPeerSnapshot input.previous candidate (input.receiving.changes.map (·.entry))
        input.localPeer input.candidateBytes (historyBytes input.receiving.changes)
        input.lockOK input.accessOK input.writeOK = some (acceptResponse input).host := by
  unfold acceptResponse at wrote ⊢
  dsimp only at wrote ⊢
  split at wrote
  · contradiction
  · rename_i binding
    simp only [binding]
    split at wrote
    · contradiction
    · rename_i digest
      simp only [digest]
      split at wrote
      · contradiction
      · rename_i current
        simp only [current]
        cases decoded : input.decoded with
        | none => simp [decoded, visibleHost] at wrote
        | some candidate =>
          simp only [decoded] at wrote ⊢
          split at wrote
          · contradiction
          · rename_i pinned
            simp only [pinned]
            cases imported : importPeerSnapshot input.previous candidate (input.receiving.changes.map (·.entry))
              input.localPeer input.candidateBytes (historyBytes input.receiving.changes)
              input.lockOK input.accessOK input.writeOK with
            | none => simp [imported, visibleHost] at wrote
            | some host => exact ⟨candidate, rfl, by simp [imported, visibleHost]⟩

/-- Every publication is bound to the requested revision, base, completed suffix and advertised content. -/
theorem acceptResponse_binding {input : AcceptanceInput}
    (wrote : (acceptResponse input).host.wrote = true) :
    let snapshot := input.snapshot.getD ⟨"", 0, 0, ""⟩
    snapshot.revision = input.receiving.head.revision ∧ snapshot.base = input.receiving.base ∧
    input.receiving.cursor = input.receiving.head.configHash ∧ input.digest = input.receiving.head.stateHash ∧
    ∃ candidate, input.decoded = some candidate ∧ candidate.root.seqno = input.receiving.head.rootSeqno ∧
      candidate.config.seqno = input.receiving.head.configSeqno ∧ candidate.config.hash = input.receiving.head.configHash := by
  unfold acceptResponse at wrote
  dsimp only at wrote ⊢
  split at wrote
  · contradiction
  · split at wrote
    · contradiction
    · split at wrote
      · contradiction
      · cases decoded : input.decoded with
        | none => simp [decoded, visibleHost] at wrote
        | some candidate =>
          simp only [decoded] at wrote
          split at wrote
          · contradiction
          · refine ⟨?_, ?_, ?_, ?_, candidate, rfl, ?_, ?_, ?_⟩ <;> simp_all

/-- An advertised content digest never grants configuration authority to a published state. -/
theorem acceptResponse_authority {input : AcceptanceInput}
    (wrote : (acceptResponse input).host.wrote = true) :
    ∃ candidate, input.decoded = some candidate ∧
      verifySuffix input.previous.config candidate.config
        (unappliedEntries input.previous.config.hash (input.receiving.changes.map (·.entry))) = true := by
  obtain ⟨candidate, decoded, imported⟩ := acceptResponse_publication wrote
  exact ⟨candidate, decoded, importPeerSnapshot_authority imported⟩

/-- Catch-up cannot publish a root rollback, including a committed removal reported as an error. -/
theorem acceptResponse_progress {input : AcceptanceInput}
    (wrote : (acceptResponse input).host.wrote = true) :
    input.previous.root.seqno ≤ (acceptResponse input).host.state.root.seqno := by
  obtain ⟨_, _, imported⟩ := acceptResponse_publication wrote
  exact (importPeerSnapshot_progress imported).1

/-- A complete clean newer response succeeds from primitive and chain contracts, without assumed admission. -/
theorem acceptResponse_complete (input : AcceptanceInput) (candidate : State) (snapshot : Snapshot)
    (present : input.snapshot = some snapshot) (decoded : input.decoded = some candidate)
    (revision : snapshot.revision = input.receiving.head.revision)
    (base : snapshot.base = input.receiving.base) (cursor : input.receiving.cursor = input.receiving.head.configHash)
    (digest : input.digest = input.receiving.head.stateHash)
    (current : responseObsolete input.beforeRead input.receiving.head = false)
    (snapshotRoot : snapshot.rootSeqno = input.receiving.head.rootSeqno)
    (rootSeq : candidate.root.seqno = snapshot.rootSeqno)
    (configSeq : candidate.config.seqno = input.receiving.head.configSeqno)
    (configHash : candidate.config.hash = input.receiving.head.configHash)
    (bounded : input.candidateBytes ≤ 10 * 1024 * 1024 ∧ input.receiving.changes.length ≤ 4096 ∧
      historyBytes input.receiving.changes ≤ 8 * 1024 * 1024)
    (chain : verifySuffix input.previous.config candidate.config (input.receiving.changes.map (·.entry)) = true)
    (readable : readableBy candidate.config input.localPeer = true)
    (root : importRoot input.previous.root candidate.root candidate.config = some candidate.root)
    (progress : input.previous.root.seqno < candidate.root.seqno) (valid : candidate.validate = true)
    (localEmpty : input.previous.ops = []) (remoteEmpty : candidate.ops = [])
    (lock : input.lockOK = true) (access : input.accessOK = true) (write : input.writeOK = true) :
    (acceptResponse input).ok = true ∧ (acceptResponse input).host.wrote = true ∧
      sameCheckpoint (acceptResponse input).host.state candidate = true := by
  let next := {candidate with invites := input.previous.invites, ops := [], queued := []}
  have changed : next ≠ input.previous := by
    intro same
    have seq := congrArg (fun state : State => state.root.seqno) same
    dsimp [next] at seq
    omega
  have imported : importPeerSnapshot input.previous candidate (input.receiving.changes.map (·.entry))
      input.localPeer input.candidateBytes (historyBytes input.receiving.changes) true true true =
      some ⟨next, false, true⟩ := by
    simp [importPeerSnapshot, bounded.1, bounded.2.1, bounded.2.2,
      unappliedEntries_of_verifySuffix chain, chain, readable,
      prepareReadableSnapshot_clean root valid localEmpty remoteEmpty, publishHost, next, changed]
  simp [acceptResponse, present, decoded, revision, base, cursor, digest, current, snapshotRoot,
    rootSeq, configSeq, configHash, lock, access, write, imported, visibleHost, next,
    sameCheckpoint, Config.same, List.isPerm_iff, Root.sameContent]

/-- Local invitation and reservation changes cannot alter the disclosed projection. -/
theorem wireState_local (state : State) (invites : List Invite) (queued : List AccountNonce) :
    wireState {state with invites, queued} = wireState state := rfl

/-- With the same primitive serializer, local capability changes cannot alter the advertised digest. -/
theorem syncStateHash_local (state : State) (invites : List Invite) (queued : List AccountNonce)
    (bytes : State → Nat) (encode : State → Option String) (digest : String → String) :
    syncStateHash {state with invites, queued} bytes encode digest = syncStateHash state bytes encode digest := rfl

/-- Preparation discloses the same bytes independently of local capabilities and reservations. -/
theorem prepareResponse_local (state : State) (invites : List Invite) (queued : List AccountNonce)
    (request : Request) (history : Option (List HistoryChange))
    (encode : State → Option String) (frameBytes : Snapshot → Nat) :
    prepareResponse {state with invites, queued} request history encode frameBytes =
      prepareResponse state request history encode frameBytes := rfl

/-- Healthy provider output and fitting real encodings produce the requested complete response. -/
theorem prepareResponse_complete (state : State) (request : Request) (history : Option (List HistoryChange))
    (encode : State → Option String) (frameBytes : Snapshot → Nat) (changes : List HistoryChange) (data : String)
    (read : retainedHistory state request history = some changes)
    (entries : ∀ change ∈ changes, change.bytes + 256 ≤ maxHistoryPageBytes)
    (encoded : encode (wireState state) = some data)
    (bounded : frameBytes ⟨data, state.root.seqno, request.revision, request.base⟩ ≤ 10 * 1024 * 1024) :
    prepareResponse state request history encode frameBytes =
      some ⟨request.revision, request.base, changes, some ⟨data, state.root.seqno, request.revision, request.base⟩⟩ := by
  simpa [prepareResponse, read, encoded, Nat.not_lt.mpr bounded] using entries

/-- Every sender step preserves the advertised revision and final snapshot. -/
theorem nextMessage_binding (before : Response) (sizes : List Nat) :
    (nextMessage before sizes).state.revision = before.revision ∧
    (nextMessage before sizes).state.snapshot = before.snapshot := by
  have binding := buildPage_binding before [] sizes
  unfold nextMessage
  split
  · exact ⟨rfl, rfl⟩
  · dsimp only
    split <;> exact binding

/-- drainPages is the finite sequence of paging calls, with fuel bounding the number of frames. -/
def drainPages : Nat → Response → (Response → List Nat) → Option (List HistoryPage × Option Snapshot)
  | 0, before, _ => if before.changes.isEmpty then some ([], before.snapshot) else none
  | fuel + 1, before, sizes =>
    if before.changes.isEmpty then some ([], before.snapshot)
    else do
      let page ← (nextMessage before (sizes before)).page
      let (pages, snapshot) ← drainPages fuel (nextMessage before (sizes before)).state sizes
      return (page :: pages, snapshot)

/-- A drained response contains every original entry exactly once, followed by its pinned snapshot. -/
theorem drainPages_exact {fuel : Nat} {before : Response} {sizes : Response → List Nat}
    {pages : List HistoryPage} {snapshot : Option Snapshot}
    (drained : drainPages fuel before sizes = some (pages, snapshot)) :
    pages.flatMap (·.changes) = before.changes ∧ snapshot = before.snapshot := by
  induction fuel generalizing before pages snapshot with
  | zero =>
    simp only [drainPages] at drained
    split at drained
    · cases drained
      simp_all
    · contradiction
  | succ fuel ih =>
    simp only [drainPages] at drained
    split at drained
    · cases drained
      simp_all
    · cases emitted : (nextMessage before (sizes before)).page with
      | none => simp [emitted] at drained
      | some page =>
        simp only [emitted, Option.bind_eq_bind, Option.bind_some] at drained
        cases rest : drainPages fuel (nextMessage before (sizes before)).state sizes with
        | none => simp [rest] at drained
        | some pair =>
          rcases pair with ⟨tail, final⟩
          simp only [rest, Option.bind_some] at drained
          rcases drained with ⟨rfl, rfl⟩
          obtain ⟨entries, pinned⟩ := ih rest
          have partition := (nextMessage_page emitted).2.2.2.2.2
          exact ⟨by simpa [entries] using partition, pinned.trans (nextMessage_binding before _).2⟩

/-- responseSuffix records exactly the cursor and pinned fields after consuming a prefix. -/
def responseSuffix (tail head : Response) : Prop :=
  ∃ consumed, consumed ++ tail.changes = head.changes ∧ tail.revision = head.revision ∧
    tail.snapshot = head.snapshot ∧ tail.cursor = historyCursor head.cursor consumed

/-- The initial response is its own suffix with no consumed entries. -/
theorem responseSuffix_self (response : Response) : responseSuffix response response :=
  ⟨[], rfl, rfl, rfl, rfl⟩

/-- Each emitted page leaves the precise remaining suffix of the original response. -/
theorem nextMessage_suffix {before : Response} {sizes : List Nat} {page : HistoryPage}
    (returned : (nextMessage before sizes).page = some page) :
    responseSuffix (nextMessage before sizes).state before := by
  have binding := nextMessage_binding before sizes
  exact ⟨page.changes, (nextMessage_page returned).2.2.2.2.2, binding.1, binding.2, nextMessage_cursor returned⟩

/-- Consuming two prefixes retains the same original binding and composes their cursors. -/
theorem responseSuffix_trans {tail middle head : Response}
    (left : responseSuffix tail middle) (right : responseSuffix middle head) : responseSuffix tail head := by
  obtain ⟨a, aEntries, aRevision, aSnapshot, aCursor⟩ := left
  obtain ⟨b, bEntries, bRevision, bSnapshot, bCursor⟩ := right
  refine ⟨b ++ a, ?_, aRevision.trans bRevision, aSnapshot.trans bSnapshot, ?_⟩
  · simpa [List.append_assoc, aEntries] using bEntries
  · simp [historyCursor, List.foldl_append, aCursor, bCursor, historyCursor]

/-- A finite healthy suffix drains with at most one page per entry under fitting encoding observations. -/
theorem drainPages_available (fuel : Nat) (before : Response) (sizes : Response → List Nat)
    (counted : before.changes.length ≤ fuel) (hashed : before.changes.all (·.hashOK) = true)
    (fits : ∀ response, responseSuffix response before → response.changes ≠ [] →
      prefixBytes (sizes response) 1 ≤ maxHistoryPageBytes) :
    ∃ pages snapshot, drainPages fuel before sizes = some (pages, snapshot) := by
  induction fuel generalizing before with
  | zero =>
    have empty : before.changes = [] := by simpa using counted
    exact ⟨[], before.snapshot, by simp [drainPages, empty]⟩
  | succ fuel ih =>
    by_cases empty : before.changes = []
    · exact ⟨[], before.snapshot, by simp [drainPages, empty]⟩
    · have available := nextMessage_available before (sizes before) empty hashed (fits before (responseSuffix_self before) empty)
      cases emitted : (nextMessage before (sizes before)).page with
      | none => simp [emitted] at available
      | some page =>
        have partition := (nextMessage_page emitted).2.2.2.2.2
        have retained : (page.changes ++ (nextMessage before (sizes before)).state.changes).all (·.hashOK) = true := by
          rw [partition]
          exact hashed
        simp only [List.all_append, Bool.and_eq_true] at retained
        have progress := nextMessage_progress emitted
        obtain ⟨pages, snapshot, tail⟩ := ih (nextMessage before (sizes before)).state (by omega) retained.2
          (fun response suffix nonempty => fits response (responseSuffix_trans suffix (nextMessage_suffix emitted)) nonempty)
        exact ⟨page :: pages, snapshot, by simp [drainPages, empty, emitted, tail]⟩

/-- pageSequence records the exact binding, bounds and suffix partition of consecutive pages. -/
def pageSequence (revision : Nat) (cursor : String) (changes : List HistoryChange) : List HistoryPage → Prop
  | [] => changes = []
  | page :: pages =>
    page.revision = revision ∧ page.cursor = cursor ∧ page.changes ≠ [] ∧
    page.bytes ≤ maxHistoryPageBytes ∧ page.changes.length ≤ maxHistoryPageEntries ∧
    ∃ remaining, page.changes ++ remaining = changes ∧
      pageSequence revision (historyCursor cursor page.changes) remaining pages

/-- History byte totals compose across the exact prefix emitted by a page. -/
theorem historyBytes_append (left right : List HistoryChange) :
    historyBytes (left ++ right) = historyBytes left + historyBytes right := by
  simp [historyBytes]

/-- Causal history splits at any prefix without changing the suffix's starting digest. -/
theorem follows_append (cursor : String) (left right : List HistoryChange) :
    follows cursor (left ++ right) =
      (follows cursor left && follows (historyCursor cursor left) right) := by
  induction left generalizing cursor with
  | nil => simp [follows, historyCursor]
  | cons change rest ih => simp [follows, historyCursor, ih, Bool.and_assoc]

/-- A causal suffix contains only successful primitive hash observations. -/
theorem follows_hashes {cursor : String} {changes : List HistoryChange}
    (linked : follows cursor changes = true) : changes.all (·.hashOK) = true := by
  induction changes generalizing cursor with
  | nil => rfl
  | cons change rest ih =>
    simp only [follows, Bool.and_eq_true] at linked
    simp only [List.all_cons, linked.1.2, Bool.true_and]
    exact ih linked.2

/-- Every successful page has exact retained-byte accounting. -/
theorem appendPage_bytes {before : Receive} {page : HistoryPage}
    (accepted : (appendPage before (some page)).ok = true) :
    (appendPage before (some page)).state.bytes = before.bytes + historyBytes page.changes := by
  unfold appendPage at accepted ⊢
  dsimp only at accepted ⊢
  split at accepted
  · contradiction
  · rename_i binding
    simp only [binding]
    split at accepted
    · contradiction
    · rename_i bounded
      simp only [bounded]
      exact (appendChanges_exact accepted).2.1

/-- A complete sender trace provides a bounded and consistently bound receiver schedule. -/
theorem drainPages_schedule {fuel : Nat} {before : Response} {sizes : Response → List Nat}
    {pages : List HistoryPage} {snapshot : Option Snapshot}
    (drained : drainPages fuel before sizes = some (pages, snapshot)) :
    pageSequence before.revision before.cursor before.changes pages := by
  induction fuel generalizing before pages snapshot with
  | zero =>
    simp only [drainPages] at drained
    split at drained
    · cases drained
      simpa [pageSequence] using ‹before.changes.isEmpty = true›
    · contradiction
  | succ fuel ih =>
    simp only [drainPages] at drained
    split at drained
    · cases drained
      simpa [pageSequence] using ‹before.changes.isEmpty = true›
    · cases emitted : (nextMessage before (sizes before)).page with
      | none => simp [emitted] at drained
      | some page =>
        simp only [emitted, Option.bind_eq_bind, Option.bind_some] at drained
        cases rest : drainPages fuel (nextMessage before (sizes before)).state sizes with
        | none => simp [rest] at drained
        | some pair =>
          rcases pair with ⟨tail, final⟩
          simp only [rest, Option.bind_some] at drained
          rcases drained with ⟨rfl, rfl⟩
          have schedule := ih rest
          obtain ⟨revision, cursor, nonempty, count, bytes, partition⟩ := nextMessage_page emitted
          refine ⟨revision, cursor, nonempty, bytes, count, _, partition, ?_⟩
          simpa only [(nextMessage_binding before (sizes before)).1, nextMessage_cursor emitted] using schedule

/-- receivePages composes successful appendPage calls; rejection never yields a complete receiver. -/
def receivePages (before : Receive) : List HistoryPage → Option Receive
  | [] => some before
  | page :: pages =>
    let received := appendPage before (some page)
    if received.ok then receivePages received.state pages else none

/-- Successful page reception retains its pinned target and accumulates precisely the sent entries. -/
theorem receivePages_exact {before after : Receive} {pages : List HistoryPage}
    (received : receivePages before pages = some after) :
    after.head = before.head ∧ after.base = before.base ∧
    after.changes = before.changes ++ pages.flatMap (·.changes) ∧
    after.bytes = before.bytes + historyBytes (pages.flatMap (·.changes)) ∧
    after.cursor = historyCursor before.cursor (pages.flatMap (·.changes)) := by
  induction pages generalizing before with
  | nil =>
    cases received
    simp [historyBytes, historyCursor]
  | cons page pages ih =>
    simp only [receivePages] at received
    split at received
    · rename_i accepted
      obtain ⟨head, base, entries, bytes, cursor⟩ := ih received
      have binding := appendPage_binding before (some page)
      obtain ⟨_, _, _, _, _, firstEntries, firstCursor, _⟩ := appendPage_exact accepted
      refine ⟨head.trans binding.1, base.trans binding.2, ?_, ?_, ?_⟩
      · simpa [firstEntries, List.append_assoc] using entries
      · simpa [appendPage_bytes accepted, historyBytes_append, Nat.add_assoc] using bytes
      · simpa [firstCursor, historyCursor, List.foldl_append] using cursor
    · contradiction

/-- A bounded causal sender schedule is completely received without assuming any page admission. -/
theorem receivePages_complete (before : Receive) (changes : List HistoryChange) (pages : List HistoryPage)
    (schedule : pageSequence before.head.revision before.cursor changes pages)
    (linked : follows before.cursor changes = true)
    (sized : before.bytes + historyBytes changes ≤ maxSuffixBytes)
    (counted : before.changes.length + changes.length ≤ maxSuffixEntries) :
    ∃ after, receivePages before pages = some after := by
  induction pages generalizing before changes with
  | nil => exact ⟨before, rfl⟩
  | cons page pages ih =>
    obtain ⟨revision, cursor, nonempty, pageBytes, pageCount, remaining, partition, tail⟩ := schedule
    rw [← partition, follows_append] at linked
    simp only [Bool.and_eq_true] at linked
    rw [← partition, historyBytes_append] at sized
    have length := congrArg List.length partition
    simp only [List.length_append] at length
    have admitted := appendPage_complete before page revision cursor nonempty pageBytes pageCount linked.1
      (by omega) (by omega)
    have binding := appendPage_binding before (some page)
    obtain ⟨_, _, _, _, _, entries, finish, _⟩ := appendPage_exact admitted
    have bytes := appendPage_bytes admitted
    obtain ⟨after, received⟩ := ih (appendPage before (some page)).state remaining
      (by simpa only [binding.1, finish] using tail)
      (by simpa only [finish] using linked.2)
      (by rw [bytes]; omega)
      (by rw [entries, List.length_append]; omega)
    exact ⟨after, by simp only [receivePages, admitted, ↓reduceIte]; exact received⟩

/-- A healthy complete response drains and reconstructs the same suffix with its original snapshot. -/
theorem pagesExchange_complete (sender : Response) (receiver : Receive) (sizes : Response → List Nat)
    (revision : sender.revision = receiver.head.revision) (cursor : sender.cursor = receiver.cursor)
    (linked : follows sender.cursor sender.changes = true)
    (sized : receiver.bytes + historyBytes sender.changes ≤ maxSuffixBytes)
    (counted : receiver.changes.length + sender.changes.length ≤ maxSuffixEntries)
    (fits : ∀ response, responseSuffix response sender → response.changes ≠ [] →
      prefixBytes (sizes response) 1 ≤ maxHistoryPageBytes) :
    ∃ pages snapshot after, drainPages sender.changes.length sender sizes = some (pages, snapshot) ∧
      receivePages receiver pages = some after ∧ after.head = receiver.head ∧ after.base = receiver.base ∧
      after.changes = receiver.changes ++ sender.changes ∧
      after.cursor = historyCursor sender.cursor sender.changes ∧ snapshot = sender.snapshot := by
  obtain ⟨pages, snapshot, sent⟩ := drainPages_available sender.changes.length sender sizes
    (Nat.le_refl _) (follows_hashes linked) fits
  have schedule := drainPages_schedule sent
  obtain ⟨after, received⟩ := receivePages_complete receiver sender.changes pages
    (by simpa only [revision, cursor] using schedule) (by simpa only [cursor] using linked) sized counted
  obtain ⟨head, base, entries, _, finish⟩ := receivePages_exact received
  obtain ⟨complete, pinned⟩ := drainPages_exact sent
  exact ⟨pages, snapshot, after, sent, received, head, base,
    by simpa only [complete] using entries, by simpa only [complete, cursor] using finish, pinned⟩

/-- Complete bounded page delivery reaches host publication without an assumed page or import result. -/
theorem pagesExchange_imports (sender : Response) (input : AcceptanceInput) (sizes : Response → List Nat)
    (candidate : State) (snapshot : Snapshot)
    (revision : sender.revision = input.receiving.head.revision) (cursor : sender.cursor = input.receiving.cursor)
    (linked : follows sender.cursor sender.changes = true)
    (sized : input.receiving.bytes + historyBytes sender.changes ≤ maxSuffixBytes)
    (counted : input.receiving.changes.length + sender.changes.length ≤ maxSuffixEntries)
    (fits : ∀ response, responseSuffix response sender → response.changes ≠ [] →
      prefixBytes (sizes response) 1 ≤ maxHistoryPageBytes)
    (empty : input.receiving.changes = []) (pinned : sender.snapshot = input.snapshot)
    (present : input.snapshot = some snapshot) (decoded : input.decoded = some candidate)
    (snapshotRevision : snapshot.revision = input.receiving.head.revision)
    (snapshotBase : snapshot.base = input.receiving.base)
    (targetCursor : historyCursor sender.cursor sender.changes = input.receiving.head.configHash)
    (digest : input.digest = input.receiving.head.stateHash)
    (current : responseObsolete input.beforeRead input.receiving.head = false)
    (snapshotRoot : snapshot.rootSeqno = input.receiving.head.rootSeqno)
    (rootSeq : candidate.root.seqno = snapshot.rootSeqno)
    (configSeq : candidate.config.seqno = input.receiving.head.configSeqno)
    (configHash : candidate.config.hash = input.receiving.head.configHash)
    (candidateBytes : input.candidateBytes ≤ 10 * 1024 * 1024)
    (chain : verifySuffix input.previous.config candidate.config (sender.changes.map (·.entry)) = true)
    (readable : readableBy candidate.config input.localPeer = true)
    (root : importRoot input.previous.root candidate.root candidate.config = some candidate.root)
    (progress : input.previous.root.seqno < candidate.root.seqno) (valid : candidate.validate = true)
    (localEmpty : input.previous.ops = []) (remoteEmpty : candidate.ops = [])
    (lock : input.lockOK = true) (access : input.accessOK = true) (write : input.writeOK = true) :
    ∃ pages after, drainPages sender.changes.length sender sizes = some (pages, input.snapshot) ∧
      receivePages input.receiving pages = some after ∧
      (acceptResponse {input with receiving := after}).ok = true ∧
      (acceptResponse {input with receiving := after}).host.wrote = true ∧
      sameCheckpoint (acceptResponse {input with receiving := after}).host.state candidate = true := by
  obtain ⟨pages, final, after, sent, received, head, base, entries, finish, snapshotEq⟩ :=
    pagesExchange_complete sender input.receiving sizes revision cursor linked sized counted fits
  have changes : after.changes = sender.changes := by simpa only [empty, List.nil_append] using entries
  have bounded : input.candidateBytes ≤ 10 * 1024 * 1024 ∧ after.changes.length ≤ 4096 ∧
      historyBytes after.changes ≤ 8 * 1024 * 1024 := by
    refine ⟨candidateBytes, ?_, ?_⟩
    · simpa only [changes, empty, List.length_nil, Nat.zero_add, maxSuffixEntries] using counted
    · rw [changes]
      dsimp [maxSuffixBytes] at sized
      omega
  have imported := acceptResponse_complete {input with receiving := after} candidate snapshot present decoded
    (by simpa only [head] using snapshotRevision)
    (by simpa only [base] using snapshotBase)
    (by simpa only [head] using finish.trans targetCursor)
    (by simpa only [head] using digest)
    (by simpa only [head] using current)
    (by simpa only [head] using snapshotRoot) rootSeq
    (by simpa only [head] using configSeq)
    (by simpa only [head] using configHash) bounded
    (by simpa only [changes] using chain) readable root progress valid localEmpty remoteEmpty lock access write
  exact ⟨pages, after, by simpa only [snapshotEq, pinned] using sent, received, imported⟩

/-- WriterAttempt records a channel selection, current authority and completed transport operation.
    `selected = false` means cancellation won before handoff. `reported = false`
    means cancellation won after the write. No later attempt is then reachable.
    `writeOK` is the actual transport outcome, including a denial write's outcome. -/
structure WriterAttempt where
  selected : Bool
  participants : List Participant
  hash : String
  kind : Int
  writeOK : Bool
  reported : Bool
  deriving Repr

/-- WriterResult records attempted wire body tags and delivered success results.
    Frames are transport admissions, not a claim that their bytes were delivered.
    `waiting` denotes a finite prefix paused before the next handoff. -/
structure WriterResult where
  frames : List Int
  results : List Bool
  waiting : Bool
  deriving DecidableEq, Repr

/-- writeFrames mirrors writeMessages, including ignored denial-write failures and both cancellation waits. -/
def writeFrames (localID remote : String) : List WriterAttempt → WriterResult
  | [] => ⟨[], [], true⟩
  | attempt :: rest =>
    if !attempt.selected then
      ⟨[], [], false⟩
    else
      let admitted := authorizeParticipants attempt.participants attempt.hash localID remote
      let frame := if admitted then attempt.kind else 6
      if !attempt.reported then
        ⟨[frame], [], false⟩
      else if !(admitted && attempt.writeOK) then
        ⟨[frame], [false], false⟩
      else
        let tail := writeFrames localID remote rest
        ⟨frame :: tail.frames, true :: tail.results, tail.waiting⟩

/-- A rejected current role/history observation emits only denial and never processes the suffix. -/
theorem writeFrames_rejected {localID remote : String} {attempt : WriterAttempt} {rest : List WriterAttempt}
    (selected : attempt.selected = true)
    (denied : authorizeParticipants attempt.participants attempt.hash localID remote = false) :
    writeFrames localID remote (attempt :: rest) =
      ⟨[6], if attempt.reported then [false] else [], false⟩ := by
  cases report : attempt.reported <;> simp [writeFrames, selected, denied, report]

/-- After any terminal prefix, later grants, messages or scheduling choices cannot restart this writer. -/
theorem writeFrames_stopped {localID remote : String} {before after : List WriterAttempt}
    (stopped : (writeFrames localID remote before).waiting = false) :
    writeFrames localID remote (before ++ after) = writeFrames localID remote before := by
  induction before with
  | nil => simp [writeFrames] at stopped
  | cons attempt rest ih =>
    simp only [List.cons_append, writeFrames] at *
    split at * <;> simp_all
    split at * <;> simp_all
    split at * <;> simp_all

/-- Every admitted data tag has a selected handoff with currently readable endpoints and held history. -/
theorem writeFrames_authorized {localID remote : String} {attempts : List WriterAttempt} {kind : Int}
    (sent : kind ∈ (writeFrames localID remote attempts).frames) (dataFrame : kind ≠ 6) :
    ∃ attempt ∈ attempts, attempt.selected = true ∧ attempt.kind = kind ∧
      authorizeParticipants attempt.participants attempt.hash localID remote = true := by
  induction attempts with
  | nil => simp [writeFrames] at sent
  | cons attempt rest ih =>
    simp only [writeFrames] at sent
    split at sent
    · simp at sent
    · have selected : attempt.selected = true := by simpa using ‹¬(!attempt.selected) = true›
      by_cases admitted : authorizeParticipants attempt.participants attempt.hash localID remote = true
      · simp only [admitted, ite_true] at sent
        split at sent
        · simp only [List.mem_singleton] at sent
          exact ⟨attempt, by simp, selected, sent.symm, admitted⟩
        · split at sent
          · simp only [List.mem_singleton] at sent
            exact ⟨attempt, by simp, selected, sent.symm, admitted⟩
          · simp only [List.mem_cons] at sent
            rcases sent with same | tail
            · exact ⟨attempt, by simp, selected, same.symm, admitted⟩
            · obtain ⟨previous, member, proof⟩ := ih tail
              exact ⟨previous, by simp [member], proof⟩
      · have denied : authorizeParticipants attempt.participants attempt.hash localID remote = false := by
          simpa using admitted
        cases report : attempt.reported <;> simp [denied, report, dataFrame] at sent

/-- A healthy sequence sends and reports every selected frame in order. -/
theorem writeFrames_complete {localID remote : String} {attempts : List WriterAttempt}
    (healthy : ∀ attempt ∈ attempts, attempt.selected = true ∧
      authorizeParticipants attempt.participants attempt.hash localID remote = true ∧
      attempt.writeOK = true ∧ attempt.reported = true) :
    writeFrames localID remote attempts = ⟨attempts.map (·.kind), attempts.map (fun _ => true), true⟩ := by
  induction attempts with
  | nil => rfl
  | cons attempt rest ih =>
    obtain ⟨selected, admitted, written, reported⟩ := healthy attempt (by simp)
    have tail := ih (fun item member => healthy item (by simp [member]))
    simp [writeFrames, selected, admitted, written, reported, tail]

/-- ExchangeFrame projects actual protobuf body tags and the fields each branch reads.
    Nil nested messages use protobuf getter defaults; page/snapshot presence is retained. -/
structure ExchangeFrame where
  kind : Int := 0
  head : Head := ⟨0, "", 0, 0, ""⟩
  hashBytes : Nat := 0
  stateHashBytes : Nat := 0
  request : Request := ⟨0, ""⟩
  baseBytes : Nat := 0
  page : Option HistoryPage := none
  snapshot : Option Snapshot := none
  revision : Nat := 0
  accepted : Bool := false
  recoveryPresent : Bool := false
  deriving DecidableEq, Repr

/-- Exchange retains the sole owner's pinned state and writer slots.
    Optional nanosecond timestamps distinguish Go's zero time from a real deadline. -/
structure Exchange where
  advertised : Option State
  lastAdvertised : Option State
  revision : Nat
  remoteRevision : Nat
  advertisementDeadline : Option Int
  requested : Bool
  receiving : Option Receive
  receiveDeadline : Option Int
  response : Option Response
  control : Option ExchangeFrame
  outgoing : Option ExchangeFrame
  inFlight : Option ExchangeFrame
  terminal : Bool
  deriving DecidableEq, Repr

/-- ExchangePrimitives records reads, encodings, hashing, time and observer availability.
    sameCurrent is Go's complete EqualVT observation, including unprojected protobuf fields.
    Acceptance's receiver and snapshot are overwritten by the actual owner/frame binding. -/
structure ExchangePrimitives where
  now : Int
  sameCurrent : Bool
  currentHashBytes : Nat
  stateBytes : Nat
  encoded : Option String
  digest : String
  history : Option (List HistoryChange)
  advertisedEncoded : Option String
  snapshotBytes : Nat
  sizes : List Nat
  acceptance : AcceptanceInput
  healthRead : Option State
  admissionObserver : Bool
  recoveryObserver : Bool
  deriving Repr

/-- ExchangeResult records protocol state and calls into the existing host/operation owners.
    operation means dispatch to handleRemoteOp, whose own validation remains required. -/
structure ExchangeResult where
  ok : Bool
  state : Exchange
  imported : Option AcceptanceResult := none
  operation : Bool := false
  admission : Option Bool := none
  recovery : Option Bool := none
  deriving DecidableEq, Repr

/-- exchangeDigest uses the actual stripped current-state serialization primitives. -/
def exchangeDigest (current : State) (input : ExchangePrimitives) : Option String :=
  syncStateHash current (fun _ => input.stateBytes) (fun _ => input.encoded) (fun _ => input.digest)

/-- prepareOutgoing mirrors the priority pump and exact partial mutations on failure. -/
def prepareOutgoing (before : Exchange) (current : Option State) (input : ExchangePrimitives) : ExchangeResult := Id.run do
  if before.outgoing.isSome || before.inFlight.isSome then
    return ⟨true, before, none, false, none, none⟩
  if let some control := before.control then
    return ⟨true, {before with outgoing := some control, control := none}, none, false, none, none⟩
  if let some response := before.response then
    let next := nextMessage response input.sizes
    let state := {before with
      response := some next.state
      outgoing := if next.ok then some {kind := next.kind, page := next.page, snapshot := next.snapshot} else none}
    if !next.ok then
      return ⟨false, state, none, false, none, none⟩
    return ⟨true, if next.snapshot.isSome then {state with response := none} else state, none, false, none, none⟩
  let some current := current | return ⟨true, before, none, false, none, none⟩
  if before.advertised.isSome || input.sameCurrent then
    return ⟨true, before, none, false, none, none⟩
  let revision := (before.revision + 1) % (2 ^ 64)
  let state := {before with revision := revision}
  if revision == 0 then
    return ⟨false, state, none, false, none, none⟩
  let some digest := exchangeDigest current input | return ⟨false, state, none, false, none, none⟩
  let head : Head := ⟨revision, current.config.hash, current.config.seqno, current.root.seqno, digest⟩
  return ⟨true, {state with
    advertised := some current
    lastAdvertised := some current
    advertisementDeadline := some (input.now + 60000000000)
    requested := false
    outgoing := some {kind := 7, head := head, hashBytes := input.currentHashBytes, stateHashBytes := 32}}, none, false, none, none⟩

/-- receiveExchange mirrors the authorized owner's dispatch, retaining primitive failure ordering. -/
def receiveExchange (before : Exchange) (current : State) (frame : ExchangeFrame)
    (input : ExchangePrimitives) : ExchangeResult :=
  if frame.kind == 6 then
    {ok := false, state := before, admission := if !frame.accepted && input.admissionObserver then some false else none}
  else if before.terminal then
    ⟨true, before, none, false, none, none⟩
  else
    match frame.kind with
    | 7 => Id.run do
      let head := frame.head
      if head.configHash == "" then
        return ⟨false, before, none, false, none, none⟩
      if head.revision ≤ before.remoteRevision || frame.hashBytes > 128 || frame.stateHashBytes != 32 ||
          before.receiving.isSome || before.control.isSome then
        return ⟨false, before, none, false, none, none⟩
      let state := {before with remoteRevision := head.revision}
      let some digest := exchangeDigest current input | return ⟨false, state, none, false, none, none⟩
      let recovery := if head.stateHash == digest && input.recoveryObserver then some false else none
      if head.stateHash == digest || head.configSeqno < current.config.seqno ||
          (head.configHash == current.config.hash && head.rootSeqno < current.root.seqno) then
        return {ok := true, state := {state with control := some {kind := 3, revision := head.revision}}, recovery := recovery}
      let base := current.config.hash
      let receiving : Receive := ⟨head, base, base, [], 0⟩
      let control : ExchangeFrame := {kind := 8, request := ⟨head.revision, base⟩, baseBytes := input.currentHashBytes}
      let state := {state with
        receiving := some receiving
        receiveDeadline := some (input.now + 60000000000)
        control := some control}
      return {ok := true, state := state, recovery := recovery}
    | 8 => Id.run do
      let some advertised := before.advertised | return ⟨false, before, none, false, none, none⟩
      if frame.request.revision != before.revision || before.requested || frame.request.base == "" || frame.baseBytes > 128 then
        return ⟨false, before, none, false, none, none⟩
      let response := prepareResponse advertised frame.request input.history (fun _ => input.advertisedEncoded)
        (fun _ => input.snapshotBytes)
      let state := {before with requested := true, response := response}
      if response.isNone then
        return ⟨true, {state with terminal := true, control := some {kind := 10, revision := before.revision, recoveryPresent := true}}, none, false, none, none⟩
      return ⟨true, state, none, false, none, none⟩
    | 9 => Id.run do
      let some receiving := before.receiving | return ⟨false, before, none, false, none, none⟩
      let page := appendPage receiving frame.page
      let state := {before with receiving := some page.state}
      if page.ok then
        return ⟨true, state, none, false, none, none⟩
      if !page.recovery then
        return ⟨false, state, none, false, none, none⟩
      return ⟨true, {state with terminal := true, control := some {kind := 10, revision := receiving.head.revision, recoveryPresent := true}}, none, false, none, none⟩
    | 1 => Id.run do
      let some receiving := before.receiving | return ⟨false, before, none, false, none, none⟩
      if before.control.isSome then
        return ⟨false, before, none, false, none, none⟩
      let accepted := acceptResponse {input.acceptance with receiving := receiving, snapshot := frame.snapshot}
      if !accepted.ok then
        return {ok := false, state := before, imported := some accepted}
      let state := {before with
        control := some {kind := 3, revision := receiving.head.revision}
        receiving := none
        receiveDeadline := none}
      let recovery := if input.recoveryObserver && !responseObsolete input.healthRead receiving.head then some false else none
      return {ok := true, state := state, imported := some accepted, recovery := recovery}
    | 3 => Id.run do
      if before.advertised.isNone || frame.revision != before.revision || before.response.isSome then
        return ⟨false, before, none, false, none, none⟩
      return ⟨true, {before with advertised := none, advertisementDeadline := none}, none, false, none, none⟩
    | 10 => Id.run do
      return ⟨false, before, none, false, none, none⟩
    | 2 => Id.run do
      return {ok := true, state := before, operation := true}
    | _ => ⟨false, before, none, false, none, none⟩

/-- Terminal recovery discards later data without any import or operation dispatch. -/
theorem receiveExchange_terminal {before : Exchange} {current : State} {frame : ExchangeFrame}
    {input : ExchangePrimitives} (terminal : before.terminal = true) (dataFrame : frame.kind ≠ 6) :
    receiveExchange before current frame input = ⟨true, before, none, false, none, none⟩ := by
  simp [receiveExchange, terminal, dataFrame]

/-- A queued or in-flight frame cannot be replaced by the pump. -/
theorem prepareOutgoing_occupied {before : Exchange} {current : Option State} {input : ExchangePrimitives}
    (occupied : before.outgoing.isSome = true ∨ before.inFlight.isSome = true) :
    prepareOutgoing before current input = ⟨true, before, none, false, none, none⟩ := by
  rcases occupied with occupied | occupied <;> simp [prepareOutgoing, occupied]

/-- Only an unblocked, nonterminal pinned snapshot dispatch can call host acceptance. -/
theorem receiveExchange_import {before : Exchange} {current : State} {frame : ExchangeFrame}
    {input : ExchangePrimitives} {accepted : AcceptanceResult}
    (imported : (receiveExchange before current frame input).imported = some accepted) :
    before.terminal = false ∧ frame.kind = 1 ∧ before.control.isSome = false ∧
      ∃ receiving, before.receiving = some receiving ∧
        accepted = acceptResponse {input.acceptance with receiving := receiving, snapshot := frame.snapshot} := by
  unfold receiveExchange at imported
  simp only [Id.run, pure] at imported
  repeat' first | split at imported | simp_all

/-- Every host publication through the receiver uses the same complete suffix and atomic host validator. -/
theorem receiveExchange_publication {before : Exchange} {current : State} {frame : ExchangeFrame}
    {input : ExchangePrimitives} {accepted : AcceptanceResult}
    (imported : (receiveExchange before current frame input).imported = some accepted)
    (wrote : accepted.host.wrote = true) :
    ∃ receiving candidate, before.receiving = some receiving ∧ input.acceptance.decoded = some candidate ∧
      importPeerSnapshot input.acceptance.previous candidate (receiving.changes.map (·.entry))
        input.acceptance.localPeer input.acceptance.candidateBytes (historyBytes receiving.changes)
        input.acceptance.lockOK input.acceptance.accessOK input.acceptance.writeOK = some accepted.host := by
  obtain ⟨_, _, _, receiving, retained, same⟩ := receiveExchange_import imported
  rw [same] at wrote ⊢
  obtain ⟨candidate, decoded, publication⟩ := acceptResponse_publication wrote
  exact ⟨receiving, candidate, retained, decoded, publication⟩

/-- The owner processes no host operation when terminal recovery is pending. -/
theorem receiveExchange_no_terminal_effects {before : Exchange} {current : State} {frame : ExchangeFrame}
    {input : ExchangePrimitives} (terminal : before.terminal = true) :
    (receiveExchange before current frame input).state = before ∧
    (receiveExchange before current frame input).imported = none ∧
    (receiveExchange before current frame input).operation = false := by
  by_cases authorization : frame.kind = 6
  · simp [receiveExchange, authorization]
  · simp [receiveExchange_terminal terminal authorization]

/-- Pages retain the original receive deadline and cannot invoke host acceptance or operations. -/
theorem receiveExchange_page_local {before : Exchange} {current : State} {frame : ExchangeFrame}
    {input : ExchangePrimitives} (page : frame.kind = 9) :
    (receiveExchange before current frame input).state.receiveDeadline = before.receiveDeadline ∧
    (receiveExchange before current frame input).imported = none ∧
    (receiveExchange before current frame input).operation = false := by
  cases terminal : before.terminal
  · cases retained : before.receiving with
    | none => simp [receiveExchange, page, terminal, retained]
    | some receiving =>
      cases accepted : (appendPage receiving frame.page).ok <;>
        cases recovery : (appendPage receiving frame.page).recovery <;>
          simp [receiveExchange, page, terminal, retained, accepted, recovery]
  · simp [receiveExchange, page, terminal]

/-- An existing request flag cannot be bypassed with another request under the same advertisement. -/
theorem receiveExchange_repeated_request {before : Exchange} {current : State} {frame : ExchangeFrame}
    {input : ExchangePrimitives} (request : frame.kind = 8) (live : before.terminal = false)
    (requested : before.requested = true) :
    receiveExchange before current frame input = ⟨false, before, none, false, none, none⟩ := by
  cases advertised : before.advertised <;> simp [receiveExchange, request, live, requested, advertised]

/-- A pending control takes the free writer slot while retaining the advertised response. -/
theorem prepareOutgoing_control {before : Exchange} {current : Option State} {input : ExchangePrimitives}
    {control : ExchangeFrame} (queued : before.control = some control)
    (outgoing : before.outgoing = none) (inFlight : before.inFlight = none) :
    prepareOutgoing before current input =
      ⟨true, {before with outgoing := some control, control := none}, none, false, none, none⟩ := by
  simp [prepareOutgoing, queued, outgoing, inFlight]

/-- A healthy complete pinned response reaches publication and acknowledgment through the real dispatch. -/
theorem receiveExchange_complete (before : Exchange) (current : State) (frame : ExchangeFrame)
    (input : ExchangePrimitives) (candidate : State) (snapshot : Snapshot)
    (kind : frame.kind = 1) (live : before.terminal = false) (control : before.control = none)
    (receiving : before.receiving = some input.acceptance.receiving)
    (frameSnapshot : frame.snapshot = input.acceptance.snapshot)
    (present : input.acceptance.snapshot = some snapshot) (decoded : input.acceptance.decoded = some candidate)
    (revision : snapshot.revision = input.acceptance.receiving.head.revision)
    (base : snapshot.base = input.acceptance.receiving.base)
    (cursor : input.acceptance.receiving.cursor = input.acceptance.receiving.head.configHash)
    (digest : input.acceptance.digest = input.acceptance.receiving.head.stateHash)
    (notObsolete : responseObsolete input.acceptance.beforeRead input.acceptance.receiving.head = false)
    (snapshotRoot : snapshot.rootSeqno = input.acceptance.receiving.head.rootSeqno)
    (rootSeq : candidate.root.seqno = snapshot.rootSeqno)
    (configSeq : candidate.config.seqno = input.acceptance.receiving.head.configSeqno)
    (configHash : candidate.config.hash = input.acceptance.receiving.head.configHash)
    (bounded : input.acceptance.candidateBytes ≤ 10 * 1024 * 1024 ∧ input.acceptance.receiving.changes.length ≤ 4096 ∧
      historyBytes input.acceptance.receiving.changes ≤ 8 * 1024 * 1024)
    (chain : verifySuffix input.acceptance.previous.config candidate.config (input.acceptance.receiving.changes.map (·.entry)) = true)
    (readable : readableBy candidate.config input.acceptance.localPeer = true)
    (root : importRoot input.acceptance.previous.root candidate.root candidate.config = some candidate.root)
    (progress : input.acceptance.previous.root.seqno < candidate.root.seqno) (valid : candidate.validate = true)
    (localEmpty : input.acceptance.previous.ops = []) (remoteEmpty : candidate.ops = [])
    (lock : input.acceptance.lockOK = true) (access : input.acceptance.accessOK = true) (write : input.acceptance.writeOK = true) :
    (receiveExchange before current frame input).ok = true ∧
    (receiveExchange before current frame input).state.receiving = none ∧
    (receiveExchange before current frame input).state.control = some {kind := 3, revision := snapshot.revision} ∧
    ∃ accepted, (receiveExchange before current frame input).imported = some accepted ∧
      accepted.host.wrote = true ∧ sameCheckpoint accepted.host.state candidate = true := by
  have accepted := acceptResponse_complete input.acceptance candidate snapshot present decoded revision base cursor digest
    notObsolete snapshotRoot rootSeq configSeq configHash bounded chain readable root progress valid localEmpty remoteEmpty lock access write
  have sameInput : {input.acceptance with receiving := input.acceptance.receiving, snapshot := frame.snapshot} = input.acceptance := by
    simp [frameSnapshot]
  simp [receiveExchange, kind, live, control, receiving, sameInput, accepted, revision]

/-- LoopInput records the two actual held-state reads and one selected event.
    Event codes are oracle tags: 0 cancellation, 1 expiry, 2 state change, 3 handoff,
    4 writer result and 5 reader result. Event readiness is a primitive scheduler contract.
    drainAuthorization is a nonnil authorization encountered by drainDenial before it stops. -/
structure LoopInput where
  current : Option State
  incomingCurrent : Option State
  event : Int
  eventOK : Bool
  drainAuthorization : Option Bool
  preparation : ExchangePrimitives
  reception : ExchangePrimitives
  deriving Repr

/-- LoopResult retains exact synchronous effects, attempted local denial and writer handoff. -/
structure LoopResult where
  exchange : ExchangeResult
  denial : Bool := false
  handed : Option ExchangeFrame := none
  deriving DecidableEq, Repr

/-- authorizedState applies the existing role and history gate, including a nil held state. -/
def authorizedState (current : Option State) (localID remote : String) : Bool :=
  match current with
  | none => false
  | some state => authorizeParticipants state.config.participants state.config.hash localID remote

/-- exchangeDeadline mirrors zero-time handling and the earliest pinned timer.
    A retained receiver whose Go deadline is zero disables the timer even with a live advertisement. -/
def exchangeDeadline (before : Exchange) : Option Int :=
  if before.receiving.isNone then before.advertisementDeadline
  else match before.advertisementDeadline, before.receiveDeadline with
    | none, receive => receive
    | some _, none => none
    | some advertisement, some receive => some (min advertisement receive)

/-- advanceExchange mirrors one production iteration; none denotes an impossible selected event.
    A writer failure drains only explicit authorization and cannot dispatch any drained data. -/
def advanceExchange (before : Exchange) (localID remote : String) (input : LoopInput)
    (frame : ExchangeFrame) : Option LoopResult := do
  if !authorizedState input.current localID remote then
    return {exchange := {ok := false, state := before}, denial := true}
  let prepared := prepareOutgoing before input.current input.preparation
  if !prepared.ok then
    return {exchange := prepared}
  let state := prepared.state
  match input.event with
  | 0 => return {exchange := {ok := false, state := state}}
  | 1 =>
    if (exchangeDeadline state).isNone then none
    else return {exchange := {ok := false, state := state}}
  | 2 => return {exchange := {ok := true, state := state}}
  | 3 =>
    let some outgoing := state.outgoing | none
    if state.inFlight.isSome then none
    else return {exchange := {ok := true, state := {state with outgoing := none, inFlight := some outgoing}}, handed := some outgoing}
  | 4 =>
    if !input.eventOK then
      return {exchange := {ok := false, state := state, admission :=
        if input.drainAuthorization == some false && input.reception.admissionObserver then some false else none}}
    if state.inFlight.any (fun message => message.kind == 10 && message.recoveryPresent) then
      return {exchange := {ok := false, state := state}}
    return {exchange := {ok := true, state := {state with inFlight := none}}}
  | 5 =>
    if !input.eventOK then
      return {exchange := {ok := false, state := state}}
    if !authorizedState input.incomingCurrent localID remote then
      return {exchange := {ok := false, state := state}, denial := true}
    let some current := input.incomingCurrent | none
    return {exchange := receiveExchange state current frame input.reception}
  | _ => none

/-- Preparing output never calls host acceptance. -/
theorem prepareOutgoing_no_import (before : Exchange) (current : Option State) (input : ExchangePrimitives) :
    (prepareOutgoing before current input).imported = none := by
  unfold prepareOutgoing
  simp only [Id.run, pure]
  repeat' first | split | rfl

/-- Only a selected successful reader result under both fresh authority checks can invoke acceptance. -/
theorem advanceExchange_import {before : Exchange} {localID remote : String} {input : LoopInput}
    {frame : ExchangeFrame} {result : LoopResult} {accepted : AcceptanceResult}
    (advanced : advanceExchange before localID remote input frame = some result)
    (imported : result.exchange.imported = some accepted) :
    authorizedState input.current localID remote = true ∧
    authorizedState input.incomingCurrent localID remote = true ∧
    input.event = 5 ∧ input.eventOK = true ∧
    ∃ current, input.incomingCurrent = some current ∧
      result.exchange = receiveExchange (prepareOutgoing before input.current input.preparation).state current frame input.reception := by
  have prepared := prepareOutgoing_no_import before input.current input.preparation
  unfold advanceExchange at advanced
  simp only [pure] at advanced
  repeat' first | split at advanced | subst result | simp_all

/-- A rejected initial authority check has no preparation, import, operation or handoff effects. -/
theorem advanceExchange_rejected {before : Exchange} {localID remote : String} {input : LoopInput}
    {frame : ExchangeFrame} (rejected : authorizedState input.current localID remote = false) :
    advanceExchange before localID remote input frame =
      some {exchange := {ok := false, state := before}, denial := true} := by
  simp [advanceExchange, rejected]

/-- The actual loop dispatch preserves the complete atomic host-publication contract. -/
theorem advanceExchange_publication {before : Exchange} {localID remote : String} {input : LoopInput}
    {frame : ExchangeFrame} {result : LoopResult} {accepted : AcceptanceResult}
    (advanced : advanceExchange before localID remote input frame = some result)
    (imported : result.exchange.imported = some accepted) (wrote : accepted.host.wrote = true) :
    ∃ receiving candidate, (prepareOutgoing before input.current input.preparation).state.receiving = some receiving ∧
      input.reception.acceptance.decoded = some candidate ∧
      importPeerSnapshot input.reception.acceptance.previous candidate (receiving.changes.map (·.entry))
        input.reception.acceptance.localPeer input.reception.acceptance.candidateBytes (historyBytes receiving.changes)
        input.reception.acceptance.lockOK input.reception.acceptance.accessOK input.reception.acceptance.writeOK = some accepted.host := by
  obtain ⟨_, _, _, _, current, _, same⟩ := advanceExchange_import advanced imported
  rw [same] at imported
  exact receiveExchange_publication imported wrote

/-- A valid selected read reaches exactly the existing complete-response dispatch. -/
theorem advanceExchange_receive (before : Exchange) (localID remote : String) (input : LoopInput)
    (frame : ExchangeFrame) (current : State)
    (initial : authorizedState input.current localID remote = true)
    (fresh : authorizedState input.incomingCurrent localID remote = true)
    (read : input.incomingCurrent = some current)
    (prepared : (prepareOutgoing before input.current input.preparation).ok = true)
    (event : input.event = 5) (available : input.eventOK = true) :
    advanceExchange before localID remote input frame =
      some {exchange := receiveExchange (prepareOutgoing before input.current input.preparation).state current frame input.reception} := by
  rw [read] at fresh
  simp [advanceExchange, initial, prepared, event, available, read, fresh]

/-- LoopObservation contains the selected frame alongside its primitive observations. -/
structure LoopObservation where
  input : LoopInput
  frame : ExchangeFrame
  deriving Repr

/-- runExchangeTrace stops permanently at the first failed iteration.
    A finite healthy prefix has not returned from the production loop. -/
def runExchangeTrace (before : Exchange) (localID remote : String) : List LoopObservation → Option LoopResult
  | [] => some {exchange := {ok := true, state := before}}
  | observation :: rest => do
    let next ← advanceExchange before localID remote observation.input observation.frame
    if next.exchange.ok then runExchangeTrace next.exchange.state localID remote rest else some next

/-- A stopped owner cannot be revived by later channel events or a regrant. -/
theorem runExchangeTrace_terminal (before : Exchange) (localID remote : String)
    (consumed suffix : List LoopObservation) (result : LoopResult)
    (ran : runExchangeTrace before localID remote consumed = some result) (stopped : result.exchange.ok = false) :
    runExchangeTrace before localID remote (consumed ++ suffix) = some result := by
  induction consumed generalizing before with
  | nil =>
    simp only [runExchangeTrace, Option.some.injEq] at ran
    subst result
    contradiction
  | cons observation rest ih =>
    simp only [List.cons_append, runExchangeTrace] at ran ⊢
    cases advanced : advanceExchange before localID remote observation.input observation.frame with
    | none => simp [advanced] at ran
    | some next =>
      cases healthy : next.exchange.ok
      · simpa [advanced, healthy] using ran
      · simp [advanced, healthy] at ran ⊢
        exact ih next.exchange.state ran

end Spacewave.SObject.Sync
