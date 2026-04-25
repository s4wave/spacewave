//go:build !prod_signing

package spacewave_launcher_controller

// BuildTimeDistConfigEndpoints is the list of DistConfig fetch URLs embedded
// at build time. Merged with Config.Endpoints at Factory.Construct time.
//
// Non-prod builds target the staging Worker, which reads
// spacewave-dist-staging via its APP_DIST binding and cache-flushes on
// staging notify.
var BuildTimeDistConfigEndpoints = []string{
	"https://staging.spacewave.app/api/release/config",
}
