import Spacewave.SObject.Host

/-!
# SharedObject invitations

Mirrors `core/sobject/invite.go`, including admission against the lock-held
checkpoint. Signature, hash, watch/build success and clock comparison are
external primitive boundaries. The entry's kind, configuration and sequence
are constructed here, not supplied as a prevalidated authority decision.
`Invite.data` retains immutable serialized metadata; uses/revoked are explicit.
Nil records use `data = "nil"`. Uses and limits project Go's uint32 values.
-/

namespace Spacewave.SObject

/-- Invite.bounded is the finite-use accounting invariant; zero means unlimited. -/
abbrev Invite.bounded (inv : Invite) : Prop := inv.maxUses = 0 ∨ inv.uses ≤ inv.maxUses

/-- validateInviteUsable checks revocation, the external clock comparison and capacity. -/
def validateInviteUsable (inv : Invite) (expired : Bool) : Bool :=
  !inv.revoked && !expired && (inv.maxUses == 0 || inv.uses < inv.maxUses)

/-- findInvite returns the first ID match, including Go's nil-pointer result. -/
def findInvite (invs : List Invite) (id : String) : Option Invite := do
  let inv ← invs.find? (·.id == id)
  if inv.data = "nil" then none else some inv

/-- changeInvite performs a revoke or increment on the current record. -/
def changeInvite (kind : ChangeType) (expired : Bool) (inv : Invite) : Option Invite :=
  if kind = ChangeType.revokeInvite then
    if inv.revoked then none else some {inv with revoked := true}
  else if kind = ChangeType.incrementInviteUses then
    if validateInviteUsable inv expired then
      some {inv with uses := (inv.uses + 1) % (2 ^ 32)}
    else none
  else none

/-- editInvite changes exactly the first matching record, failing for nil or absent records. -/
def editInvite (id : String) (kind : ChangeType) (expired : Bool) :
    List Invite → Option (List Invite)
  | [] => none
  | inv :: rest =>
    if inv.id = id then
      if inv.data = "nil" then none
      else (changeInvite kind expired inv).map (· :: rest)
    else (editInvite id kind expired rest).map (inv :: ·)

/-- inviteCallback runs all mutable invitation checks against the held checkpoint. -/
def inviteCallback (kind : ChangeType) (inv : Invite) (id : String) (expired : Bool)
    (s : State) : Option State :=
  if kind = ChangeType.addInvite then
    if (findInvite s.invites inv.id).isSome then none
    else some {s with invites := s.invites ++ [inv]}
  else (editInvite id kind expired s.invites).map fun invs => {s with invites := invs}

/-- inviteArguments checks the public methods' arguments before watching state. -/
def inviteArguments (kind : ChangeType) (inv : Invite) (id : String) : Bool :=
  if kind = ChangeType.addInvite then
    inv.data != "nil" && inv.id != "" && inv.tokenHash != "" && decide inv.bounded
  else (kind == ChangeType.revokeInvite || kind == ChangeType.incrementInviteUses) && id != ""

/-- buildInviteEntry mirrors BuildSOConfigChange for a membership-preserving invite entry. -/
def buildInviteEntry (snapshot : Config) (kind : ChangeType) (sig : Sig) (hash : String) : Entry :=
  ⟨expectedSeqno snapshot % seqnoLimit, some snapshot, some sig, snapshot.hash, kind, hash⟩

/-- mutateInvite separates the watched configuration from the lock-held state. -/
def mutateInvite (previous : State) (snapshot : Config) (kind : ChangeType)
    (inv : Invite) (id : String) (expired : Bool) (sig : Sig) (hash : String)
    (buildOK lockOK writeOK : Bool) : Option HostResult :=
  if inviteArguments kind inv id && buildOK then
    applyConfigChange previous (some (buildInviteEntry snapshot kind sig hash))
      (inviteCallback kind inv id expired) lockOK writeOK
  else none

/-- Successful increments were neither revoked, expired nor already at a finite limit. -/
theorem incrementInvite_admission {inv out : Invite} {expired : Bool}
    (h : changeInvite ChangeType.incrementInviteUses expired inv = some out) :
    inv.revoked = false ∧ expired = false ∧
      (inv.maxUses = 0 ∨ inv.uses < inv.maxUses) ∧
      out = {inv with uses := (inv.uses + 1) % (2 ^ 32)} := by
  simp only [changeInvite, ChangeType.incrementInviteUses, ChangeType.revokeInvite,
    Int.reduceEq, ↓reduceIte] at h
  split at h
  · rename_i usable
    simp only [validateInviteUsable, Bool.and_eq_true, Bool.not_eq_true',
      Bool.or_eq_true, beq_iff_eq, decide_eq_true_eq] at usable
    exact ⟨usable.1.1, usable.1.2, usable.2, Option.some.inj h |>.symm⟩
  · contradiction

/-- A finite accepted increment is exactly one use and stays within its uint32 limit. -/
theorem incrementInvite_bounded {inv out : Invite} {expired : Bool}
    (finite : inv.maxUses ≠ 0) (width : inv.maxUses < 2 ^ 32)
    (h : changeInvite ChangeType.incrementInviteUses expired inv = some out) :
    out.uses = inv.uses + 1 ∧ out.maxUses = inv.maxUses ∧ out.bounded := by
  obtain ⟨_, _, capacity, shape⟩ := incrementInvite_admission h
  have room : inv.uses < inv.maxUses := capacity.resolve_left finite
  have small : inv.uses + 1 < 2 ^ 32 := by omega
  simp [shape, Nat.mod_eq_of_lt small, Invite.bounded]
  omega

/-- Every successful record edit preserves finite accounting from a bounded input. -/
theorem changeInvite_bounded {kind : ChangeType} {expired : Bool} {inv out : Invite}
    (bounded : inv.bounded) (width : inv.maxUses < 2 ^ 32)
    (h : changeInvite kind expired inv = some out) : out.bounded := by
  unfold changeInvite at h
  split at h
  · split at h
    · contradiction
    · cases h; exact bounded
  · split at h
    · split at h
      · rename_i usable
        cases h
        simp only [validateInviteUsable, Bool.and_eq_true, Bool.or_eq_true,
          beq_iff_eq, decide_eq_true_eq] at usable
        rcases usable.2 with unlimited | room
        · exact Or.inl unlimited
        · right
          have small : inv.uses + 1 < 2 ^ 32 := by omega
          simp only [Nat.mod_eq_of_lt small]
          omega
      · contradiction
    · contradiction

/-- Editing the first record preserves bounds on every record in the list. -/
theorem editInvite_bounded {id : String} {kind : ChangeType} {expired : Bool}
    {invs out : List Invite}
    (valid : ∀ inv ∈ invs, inv.bounded ∧ inv.maxUses < 2 ^ 32)
    (h : editInvite id kind expired invs = some out) : ∀ inv ∈ out, inv.bounded := by
  induction invs generalizing out with
  | nil => simp [editInvite] at h
  | cons inv rest ih =>
    simp only [editInvite] at h
    split at h
    · split at h
      · contradiction
      · cases changed : changeInvite kind expired inv with
        | none => simp [changed] at h
        | some next =>
          simp [changed] at h
          subst out
          intro x member
          rcases List.mem_cons.mp member with equal | tail
          · subst x
            exact changeInvite_bounded (valid inv (by simp)).1 (valid inv (by simp)).2 changed
          · exact (valid x (by simp [tail])).1
    · cases edited : editInvite id kind expired rest with
      | none => simp [edited] at h
      | some tail =>
        simp [edited] at h
        subst out
        intro x member
        rcases List.mem_cons.mp member with equal | member
        · subst x; exact (valid inv (by simp)).1
        · exact ih (fun y hy => valid y (by simp [hy])) edited x member

/-- Invitation callbacks preserve the verified configuration, discharging the host contract. -/
theorem inviteCallback_config {kind : ChangeType} {inv : Invite} {id : String}
    {expired : Bool} {s next : State}
    (h : inviteCallback kind inv id expired s = some next) : next.config = s.config := by
  unfold inviteCallback at h
  split at h
  · split at h
    · contradiction
    · cases h; rfl
  · cases edited : editInvite id kind expired s.invites with
    | none => simp [edited] at h
    | some invs => simp [edited] at h; subst next; rfl

/-- Public admission plus a bounded held checkpoint preserves all finite use limits. -/
theorem inviteCallback_bounded {kind : ChangeType} {inv : Invite} {id : String}
    {expired : Bool} {s next : State}
    (args : inviteArguments kind inv id = true)
    (valid : ∀ i ∈ s.invites, i.bounded ∧ i.maxUses < 2 ^ 32)
    (h : inviteCallback kind inv id expired s = some next) :
    ∀ i ∈ next.invites, i.bounded := by
  unfold inviteCallback at h
  split at h
  · rename_i add
    have bound : inv.bounded := by
      simp only [inviteArguments, add, ↓reduceIte, Bool.and_eq_true,
        decide_eq_true_eq] at args
      exact args.2
    split at h
    · contradiction
    · cases h
      intro i member
      rcases List.mem_append.mp member with old | new
      · exact (valid i old).1
      · simpa [List.mem_singleton.mp new] using bound
  · cases edited : editInvite id kind expired s.invites with
    | none => simp [edited] at h
    | some invs =>
      simp [edited] at h
      subst next
      exact editInvite_bounded valid edited

/-- Accepted host mutations preserve finite capacity from a bounded held checkpoint. -/
theorem mutateInvite_bounded {previous : State} {snapshot : Config} {kind : ChangeType}
    {inv : Invite} {id : String} {expired : Bool} {sig : Sig} {hash : String}
    {buildOK lockOK writeOK : Bool} {out : HostResult}
    (valid : ∀ i ∈ previous.invites, i.bounded ∧ i.maxUses < 2 ^ 32)
    (h : mutateInvite previous snapshot kind inv id expired sig hash buildOK lockOK writeOK =
      some out) : ∀ i ∈ out.state.invites, i.bounded := by
  unfold mutateInvite at h
  split at h
  · rename_i admitted
    simp only [Bool.and_eq_true] at admitted
    obtain ⟨_, config, _, _, called, _, _⟩ := applyConfigChange_spec h
    exact inviteCallback_bounded (s := {previous with config}) admitted.1 valid called
  · contradiction

/-- An accepted public mutation is a configuration-authorized change of the held checkpoint. -/
theorem mutateInvite_authorized {previous : State} {snapshot : Config} {kind : ChangeType}
    {inv : Invite} {id : String} {expired : Bool} {sig : Sig} {hash : String}
    {buildOK lockOK writeOK : Bool} {out : HostResult}
    (h : mutateInvite previous snapshot kind inv id expired sig hash buildOK lockOK writeOK =
      some out) :
    verifyChange previous.config (buildInviteEntry snapshot kind sig hash) =
      some out.state.config := by
  unfold mutateInvite at h
  split at h
  · obtain ⟨entry, equal, checked⟩ :=
      applyConfigChange_authorized (fun _ _ => inviteCallback_config) h
    cases equal
    exact checked
  · contradiction

/-- Every invitation mutation requires a valid signature by an OWNER of the held configuration. -/
theorem mutateInvite_owner {previous : State} {snapshot : Config} {kind : ChangeType}
    {inv : Invite} {id : String} {expired : Bool} {sig : Sig} {hash : String}
    {buildOK lockOK writeOK : Bool} {out : HostResult}
    (h : mutateInvite previous snapshot kind inv id expired sig hash buildOK lockOK writeOK =
      some out) : sig.valid = true ∧ isOwner previous.config sig.signer = true := by
  have checked := mutateInvite_authorized h
  have kindNe : kind ≠ ChangeType.selfEnrollPeer := by
    intro equal
    simp [mutateInvite, inviteArguments, equal, ChangeType.selfEnrollPeer,
      ChangeType.addInvite, ChangeType.revokeInvite, ChangeType.incrementInviteUses] at h
  obtain ⟨signer, present, valid, owner⟩ := verifyChange_authorized checked
  simp only [buildInviteEntry, Option.some.injEq] at present
  subst signer
  exact ⟨valid, owner kindNe⟩

end Spacewave.SObject
