import Spacewave.SObject.Crypto

/-!
# Resolved operation filtering

Mirrors `core/sobject/resolved-ops.go` at Spacewave `f98a55b07`. Raw protobuf
decoding is `Operation.parsed`; decoded-and-validated inner messages are
`Operation.innerValid`. Null and unparseable messages have those flags false.
Account nonce maps retain the maximum; rejection maps use the group's peer.
-/

namespace Spacewave.SObject

/-- nonceMaximum is Go's maximum matching nonce, with zero for a missing peer. -/
def nonceMaximum (peer : String) : List AccountNonce → Nat
  | [] => 0
  | n :: rest => max (if n.peer == peer then n.nonce else 0) (nonceMaximum peer rest)

/-- filterResolvedOperations removes operations resolved by root, batch, or rejection. -/
def filterResolvedOperations (ops : List Operation) (ns : List AccountNonce)
    (accepted : List Operation) (rejected : List Rejections) : List Operation :=
  let committed := ns.filter (·.peer != "") ++
    (accepted.filter (·.peer != "")).map (fun o => AccountNonce.mk o.peer o.nonce)
  ops.filter fun o => !o.parsed ||
    (!(committed.any fun n => n.peer == o.peer && o.nonce ≤ n.nonce) &&
      !(rejected.any fun g => g.peer != "" && g.peer == o.peer &&
        g.entries.any (fun r => r.innerValid && r.nonce == o.nonce)))

/-- Filtering preserves order and never introduces an operation. -/
theorem filterResolvedOperations_sublist (ops : List Operation) (ns : List AccountNonce)
    (accepted : List Operation) (rejected : List Rejections) :
    (filterResolvedOperations ops ns accepted rejected).Sublist ops := by
  exact List.filter_sublist

/-- A decoded operation cannot survive a recorded rejection of its nonce. -/
theorem filterResolvedOperations_rejected {ops : List Operation} {ns : List AccountNonce}
    {accepted : List Operation} {rejected : List Rejections} {o r : Operation} {g : Rejections}
    (hg : g ∈ rejected) (hr : r ∈ g.entries) (peer : g.peer = o.peer)
    (nonempty : g.peer ≠ "") (nonce : r.nonce = o.nonce)
    (decoded : o.parsed = true) (valid : r.innerValid = true) :
    o ∉ filterResolvedOperations ops ns accepted rejected := by
  simp only [filterResolvedOperations, List.mem_filter]
  intro h
  have member : rejected.any (fun group => group.peer != "" && group.peer == o.peer &&
      group.entries.any (fun rejection => rejection.innerValid && rejection.nonce == o.nonce)) = true := by
    simp only [List.any_eq_true, Bool.and_eq_true, bne_iff_ne, beq_iff_eq]
    exact ⟨g, hg, ⟨nonempty, peer⟩, r, hr, valid, nonce⟩
  simp [decoded, member] at h

end Spacewave.SObject
