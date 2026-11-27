//go:build !js

package resource_world

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TestGetEngineInfoReportsNonParticipantWithoutParticipant proves a session
// that is not an accepted participant still receives engine info, so consumers
// deny its mutations instead of failing the mount.
func TestGetEngineInfoReportsNonParticipantWithoutParticipant(t *testing.T) {
	// Mount an engine whose participant lookup rejects the session peer.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	defer tb.Release()
	sessionPeerID := peer.ID("session-peer")
	engine := NewMount(
		logrus.NewEntry(logrus.New()), tb.Bus, nil, WithSessionPeerID(sessionPeerID), WithCallerParticipantLookup(func(context.Context, peer.ID) (*sobject.SOParticipantConfig, error) {
			return nil, sobject.ErrNotParticipant
		})).NewEngineResource(tb.Engine, &s4wave_world.EngineInfo{EngineId: "engine"})
	defer engine.Close()

	// Read the engine info for the non-participant session.
	info, err := engine.GetEngineInfo(ctx, &s4wave_world.GetEngineInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if info.GetSessionPeerId() != sessionPeerID.String() || info.GetCallerParticipant() != nil {
		t.Fatalf("engine info = %+v, want the session peer without a participant", info)
	}
}
