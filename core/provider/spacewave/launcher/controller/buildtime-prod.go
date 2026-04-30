//go:build prod_signing

package spacewave_launcher_controller

// BuildTimeDistPeerIDs is the list of base58-encoded bifrost peer IDs whose
// signatures on DistConfig packedmsgs are trusted at build time. Merged with
// Config.DistPeerIds at Factory.Construct time.
//
// Prod builds trust the prod signing key only. The corresponding private key is
// supplied by release tooling and is not stored in this repo.
var BuildTimeDistPeerIDs = []string{
	"12D3KooWRWx2xR5SFj3qznZ1FujY26jUjWiHKKBMzpwDPScdoqaE",
}
