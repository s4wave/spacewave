/-!
# SharedObject configuration chain

Models the configuration chain decisions in `core/sobject/config-chain.go` and
`SharedObjectConfig.Validate` in `core/sobject/sobject.go`, checked against
Spacewave `cc4d9007e` plus the owner-retention fix:

- `Config.validate` mirrors `SharedObjectConfig.Validate`.
- `verifySignature` mirrors `verifyConfigChangeSignature`.
- `validateSelfEnroll` mirrors `validateSelfEnrollPeerChange`.
- `verifyChange` mirrors `VerifyConfigChange`.
- `verifySuffix` mirrors `VerifyConfigChainSuffix`.
- `verifyChain` mirrors `VerifyConfigChain`.

Abstraction. Peer IDs, entity IDs and hashes are opaque strings. The Go
projection maps a peer ID that fails `ParsePeerID` to the empty string and
writes each hash as lowercase hex, so a 32-byte digest is 64 characters. An
entry carries its own hash, computed by Go over the entry without its
signature. A signature is its signer and whether it parsed and verified over
the same bytes; unforgeability is the assumption that `valid` holds only for
the named signer's key. Role and change type codes stay integers because
protobuf decoding keeps values outside the enum.
-/

namespace Spacewave.SObject

/-- Role is a `SOParticipantRole` code. -/
abbrev Role := Int

namespace Role

def unknown : Role := 0
def reader : Role := 1
def writer : Role := 2
def validator : Role := 3
def owner : Role := 4

end Role

/-- ChangeType is a `SOConfigChangeType` code. -/
abbrev ChangeType := Int

namespace ChangeType

def genesis : ChangeType := 1
def addParticipant : ChangeType := 2
def removeParticipant : ChangeType := 3
def addInvite : ChangeType := 4
def revokeInvite : ChangeType := 5
def incrementInviteUses : ChangeType := 6
def selfEnrollPeer : ChangeType := 7

/-- peerChanges are the change types a peer may relay in a suffix. -/
def peerChanges : List ChangeType :=
  [addParticipant, removeParticipant, addInvite, revokeInvite, incrementInviteUses]

end ChangeType

/-- maxParticipants mirrors Go `MaxParticipants`. -/
def maxParticipants : Nat := 100

/-- seqnoLimit bounds a `uint64` configuration sequence number. -/
def seqnoLimit : Nat := 2 ^ 64

/-- digestLength is the hex length of a 32-byte SHA-256 digest. -/
def digestLength : Nat := 64

/-- Participant is one `SOParticipantConfig`. -/
structure Participant where
  peer : String
  role : Role
  entity : String
  deriving DecidableEq, Repr

/-- Config is a `SharedObjectConfig` with its applied chain head. -/
structure Config where
  participants : List Participant
  mode : Int
  hash : String
  seqno : Nat
  deriving DecidableEq, Repr

/-- Sig is the verification outcome of a config change signature. -/
structure Sig where
  signer : String
  valid : Bool
  deriving DecidableEq, Repr

/-- Entry is one `SOConfigChange`; `config` is none when Go's field is nil. -/
structure Entry where
  seqno : Nat
  config : Option Config
  sig : Option Sig
  prev : String
  kind : ChangeType
  hash : String
  deriving DecidableEq, Repr

/-- Participant.valid mirrors `SOParticipantConfig.Validate`. -/
def Participant.valid (p : Participant) : Bool :=
  p.peer != "" && Role.reader ≤ p.role && p.role ≤ Role.owner

/-- Config.hasOwner reports whether any participant is an OWNER. -/
def Config.hasOwner (c : Config) : Bool :=
  c.participants.any (·.role == Role.owner)

/--
Config.validate mirrors `SharedObjectConfig.Validate`. An empty participant
list is valid only as a signed terminal head; a nonempty one needs bounded,
valid, unique participants and an OWNER.
-/
def Config.validate (c : Config) : Bool :=
  if c.participants.isEmpty then
    c.hash.length == digestLength
  else
    c.participants.length ≤ maxParticipants &&
      c.participants.all Participant.valid &&
      decide (c.participants.map (·.peer)).Nodup &&
      c.hasOwner

/-- Config.withHead mirrors `configWithAppliedConfigChainHead`. -/
def Config.withHead (c : Config) (seqno : Nat) (hash : String) : Config :=
  { c with seqno, hash }

/--
Config.same mirrors `EqualSOConfigs`: equal metadata and the same participants
in any order. Go sorts by peer ID, which matches a permutation whenever one
side has unique peers, as every validated configuration does.
-/
def Config.same (a b : Config) : Bool :=
  a.mode == b.mode && a.hash == b.hash && a.seqno == b.seqno &&
    a.participants.isPerm b.participants

/-- isOwner mirrors `isOwnerPeer`. -/
def isOwner (c : Config) (peer : String) : Bool :=
  c.participants.any fun p => p.peer == peer && p.role == Role.owner

/-- lookupPeer mirrors a Go map built from participants: the last match wins. -/
def lookupPeer (ps : List Participant) (peer : String) : Option Participant :=
  ps.reverse.find? (·.peer == peer)

/-- entityRole mirrors `participantRoleForEntity`. -/
def entityRole (c : Config) (entity : String) : Role :=
  c.participants.foldl (fun r p => if p.entity == entity && p.role > r then p.role else r)
    Role.unknown

/--
validateSelfEnroll mirrors `validateSelfEnrollPeerChange`: metadata is
unchanged, every existing participant is preserved, and exactly one new
participant is added, the signer, bound to an existing entity with no more
than that entity's strongest role.
-/
def validateSelfEnroll (cur next : Config) (signer : String) : Bool :=
  let added := next.participants.filter fun p => !cur.participants.any (·.peer == p.peer)
  next.mode == cur.mode && next.hash == cur.hash && next.seqno == cur.seqno &&
    next.participants.length == cur.participants.length + 1 &&
    cur.participants.all (fun p => lookupPeer next.participants p.peer ==
      lookupPeer cur.participants p.peer) &&
    cur.participants.all (·.peer != signer) &&
    match added with
    | [a] =>
      a.peer == signer && a.entity != "" && entityRole cur a.entity != Role.unknown &&
        a.role ≤ entityRole cur a.entity
    | _ => false

/--
verifySignature mirrors `verifyConfigChangeSignature`: a verified signature
from an OWNER of `cfg`, or an admissible self-enrollment by its signer.
-/
def verifySignature (e : Entry) (cfg : Config) : Bool :=
  match e.sig with
  | none => false
  | some s =>
    s.valid &&
      if e.kind == ChangeType.selfEnrollPeer then
        match e.config with
        | some next => validateSelfEnroll cfg next s.signer
        | none => false
      else
        isOwner cfg s.signer

/-- expectedSeqno is the sequence number that follows the head of `cur`. -/
def expectedSeqno (cur : Config) : Nat :=
  if cur.hash = "" then 0 else cur.seqno + 1

/--
verifyChange mirrors `VerifyConfigChange`. It accepts an entry linked to the
head of `cur`, at the next sequence number, authorized by `cur`, whose
configuration with the entry as its head is valid, and returns that
configuration.
-/
def verifyChange (cur : Config) (e : Entry) : Option Config :=
  match e.config with
  | none => none
  | some next =>
    let result := next.withHead e.seqno e.hash
    if e.prev = cur.hash ∧ expectedSeqno cur < seqnoLimit ∧ e.seqno = expectedSeqno cur ∧
        verifySignature e cur ∧ result.validate then
      some result
    else
      none

/-- suffixStep is one `VerifyConfigChainSuffix` step: a peer change type, then `verifyChange`. -/
def suffixStep (cur : Config) (e : Entry) : Option Config :=
  if e.kind ∈ ChangeType.peerChanges then verifyChange cur e else none

/-- applySuffix folds `suffixStep` over entries from a checkpoint. -/
def applySuffix (cur : Config) (es : List Entry) : Option Config :=
  es.foldlM suffixStep cur

/--
verifySuffix mirrors `VerifyConfigChainSuffix`: from a nonempty valid
checkpoint, the entries must reach a configuration equal to the candidate.
-/
def verifySuffix (cur cand : Config) (es : List Entry) : Bool :=
  cur.hash != "" && cur.validate &&
    match applySuffix cur es with
    | some c => c.same cand
    | none => false

/-- genesisValid mirrors the genesis checks of `VerifyConfigChain`. -/
def genesisValid (g : Entry) (c : Config) : Bool :=
  g.seqno == 0 && g.prev == "" && g.kind == ChangeType.genesis &&
    !c.participants.isEmpty && c.validate &&
    match g.sig with
    | none => true
    | some _ => verifySignature g c

/--
verifyChain mirrors `VerifyConfigChain`: a valid genesis, then every later
entry verified against the configuration before it. Returns the final
configuration, which Go computes and discards.
-/
def verifyChain : List Entry → Option Config
  | [] => none
  | g :: rest =>
    match g.config with
    | none => none
    | some c => if genesisValid g c then rest.foldlM verifyChange (c.withHead 0 g.hash) else none

/-! ## Configuration validity -/

/-- A valid nonempty configuration has an OWNER. -/
theorem Config.validate_owner {c : Config} (hv : c.validate = true) (hne : c.participants ≠ []) :
    ∃ p ∈ c.participants, p.role = Role.owner := by
  simp [Config.validate, Config.hasOwner, hne] at hv
  exact hv.2

/-- Moving the head of a nonempty configuration does not change its validity. -/
theorem Config.validate_withHead {c : Config} (hne : c.participants ≠ []) (seqno : Nat)
    (hash : String) : (c.withHead seqno hash).validate = c.validate := by
  have hne' : (c.withHead seqno hash).participants ≠ [] := hne
  simp only [Config.validate, List.isEmpty_eq_false_iff.mpr hne,
    List.isEmpty_eq_false_iff.mpr hne']
  rfl

/-- Configuration equality as Go compares it is transitive. -/
theorem Config.same_trans {a b c : Config} (hab : a.same b = true) (hbc : b.same c = true) :
    a.same c = true := by
  simp only [Config.same, Bool.and_eq_true, beq_iff_eq, List.isPerm_iff] at hab hbc ⊢
  obtain ⟨⟨⟨hm, hh⟩, hs⟩, hp⟩ := hab
  obtain ⟨⟨⟨hm', hh'⟩, hs'⟩, hp'⟩ := hbc
  exact ⟨⟨⟨hm.trans hm', hh.trans hh'⟩, hs.trans hs'⟩, hp.trans hp'⟩

/-- OWNER authority does not depend on participant order. -/
theorem isOwner_perm {a b : Config} (hp : a.participants.Perm b.participants) (peer : String) :
    isOwner a peer = isOwner b peer := by
  apply Bool.eq_iff_iff.mpr
  simp only [isOwner, List.any_eq_true]
  exact ⟨fun ⟨p, hm, h⟩ => ⟨p, hp.mem_iff.1 hm, h⟩,
    fun ⟨p, hm, h⟩ => ⟨p, hp.mem_iff.2 hm, h⟩⟩

/-! ## Single changes -/

/--
A verified change is the entry's configuration at the entry's head, linked to the
previous head at the next sequence number, signed with authority, and valid.
-/
theorem verifyChange_spec {cur : Config} {e : Entry} {next : Config}
    (h : verifyChange cur e = some next) :
    ∃ c, e.config = some c ∧ next = c.withHead e.seqno e.hash ∧ e.prev = cur.hash ∧
      expectedSeqno cur < seqnoLimit ∧ e.seqno = expectedSeqno cur ∧
      verifySignature e cur = true ∧ next.validate = true := by
  unfold verifyChange at h
  split at h
  · contradiction
  · rename_i c hc
    dsimp only at h
    split at h
    · rename_i hok
      cases h
      exact ⟨c, hc, rfl, hok⟩
    · contradiction

/--
A verified change carries a valid signature, from an OWNER of the previous
configuration unless it is a self-enrollment.
-/
theorem verifyChange_authorized {cur next : Config} {e : Entry}
    (h : verifyChange cur e = some next) :
    ∃ s, e.sig = some s ∧ s.valid = true ∧
      (e.kind ≠ ChangeType.selfEnrollPeer → isOwner cur s.signer = true) := by
  obtain ⟨_, _, _, _, _, _, hsig, _⟩ := verifyChange_spec h
  unfold verifySignature at hsig
  split at hsig
  · contradiction
  · rename_i s hs
    simp only [Bool.and_eq_true] at hsig
    refine ⟨s, hs, hsig.1, fun hk => ?_⟩
    have hk' : (e.kind == ChangeType.selfEnrollPeer) = false := by simpa using hk
    simpa [hk'] using hsig.2

/-- A verified change leaves an OWNER in every nonempty configuration. -/
theorem verifyChange_owner {cur next : Config} {e : Entry} (h : verifyChange cur e = some next)
    (hne : next.participants ≠ []) : ∃ p ∈ next.participants, p.role = Role.owner := by
  obtain ⟨_, _, _, _, _, _, _, hv⟩ := verifyChange_spec h
  exact Config.validate_owner hv hne

/--
A change at or below the held sequence number is rejected, so a held head
cannot be forked or rolled back.
-/
theorem verifyChange_stale {cur : Config} {e : Entry} (hh : cur.hash ≠ "")
    (hs : e.seqno ≤ cur.seqno) : verifyChange cur e = none := by
  cases h : verifyChange cur e with
  | none => rfl
  | some next =>
    obtain ⟨_, _, _, _, _, hseq, _, _⟩ := verifyChange_spec h
    simp [expectedSeqno, hh] at hseq
    omega

/-! ## Suffix verification -/

/-- A suffix step depends on the configuration only up to Go's equality. -/
theorem suffixStep_congr {a b : Config} (h : a.same b = true) (e : Entry) :
    suffixStep a e = suffixStep b e := by
  simp only [Config.same, Bool.and_eq_true, beq_iff_eq, List.isPerm_iff] at h
  obtain ⟨⟨⟨_, hh⟩, hs⟩, hp⟩ := h
  unfold suffixStep
  split
  · rename_i hk
    have hk' : (e.kind == ChangeType.selfEnrollPeer) = false := by
      simp only [ChangeType.peerChanges, List.mem_cons, List.not_mem_nil] at hk
      simp only [beq_eq_false_iff_ne]
      intro heq
      simp [heq, ChangeType.selfEnrollPeer, ChangeType.addParticipant,
        ChangeType.removeParticipant, ChangeType.addInvite, ChangeType.revokeInvite,
        ChangeType.incrementInviteUses] at hk
    simp [verifyChange, verifySignature, expectedSeqno, hh, hs, hk', isOwner_perm hp]
  · rfl

/-- Folding a suffix in two parts equals folding it at once. -/
theorem applySuffix_append (a : Config) (xs ys : List Entry) :
    applySuffix a (xs ++ ys) = (applySuffix a xs).bind (applySuffix · ys) := by
  simp [applySuffix, List.foldlM_append]

/-- A nonempty suffix gives the same result from Go-equal checkpoints. -/
theorem applySuffix_congr {a b : Config} (h : a.same b = true) {ys : List Entry} (hne : ys ≠ []) :
    applySuffix a ys = applySuffix b ys := by
  cases ys with
  | nil => contradiction
  | cons e ys => simp [applySuffix, List.foldlM_cons, suffixStep_congr h]

/-- An empty suffix accepts only a candidate equal to the checkpoint. -/
theorem verifySuffix_nil {a c : Config} (h : verifySuffix a c [] = true) : a.same c = true := by
  simp [verifySuffix, applySuffix] at h
  exact h.2

/--
Suffix verification composes: a verified path from `a` to `b` and one from `b`
to `c` join into a verified path from `a` to `c`.
-/
theorem verifySuffix_trans {a b c : Config} {xs ys : List Entry}
    (hab : verifySuffix a b xs = true) (hbc : verifySuffix b c ys = true) :
    verifySuffix a c (xs ++ ys) = true := by
  simp only [verifySuffix, Bool.and_eq_true] at hab hbc ⊢
  obtain ⟨hcheck, hab⟩ := hab
  obtain ⟨_, hbc⟩ := hbc
  refine ⟨hcheck, ?_⟩
  split at hab
  · rename_i b' hb'
    rw [applySuffix_append, hb', Option.bind_some]
    cases ys with
    | nil =>
      simp only [applySuffix, List.foldlM_nil] at hbc ⊢
      exact Config.same_trans hab hbc
    | cons e ys =>
      rw [applySuffix_congr hab (List.cons_ne_nil e ys)]
      exact hbc
  · contradiction

/-! ## Full chains -/

/-- Every configuration reached through verified changes from a valid start is valid. -/
theorem foldlM_verifyChange_valid {es : List Entry} :
    ∀ {init c : Config}, init.validate = true → es.foldlM verifyChange init = some c →
      c.validate = true := by
  induction es with
  | nil => intro init c hv h; simp at h; exact h ▸ hv
  | cons e es ih =>
    intro init c _ h
    simp only [List.foldlM_cons] at h
    cases hstep : verifyChange init e with
    | none => simp [hstep] at h
    | some next =>
      simp only [hstep, Option.bind_eq_bind, Option.bind_some] at h
      obtain ⟨_, _, _, _, _, _, _, hv⟩ := verifyChange_spec hstep
      exact ih hv h

/--
Each verified change advances a nonempty head by exactly one, given nonempty
entry hashes, which SHA-256 digests always are.
-/
theorem foldlM_verifyChange_seqno {es : List Entry} :
    ∀ {init c : Config}, init.hash ≠ "" → (∀ e ∈ es, e.hash ≠ "") →
      es.foldlM verifyChange init = some c →
      c.seqno = init.seqno + es.length ∧ c.hash ≠ "" := by
  induction es with
  | nil => intro init c hh _ h; simp at h; subst h; exact ⟨rfl, hh⟩
  | cons e es ih =>
    intro init c hh hes h
    simp only [List.foldlM_cons] at h
    cases hstep : verifyChange init e with
    | none => simp [hstep] at h
    | some next =>
      simp only [hstep, Option.bind_eq_bind, Option.bind_some] at h
      obtain ⟨_, _, hnext, _, _, hseq, _, _⟩ := verifyChange_spec hstep
      have hnh : next.hash ≠ "" := by
        simpa [hnext, Config.withHead] using hes e (List.mem_cons_self ..)
      obtain ⟨hs, hc⟩ := ih hnh (fun x hx => hes x (List.mem_cons_of_mem e hx)) h
      simp [expectedSeqno, hh] at hseq
      simp [hnext, Config.withHead] at hs
      simp only [List.length_cons]
      exact ⟨by omega, hc⟩

/-- A verified chain is a valid genesis followed by verified changes. -/
theorem verifyChain_spec {es : List Entry} {c : Config} (h : verifyChain es = some c) :
    ∃ g rest gc, es = g :: rest ∧ g.config = some gc ∧ genesisValid g gc = true ∧
      rest.foldlM verifyChange (gc.withHead 0 g.hash) = some c := by
  unfold verifyChain at h
  split at h
  · contradiction
  · rename_i g rest
    split at h
    · contradiction
    · rename_i gc hgc
      split at h
      · rename_i hg
        exact ⟨g, rest, gc, rfl, hgc, hg, h⟩
      · contradiction

/-- Every nonempty configuration a verified chain reaches has an OWNER. -/
theorem verifyChain_owner {es : List Entry} {c : Config} (h : verifyChain es = some c)
    (hne : c.participants ≠ []) : ∃ p ∈ c.participants, p.role = Role.owner := by
  obtain ⟨g, _, gc, _, _, hg, hfold⟩ := verifyChain_spec h
  simp only [genesisValid, Bool.and_eq_true, Bool.not_eq_true', List.isEmpty_eq_false_iff] at hg
  obtain ⟨⟨⟨_, hgne⟩, hgv⟩, _⟩ := hg
  have hv := foldlM_verifyChange_valid (by rwa [Config.validate_withHead hgne]) hfold
  exact Config.validate_owner hv hne

/--
A verified chain's head sequence number counts its entries after genesis,
given nonempty entry hashes.
-/
theorem verifyChain_seqno {es : List Entry} {c : Config} (h : verifyChain es = some c)
    (hes : ∀ e ∈ es, e.hash ≠ "") : c.seqno + 1 = es.length := by
  obtain ⟨g, rest, gc, rfl, _, _, hfold⟩ := verifyChain_spec h
  have hgh : (gc.withHead 0 g.hash).hash ≠ "" := hes g (List.mem_cons_self ..)
  obtain ⟨hs, _⟩ := foldlM_verifyChange_seqno hgh
    (fun e he => hes e (List.mem_cons_of_mem g he)) hfold
  simp [Config.withHead] at hs
  simp [hs]

/-! ## Self-enrollment -/

/--
Self-enrollment adds only its signer, with no more than the strongest role its
entity already holds; every other participant was already present.
-/
theorem validateSelfEnroll_added {cur next : Config} {signer : String}
    (h : validateSelfEnroll cur next signer = true) :
    ∃ a ∈ next.participants, a.peer = signer ∧ entityRole cur a.entity ≠ Role.unknown ∧
      a.role ≤ entityRole cur a.entity ∧
      ∀ p ∈ next.participants, p = a ∨ ∃ q ∈ cur.participants, q.peer = p.peer := by
  unfold validateSelfEnroll at h
  simp only [Bool.and_eq_true] at h
  obtain ⟨_, hadd⟩ := h
  split at hadd
  · rename_i a hsplit
    simp only [Bool.and_eq_true, beq_iff_eq, bne_iff_ne, ne_eq, decide_eq_true_eq] at hadd
    obtain ⟨⟨⟨hpeer, _⟩, hrole⟩, hle⟩ := hadd
    have ha : a ∈ next.participants.filter
        (fun p => !cur.participants.any (·.peer == p.peer)) := by
      rw [hsplit]; exact List.mem_singleton_self a
    refine ⟨a, (List.mem_filter.1 ha).1, hpeer, hrole, hle, fun p hp => ?_⟩
    by_cases hin : cur.participants.any (·.peer == p.peer) = true
    · simp only [List.any_eq_true, beq_iff_eq] at hin
      exact Or.inr hin
    · have hpf : p ∈ next.participants.filter
          (fun p => !cur.participants.any (·.peer == p.peer)) :=
        List.mem_filter.2 ⟨hp, by simpa using hin⟩
      rw [hsplit] at hpf
      exact Or.inl (List.mem_singleton.1 hpf)
  · contradiction

end Spacewave.SObject
