import Spacewave.SObject.Host

/-!
# SharedObject paginated catch-up

Mirrors paging, response preparation and acceptance in `core/sobject/sync/catchup.go` at `a7b6337d4`.
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
  deriving DecidableEq, Repr

/-- appendChanges follows continuity, size increment, budget, hash and append order. -/
def appendChanges (before : Receive) (changes : List HistoryChange) : ReceiveResult :=
  match changes with
  | [] => ⟨true, before⟩
  | change :: rest =>
    if change.entry.prev != before.cursor then ⟨false, before⟩
    else
      let sized := {before with bytes := before.bytes + change.bytes}
      if sized.bytes > maxSuffixBytes || before.changes.length == maxSuffixEntries then ⟨false, sized⟩
      else if !change.hashOK then ⟨false, sized⟩
      else appendChanges {sized with cursor := change.entry.hash, changes := before.changes ++ [change]} rest

/-- appendPage checks its pinned revision, current cursor and per-page budget first. -/
def appendPage (before : Receive) (page : Option HistoryPage) : ReceiveResult :=
  let page := page.getD ⟨0, "", [], 0⟩
  if page.revision != before.head.revision || page.cursor != before.cursor || page.changes.isEmpty then ⟨false, before⟩
  else if page.bytes > maxHistoryPageBytes || page.changes.length > maxHistoryPageEntries then ⟨false, before⟩
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
      verifySuffix input.previous.config candidate.config (input.receiving.changes.map (·.entry)) = true := by
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
    simp [importPeerSnapshot, bounded.1, bounded.2.1, bounded.2.2, chain, readable,
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

end Spacewave.SObject.Sync
