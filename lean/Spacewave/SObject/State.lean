import Spacewave.SObject.Crypto

/-!
# SharedObject state

Models the `SOState` fields that peer import and catch-up read or change in
`core/sobject/state.go`. The checkpoint, each key epoch, each operation and
each invitation are opaque encodings: their validation, adoption and merge
belong to Go and enter the host model as one observed merge result.
-/

namespace Spacewave.SObject

/-- State is an `SOState`: a configuration and the opaque merged content. -/
structure State where
  config : Config
  checkpoint : String
  epochs : List String
  ops : List String
  invites : List String
  deriving DecidableEq, Repr

end Spacewave.SObject
