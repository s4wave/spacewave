package git_lfs

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
)

// rejectingUploads answers every UploadTree with an error after its first
// message, as a daemon does when the upload's parent directory cannot be read.
type rejectingUploads struct {
	s4wave_unixfs.SRPCFSHandleResourceServiceServer
}

// UploadTree reads the file start, then rejects the upload.
func (rejectingUploads) UploadTree(strm s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeStream) (*s4wave_unixfs.HandleUploadTreeResponse, error) {
	if _, err := strm.Recv(); err != nil {
		return nil, err
	}
	return nil, errors.New("block unavailable: parent directory")
}

// TestUnixFSStorePutReportsRejection checks that a Put the daemon rejects
// while the payload is still streaming reports the daemon's error, not the
// stream's refusal of the frames sent after the rejection.
func TestUnixFSStorePutReportsRejection(t *testing.T) {
	// Serve the rejecting handler over an in-memory muxed connection.
	ctx := t.Context()
	mux := srpc.NewMux()
	if err := s4wave_unixfs.SRPCRegisterFSHandleResourceService(mux, rejectingUploads{}); err != nil {
		t.Fatal(err)
	}
	clientPipe, serverPipe := net.Pipe()
	t.Cleanup(func() {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
	})
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srpc.NewServer(mux).AcceptMuxedConn(ctx, serverMp) }()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(srpc.NewClientWithMuxedConn(clientMp))
	store := NewUnixFSStore(root, nil)

	// Put an object of many frames; the daemon rejects it after the first.
	data := bytes.Repeat([]byte{7}, 64*chunkSize)
	_, oid := testObject(string(data))
	for range 20 {
		err := store.Put(ctx, oid, int64(len(data)), bytes.NewReader(data))
		if err == nil || !strings.Contains(err.Error(), "block unavailable: parent directory") {
			t.Fatalf("Put = %v, want the daemon's rejection", err)
		}
	}
}
