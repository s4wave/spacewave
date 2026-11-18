//go:build !js

package spacewave_cli

import (
	"errors"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/structpb"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/wrapperspb"
	"github.com/aperturerobotics/starpc/srpc"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

const (
	// coldPackControlServiceID is registered only on the daemon's protected Unix socket.
	coldPackControlServiceID = "spacewave.cli.cold_pack"
	// coldPackSetReadsMethodID toggles cold reads for the current daemon process.
	coldPackSetReadsMethodID = "SetColdPackReads"
	// coldPackStatsMethodID snapshots the configured placement's cold counters.
	coldPackStatsMethodID = "Stats"
)

// coldPackControlHandler administers the native daemon's configured cold tier.
type coldPackControlHandler struct{}

// GetServiceID returns the cold pack control service identifier.
func (coldPackControlHandler) GetServiceID() string {
	return coldPackControlServiceID
}

// GetMethodIDs returns the cold pack admin methods.
func (coldPackControlHandler) GetMethodIDs() []string {
	return []string{coldPackSetReadsMethodID, coldPackStatsMethodID}
}

// InvokeMethod dispatches cold pack controls without restarting the store.
func (coldPackControlHandler) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Ignore methods outside the cold pack service.
	if serviceID != coldPackControlServiceID {
		return false, nil
	}

	// Apply the toggle or snapshot the current counters.
	var response srpc.Message
	switch methodID {
	case coldPackSetReadsMethodID:
		var req wrapperspb.BoolValue
		if err := strm.MsgRecv(&req); err != nil {
			return true, err
		}
		block_store_s3.SetColdPackReads(req.GetValue())
		response = &emptypb.Empty{}
	case coldPackStatsMethodID:
		if err := strm.MsgRecv(&emptypb.Empty{}); err != nil {
			return true, err
		}
		stats, ok := provider_local.GetColdPackStats()
		if !ok {
			return true, errors.New("the configured cold pack tier is not mounted")
		}
		var err error
		response, err = coldPackStatsResponse(stats)
		if err != nil {
			return true, err
		}
	default:
		return false, nil
	}

	// Acknowledge only after the control has taken effect.
	if err := strm.MsgSend(response); err != nil {
		return true, err
	}
	return true, strm.CloseSend()
}

// coldPackStatsResponse gives the admin socket its stable JSON counter names.
func coldPackStatsResponse(stats block_store_s3.ColdPackStats) (*structpb.Struct, error) {
	// The missing-pack IDs are a list even when no object is gone.
	gone := make([]any, len(stats.GonePackIDs))
	for i, id := range stats.GonePackIDs {
		gone[i] = id
	}

	// Use the well-known protobuf struct for the JSON fields.
	return structpb.NewStruct(map[string]any{
		"cold_bloom_probes":              stats.BloomProbes,
		"cold_bloom_hits":                stats.BloomHits,
		"cold_tail_fetches":              stats.TailFetches,
		"cold_false_positives":           stats.FalsePositives,
		"cold_true_hits":                 stats.TrueHits,
		"cold_repeat_extent_reads":       stats.RepeatExtentReads,
		"cold_promoted_keys":             stats.PromotedKeys,
		"cold_promote_dropped":           stats.PromoteDropped,
		"cold_promote_cap_dropped":       stats.PromoteCapDropped,
		"cold_tail_fill_dropped":         stats.TailFillDropped,
		"cold_last_new_key_unix":         stats.LastNewKeyUnix,
		"cold_uncached_existence_misses": stats.UncachedExistenceMisses,
		"cold_ready":                     stats.Ready,
		"cold_loaded_packs":              stats.LoadedPacks,
		"cold_gone_pack_ids":             gone,
	})
}

// _ checks the daemon mux's handler contract.
var _ srpc.Handler = coldPackControlHandler{}
