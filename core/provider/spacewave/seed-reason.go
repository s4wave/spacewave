package provider_spacewave

import "github.com/s4wave/spacewave/core/provider/spacewave/seedreason"

// SeedReason is the typed taxonomy tagging every HTTP request issued by
// SessionClient.
type SeedReason = seedreason.Reason

const (
	// SeedReasonColdSeed tags the first HTTP GET that populates a cold cache on
	// a session.
	SeedReasonColdSeed = seedreason.ColdSeed
	// SeedReasonReconnect tags requests issued after a WS reconnect to re-seed
	// state that may have drifted while the socket was disconnected.
	SeedReasonReconnect = seedreason.Reconnect
	// SeedReasonMutation tags write operations against the cloud.
	SeedReasonMutation = seedreason.Mutation
	// SeedReasonGapRecovery tags recovery fetches triggered by event seqno gaps.
	SeedReasonGapRecovery = seedreason.GapRecovery
	// SeedReasonRejoin tags recovery fetches issued during the self-rejoin
	// sweep.
	SeedReasonRejoin = seedreason.Rejoin
	// SeedReasonConfigChainVerify tags config-chain verifier fetches.
	SeedReasonConfigChainVerify = seedreason.ConfigChainVerify
	// SeedReasonListBootstrap tags initial list seed fetches.
	SeedReasonListBootstrap = seedreason.ListBootstrap
)

// SeedReasonHeader is the HTTP header name that carries the SeedReason.
const SeedReasonHeader = seedreason.Header

// SeedReasons enumerates the full taxonomy.
var SeedReasons = seedreason.Reasons
