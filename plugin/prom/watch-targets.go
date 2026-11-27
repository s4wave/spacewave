package prom_plugin

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/world"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
)

// watchTargetsGap is the shortest time between two WatchTargets responses.
const watchTargetsGap = time.Second

// WatchTargets streams every target's meta and status: all of them first, then
// the targets that changed or were removed since the response before.
func (s *queryService) WatchTargets(_ *s4wave_prom.WatchTargetsRequest, strm s4wave_prom.SRPCPromQueryResourceService_WatchTargetsStream) error {
	sent := make(map[string]*s4wave_prom.TargetInfo)
	first := true
	return watchWorld(strm.Context(), s.eng, watchTargetsGap, func(ctx context.Context, ws world.WorldState) error {
		// Read every target.
		resp := &s4wave_prom.WatchTargetsResponse{}
		seen := make(map[string]struct{}, len(sent))
		err := s4wave_prom.WalkTargets(ctx, ws, func(key string, meta *s4wave_prom.TargetMeta, status *s4wave_prom.TargetStatus) error {
			info := &s4wave_prom.TargetInfo{ObjectKey: key, Meta: meta, Status: status}
			seen[key] = struct{}{}
			if !sent[key].EqualVT(info) {
				resp.Changed = append(resp.Changed, info)
				sent[key] = info
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Collect the removed targets.
		for key := range sent {
			if _, ok := seen[key]; !ok {
				resp.Removed = append(resp.Removed, key)
				delete(sent, key)
			}
		}

		// Send the first response even when empty, then only changes.
		if !first && len(resp.GetChanged()) == 0 && len(resp.GetRemoved()) == 0 {
			return nil
		}
		first = false
		return strm.Send(resp)
	})
}
