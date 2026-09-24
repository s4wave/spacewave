import Spacewave.SObject.Host

/-!
# SharedObject paginated catch-up

Mirrors the paging decisions in `core/sobject/sync/catchup.go` at `ac37a7653`.
`appendPage` preserves partial receive-buffer mutations on failure; `nextMessage`
preserves the consumed response prefix even when hashing a later entry fails.
Neither operation has access to held host state.

Entries retain the configuration-chain projection. Serialized entry and complete
frame sizes, and signature-free entry hashes, are primitive observations. The
response's prefix-size table measures each candidate page with its original
revision and cursor. Missing size observations exceed the frame budget. The Go
projection supplies every measured prefix; no admission decision is an input.
Byte counters use naturals under the nonnegative, nonoverflowing Go-int contract.
Hashed entries are nonnil: verified host history supplies sender entries, and a
receiver with a nonempty reachable cursor rejects nil entries before hashing.
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

end Spacewave.SObject.Sync
