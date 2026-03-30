package forge_lib_v86_bun

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	v86fs "github.com/aperturerobotics/hydra/unixfs/v86fs"
	"github.com/aperturerobotics/starpc/srpc"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/sirupsen/logrus"
)

// sendRecv sends a message and returns the reply, skipping notifications (tag=0).
func sendRecv(t *testing.T, strm v86fs.SRPCV86FsService_RelayV86FsClient, msg *v86fs.V86FsMessage) *v86fs.V86FsMessage {
	t.Helper()
	if err := strm.Send(msg); err != nil {
		t.Fatal(err.Error())
	}
	for {
		reply, err := strm.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}
		if reply.GetTag() == 0 {
			continue // skip notifications
		}
		return reply
	}
}

// TestInitOutputMount tests that initOutputMount creates a writable FSHandle
// backed by a block transaction. Writes persist to the block store and
// data is readable from a fresh BillyFS view on the same handle.
func TestInitOutputMount(t *testing.T) {
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

	handle, err := initOutputMount(ctx, oc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	// Create a file with content.
	data := []byte("hello from output mount")
	err = handle.MknodWithContent(
		ctx,
		"test.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(data)),
		bytes.NewReader(data),
		0o644,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("mknod with content: %v", err)
	}

	// Verify root ref is non-empty after write.
	ref := oc.GetRef()
	if ref.GetRootRef().GetEmpty() {
		t.Fatal("expected non-empty root ref after writes")
	}

	// Read back via BillyFS (established pattern from hydra block FS tests).
	bfs := unixfs_billy.NewBillyFS(ctx, handle, "", time.Now())
	readData, err := billy_util.ReadFile(bfs, "test.txt")
	if err != nil {
		t.Fatalf("read via billy: %v", err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatalf("expected %q, got %q", string(data), string(readData))
	}
}

// TestOutputMountViaSRPC tests the output mount through the v86fs SRPC
// relay, verifying that writes through SRPC persist to the block store.
func TestOutputMountViaSRPC(t *testing.T) {
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

	handle, err := initOutputMount(ctx, oc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	// Set up v86fs server with the output mount.
	srv := v86fs.NewServer(nil)
	srv.AddMount("output", "/output", handle)

	mux := srpc.NewMux()
	if err := v86fs.SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)
	v86fsClient := v86fs.NewSRPCV86FsServiceClient(client)

	strm, err := v86fsClient.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// MOUNT output
	reply := sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag:  1,
		Body: &v86fs.V86FsMessage_MountRequest{MountRequest: &v86fs.V86FsMountRequest{Name: "output"}},
	})
	mountReply := reply.GetMountReply()
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("mount failed: %v", reply.GetBody())
	}
	rootID := mountReply.GetRootInodeId()

	// CREATE test.txt
	reply = sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag: 2,
		Body: &v86fs.V86FsMessage_CreateRequest{CreateRequest: &v86fs.V86FsCreateRequest{
			ParentId: rootID,
			Name:     "test.txt",
			Mode:     0o100644, // S_IFREG | 0644
		}},
	})
	createReply := reply.GetCreateReply()
	if createReply == nil || createReply.GetStatus() != 0 {
		t.Fatalf("create failed: %v", reply.GetBody())
	}
	fileID := createReply.GetInodeId()

	// WRITE data
	fileData := []byte("hello\n")
	reply = sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag: 3,
		Body: &v86fs.V86FsMessage_WriteRequest{WriteRequest: &v86fs.V86FsWriteRequest{
			InodeId: fileID,
			Offset:  0,
			Data:    fileData,
		}},
	})
	writeReply := reply.GetWriteReply()
	if writeReply == nil || writeReply.GetStatus() != 0 {
		t.Fatalf("write failed: %v", reply.GetBody())
	}
	if writeReply.GetBytesWritten() != uint32(len(fileData)) {
		t.Fatalf("expected %d bytes written, got %d", len(fileData), writeReply.GetBytesWritten())
	}

	// Verify root ref is non-empty (data persisted to blocks).
	ref := oc.GetRef()
	if ref.GetRootRef().GetEmpty() {
		t.Fatal("expected non-empty root ref after SRPC writes")
	}

	// Verify file content via BillyFS view on the handle.
	bfs := unixfs_billy.NewBillyFS(ctx, handle, "", time.Now())
	readData, err := billy_util.ReadFile(bfs, "test.txt")
	if err != nil {
		t.Fatalf("read via billy after SRPC write: %v", err)
	}
	if !bytes.Equal(readData, fileData) {
		t.Fatalf("expected %q, got %q", string(fileData), string(readData))
	}
}

// seedUnixFSTree creates a UnixFS tree with a test file and returns its BlockRef.
// This simulates a forge input providing a UnixFS tree for mounting.
func seedUnixFSTree(t *testing.T, ctx context.Context, cs *bucket_lookup.Cursor, filename string, content []byte) *block.BlockRef {
	t.Helper()

	// Create writable handle to build the tree.
	handle, err := initOutputMount(ctx, cs)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	// Write a file into the tree.
	err = handle.MknodWithContent(
		ctx,
		filename,
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		bytes.NewReader(content),
		0o644,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("seed file: %v", err)
	}

	ref := cs.GetRef().GetRootRef()
	if ref.GetEmpty() {
		t.Fatal("expected non-empty ref after seeding tree")
	}
	return ref
}

// TestInputMountReadOnly tests that input mounts create read-only FSHandles
// backed by existing UnixFS trees. Files seeded into the tree are readable
// through the mount.
func TestInputMountReadOnly(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a populated UnixFS tree (simulates forge input).
	inputCs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	inputContent := []byte("toolchain-file-content\n")
	inputRef := seedUnixFSTree(t, ctx, inputCs, "gcc", inputContent)

	// Create a fresh cursor for mounting (simulates AccessStorage cursor).
	mountCs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	mountCs.SetRootRef(inputRef)

	// Create read-only FS from the input ref (same pattern as resolveInputMount).
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, mountCs, nil)
	defer fs.Release()

	inputHandle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer inputHandle.Release()

	// Verify file is readable via BillyFS.
	bfs := unixfs_billy.NewBillyFS(ctx, inputHandle, "", time.Now())
	readData, err := billy_util.ReadFile(bfs, "gcc")
	if err != nil {
		t.Fatalf("read input file: %v", err)
	}
	if !bytes.Equal(readData, inputContent) {
		t.Fatalf("expected %q, got %q", string(inputContent), string(readData))
	}
}

// TestInputMountViaSRPC tests an input mount served through the v86fs SRPC
// relay. The guest can read files from a pre-populated UnixFS tree.
func TestInputMountViaSRPC(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Seed an input tree.
	inputCs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	inputContent := []byte("input-data-from-previous-stage\n")
	inputRef := seedUnixFSTree(t, ctx, inputCs, "result.txt", inputContent)

	// Create mount cursor at input ref.
	mountCs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	mountCs.SetRootRef(inputRef)

	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, mountCs, nil)
	defer fs.Release()
	inputHandle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer inputHandle.Release()

	// Set up v86fs server with the input mount.
	srv := v86fs.NewServer(nil)
	srv.AddMount("toolchain", "/opt/toolchain", inputHandle)

	mux := srpc.NewMux()
	if err := v86fs.SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)
	v86fsClient := v86fs.NewSRPCV86FsServiceClient(client)

	strm, err := v86fsClient.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// MOUNT toolchain
	reply := sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag:  1,
		Body: &v86fs.V86FsMessage_MountRequest{MountRequest: &v86fs.V86FsMountRequest{Name: "toolchain"}},
	})
	mountReply := reply.GetMountReply()
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("mount failed: %v", reply.GetBody())
	}
	rootID := mountReply.GetRootInodeId()

	// LOOKUP result.txt
	reply = sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag: 2,
		Body: &v86fs.V86FsMessage_LookupRequest{LookupRequest: &v86fs.V86FsLookupRequest{
			ParentId: rootID,
			Name:     "result.txt",
		}},
	})
	lookupReply := reply.GetLookupReply()
	if lookupReply == nil || lookupReply.GetStatus() != 0 {
		t.Fatalf("lookup failed: %v", reply.GetBody())
	}
	fileInodeID := lookupReply.GetInodeId()
	if lookupReply.GetSize() != uint64(len(inputContent)) {
		t.Fatalf("expected size %d, got %d", len(inputContent), lookupReply.GetSize())
	}

	// OPEN result.txt
	reply = sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag: 3,
		Body: &v86fs.V86FsMessage_OpenRequest{OpenRequest: &v86fs.V86FsOpenRequest{
			InodeId: fileInodeID,
			Flags:   0,
		}},
	})
	openReply := reply.GetOpenReply()
	if openReply == nil || openReply.GetStatus() != 0 {
		t.Fatalf("open failed: %v", reply.GetBody())
	}
	handleID := openReply.GetHandleId()

	// READ result.txt
	reply = sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag: 4,
		Body: &v86fs.V86FsMessage_ReadRequest{ReadRequest: &v86fs.V86FsReadRequest{
			HandleId: handleID,
			Offset:   0,
			Size:     1024,
		}},
	})
	readReply := reply.GetReadReply()
	if readReply == nil || readReply.GetStatus() != 0 {
		t.Fatalf("read failed: %v", reply.GetBody())
	}
	if !bytes.Equal(readReply.GetData(), inputContent) {
		t.Fatalf("expected %q, got %q", string(inputContent), string(readReply.GetData()))
	}
}

// TestOutputInputChain tests the two-stage pipeline: task A writes files
// to an output mount, task B mounts that output as a read-only input and
// reads the files back. Proves the core forge I/O model works end-to-end.
func TestOutputInputChain(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// --- Stage A: write output ---
	stageACs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	outputHandle, err := initOutputMount(ctx, stageACs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write two files to simulate a build producing artifacts.
	for _, f := range []struct {
		name string
		data []byte
	}{
		{"result.txt", []byte("build output from stage A\n")},
		{"meta.json", []byte(`{"status":"ok","stage":"A"}` + "\n")},
	} {
		err = outputHandle.MknodWithContent(
			ctx,
			f.name,
			unixfs.NewFSCursorNodeType_File(),
			int64(len(f.data)),
			bytes.NewReader(f.data),
			0o644,
			time.Now(),
		)
		if err != nil {
			t.Fatalf("stage A write %s: %v", f.name, err)
		}
	}
	outputHandle.Release()

	// Extract output BlockRef (same as controller.Execute does after VM exits).
	outputRef := stageACs.GetRef().GetRootRef()
	if outputRef.GetEmpty() {
		t.Fatal("stage A produced empty root ref")
	}

	// --- Stage B: mount stage A output as read-only input ---
	stageBCs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	stageBCs.SetRootRef(outputRef)

	inputFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, stageBCs, nil)
	defer inputFS.Release()
	inputHandle, err := unixfs.NewFSHandle(inputFS)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer inputHandle.Release()

	// Read both files via BillyFS.
	bfs := unixfs_billy.NewBillyFS(ctx, inputHandle, "", time.Now())
	for _, f := range []struct {
		name string
		data []byte
	}{
		{"result.txt", []byte("build output from stage A\n")},
		{"meta.json", []byte(`{"status":"ok","stage":"A"}` + "\n")},
	} {
		got, err := billy_util.ReadFile(bfs, f.name)
		if err != nil {
			t.Fatalf("stage B read %s: %v", f.name, err)
		}
		if !bytes.Equal(got, f.data) {
			t.Fatalf("stage B %s: expected %q, got %q", f.name, string(f.data), string(got))
		}
	}
}

// TestOutputInputChainViaSRPC tests the two-stage pipeline over the v86fs
// SRPC protocol. Stage A writes files through SRPC (simulating VM guest
// writes). Stage B reads those files through SRPC (simulating VM guest
// reads from an input mount). This is the full data path a real v86 task
// chain would exercise.
func TestOutputInputChainViaSRPC(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// --- Stage A: write output via SRPC ---
	stageACs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	outputHandle, err := initOutputMount(ctx, stageACs)
	if err != nil {
		t.Fatal(err.Error())
	}

	srvA := v86fs.NewServer(nil)
	srvA.AddMount("output", "/output", outputHandle)

	muxA := srpc.NewMux()
	if err := v86fs.SRPCRegisterV86FsService(muxA, srvA); err != nil {
		t.Fatal(err.Error())
	}
	serverA := srpc.NewServer(muxA)
	pipeA := srpc.NewServerPipe(serverA)
	clientA := srpc.NewClient(pipeA)
	v86fsClientA := v86fs.NewSRPCV86FsServiceClient(clientA)

	strmA, err := v86fsClientA.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// MOUNT output
	reply := sendRecv(t, strmA, &v86fs.V86FsMessage{
		Tag:  1,
		Body: &v86fs.V86FsMessage_MountRequest{MountRequest: &v86fs.V86FsMountRequest{Name: "output"}},
	})
	mountReply := reply.GetMountReply()
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("stage A mount failed: %v", reply.GetBody())
	}
	rootID := mountReply.GetRootInodeId()

	// CREATE + WRITE hello.txt
	reply = sendRecv(t, strmA, &v86fs.V86FsMessage{
		Tag: 2,
		Body: &v86fs.V86FsMessage_CreateRequest{CreateRequest: &v86fs.V86FsCreateRequest{
			ParentId: rootID,
			Name:     "hello.txt",
			Mode:     0o100644,
		}},
	})
	createReply := reply.GetCreateReply()
	if createReply == nil || createReply.GetStatus() != 0 {
		t.Fatalf("stage A create failed: %v", reply.GetBody())
	}

	helloData := []byte("hello from stage A\n")
	reply = sendRecv(t, strmA, &v86fs.V86FsMessage{
		Tag: 3,
		Body: &v86fs.V86FsMessage_WriteRequest{WriteRequest: &v86fs.V86FsWriteRequest{
			InodeId: createReply.GetInodeId(),
			Offset:  0,
			Data:    helloData,
		}},
	})
	writeReply := reply.GetWriteReply()
	if writeReply == nil || writeReply.GetStatus() != 0 {
		t.Fatalf("stage A write failed: %v", reply.GetBody())
	}

	strmA.Close()
	outputHandle.Release()

	// Extract the output BlockRef.
	outputRef := stageACs.GetRef().GetRootRef()
	if outputRef.GetEmpty() {
		t.Fatal("stage A produced empty root ref")
	}
	t.Logf("stage A output ref: %v", outputRef.GetHash())

	// --- Stage B: read stage A output as input mount via SRPC ---
	stageBCs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	stageBCs.SetRootRef(outputRef)

	inputFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, stageBCs, nil)
	inputHandle, err := unixfs.NewFSHandle(inputFS)
	if err != nil {
		inputFS.Release()
		t.Fatal(err.Error())
	}

	// Also create an output mount for stage B to write derived output.
	stageBOutCs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	stageBOutHandle, err := initOutputMount(ctx, stageBOutCs)
	if err != nil {
		t.Fatal(err.Error())
	}

	srvB := v86fs.NewServer(nil)
	srvB.AddMount("prev", "/input/prev", inputHandle)
	srvB.AddMount("output", "/output", stageBOutHandle)

	muxB := srpc.NewMux()
	if err := v86fs.SRPCRegisterV86FsService(muxB, srvB); err != nil {
		t.Fatal(err.Error())
	}
	serverB := srpc.NewServer(muxB)
	pipeB := srpc.NewServerPipe(serverB)
	clientB := srpc.NewClient(pipeB)
	v86fsClientB := v86fs.NewSRPCV86FsServiceClient(clientB)

	strmB, err := v86fsClientB.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strmB.Close()

	// MOUNT prev (input from stage A)
	reply = sendRecv(t, strmB, &v86fs.V86FsMessage{
		Tag:  1,
		Body: &v86fs.V86FsMessage_MountRequest{MountRequest: &v86fs.V86FsMountRequest{Name: "prev"}},
	})
	mountReply = reply.GetMountReply()
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("stage B mount prev failed: %v", reply.GetBody())
	}
	prevRootID := mountReply.GetRootInodeId()

	// LOOKUP hello.txt from stage A's output
	reply = sendRecv(t, strmB, &v86fs.V86FsMessage{
		Tag: 2,
		Body: &v86fs.V86FsMessage_LookupRequest{LookupRequest: &v86fs.V86FsLookupRequest{
			ParentId: prevRootID,
			Name:     "hello.txt",
		}},
	})
	lookupReply := reply.GetLookupReply()
	if lookupReply == nil || lookupReply.GetStatus() != 0 {
		t.Fatalf("stage B lookup hello.txt failed: %v", reply.GetBody())
	}
	if lookupReply.GetSize() != uint64(len(helloData)) {
		t.Fatalf("stage B hello.txt size: expected %d, got %d", len(helloData), lookupReply.GetSize())
	}

	// OPEN + READ hello.txt
	reply = sendRecv(t, strmB, &v86fs.V86FsMessage{
		Tag: 3,
		Body: &v86fs.V86FsMessage_OpenRequest{OpenRequest: &v86fs.V86FsOpenRequest{
			InodeId: lookupReply.GetInodeId(),
			Flags:   0,
		}},
	})
	openReply := reply.GetOpenReply()
	if openReply == nil || openReply.GetStatus() != 0 {
		t.Fatalf("stage B open failed: %v", reply.GetBody())
	}

	reply = sendRecv(t, strmB, &v86fs.V86FsMessage{
		Tag: 4,
		Body: &v86fs.V86FsMessage_ReadRequest{ReadRequest: &v86fs.V86FsReadRequest{
			HandleId: openReply.GetHandleId(),
			Offset:   0,
			Size:     1024,
		}},
	})
	readReply := reply.GetReadReply()
	if readReply == nil || readReply.GetStatus() != 0 {
		t.Fatalf("stage B read failed: %v", reply.GetBody())
	}
	if !bytes.Equal(readReply.GetData(), helloData) {
		t.Fatalf("stage B read: expected %q, got %q", string(helloData), string(readReply.GetData()))
	}

	// MOUNT output (stage B writes derived output)
	reply = sendRecv(t, strmB, &v86fs.V86FsMessage{
		Tag:  5,
		Body: &v86fs.V86FsMessage_MountRequest{MountRequest: &v86fs.V86FsMountRequest{Name: "output"}},
	})
	mountReply = reply.GetMountReply()
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("stage B mount output failed: %v", reply.GetBody())
	}
	outRootID := mountReply.GetRootInodeId()

	// CREATE + WRITE derived.txt (appends to stage A data)
	reply = sendRecv(t, strmB, &v86fs.V86FsMessage{
		Tag: 6,
		Body: &v86fs.V86FsMessage_CreateRequest{CreateRequest: &v86fs.V86FsCreateRequest{
			ParentId: outRootID,
			Name:     "derived.txt",
			Mode:     0o100644,
		}},
	})
	createReply = reply.GetCreateReply()
	if createReply == nil || createReply.GetStatus() != 0 {
		t.Fatalf("stage B create derived.txt failed: %v", reply.GetBody())
	}

	derivedData := append(readReply.GetData(), []byte("processed by stage B\n")...)
	reply = sendRecv(t, strmB, &v86fs.V86FsMessage{
		Tag: 7,
		Body: &v86fs.V86FsMessage_WriteRequest{WriteRequest: &v86fs.V86FsWriteRequest{
			InodeId: createReply.GetInodeId(),
			Offset:  0,
			Data:    derivedData,
		}},
	})
	writeReply = reply.GetWriteReply()
	if writeReply == nil || writeReply.GetStatus() != 0 {
		t.Fatalf("stage B write derived.txt failed: %v", reply.GetBody())
	}

	strmB.Close()
	inputHandle.Release()
	inputFS.Release()
	stageBOutHandle.Release()

	// Verify stage B output ref.
	stageBOutRef := stageBOutCs.GetRef().GetRootRef()
	if stageBOutRef.GetEmpty() {
		t.Fatal("stage B produced empty output ref")
	}
	t.Logf("stage B output ref: %v", stageBOutRef.GetHash())

	// Verify stage B output content from a fresh read-only mount.
	verifyCs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	verifyCs.SetRootRef(stageBOutRef)

	verifyFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, verifyCs, nil)
	defer verifyFS.Release()
	verifyHandle, err := unixfs.NewFSHandle(verifyFS)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer verifyHandle.Release()

	bfs := unixfs_billy.NewBillyFS(ctx, verifyHandle, "", time.Now())
	got, err := billy_util.ReadFile(bfs, "derived.txt")
	if err != nil {
		t.Fatalf("verify stage B derived.txt: %v", err)
	}
	expected := []byte("hello from stage A\nprocessed by stage B\n")
	if !bytes.Equal(got, expected) {
		t.Fatalf("stage B derived.txt: expected %q, got %q", string(expected), string(got))
	}
}
