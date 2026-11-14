//go:build !js

package resource_world_test

import (
	"testing"

	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TestExecuteWorldBatch checks that a batch commits every entry together, and
// that a refused entry leaves nothing written by the entries before it.
func TestExecuteWorldBatch(t *testing.T) {
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	defer cleanup()
	engineResource := resource_world.NewEngineResource(logrus.NewEntry(logrus.New()), tb.Bus, tb.Engine, nil, nil)
	defer engineResource.Close()
	ws := world.NewEngineWorldState(tb.Engine, true)

	create := func(key string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_CreateObject{
			CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: key, RootRef: &bucket.ObjectRef{}},
		}}
	}
	remove := func(key string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_DeleteObject{
			DeleteObject: &s4wave_world.DeleteObjectRequest{ObjectKey: key},
		}}
	}
	setQuad := func(subject string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_SetGraphQuad{
			SetGraphQuad: &s4wave_world.SetGraphQuadRequest{Quad: &quad.Quad{
				Subject: world.KeyToGraphValue(subject).String(), Predicate: "<type>", Obj: "\"batch\"",
			}},
		}}
	}
	exists := func(key string) bool {
		obj, found, err := ws.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		return found
	}

	// A batch refused at its last entry writes none of the earlier ones.
	resp, err := engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{
		Mutations: []*s4wave_world.WorldMutation{create("refused"), setQuad("refused"), remove("missing")},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetFailedIndex() != 2 || resp.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND || resp.GetSeqno() != 0 {
		t.Fatalf("refused batch: %v", resp)
	}
	if exists("refused") {
		t.Fatal("refused batch created its object")
	}

	// An op the mount cannot resolve refuses the batch as unhandled.
	resp, err = engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{
		Mutations: []*s4wave_world.WorldMutation{create("unhandled"), {Mutation: &s4wave_world.WorldMutation_Op{
			Op: &s4wave_world.ApplyWorldOpRequest{OpTypeId: "test/unknown"},
		}}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetFailedIndex() != 1 || resp.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP {
		t.Fatalf("unhandled batch: %v", resp)
	}
	if exists("unhandled") {
		t.Fatal("unhandled batch created its object")
	}

	// An applied batch commits every entry, including a delete of an object an
	// earlier entry created.
	resp, err = engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{
		Mutations: []*s4wave_world.WorldMutation{create("kept"), create("removed"), setQuad("kept"), remove("removed")},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetSeqno() == 0 || resp.GetErrorCode() != 0 {
		t.Fatalf("applied batch: %v", resp)
	}
	if !exists("kept") || exists("removed") {
		t.Fatal("applied batch did not commit its entries")
	}
	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if seqno != resp.GetSeqno() {
		t.Fatalf("applied batch reported seqno %d, World is at %d", resp.GetSeqno(), seqno)
	}
}
