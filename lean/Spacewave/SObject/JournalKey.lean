import Spacewave.SObject.ConfigChain

/-!
# Journal mutation identity

Mirrors `core/sobject/journal-key.go` at Spacewave `8ccfce160`.
Strings retain the four semantic identity fields; origin bytes use lowercase
hex. `digest` is the primitive SHA-256 result over Go's length-delimited fields.
Exact-key equality ignores this annotation, just as Go ignores protobuf unknown
fields. Digest collision resistance is an explicit hypothesis wherever needed.
-/

namespace Spacewave.SObject.Journal

/-- Key is an exact participant-scoped mutation identity and its computed digest. -/
structure Key where
  origin : String
  object : String
  peer : String
  localID : String
  digest : String
  deriving DecidableEq, Repr, Inhabited

/-- Key.valid mirrors SOMutationKey.Validate; peer and object strings are opaque. -/
def Key.valid (key : Key) : Bool :=
  key.origin.length == digestLength && key.object != "" && key.peer != "" && key.localID != ""

/-- Key.same compares only the four semantic identity fields. -/
def Key.same (a b : Key) : Bool :=
  a.origin == b.origin && a.object == b.object && a.peer == b.peer && a.localID == b.localID

/-- sameKey preserves nil equality while excluding digest annotations from identity. -/
def sameKey (a b : Option Key) : Bool :=
  match a, b with
  | none, none => true
  | some a, some b => a.same b
  | _, _ => false

/-- Lineage binds the current key to an optional same-scope predecessor. -/
structure Lineage where
  root : Option Key
  supersedes : Option Key
  deriving DecidableEq, Repr, Inhabited

/-- validLineage mirrors root equality, predecessor validity and namespace isolation. -/
def validLineage (key : Option Key) (lineage : Option Lineage) : Bool :=
  match key, lineage with
  | some key, some lineage =>
    key.valid && sameKey lineage.root (some key) &&
      match lineage.supersedes with
      | none => true
      | some previous => previous.valid && !previous.same key &&
        previous.origin == key.origin && previous.object == key.object && previous.peer == key.peer
  | _, _ => false

/-- Every accepted supersession stays within the same origin, object and participant. -/
theorem lineage_namespace {key previous : Key} {root : Option Key}
    (h : validLineage (some key) (some ⟨root, some previous⟩) = true) :
    previous.origin = key.origin ∧ previous.object = key.object ∧ previous.peer = key.peer ∧
      previous.same key = false := by
  simp only [validLineage, Bool.and_eq_true, beq_iff_eq, Bool.not_eq_true'] at h
  exact ⟨h.2.1.1.2, h.2.1.2, h.2.2, h.2.1.1.1.2⟩

end Spacewave.SObject.Journal
