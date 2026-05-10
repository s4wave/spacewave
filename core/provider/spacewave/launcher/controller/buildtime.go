package spacewave_launcher_controller

// BuildTimeDistPeerIDs is the list of base58-encoded bifrost peer IDs whose
// signatures on DistConfig packedmsgs are trusted when Config.DistPeerIds is
// empty.
//
// Public project defaults are production-only. Release-owned overlays replace
// Config.DistPeerIds when building artifacts for another release environment.
var BuildTimeDistPeerIDs = []string{
	"12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW",
}
