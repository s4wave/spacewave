/-!
# SharedObject operation order and stable point

Models the replay order and the stable point of `SOOperationSet` in
`core/sobject/operation-log.go` and `core/sobject/stable-point.go`:

- `order` mirrors `Order`.
- `latest` and `builtOn` mirror `builtOn`, with `reaches` for `Ancestors`.
- `stablePoint` mirrors `StablePoint`.

The safety theorem `stablePoint_prefix` states that adding operations which
descend from a roster member's latest placed operation never places anything
inside the stable point: the stable point stays a prefix of the new order. A
checkpoint covering the stable point therefore never covers a position a later
operation of a roster member would take.

Abstraction. An operation hash is a natural number. The Go projection maps
each hash to its rank among the hashes of one case in byte order, which keeps
the order of hashes. Peer IDs are opaque strings. The set is a list of
operations with distinct hashes, as the Go set is a map keyed by hash. The
checkpoint is its list of author heads.
-/

namespace Spacewave.SObject.Order

/-- Pos is an `SOOperationPosition`: an operation an operation names. -/
structure Pos where
  peer : String
  nonce : Nat
  hash : Nat
  deriving DecidableEq, Repr

/-- Op is the part of a verified `SOOperationInner` the order reads. -/
structure Op where
  hash : Nat
  peer : String
  nonce : Nat
  parents : List Pos
  prev : Option Pos
  deriving DecidableEq, Repr

/-- links mirrors `links`: the causal parents, then the previous operation. -/
def Op.links (o : Op) : List Pos :=
  o.parents ++ o.prev.toList

/-- covers mirrors `Covers` over the checkpoint's author heads. -/
def covers (cp : List Pos) (l : Pos) : Bool :=
  cp.any fun a => a.peer == l.peer && l.nonce ≤ a.nonce

/-- satisfied holds for a link to a placed operation or a covered position. -/
def satisfied (cp : List Pos) (placed : List Nat) (l : Pos) : Bool :=
  placed.contains l.hash || covers cp l

/-- ready lists the unplaced operations whose links are all satisfied. -/
def ready (ops : List Op) (cp : List Pos) (placed : List Nat) : List Op :=
  ops.filter fun o => !placed.contains o.hash && o.links.all (satisfied cp placed)

/-- lowest returns the operation with the lowest hash. -/
def lowest : List Op → Option Op
  | [] => none
  | o :: rest =>
    match lowest rest with
    | none => some o
    | some m => if o.hash ≤ m.hash then some o else some m

/-- orderFrom places the lowest ready operation until none is ready. -/
def orderFrom (ops : List Op) (cp : List Pos) : Nat → List Nat → List Nat
  | 0, placed => placed
  | n + 1, placed =>
    match lowest (ready ops cp placed) with
    | none => placed
    | some o => orderFrom ops cp n (placed ++ [o.hash])

/--
order mirrors `Order`: Kahn's algorithm placing the lowest ready hash first.
Each step places a distinct operation, so the number of operations bounds it.
-/
def order (ops : List Op) (cp : List Pos) : List Nat :=
  orderFrom ops cp ops.length []

/--
reaches mirrors `Ancestors`: whether a path of at most `n` uncovered links from
`x` reaches a held operation with hash `h`.
-/
def reaches (ops : List Op) (cp : List Pos) : Nat → Op → Nat → Bool
  | 0, _, _ => false
  | n + 1, x, h => x.links.any fun l => !covers cp l &&
    ops.any fun y => y.hash == l.hash && (y.hash == h || reaches ops cp n y h)

/--
latest lists the placed operations of `peer` that no placed operation of
`peer` follows.
-/
def latest (ops : List Op) (placed : List Nat) (peer : String) : List Op :=
  let own := (placed.filterMap fun h => ops.find? (·.hash == h)).filter (·.peer == peer)
  own.filter fun o => !own.any fun x => x.prev.map (·.hash) == some o.hash

/--
builtOn mirrors `builtOn`: whether `h` is each of `peer`'s latest placed
operations or one of its ancestors. It is false when `peer` has none.
-/
def builtOn (ops : List Op) (cp : List Pos) (placed : List Nat) (peer : String) (h : Nat) : Bool :=
  let ls := latest ops placed peer
  !ls.isEmpty && ls.all fun l => l.hash == h || reaches ops cp ops.length l h

/-- stablePoint mirrors `StablePoint`. -/
def stablePoint (ops : List Op) (cp : List Pos) (roster : List String) : List Nat :=
  let o := order ops cp
  if roster.isEmpty then o
  else o.takeWhile fun h => roster.all fun r => builtOn ops cp o r h

/-! ## Descent -/

/-- Names x y: `x` links `y`, which the set holds, through an uncovered link. -/
def Names (ops : List Op) (cp : List Pos) (x y : Op) : Prop :=
  y ∈ ops ∧ ∃ l ∈ x.links, covers cp l = false ∧ l.hash = y.hash

/-- Desc x y: a path of uncovered links leads from `x` to `y`. -/
inductive Desc (ops : List Op) (cp : List Pos) : Op → Op → Prop
  | name {x y : Op} : Names ops cp x y → Desc ops cp x y
  | step {x y z : Op} : Names ops cp x y → Desc ops cp y z → Desc ops cp x z

theorem Desc.trans {ops : List Op} {cp : List Pos} {x y z : Op}
    (h₁ : Desc ops cp x y) (h₂ : Desc ops cp y z) : Desc ops cp x z := by
  induction h₁ with
  | name n => exact .step n h₂
  | step n _ ih => exact .step n (ih h₂)

theorem Desc.mono {ops ops' : List Op} {cp : List Pos} {x y : Op}
    (sub : ∀ o ∈ ops, o ∈ ops') (h : Desc ops cp x y) : Desc ops' cp x y := by
  induction h with
  | name n => exact .name ⟨sub _ n.1, n.2⟩
  | step n _ ih => exact .step ⟨sub _ n.1, n.2⟩ ih

/-- reaches finds only real descendants. -/
theorem reaches_sound {ops : List Op} {cp : List Pos} :
    ∀ {n : Nat} {x : Op} {h : Nat}, reaches ops cp n x h = true →
      ∃ y ∈ ops, y.hash = h ∧ Desc ops cp x y := by
  intro n
  induction n with
  | zero => intro _ _ hr; simp [reaches] at hr
  | succ n ih =>
    intro x h hr
    simp only [reaches, List.any_eq_true, Bool.and_eq_true, Bool.not_eq_true',
      Bool.or_eq_true, beq_iff_eq] at hr
    obtain ⟨l, hl, hc, y, hy, hyl, hyh⟩ := hr
    have n₁ : Names ops cp x y := ⟨hy, l, hl, hc, hyl.symm⟩
    rcases hyh with e | r
    · exact ⟨y, hy, e, .name n₁⟩
    · obtain ⟨z, hz, e, d⟩ := ih r
      exact ⟨z, hz, e, .step n₁ d⟩

/-! ## The order -/

theorem lowest_mem : ∀ {l : List Op} {m : Op}, lowest l = some m → m ∈ l
  | [], _, h => by simp [lowest] at h
  | o :: rest, m, h => by
    simp only [lowest] at h
    split at h
    · cases h; exact List.mem_cons_self
    · rename_i m' hm'
      split at h <;> cases h
      · exact List.mem_cons_self
      · exact List.mem_cons_of_mem _ (lowest_mem hm')

theorem lowest_cons (a : Op) (rest : List Op) : lowest (a :: rest) ≠ none := by
  simp only [lowest]
  split
  · simp
  · split <;> simp

theorem lowest_le : ∀ {l : List Op} {m : Op}, lowest l = some m → ∀ r ∈ l, m.hash ≤ r.hash
  | [], _, h => by simp [lowest] at h
  | o :: rest, m, h => by
    simp only [lowest] at h
    intro r hr
    rcases List.mem_cons.mp hr with e | hr
    · subst e
      split at h
      · cases h; exact Nat.le_refl _
      · split at h <;> cases h
        · exact Nat.le_refl _
        · omega
    · split at h
      · rename_i hn
        cases rest with
        | nil => simp at hr
        | cons a t => exact absurd hn (lowest_cons a t)
      · rename_i m' hm'
        have := lowest_le hm' r hr
        split at h <;> cases h <;> omega

theorem lowest_some : ∀ {l : List Op} {o : Op}, o ∈ l → ∃ m, lowest l = some m
  | [], _, h => by simp at h
  | a :: rest, _, _ => by
    simp only [lowest]
    split
    · exact ⟨a, rfl⟩
    · split
      · exact ⟨a, rfl⟩
      · exact ⟨_, rfl⟩

theorem not_mem_of_contains {l : List Nat} {x : Nat} (h : l.contains x = false) : x ∉ l := by
  simpa using h

theorem mem_takeWhile {p : Nat → Bool} : ∀ {l : List Nat} {x : Nat}, x ∈ l.takeWhile p → p x = true
  | [], _, h => by simp at h
  | a :: rest, x, h => by
    simp only [List.takeWhile_cons] at h
    split at h
    · rename_i ha
      rcases List.mem_cons.mp h with e | h
      · exact e ▸ ha
      · exact mem_takeWhile h
    · simp at h

/-- Placed Q: `orderFrom` places the hashes `Q`, in this order, from nothing. -/
inductive Placed (ops : List Op) (cp : List Pos) : List Nat → Prop
  | nil : Placed ops cp []
  | snoc {placed : List Nat} {m : Op} : Placed ops cp placed →
      lowest (ready ops cp placed) = some m → Placed ops cp (placed ++ [m.hash])

theorem orderFrom_prefix {ops : List Op} {cp : List Pos} :
    ∀ (n : Nat) (placed : List Nat), placed <+: orderFrom ops cp n placed := by
  intro n
  induction n with
  | zero => intro placed; exact List.prefix_refl _
  | succ n ih =>
    intro placed
    simp only [orderFrom]
    split
    · exact List.prefix_refl _
    · exact List.IsPrefix.trans (List.prefix_append _ _) (ih _)

/-- Every prefix of the order between `placed` and the result is placed. -/
theorem placed_of_prefix {ops : List Op} {cp : List Pos} :
    ∀ (n : Nat) {placed r : List Nat}, Placed ops cp placed → placed <+: r →
      r <+: orderFrom ops cp n placed → Placed ops cp r := by
  intro n
  induction n with
  | zero =>
    intro placed r hp h₁ h₂
    simp only [orderFrom] at h₂
    rw [h₂.eq_of_length (Nat.le_antisymm h₂.length_le h₁.length_le)]
    exact hp
  | succ n ih =>
    intro placed r hp h₁ h₂
    simp only [orderFrom] at h₂
    split at h₂
    · rw [h₂.eq_of_length (Nat.le_antisymm h₂.length_le h₁.length_le)]
      exact hp
    · rename_i m hm
      have hq : Placed ops cp (placed ++ [m.hash]) := .snoc hp hm
      by_cases hl : r.length ≤ placed.length
      · rw [(h₁.eq_of_length (Nat.le_antisymm h₁.length_le hl)).symm]
        exact hp
      · have hq' : placed ++ [m.hash] <+: r :=
          List.prefix_of_prefix_length_le ((orderFrom_prefix n _)) h₂ (by simp; omega)
        exact ih hq hq' h₂

/-- Placing every hash of `placed` leaves `n - placed.length` steps of fuel. -/
theorem orderFrom_of_placed {ops : List Op} {cp : List Pos} {placed : List Nat}
    (hp : Placed ops cp placed) : ∀ n, placed.length ≤ n →
      orderFrom ops cp n [] = orderFrom ops cp (n - placed.length) placed := by
  induction hp with
  | nil => intro n _; rfl
  | @snoc placed m _ hm ih =>
    intro n hn
    simp only [List.length_append, List.length_singleton] at hn
    rw [ih n (by omega)]
    have e : n - placed.length = (n - (placed.length + 1)) + 1 := by omega
    rw [e]
    simp only [orderFrom, hm, List.length_append, List.length_singleton]

/-- Placed hashes are distinct hashes of held operations. -/
theorem Placed.nodup {ops : List Op} {cp : List Pos} {placed : List Nat}
    (hp : Placed ops cp placed) :
    placed.Nodup ∧ ∀ h ∈ placed, ∃ o ∈ ops, o.hash = h := by
  induction hp with
  | nil => simp
  | @snoc placed m _ hm ih =>
    have hr := lowest_mem hm
    simp only [ready, List.mem_filter, Bool.and_eq_true, Bool.not_eq_true'] at hr
    refine ⟨List.nodup_append.mpr ⟨ih.1, by simp, ?_⟩, ?_⟩
    · intro a ha b hb e
      simp only [List.mem_singleton] at hb
      subst hb e
      exact not_mem_of_contains hr.2.1 ha
    · intro h hh
      rcases List.mem_append.mp hh with hh | hh
      · exact ih.2 h hh
      · simp at hh; exact ⟨m, hr.1, hh.symm⟩

/-! ## Safety -/

/-- Closed Q: every uncovered link of a placed operation names a placed one. -/
def Closed (ops : List Op) (cp : List Pos) (placed : List Nat) : Prop :=
  ∀ z ∈ ops, z.hash ∈ placed → ∀ y, Names ops cp z y → y.hash ∈ placed

theorem Closed.desc {ops : List Op} {cp : List Pos} {placed : List Nat}
    (hc : Closed ops cp placed) {z w : Op} (hz : z ∈ ops) (hzp : z.hash ∈ placed)
    (d : Desc ops cp z w) : w.hash ∈ placed := by
  induction d with
  | name n => exact hc _ hz hzp _ n
  | step n _ ih => exact ih n.1 (hc _ hz hzp _ n)

/-- A ready operation descends only from placed operations. -/
theorem ready_desc {ops : List Op} {cp : List Pos} {placed : List Nat}
    (hc : Closed ops cp placed) {x y : Op} (hx : x ∈ ready ops cp placed)
    (d : Desc ops cp x y) : y.hash ∈ placed := by
  -- Each uncovered link of a ready operation names a placed operation.
  have first : ∀ {w}, Names ops cp x w → w.hash ∈ placed := by
    intro w ⟨_, l, hl, hcv, hlw⟩
    simp only [ready, List.mem_filter, Bool.and_eq_true, List.all_eq_true] at hx
    have hs := hx.2.2 l hl
    simp only [satisfied, Bool.or_eq_true, hcv, List.contains_iff_mem] at hs
    rw [← hlw]
    simpa using hs
  cases d with
  | name n => exact first n
  | step n d => exact hc.desc n.1 (first n) d

theorem eq_of_hash_eq : ∀ {l : List Op}, (l.map (·.hash)).Nodup →
    ∀ {x y : Op}, x ∈ l → y ∈ l → x.hash = y.hash → x = y
  | [], _, _, _, hx, _, _ => by simp at hx
  | a :: rest, hn, x, y, hx, hy, e => by
    simp only [List.map_cons, List.nodup_cons, List.mem_map] at hn
    rcases List.mem_cons.mp hx with ex | rx <;> rcases List.mem_cons.mp hy with ey | ry
    · rw [ex, ey]
    · exact absurd ⟨y, ry, ex ▸ e.symm⟩ hn.1
    · exact absurd ⟨x, rx, ey ▸ e⟩ hn.1
    · exact eq_of_hash_eq hn.2 rx ry e

theorem ready_mono {S S' : List Op} {cp : List Pos} {placed : List Nat}
    (sub : ∀ o ∈ S, o ∈ S') {o : Op} (h : o ∈ ready S cp placed) : o ∈ ready S' cp placed := by
  simp only [ready, List.mem_filter] at h ⊢
  exact ⟨sub _ h.1, h.2⟩

theorem ready_of_mem {S S' : List Op} {cp : List Pos} {placed : List Nat}
    {o : Op} (h : o ∈ ready S' cp placed) (ho : o ∈ S) : o ∈ ready S cp placed := by
  simp only [ready, List.mem_filter] at h ⊢
  exact ⟨ho, h.2⟩

/--
Adding operations that each descend from every placed operation places the
same operations first, and leaves them closed under uncovered links.
-/
theorem placed_extend {S S' : List Op} {cp : List Pos}
    (sub : ∀ o ∈ S, o ∈ S') (nodup : (S'.map (·.hash)).Nodup) {placed : List Nat}
    (hp : Placed S cp placed)
    (later : ∀ o ∈ S', o ∉ S → ∀ h ∈ placed, ∃ p ∈ S, p.hash = h ∧ Desc S' cp o p) :
    Placed S' cp placed ∧ Closed S' cp placed := by
  induction hp with
  | nil => exact ⟨.nil, fun _ _ h => by simp at h⟩
  | @snoc placed m hp hm ih =>
    obtain ⟨hp', hc⟩ := ih fun o ho hn h hh => later o ho hn h (List.mem_append_left _ hh)
    have hmr := lowest_mem hm
    have hmS : m ∈ S := (List.mem_filter.mp hmr).1
    have hmQ : m.hash ∉ placed := by
      have := (List.mem_filter.mp hmr).2
      simp only [Bool.and_eq_true, Bool.not_eq_true'] at this
      exact not_mem_of_contains this.1
    refine ⟨?_, ?_⟩
    · -- The lowest ready operation of S' has the hash of m.
      obtain ⟨m', hm'⟩ := lowest_some (ready_mono sub hmr)
      have le₁ := lowest_le hm' m (ready_mono sub hmr)
      have hm'r := lowest_mem hm'
      have le₂ : m.hash ≤ m'.hash := by
        by_cases hm'S : m' ∈ S
        · exact lowest_le hm m' (ready_of_mem hm'r hm'S)
        · obtain ⟨p, _, hph, d⟩ := later m' (List.mem_filter.mp hm'r).1 hm'S m.hash (by simp)
          have := ready_desc hc hm'r d
          rw [hph] at this
          exact absurd this hmQ
      have e : m'.hash = m.hash := Nat.le_antisymm le₁ le₂
      rw [← e]
      exact .snoc hp' hm'
    · -- m names only operations placed before it.
      intro z hz hzp y n
      rcases List.mem_append.mp hzp with hzp | hzp
      · exact List.mem_append_left _ (hc z hz hzp y n)
      · simp only [List.mem_singleton] at hzp
        have hzm : z = m := eq_of_hash_eq nodup hz (sub _ hmS) hzp
        subst hzm
        obtain ⟨_, l, hl, hcv, hly⟩ := n
        have hs := (List.all_eq_true.mp (Bool.and_eq_true _ _ ▸ (List.mem_filter.mp hmr).2).2) l hl
        simp only [satisfied, Bool.or_eq_true, hcv, List.contains_iff_mem] at hs
        apply List.mem_append_left
        rw [← hly]
        simpa using hs

/--
A prefix of the order of `S` that every operation added to reach `S'`
descends from stays a prefix of the order of `S'`.
-/
theorem order_prefix {S S' : List Op} {cp : List Pos} {P : List Nat}
    (hP : P <+: order S cp) (sub : ∀ o ∈ S, o ∈ S') (nodup : (S'.map (·.hash)).Nodup)
    (later : ∀ o ∈ S', o ∉ S → ∀ h ∈ P, ∃ p ∈ S, p.hash = h ∧ Desc S' cp o p) :
    P <+: order S' cp := by
  have hp : Placed S cp P := placed_of_prefix _ .nil (List.nil_prefix) hP
  have hp' := (placed_extend sub nodup hp later).1
  -- The added operations leave enough fuel to place P.
  have hlen : P.length ≤ S'.length := by
    obtain ⟨hn, hm⟩ := hp'.nodup
    have := hn.length_le_of_subset (l₂ := S'.map (·.hash)) fun h hh => by
      obtain ⟨o, ho, e⟩ := hm h hh
      exact List.mem_map.mpr ⟨o, ho, e⟩
    simpa using this
  simp only [order]
  rw [orderFrom_of_placed hp' _ hlen]
  exact orderFrom_prefix _ _

/-- Each latest operation is a held operation. -/
theorem mem_of_latest {ops : List Op} {placed : List Nat} {peer : String} {l : Op}
    (h : l ∈ latest ops placed peer) : l ∈ ops := by
  simp only [latest, List.mem_filter, List.mem_filterMap] at h
  obtain ⟨⟨⟨_, _, hf⟩, _⟩, _⟩ := h
  exact List.mem_of_find?_eq_some hf

/--
Safety: when every added operation descends from the latest placed operation
of a roster member, the stable point stays a prefix of the order. Hence no
later operation of a roster member sorts below the stable point.
-/
theorem stablePoint_prefix {S S' : List Op} {cp : List Pos} {roster : List String}
    (sub : ∀ o ∈ S, o ∈ S') (nodup : (S'.map (·.hash)).Nodup)
    (later : ∀ o ∈ S', o ∉ S →
      ∃ r ∈ roster, ∃ l ∈ latest S (order S cp) r, Desc S' cp o l) :
    stablePoint S cp roster <+: order S' cp := by
  -- The stable point is a prefix of the order of S.
  have hP : stablePoint S cp roster <+: order S cp := by
    simp only [stablePoint]
    split
    · exact List.prefix_refl _
    · exact List.takeWhile_prefix _
  apply order_prefix hP sub nodup
  intro o ho hn h hh
  obtain ⟨r, hr, l, hl, d⟩ := later o ho hn
  -- Each stable hash is the latest operation of r or one of its ancestors.
  have hne : roster.isEmpty = false := by
    cases roster with
    | nil => simp at hr
    | cons _ _ => rfl
  simp only [stablePoint, hne, Bool.false_eq_true, ↓reduceIte] at hh
  have hb := List.all_eq_true.mp (mem_takeWhile hh) r hr
  simp only [builtOn, Bool.and_eq_true, List.all_eq_true] at hb
  have hlr := hb.2 l hl
  simp only [Bool.or_eq_true, beq_iff_eq] at hlr
  rcases hlr with e | rch
  · exact ⟨l, mem_of_latest hl, e, d⟩
  · obtain ⟨y, hy, e, dl⟩ := reaches_sound rch
    exact ⟨y, hy, e, d.trans (dl.mono sub)⟩

end Spacewave.SObject.Order
