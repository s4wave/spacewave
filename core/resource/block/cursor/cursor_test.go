//go:build !goscript

package resource_block_cursor

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/db/testbed"
	s4wave_block_cursor "github.com/s4wave/spacewave/sdk/block/cursor"
	"github.com/sirupsen/logrus"
)

const exampleBlockTypeID = "github.com/s4wave/spacewave/db/block/mock.Example"

func TestUnmarshalWithBlockTypeReusesDecodedCursor(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	addExampleBlockTypeController(t, ctx, tb)

	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "typed cursor"})
	if err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor := block.NewTransaction(store, nil, ref, nil)
	resource := NewBlockCursorResource(le, tb.Bus, tx, cursor)

	opCtx, counter := block.WithReadCounter(ctx)
	resp, err := resource.Unmarshal(opCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, resp.GetData(), "typed cursor")

	resp, err = resource.Unmarshal(opCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, resp.GetData(), "typed cursor")

	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheAttemptCount != 0 ||
		snapshot.DecodedBlockCacheMissCount != 0 ||
		snapshot.DecodedBlockUncacheableCount != 1 {
		t.Fatalf("unexpected typed cursor counters: %+v", snapshot)
	}
}

func addExampleBlockTypeController(t *testing.T, ctx context.Context, tb *testbed.Testbed) {
	t.Helper()
	controller := blocktype_controller.NewController(func(ctx context.Context, typeID string) (blocktype.BlockType, error) {
		if typeID == exampleBlockTypeID {
			return blocktype.NewBlockType(exampleBlockTypeID, func() *block_mock.Example {
				return &block_mock.Example{}
			}), nil
		}
		return nil, nil
	})
	release, err := tb.Bus.AddController(ctx, controller, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(release)
}

func assertExampleResponse(t *testing.T, data []byte, want string) {
	t.Helper()
	example := &block_mock.Example{}
	if err := example.UnmarshalBlock(data); err != nil {
		t.Fatal(err.Error())
	}
	if example.GetMsg() != want {
		t.Fatalf("message = %q, want %q", example.GetMsg(), want)
	}
}
