//go:build !js

package spacewave_cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_apt "github.com/s4wave/spacewave/sdk/apt"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

func TestAptImportDebCommandRegistered(t *testing.T) {
	for _, cmd := range NewCliCommands(nil, nil) {
		if cmd.Name != "apt" {
			continue
		}
		for _, subcommand := range cmd.Subcommands {
			if subcommand.Name == "import-deb" {
				return
			}
		}
	}
	t.Fatal("apt import-deb command is not registered")
}

func TestAptImportDebArgumentsAndIntake(t *testing.T) {
	tests := []struct {
		name string
		args func(t *testing.T) []string
		want string
	}{
		{
			name: "missing arguments",
			args: func(t *testing.T) []string { return nil },
			want: "repository key, package key, and deb path required",
		},
		{
			name: "extra argument",
			args: func(t *testing.T) []string { return []string{"repo", "package", "file", "extra"} },
			want: "repository key, package key, and deb path required",
		},
		{
			name: "empty file",
			args: func(t *testing.T) []string {
				path := filepath.Join(t.TempDir(), "empty.deb")
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return []string{"repo", "package", path}
			},
			want: "deb package must not be empty",
		},
		{
			name: "directory",
			args: func(t *testing.T) []string { return []string{"repo", "package", t.TempDir()} },
			want: "deb package must be a regular file",
		},
		{
			name: "oversized file",
			args: func(t *testing.T) []string {
				path := filepath.Join(t.TempDir(), "large.deb")
				file, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Truncate(block.MaxBlockSize + 1); err != nil {
					file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				return []string{"repo", "package", path}
			},
			want: "deb package exceeds maximum block size",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runAptCLI(t, test.args(t)...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestAptImportDebCommitsAndReadsBack(t *testing.T) {
	ctx := t.Context()
	engine := setupAptSDKEngine(t, ctx)
	createAptRepository(t, ctx, engine, "apt/repos/stable")
	deb := buildAptDebFixture(t)
	var out bytes.Buffer

	if err := importAptDebPackage(
		ctx,
		engine,
		"apt/repos/stable",
		"apt/repos/stable/packages/busybox",
		deb,
		&out,
	); err != nil {
		t.Fatalf("importAptDebPackage: %v", err)
	}

	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	packageKey := "apt/repos/stable/packages/busybox"
	objectState, found, err := readTx.GetObject(ctx, packageKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("committed AptPackage not found")
	}
	var aptPackage *s4wave_apt.AptPackage
	_, _, err = world.AccessObjectState(ctx, objectState, false, func(cursor *block.Cursor) error {
		var unmarshalErr error
		aptPackage, unmarshalErr = block.UnmarshalBlock[*s4wave_apt.AptPackage](ctx, cursor, func() block.Block {
			return &s4wave_apt.AptPackage{}
		})
		return unmarshalErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if aptPackage.GetName() != "busybox" || aptPackage.GetVersion() != "1:1.36.1-7" || aptPackage.GetArchitecture() != "i386" {
		t.Fatalf("package identity = %s %s %s", aptPackage.GetName(), aptPackage.GetVersion(), aptPackage.GetArchitecture())
	}
	if aptPackage.GetState() != s4wave_apt.AptPackageState_AptPackageState_BUILT {
		t.Fatalf("package state = %s, want BUILT", aptPackage.GetState().String())
	}

	cursor, err := readTx.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()
	payload, found, err := cursor.GetBlock(ctx, aptPackage.GetDebRef())
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(payload, deb) {
		t.Fatal("committed deb payload does not match input")
	}

	quads, err := readTx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(
		"apt/repos/stable",
		s4wave_apt.PredAptRepoPackage.String(),
		packageKey,
		"",
	), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(quads) != 1 {
		t.Fatalf("repository graph edges = %d, want 1", len(quads))
	}

	wantOutput := "Package Key:     " + packageKey + "\n" +
		"Package:         busybox\n" +
		"Version:         1:1.36.1-7\n" +
		"Architecture:    i386\n" +
		"State:           BUILT\n" +
		"Deb Ref:         " + aptPackage.GetDebRef().MarshalString() + "\n"
	if out.String() != wantOutput {
		t.Fatalf("output:\n%s\nwant:\n%s", out.String(), wantOutput)
	}
}

func TestAptImportDebInvalidPackageAborts(t *testing.T) {
	ctx := t.Context()
	engine := setupAptSDKEngine(t, ctx)
	createAptRepository(t, ctx, engine, "apt/repos/stable")
	packageKey := "apt/repos/stable/packages/invalid"
	var out bytes.Buffer

	err := importAptDebPackage(ctx, engine, "apt/repos/stable", packageKey, []byte("not a deb"), &out)
	if err == nil || !strings.Contains(err.Error(), "import deb package") {
		t.Fatalf("error = %v, want wrapped import error", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output before commit: %q", out.String())
	}

	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	if _, found, err := readTx.GetObject(ctx, packageKey); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("invalid import committed a package object")
	}
	quads, err := readTx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(
		"apt/repos/stable",
		s4wave_apt.PredAptRepoPackage.String(),
		packageKey,
		"",
	), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(quads) != 0 {
		t.Fatalf("invalid import committed %d repository edges", len(quads))
	}
}

func TestAptImportDebWrapsCommitError(t *testing.T) {
	ctx := t.Context()
	failure := &aptCommitFailureClient{err: errors.New("storage commit failed")}
	engine := setupAptSDKEngineWithClient(t, ctx, func(client *resource_client.Client) sdk_engine.ResourceClient {
		failure.client = client
		return failure
	})
	createAptRepository(t, ctx, engine, "apt/repos/stable")
	failure.enabled = true
	var out bytes.Buffer

	err := importAptDebPackage(
		ctx,
		engine,
		"apt/repos/stable",
		"apt/repos/stable/packages/busybox",
		buildAptDebFixture(t),
		&out,
	)
	if err == nil || !strings.Contains(err.Error(), "commit transaction: storage commit failed") {
		t.Fatalf("error = %v, want wrapped commit error", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output after failed commit: %q", out.String())
	}

	failure.enabled = false
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	if _, found, err := readTx.GetObject(ctx, "apt/repos/stable/packages/busybox"); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("failed commit made package visible")
	}
}

func runAptCLI(t *testing.T, args ...string) error {
	t.Helper()

	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Commands = []*cli.Command{newAptCommand(nil)}
	return app.RunContext(t.Context(), append([]string{"spacewave", "apt", "import-deb"}, args...))
}

func setupAptSDKEngine(t *testing.T, ctx context.Context) *sdk_engine.SDKEngine {
	t.Helper()

	return setupAptSDKEngineWithClient(t, ctx, func(client *resource_client.Client) sdk_engine.ResourceClient {
		return client
	})
}

func setupAptSDKEngineWithClient(
	t *testing.T,
	ctx context.Context,
	wrap func(*resource_client.Client) sdk_engine.ResourceClient,
) *sdk_engine.SDKEngine {
	t.Helper()

	_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
	rootRef := resClient.AccessRootResource()
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		cleanup()
		t.Fatal(err)
	}
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		rootRef.Release()
		cleanup()
		t.Fatal(err)
	}
	engineRef := resClient.CreateResourceReference(createResp.GetResourceId())
	engine, err := sdk_engine.NewSDKEngine(wrap(resClient), engineRef)
	if err != nil {
		engineRef.Release()
		rootRef.Release()
		cleanup()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		engine.Release()
		rootRef.Release()
		cleanup()
	})
	return engine
}

func createAptRepository(t *testing.T, ctx context.Context, engine *sdk_engine.SDKEngine, repositoryKey string) {
	t.Helper()

	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	_, _, err = tx.ApplyWorldOp(ctx, s4wave_apt.NewCreateAptRepositoryOp(repositoryKey, &s4wave_apt.AptRepository{
		State:         s4wave_apt.AptRepositoryState_AptRepositoryState_EMPTY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

type aptCommitFailureClient struct {
	client  *resource_client.Client
	enabled bool
	err     error
}

func (c *aptCommitFailureClient) CreateResourceReference(resourceID uint32) resource_client.ResourceRef {
	return &aptCommitFailureRef{ResourceRef: c.client.CreateResourceReference(resourceID), failure: c}
}

type aptCommitFailureRef struct {
	resource_client.ResourceRef
	failure *aptCommitFailureClient
}

func (r *aptCommitFailureRef) GetClient() (srpc.Client, error) {
	client, err := r.ResourceRef.GetClient()
	if err != nil {
		return nil, err
	}
	return &aptCommitFailureSRPCClient{Client: client, failure: r.failure}, nil
}

type aptCommitFailureSRPCClient struct {
	srpc.Client
	failure *aptCommitFailureClient
}

func (c *aptCommitFailureSRPCClient) ExecCall(
	ctx context.Context,
	service string,
	method string,
	in srpc.Message,
	out srpc.Message,
) error {
	if c.failure.enabled && method == "Commit" {
		return c.failure.err
	}
	return c.Client.ExecCall(ctx, service, method, in, out)
}

func buildAptDebFixture(t *testing.T) []byte {
	t.Helper()

	control := []byte("Package: busybox\nVersion: 1:1.36.1-7\nArchitecture: i386\nDescription: Tiny utilities\n\n")
	var tarData bytes.Buffer
	tarWriter := tar.NewWriter(&tarData)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "./control", Mode: 0o644, Size: int64(len(control))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(control); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	var controlArchive bytes.Buffer
	gzipWriter := gzip.NewWriter(&controlArchive)
	if _, err := gzipWriter.Write(tarData.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	writeAptArMember(t, &deb, "debian-binary", []byte("2.0\n"))
	writeAptArMember(t, &deb, "control.tar.gz", controlArchive.Bytes())
	writeAptArMember(t, &deb, "data.tar", nil)
	return deb.Bytes()
}

func writeAptArMember(t *testing.T, dst *bytes.Buffer, name string, data []byte) {
	t.Helper()

	field := func(value string, width int) string {
		return value + strings.Repeat(" ", width-len(value))
	}
	header := field(name+"/", 16) + field("0", 12) + field("0", 6) + field("0", 6) +
		field("100644", 8) + field(strconv.Itoa(len(data)), 10) + "`\n"
	if len(header) != 60 {
		t.Fatalf("ar header size = %d, want 60", len(header))
	}
	dst.WriteString(header)
	dst.Write(data)
	if len(data)%2 != 0 {
		dst.WriteByte('\n')
	}
}
