//go:build prod_signing

package spacewave_launcher_controller

// BuildTimeDistConfigEndpoints is the list of DistConfig fetch URLs embedded
// at build time. Merged with Config.Endpoints at Factory.Construct time.
//
// Prod builds target the prod Worker on spacewave.app, which reads
// spacewave-dist via its APP_DIST binding and cache-flushes on notify.
var BuildTimeDistConfigEndpoints = []string{
	"https://spacewave.app/api/release/config",
}
