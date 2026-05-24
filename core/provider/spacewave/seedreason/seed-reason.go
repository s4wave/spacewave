// Package seedreason owns the HTTP seed-reason taxonomy used to classify
// provider cloud requests.
package seedreason

// Reason is the typed taxonomy tagging every HTTP request issued by the
// Spacewave provider. The value travels as Header so cold-mount fan-out is
// classifiable in cloud logs and budget tests.
type Reason string

const (
	// ColdSeed tags the first HTTP GET that populates a cold cache on a
	// session. Subsequent updates for that cache must arrive via WS payloads,
	// not another HTTP GET.
	ColdSeed Reason = "cold-seed"
	// Reconnect tags requests issued after a WS reconnect to re-seed state that
	// may have drifted while the socket was disconnected.
	Reconnect Reason = "reconnect"
	// Mutation tags write operations (POST/DELETE) against the cloud.
	Mutation Reason = "mutation"
	// GapRecovery tags a recovery fetch triggered because an event carried a
	// seqno gap the local cache cannot bridge.
	GapRecovery Reason = "gap-recovery"
	// Rejoin tags recovery-envelope and recovery-entity-keypairs fetches issued
	// during the self-rejoin sweep.
	Rejoin Reason = "rejoin"
	// ConfigChainVerify tags config-chain fetches issued by the SO host verifier
	// when the cached verified head is missing or behind the current state
	// snapshot.
	ConfigChainVerify Reason = "config-chain-verify"
	// ListBootstrap tags initial list seed fetches like /sobject/list and
	// /org/list that populate account-level list caches.
	ListBootstrap Reason = "list-bootstrap"
)

// Header is the HTTP header name that carries the Reason.
const Header = "X-Alpha-Seed-Reason"

// Reasons enumerates the full taxonomy. Tests use this to assert that every
// declared reason is referenced by at least one provider call site.
var Reasons = []Reason{
	ColdSeed,
	Reconnect,
	Mutation,
	GapRecovery,
	Rejoin,
	ConfigChainVerify,
	ListBootstrap,
}
