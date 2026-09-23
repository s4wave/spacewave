import Spacewave.SObject.ConfigChain

/-!
# Root signatures and consensus

Mirrors `SORoot.ValidateSignatures` and `CheckConsensusAcceptance` in
`core/sobject/crypto.go`, checked against Spacewave `f98a55b07`.
Opaque peer IDs retain equality and ordering. `Sig.valid` is verification over
the exact signed root bytes and context; signature unforgeability remains an
explicit boundary assumption. An empty signer represents a failed key parse.
Participant lookup is first-match, as in Go, even for malformed duplicate lists.
-/

namespace Spacewave.SObject

/-- AccountNonce is one per-peer nonce, retaining order and duplicates. -/
structure AccountNonce where
  peer : String
  nonce : Nat
  deriving DecidableEq, Repr

/-- Operation retains decoded operation identity and its signature. -/
structure Operation where
  peer : String
  localId : String
  nonce : Nat
  parsed : Bool
  innerValid : Bool
  format : Bool
  sig : Sig
  deriving DecidableEq, Repr

/-- Rejections retain Go's group peer separately from the signed submitter. -/
structure Rejections where
  peer : String
  entries : List Operation
  deriving DecidableEq, Repr

/-- validateSignatures returns the count only when every root signature verifies. -/
def validateSignatures (hasInner : Bool) (nonces : List AccountNonce)
    (sigs : List Sig) (ps : List Participant) : Option Nat :=
  if hasInner && decide (nonces.Pairwise fun a b => a.peer ≤ b.peer) &&
      decide (sigs.map (·.signer)).Nodup &&
      sigs.all (fun s => s.signer != "" && s.valid &&
        match ps.find? (·.peer == s.signer) with
        | some p => p.role == Role.validator || p.role == Role.owner
        | none => false) then
    some sigs.length
  else
    none

/-- checkConsensusAcceptance mirrors the only supported consensus mode (zero). -/
def checkConsensusAcceptance (mode : Int) (count : Nat) : Bool :=
  mode == 0 && count > 0

/-- Every counted signature is distinct, verified, and from a configured validator. -/
theorem validateSignatures_authorized {inner : Bool} {ns : List AccountNonce}
    {sigs : List Sig} {ps : List Participant} {count : Nat}
    (h : validateSignatures inner ns sigs ps = some count) :
    count = sigs.length ∧ (sigs.map (·.signer)).Nodup ∧
      ∀ s ∈ sigs, s.valid = true ∧ ∃ p,
        ps.find? (·.peer == s.signer) = some p ∧
          (p.role = Role.validator ∨ p.role = Role.owner) := by
  unfold validateSignatures at h
  split at h
  · rename_i checks
    cases h
    simp only [Bool.and_eq_true, decide_eq_true_eq, List.all_eq_true] at checks
    refine ⟨rfl, checks.1.2, fun s hs => ?_⟩
    obtain ⟨⟨_, hv⟩, hp⟩ := checks.2 s hs
    refine ⟨hv, ?_⟩
    split at hp
    · rename_i p heq
      exact ⟨p, heq, by simpa only [Bool.or_eq_true, beq_iff_eq] using hp⟩
    · contradiction
  · contradiction

/-- Consensus requires at least one counted signature and the supported mode. -/
theorem checkConsensusAcceptance_spec {mode : Int} {count : Nat}
    (h : checkConsensusAcceptance mode count = true) : mode = 0 ∧ 0 < count := by
  simpa [checkConsensusAcceptance] using h

end Spacewave.SObject
