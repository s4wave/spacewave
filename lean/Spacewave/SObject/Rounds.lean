/-!
# Group control rounds and locks

Models the voting rules of `stepRound` in `core/sobject/rounds.go`, which
follows Tendermint (Buchman, Kwon and Milosevic, "The latest gossip on BFT
consensus", 2018):

- `Group.quorum` mirrors `HasQuorum`: more than two thirds of the voting
  weight.
- `committed` is a commit `VerifyCommit` accepts: a quorum of precommits for
  one value in one round.
- `Honest` lists the rules an honest voter keeps. It prevotes at most once per
  round, precommits a value only on a polka for it in that round, and, once it
  precommitted a value, prevotes another only on a polka for the other after
  that precommit and before the current round.

The safety theorem `agreement` states that while the faulty voters hold at
most a third of the weight, two commits at one height decide the same value,
in any rounds.

Abstraction. A voter is any type, and a value is a value hash. A decision's
messages are the prevotes and precommits each voter signed; a faulty voter may
sign any of them. Nil votes carry no value and are left out. Rounds are
natural numbers, and the valid round of a proposal is a polka's round.

Correspondence with the Go code:

- `prevote_once` holds because a voter's own messages are its persisted state
  and `stepRound` never sends a second message of one type in one round. The
  state keeps a voter's newest rounds, and a restarted voter resumes at its
  highest round, so it never returns to a round whose messages were trimmed.
- `precommit_polka` is the precommit step: a precommit names the proposal with
  a polka in the current round.
- `lock` is the prevote step. The lock is the voter's highest non-nil
  precommit. A voter prevotes another value only when the proposal's valid
  round names a polka for it and is at least the lock's round. Message
  verification refuses a proposal whose valid round is not before its round.
-/

namespace Spacewave.SObject.Rounds

/-- Group is a config's voters and their voting weights. -/
structure Group (V : Type) where
  voters : List V
  weight : V → Nat

variable {V α : Type}

/-- weightOf sums the weight of the voters in l that satisfy P. -/
def weightOf (w : V → Nat) (l : List V) (P : V → Bool) : Nat :=
  ((l.filter P).map w).sum

/-- sum is the weight of the group's voters that satisfy P. -/
def Group.sum (g : Group V) (P : V → Bool) : Nat :=
  weightOf g.weight g.voters P

/-- total mirrors `TotalVotingWeight`. -/
def Group.total (g : Group V) : Nat :=
  g.sum fun _ => true

/-- quorum mirrors `HasQuorum`: weight × 3 > total × 2. -/
def Group.quorum (g : Group V) (P : V → Bool) : Prop :=
  g.sum P * 3 > g.total * 2

/-- Votes are the prevotes and precommits each voter signed in one decision. -/
structure Votes (V α : Type) where
  prevote : V → Nat → α → Bool
  precommit : V → Nat → α → Bool

/-- polka holds when a quorum prevoted x in round r. -/
def polka (g : Group V) (m : Votes V α) (r : Nat) (x : α) : Prop :=
  g.quorum fun v => m.prevote v r x

/-- committed holds when a quorum precommitted x in round r. -/
def committed (g : Group V) (m : Votes V α) (r : Nat) (x : α) : Prop :=
  g.quorum fun v => m.precommit v r x

/-- Honest lists the rules `stepRound` keeps for one voter. -/
structure Honest (g : Group V) (m : Votes V α) (v : V) : Prop where
  prevote_once : ∀ r x y, m.prevote v r x → m.prevote v r y → x = y
  precommit_polka : ∀ r x, m.precommit v r x → polka g m r x
  lock : ∀ r x s y, m.precommit v r x → r < s → m.prevote v s y → y ≠ x →
    ∃ vr, r ≤ vr ∧ vr < s ∧ polka g m vr y

/-- Two predicates weigh at most the whole list plus their overlap. -/
theorem weightOf_add_le (w : V → Nat) (P Q : V → Bool) :
    ∀ l : List V, weightOf w l P + weightOf w l Q ≤
      weightOf w l (fun _ => true) + weightOf w l (fun v => P v && Q v)
  | [] => by simp [weightOf]
  | a :: l => by
    have ih := weightOf_add_le w P Q l
    simp only [weightOf] at ih ⊢
    cases hp : P a <;> cases hq : Q a <;>
      simp [hp, hq] <;> omega

/-- A predicate implied by another weighs at least as much. -/
theorem weightOf_mono (w : V → Nat) {P Q : V → Bool} :
    ∀ l : List V, (∀ v ∈ l, P v = true → Q v = true) →
      weightOf w l P ≤ weightOf w l Q
  | [], _ => by simp [weightOf]
  | a :: l, h => by
    have ih := weightOf_mono w l fun v hv => h v (List.mem_cons_of_mem a hv)
    have ha := h a (List.mem_cons_self ..)
    simp only [weightOf] at ih ⊢
    cases hp : P a <;> cases hq : Q a <;>
      simp [hp, hq] at ha ⊢ <;> omega

/--
Two quorums share a voter outside F while F holds at most a third of the
weight.
-/
theorem quorum_overlap {g : Group V} {P Q F : V → Bool}
    (hF : g.sum F * 3 ≤ g.total) (hP : g.quorum P) (hQ : g.quorum Q) :
    ∃ v ∈ g.voters, P v = true ∧ Q v = true ∧ F v = false := by
  -- Look for such a voter.
  cases hany : g.voters.any fun v => P v && Q v && !F v with
  | true =>
    obtain ⟨v, hv, h⟩ := List.any_eq_true.mp hany
    simp only [Bool.and_eq_true, Bool.not_eq_true'] at h
    exact ⟨v, hv, h.1.1, h.1.2, h.2⟩
  | false =>
    -- Without one, the overlap lies inside F and weighs too little.
    have sub := weightOf_mono g.weight (P := fun v => P v && Q v) (Q := F) g.voters
      fun v hv h => by
        have := List.any_eq_false.mp hany v hv
        cases hf : F v <;> simp_all
    have add := weightOf_add_le g.weight P Q g.voters
    simp only [Group.quorum, Group.total, Group.sum] at hF hP hQ
    omega

section Safety

variable {g : Group V} {m : Votes V α} {F : V → Bool}

/-- A commit's value won a polka in its round. -/
theorem polka_of_committed (hF : g.sum F * 3 ≤ g.total)
    (hon : ∀ v, F v = false → Honest g m v) {r : Nat} {x : α}
    (hc : committed g m r x) : polka g m r x := by
  obtain ⟨u, _, hu, _, hfu⟩ := quorum_overlap hF hc hc
  exact (hon u hfu).precommit_polka r x hu

/-- Two polkas in one round are for the same value. -/
theorem polka_unique (hF : g.sum F * 3 ≤ g.total)
    (hon : ∀ v, F v = false → Honest g m v) {r : Nat} {x y : α}
    (hx : polka g m r x) (hy : polka g m r y) : x = y := by
  obtain ⟨w, _, hwx, hwy, hfw⟩ := quorum_overlap hF hx hy
  exact (hon w hfw).prevote_once r x y hwx hwy

/--
After a commit of x in round r, every polka of a later round is for x: the
commit's honest voters stay locked on x, and leaving the lock needs a polka for
another value after r, which by induction does not exist.
-/
theorem polka_after_commit (hF : g.sum F * 3 ≤ g.total)
    (hon : ∀ v, F v = false → Honest g m v) {r : Nat} {x : α}
    (hc : committed g m r x) : ∀ s, r < s → ∀ y, polka g m s y → y = x := by
  intro s
  induction s using Nat.strongRecOn with
  | _ s ih =>
    intro hrs y hp
    by_cases hyx : y = x
    · exact hyx
    -- A voter both locked on x and prevoting y left its lock on a polka.
    obtain ⟨v, _, hcv, hpv, hfv⟩ := quorum_overlap hF hc hp
    obtain ⟨vr, hle, hlt, hvr⟩ := (hon v hfv).lock r x s y hcv hrs hpv hyx
    rcases Nat.lt_or_eq_of_le hle with hgt | heq
    · exact ih vr hlt hgt y hvr
    · -- In round r itself, that polka meets the one behind the commit.
      subst heq
      exact polka_unique hF hon hvr (polka_of_committed hF hon hc)

/--
Safety: while the faulty voters hold at most a third of the weight, two
commits of one decision name the same value, in any rounds.
-/
theorem agreement (hF : g.sum F * 3 ≤ g.total)
    (hon : ∀ v, F v = false → Honest g m v) {r r' : Nat} {x y : α}
    (hc : committed g m r x) (hc' : committed g m r' y) : x = y := by
  rcases Nat.lt_trichotomy r r' with h | h | h
  · exact (polka_after_commit hF hon hc r' h y (polka_of_committed hF hon hc')).symm
  · subst h
    exact polka_unique hF hon (polka_of_committed hF hon hc) (polka_of_committed hF hon hc')
  · exact polka_after_commit hF hon hc' r h x (polka_of_committed hF hon hc)

end Safety

end Spacewave.SObject.Rounds
