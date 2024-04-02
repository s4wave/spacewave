package unixfs_rpc_e2e

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	unixfs_e2e "github.com/aperturerobotics/hydra/unixfs/e2e"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
	unixfs_rpc_client "github.com/aperturerobotics/hydra/unixfs/rpc/client"
	unixfs_rpc_server "github.com/aperturerobotics/hydra/unixfs/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/timestamp"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"
)

// TestUnixFsRPC tests the RPC server and client for UnixFS.
func TestUnixFsRPC(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// build the test filesystem
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)
	resRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(resRef)

	// construct the root fs
	writer := unixfs_block_fs.NewFSWriter()
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, writer)
	writer.SetFS(fs)
	writer.SetTimestamp(timestamp.Now())

	// construct the root fs cursor
	var rootFSCursor unixfs.FSCursor = fs

	// build the server
	service := unixfs_rpc_server.NewFSCursorService(rootFSCursor)
	mux := srpc.NewMux()
	if err := unixfs_rpc.SRPCRegisterFSCursorService(mux, service); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	serverOpenConn := srpc.NewServerPipe(server)

	// build the client
	client := srpc.NewClient(serverOpenConn)
	fsClient := unixfs_rpc.NewSRPCFSCursorServiceClient(client)

	// build the client cursor
	clientFsCursor := unixfs_rpc_client.NewFSCursor(ctx, fsClient)

	// access the client cursor
	clientRootRef, err := unixfs.NewFSHandle(clientFsCursor)
	if err != nil {
		t.Fatal(err.Error())
	}

	now := time.Now()

	// make a directory
	testDirPath := "test/dir"
	if err := clientRootRef.MkdirAllPath(ctx, testDirPath, 0o755, now); err != nil {
		t.Fatal(err.Error())
	}

	// make a file
	bfs := unixfs_billy.NewBillyFilesystem(ctx, clientRootRef, "", time.Now())
	filename := "test/dir/test.txt"
	data := []byte("Hello world!\n")
	err = billy_util.WriteFile(bfs, filename, data, 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	// read back the file
	contents, err := billy_util.ReadFile(bfs, filename)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(contents, data) {
		t.Fatalf("contents mismatch: %v != expected %v", contents, data)
	}

	// make another directory
	testDirPath = "unixfs-e2e"
	if err := clientRootRef.MkdirAllPath(ctx, testDirPath, 0o755, now); err != nil {
		t.Fatal(err.Error())
	}
	testDirHandle, testDirHandlePts, err := clientRootRef.LookupPath(ctx, testDirPath)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(testDirHandlePts) != 1 || testDirHandlePts[0] != testDirPath {
		t.FailNow()
	}

	// run the fs tests on the dir
	if err := unixfs_e2e.TestUnixFS(ctx, testDirHandle); err != nil {
		t.Fatal(err.Error())
	}
}
