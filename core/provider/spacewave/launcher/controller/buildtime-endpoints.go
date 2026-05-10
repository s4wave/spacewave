package spacewave_launcher_controller

// BuildTimeDistConfigEndpoints is the list of DistConfig fetch URLs embedded
// at build time when Config.Endpoints is empty.
//
// Public project defaults are production-only. Release-owned overlays replace
// Config.Endpoints when building artifacts for another release environment.
var BuildTimeDistConfigEndpoints = []string{
	"https://spacewave.app/api/release/config",
}
