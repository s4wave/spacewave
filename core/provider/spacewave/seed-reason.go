package provider_spacewave

// SeedReason is the typed taxonomy tagging every HTTP request issued by
// SessionClient. The value travels as X-Alpha-Seed-Reason so cold-mount
// fan-out is classifiable in cloud logs and the budget test.
type SeedReason string

// Seed reason constants. Keep in sync with SeedReasons below so the static
// check in the iteration tests sees every taxonomy value referenced.
const (
	// SeedReasonColdSeed tags the first HTTP GET that populates a cold cache
	// on a session. Subsequent updates for that cache must arrive via WS
	// payloads, not another HTTP GET.
	SeedReasonColdSeed SeedReason = "cold-seed"
	// SeedReasonReconnect tags requests issued after a WS reconnect to
	// re-seed state that may have drifted while the socket was disconnected.
	SeedReasonReconnect SeedReason = "reconnect"
	// SeedReasonMutation tags write operations (POST/DELETE) against the
	// cloud.
	SeedReasonMutation SeedReason = "mutation"
	// SeedReasonGapRecovery tags a recovery fetch triggered because an event
	// carried a seqno gap the local cache cannot bridge.
	SeedReasonGapRecovery SeedReason = "gap-recovery"
	// SeedReasonRejoin tags recovery-envelope and recovery-entity-keypairs
	// fetches issued during the self-rejoin sweep.
	SeedReasonRejoin SeedReason = "rejoin"
	// SeedReasonConfigChainVerify tags config-chain fetches issued by the SO
	// host verifier when the cached verified head is missing or behind the
	// current state snapshot.
	SeedReasonConfigChainVerify SeedReason = "config-chain-verify"
	// SeedReasonListBootstrap tags initial list seed fetches like
	// /sobject/list and /org/list that populate account-level list caches.
	SeedReasonListBootstrap SeedReason = "list-bootstrap"
)

// SeedReasonHeader is the HTTP header name that carries the SeedReason.
const SeedReasonHeader = "X-Alpha-Seed-Reason"

// SeedReasons enumerates the full taxonomy. Tests use this to assert that
// every declared reason is referenced by at least one SessionClient call site.
var SeedReasons = []SeedReason{
	SeedReasonColdSeed,
	SeedReasonReconnect,
	SeedReasonMutation,
	SeedReasonGapRecovery,
	SeedReasonRejoin,
	SeedReasonConfigChainVerify,
	SeedReasonListBootstrap,
}
