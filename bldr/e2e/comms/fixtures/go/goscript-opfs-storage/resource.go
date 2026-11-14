//go:build goscript

package goscript_opfs_storage

import (
	"context"
	"net"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space/world/objecttypes"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
)

// uploadEngineResource follows the public typed factory and streaming upload
// path, then closes the resource connection before its caller inspects storage.
func uploadEngineResource(ctx context.Context, tb *db_testbed.Testbed, so sobject.SharedObject, engine world.Engine, objectKey string) error {
	// Resolve the production builtin factory on the fixture bus.
	releaseTypes, err := tb.Bus.AddController(ctx, objecttype_controller.NewController(objecttypes.LookupObjectType), nil)
	if err != nil {
		return err
	}
	defer releaseTypes()

	// Grant the actual Space engine through its public resource wrapper.
	engineResource := resource_world.NewEngineResource(tb.Logger, tb.Bus, engine, space_world_optypes.LookupWorldOp, nil)
	defer engineResource.Close()
	serverMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(engineResource.GetMux()).Register(serverMux); err != nil {
		return err
	}

	// Connect real serialized Resource and UploadTree streams within the worker.
	rpcCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Open the client stream multiplexer on the fixture pipe.
	clientMux, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		return err
	}
	defer clientMux.Close()

	// Open the server stream multiplexer on the other end.
	serverMuxed, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		return err
	}
	defer serverMuxed.Close()

	// Join the server after closing both transport ends on every return path.
	server := srpc.NewServer(serverMux)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.AcceptMuxedConn(rpcCtx, serverMuxed)
	}()
	defer func() {
		cancel()
		_ = clientMux.Close()
		_ = serverMuxed.Close()
		<-done
	}()

	// Retain the Resource protocol client until its child references release.
	client, err := resource_client.NewClient(rpcCtx, resource.NewSRPCResourceServiceClient(srpc.NewClientWithMuxedConn(clientMux)))
	if err != nil {
		return err
	}
	defer client.Release()

	// Acquire the granting engine resource for typed-object access.
	root := client.AccessRootResource()
	defer root.Release()
	rootClient, err := root.GetClient()
	if err != nil {
		return err
	}

	// Resolve the builtin filesystem through the typed-object service.
	typed, err := s4wave_world.AccessTypedObject(rpcCtx, s4wave_world.NewSRPCTypedObjectResourceServiceClient(rootClient), &s4wave_world.AccessTypedObjectRequest{ObjectKey: objectKey})
	if err != nil {
		return err
	}
	if typed.GetTypeId() != s4wave_unixfs_world.UnixFSTypeID {
		return errors.Errorf("unexpected filesystem type %q", typed.GetTypeId())
	}

	// Retain the filesystem child until the upload response has arrived.
	fsRef := client.CreateResourceReference(typed.GetResourceId())
	defer fsRef.Release()
	fsClient, err := fsRef.GetClient()
	if err != nil {
		return err
	}
	fs := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fsClient)

	// Open the legacy upload with the exact Drive Quickstart starter guide.
	guide := []byte(`# Getting Started

Welcome to your new drive! This starter guide is written by the Drive
Quickstart after the generic UnixFS filesystem is initialized.

## Next steps

Try uploading a few files and opening them here. Video files are the best ones
to try first.
`)
	legacy, err := fs.UploadFile(rpcCtx)
	if err != nil {
		return err
	}

	// Send metadata and content together, as the starter guide client does.
	if err := legacy.Send(&s4wave_unixfs.HandleUploadFileRequest{
		Name: "getting-started.md", TotalSize: int64(len(guide)), Mode: 0o644, Data: guide,
	}); err != nil {
		return err
	}

	// Wait for legacy publication before any batch upload can replace its root.
	legacyResponse, err := legacy.CloseAndRecv()
	if err != nil {
		return err
	}
	if legacyResponse.GetBytesWritten() != int64(len(guide)) {
		return errors.New("legacy starter guide upload byte count differs")
	}

	// Inspect physical source bytes before reading the object or uploading again.
	if _, err := inspectEngineGraph(ctx, tb, so, "legacy upload before tree", nil); err != nil {
		return err
	}

	// Stream the exact pairing file through the handler's reader and commit.
	data := []byte("This file survives after the original pairing client leaves.\n")
	upload, err := fs.UploadTree(rpcCtx)
	if err != nil {
		return err
	}
	if err := upload.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      "paired-file.md",
				TotalSize: int64(len(data)),
				Mode:      0o644,
				Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
			},
		},
	}); err != nil {
		return err
	}
	if err := upload.Send(&s4wave_unixfs.HandleUploadTreeRequest{Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{Data: data}}); err != nil {
		return err
	}

	// Wait for handle Sync, batch commit and reload before closing the client.
	response, err := upload.CloseAndRecv()
	if err != nil {
		return err
	}
	written := response.GetWritten()
	if written.GetBytesWritten() != int64(len(data)) || written.GetFilesWritten() != 1 || written.GetDirectoriesWritten() != 0 {
		return errors.New("streamed pairing upload counters differ from its single file")
	}
	return nil
}
