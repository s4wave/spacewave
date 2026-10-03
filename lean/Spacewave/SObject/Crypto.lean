import Spacewave.SObject.ConfigChain

/-!
# Grant authority

`Grant.valid` mirrors `SOGrant.Validate` and `SOGrant.ValidateSignature` in
`core/sobject/crypto.go`: an OWNER signs any grant and a reader signs its own.
Encrypted content stays opaque. An empty signer represents a failed key parse.
`Sig.valid` is verification over the exact signed grant bytes and context;
signature unforgeability remains an explicit boundary assumption.
-/

namespace Spacewave.SObject

/-- Grant retains recipient, signature authority, and opaque encrypted content. -/
structure Grant where
  data : String
  peer : String
  format : Bool
  sig : Sig
  deriving DecidableEq, Repr

/-- Grant.valid mirrors SOGrant.Validate and SOGrant.ValidateSignature. -/
def Grant.valid (g : Grant) (ps : List Participant) : Bool :=
  g.format && g.sig.valid && g.sig.signer != "" && ps.any (fun p =>
    p.peer == g.sig.signer && (p.role == Role.owner ||
      (g.sig.signer == g.peer && p.role ∈ [Role.reader, Role.writer, Role.owner])))

end Spacewave.SObject
