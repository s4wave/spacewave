//go:build !js && !wasip1 && !bldr_sqlite

package volume_bolt_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/core/provider"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/core/sobject"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	resource_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

const (
	nativeReopenPathEnv           = "SPACEWAVE_NATIVE_REOPEN_PATH"
	nativeReopenExpectedVolumeEnv = "SPACEWAVE_NATIVE_REOPEN_EXPECTED_VOLUME"
	nativeReopenExpectedCtrlEnv   = "SPACEWAVE_NATIVE_REOPEN_EXPECTED_CONTROLLER"
	nativeReopenMetadataEnv       = "SPACEWAVE_NATIVE_REOPEN_METADATA"
	nativeReopenClosedEnv         = "SPACEWAVE_NATIVE_REOPEN_CLOSED"
	nativeReopenObjectKey         = "native-reopen/fixed-object"
	nativeReopenBoundary          = "attached-space-resource-rpc"
	nativeReopenControllerID      = "hydra/volume/bolt"
)

var nativeReopenRole = flag.String("spacewave-native-reopen-role", "", "native reopen child role")

// TestBoltVolumeFreshProcessSpaceReopen measures a clean native reopen through
// a manually attached SpaceResource and the public Space and World RPC services.
// Full Session/Space mount and persisted Space resolution remain outside this slice.
func TestBoltVolumeFreshProcessSpaceReopen(t *testing.T) {
	if *nativeReopenRole != "" {
		runNativeReopenRole(t, *nativeReopenRole)
		return
	}

	dir := t.TempDir()
	boltPath := filepath.Join(dir, "space-reopen.db")
	metadataPath := filepath.Join(dir, "seed-metadata")
	seedClosedPath := filepath.Join(dir, "seed-closed")
	reopenClosedPath := filepath.Join(dir, "reopen-closed")

	seed := nativeReopenCommand(t, "seed", boltPath, "", "", metadataPath, seedClosedPath)
	if err := seed.Run(); err != nil {
		t.Fatalf("seed failed: %v\n%s", err, cmdOutput(seed))
	}
	metadata, err := readNativeReopenMetadata(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if metadata["controller_id"] != nativeReopenControllerID {
		t.Fatalf("seed controller_id = %q, want %q", metadata["controller_id"], nativeReopenControllerID)
	}
	if _, err := os.Stat(seedClosedPath); err != nil {
		t.Fatalf("seed cleanup marker: %v", err)
	}

	reopen := nativeReopenCommand(t, "reopen", boltPath, metadata["volume_id"], metadata["controller_id"], "", reopenClosedPath)
	if err := reopen.Run(); err != nil {
		t.Fatalf("reopen failed: %v\n%s", err, cmdOutput(reopen))
	}
	if _, err := os.Stat(reopenClosedPath); err != nil {
		t.Fatalf("reopen cleanup marker: %v", err)
	}
	if !strings.Contains(cmdOutput(reopen), "NATIVE_REOPEN_RESULT") {
		t.Fatalf("reopen did not emit machine-readable result:\n%s", cmdOutput(reopen))
	}
	t.Logf("native reopen raw result:\n%s", cmdOutput(reopen))
}

func nativeReopenCommand(
	t *testing.T,
	role string,
	boltPath string,
	expectedVolume string,
	expectedController string,
	metadataPath string,
	closedPath string,
) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestBoltVolumeFreshProcessSpaceReopen$", "-test.v", "-spacewave-native-reopen-role="+role) //nolint:gosec
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Env = append(os.Environ(),
		nativeReopenPathEnv+"="+boltPath,
		nativeReopenExpectedVolumeEnv+"="+expectedVolume,
		nativeReopenExpectedCtrlEnv+"="+expectedController,
		nativeReopenMetadataEnv+"="+metadataPath,
		nativeReopenClosedEnv+"="+closedPath,
	)
	return cmd
}

func runNativeReopenRole(t *testing.T, role string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var started time.Time
	if role == "reopen" {
		started = time.Now()
	}

	env, err := newNativeReopenEnv(ctx, os.Getenv(nativeReopenPathEnv))
	if err != nil {
		t.Fatal(err)
	}
	var clientCleanup func()
	defer func() {
		env.Release(clientCleanup)
		if path := os.Getenv(nativeReopenClosedEnv); path != "" {
			if err := os.WriteFile(path, []byte("closed\n"), 0o600); err != nil {
				t.Errorf("write cleanup marker: %v", err)
			}
		}
	}()

	resClient, cleanup := resource_testbed.SetupResourceClient(ctx, t, env.tb)
	clientCleanup = cleanup

	switch role {
	case "seed":
		runNativeReopenSeed(t, ctx, env, resClient)
	case "reopen":
		observedAnswer := runNativeReopenRead(t, ctx, env, resClient)
		fmt.Printf(
			"NATIVE_REOPEN_RESULT\tboundary=%s\tcontroller_id=%s\tvolume_id=%s\tobject_key=%s\tanswer=%s\telapsed_ns=%d\n",
			nativeReopenBoundary,
			nativeReopenCompiledControllerID(env.tb),
			env.tb.Volume.GetID(),
			observedAnswer,
			observedAnswer,
			time.Since(started).Nanoseconds(),
		)
	default:
		t.Fatalf("unknown role %q", role)
	}
}

type nativeReopenEnv struct {
	tb *resource_world_testbed.Testbed
}

func newNativeReopenEnv(ctx context.Context, boltPath string) (*nativeReopenEnv, error) {
	tb, err := resource_world_testbed.WithTestbedOptions(
		ctx,
		[]db_testbed.Option{db_testbed.WithVolumeConfig(&volume_bolt.Config{
			Path:         boltPath,
			VolumeConfig: &volume_controller.Config{},
		})},
		nil,
	)
	if err != nil {
		return nil, err
	}
	return &nativeReopenEnv{tb: tb}, nil
}

func nativeReopenCompiledControllerID(tb *resource_world_testbed.Testbed) string {
	ctrl, ok := tb.VolumeController.(*volume_controller.Controller)
	if !ok {
		return ""
	}
	return ctrl.GetControllerInfo().GetId()
}

func (e *nativeReopenEnv) Release(clientCleanup func()) {
	if clientCleanup != nil {
		clientCleanup()
	}
	if e.tb != nil {
		e.tb.Release()
		e.tb = nil
	}
}

func attachNativeSpaceResource(ctx context.Context, t *testing.T, env *nativeReopenEnv, resClient *resource_client.Client) resource_client.ResourceRef {
	t.Helper()
	spaceRef := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			ProviderId:        "native-reopen",
			ProviderAccountId: "fixture",
			Id:                "space",
		},
	}
	body := space_sobject.NewSpaceBody(
		spaceRef,
		"native-reopen-space",
		env.tb.BucketId,
		env.tb.Volume.GetID(),
		nil,
		env.tb.Engine,
	)
	spaceResource := resource_space.NewSpaceResource(
		logrus.NewEntry(logrus.New()),
		env.tb.Bus,
		body,
	)
	id, err := resClient.AttachResourceTree(ctx, "native-reopen-space", spaceResource.GetMux())
	if err != nil {
		t.Fatal(err)
	}
	return resClient.CreateResourceReference(id)
}

func runNativeReopenSeed(t *testing.T, ctx context.Context, env *nativeReopenEnv, resClient *resource_client.Client) {
	spaceRef := attachNativeSpaceResource(ctx, t, env, resClient)
	spaceClient, err := spaceRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	spaceSvc := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceClient)
	worldResp, err := spaceSvc.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatal(err)
	}
	worldRef := resClient.CreateResourceReference(worldResp.GetResourceId())
	worldClient, err := worldRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	engineSvc := s4wave_world.NewSRPCEngineResourceServiceClient(worldClient)
	txResp, err := engineSvc.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true})
	if err != nil {
		t.Fatal(err)
	}
	txRef := resClient.CreateResourceReference(txResp.GetResourceId())
	txClient, err := txRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	worldTxSvc := s4wave_world.NewSRPCWorldStateResourceServiceClient(txClient)
	objResp, err := worldTxSvc.CreateObject(ctx, &s4wave_world.CreateObjectRequest{ObjectKey: nativeReopenObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s4wave_world.NewSRPCTxResourceServiceClient(txClient).Commit(ctx, &s4wave_world.CommitRequest{}); err != nil {
		t.Fatal(err)
	}
	releaseResource(resClient, objResp.GetResourceId())
	txRef.Release()
	if _, err := engineSvc.Sync(ctx, &s4wave_world.SyncRequest{}); err != nil {
		t.Fatal(err)
	}
	worldRef.Release()
	spaceRef.Release()

	metadata := fmt.Sprintf(
		"controller_id=%s\tvolume_id=%s\n",
		nativeReopenCompiledControllerID(env.tb),
		env.tb.Volume.GetID(),
	)
	if err := os.WriteFile(os.Getenv(nativeReopenMetadataEnv), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runNativeReopenRead(t *testing.T, ctx context.Context, env *nativeReopenEnv, resClient *resource_client.Client) string {
	if got, want := nativeReopenCompiledControllerID(env.tb), os.Getenv(nativeReopenExpectedCtrlEnv); got != want {
		t.Fatalf("controller_id = %q, want %q", got, want)
	}
	if got, want := env.tb.Volume.GetID(), os.Getenv(nativeReopenExpectedVolumeEnv); got != want {
		t.Fatalf("volume_id = %q, want %q", got, want)
	}

	spaceRef := attachNativeSpaceResource(ctx, t, env, resClient)
	spaceClient, err := spaceRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	spaceSvc := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceClient)
	worldResp, err := spaceSvc.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatal(err)
	}
	worldRef := resClient.CreateResourceReference(worldResp.GetResourceId())
	worldClient, err := worldRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	engineSvc := s4wave_world.NewSRPCEngineResourceServiceClient(worldClient)
	txResp, err := engineSvc.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	txRef := resClient.CreateResourceReference(txResp.GetResourceId())
	txClient, err := txRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	worldTxSvc := s4wave_world.NewSRPCWorldStateResourceServiceClient(txClient)
	obj, err := worldTxSvc.GetObject(ctx, &s4wave_world.GetObjectRequest{ObjectKey: nativeReopenObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	if !obj.GetFound() || obj.GetObjectKey() != nativeReopenObjectKey {
		t.Fatalf("fixed object read = found:%v key:%q", obj.GetFound(), obj.GetObjectKey())
	}
	observedAnswer := obj.GetObjectKey()
	releaseResource(resClient, obj.GetResourceId())
	txRef.Release()
	worldRef.Release()
	spaceRef.Release()
	return observedAnswer
}

func releaseResource(resClient *resource_client.Client, id uint32) {
	if id != 0 {
		resClient.CreateResourceReference(id).Release()
	}
}

func readNativeReopenMetadata(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(data))
	metadata := make(map[string]string, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("invalid seed metadata field %q", field)
		}
		metadata[key] = value
	}
	return metadata, nil
}
