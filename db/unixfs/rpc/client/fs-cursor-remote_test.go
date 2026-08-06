package unixfs_rpc_client

import (
	"context"
	"errors"
	"testing"

	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
)

func TestGetCursorOpsRebindsReleasedCursor(t *testing.T) {
	const (
		cursorHandleID = 7
		opsHandleID    = 9
	)
	nodeType := unixfs_block.NodeType_NodeType_FILE
	client := &FSCursorClient{
		client: &testFSCursorServiceClient{
			opsResponse: &unixfs_rpc.GetCursorOpsResponse{
				OpsHandleId: opsHandleID,
				Name:        "asset.mjs",
				NodeType:    nodeType,
			},
		},
		cursors:   make(map[uint64]*remoteFSCursor),
		ops:       make(map[uint64]*remoteFSCursorOps),
		cursorOps: map[uint64]uint64{cursorHandleID: opsHandleID},
	}
	client.remoteFSCursor = newRemoteFSCursor(client, 1)

	releasedCursor := newRemoteFSCursor(client, cursorHandleID)
	releasedCursor.released.Store(true)
	client.ops[opsHandleID] = newRemoteFSCursorOps(releasedCursor, opsHandleID, nodeType, "asset.mjs")

	currentCursor := newRemoteFSCursor(client, cursorHandleID)
	client.cursors[cursorHandleID] = currentCursor
	ops, err := currentCursor.GetCursorOps(t.Context())
	if err != nil {
		t.Fatal(err.Error())
	}
	if ops.CheckReleased() {
		t.Fatal("current cursor reused operations bound to the released cursor")
	}
	if ops.(*remoteFSCursorOps).c != currentCursor {
		t.Fatal("operations were not rebound to the current cursor")
	}
}

func TestGetCursorOpsRejectsConcurrentRelease(t *testing.T) {
	nodeType := unixfs_block.NodeType_NodeType_FILE
	serviceClient := &testFSCursorServiceClient{
		opsResponse: &unixfs_rpc.GetCursorOpsResponse{
			OpsHandleId: 9,
			Name:        "asset.mjs",
			NodeType:    nodeType,
		},
	}
	client := &FSCursorClient{
		client:    serviceClient,
		cursors:   make(map[uint64]*remoteFSCursor),
		ops:       make(map[uint64]*remoteFSCursorOps),
		cursorOps: make(map[uint64]uint64),
	}
	client.remoteFSCursor = newRemoteFSCursor(client, 1)
	currentCursor := newRemoteFSCursor(client, 7)
	client.cursors[7] = currentCursor
	serviceClient.onGetCursorOps = func() {
		currentCursor.released.Store(true)
	}

	ops, err := currentCursor.GetCursorOps(t.Context())
	if !errors.Is(err, unixfs_errors.ErrReleased) {
		t.Fatalf("expected released cursor error, got %v", err)
	}
	if ops != nil {
		t.Fatal("released cursor returned operations")
	}
}

type testFSCursorServiceClient struct {
	unixfs_rpc.SRPCFSCursorServiceClient
	opsResponse    *unixfs_rpc.GetCursorOpsResponse
	onGetCursorOps func()
}

func (c *testFSCursorServiceClient) GetCursorOps(
	context.Context,
	*unixfs_rpc.GetCursorOpsRequest,
) (*unixfs_rpc.GetCursorOpsResponse, error) {
	if c.onGetCursorOps != nil {
		c.onGetCursorOps()
	}
	return c.opsResponse, nil
}
