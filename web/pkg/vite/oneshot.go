//go:build !js

package web_pkg_vite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/bifrost/util/randstring"
	singleton_muxed_conn "github.com/aperturerobotics/bldr/util/singleton-muxed-conn"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	bldr_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/bun"
	"github.com/aperturerobotics/util/pipesock"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// RunOneShot starts a ViteBundler process, calls the provided function with the
// client, and tears down the process when done. Use this for call sites that do
// not have a long-lived ViteBundler process (esbuild compiler, plugin compiler,
// browser entrypoint).
func RunOneShot(
	ctx context.Context,
	le *logrus.Entry,
	distSourcePath string,
	sourcePath string,
	workingPath string,
	fn func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error,
) error {
	bundleID := "web-pkg-oneshot"

	// Derive a pipe UUID for IPC.
	var pipeUuidBin [32]byte
	blake3.DeriveKey(
		"bldr vite-compiler pipe uuid",
		bytes.Join([][]byte{[]byte(sourcePath), []byte(workingPath), []byte(bundleID)},
			[]byte(" -- "),
		),
		pipeUuidBin[:],
	)
	pipeUuid := "vite-" + strings.ToLower(b58.Encode(pipeUuidBin[:]))[:4] + "-" + randstring.RandomIdentifier(4)

	// Compile the vite service script with esbuild.
	if err := os.MkdirAll(workingPath, 0o755); err != nil {
		return err
	}
	viteScriptPath := filepath.Join(workingPath, "bldr-"+pipeUuid+".mjs")
	opts := esbuild.BuildOptions{
		AbsWorkingDir: distSourcePath,
		SourceRoot:    workingPath,
		Outfile:       viteScriptPath,
		EntryPoints:   []string{"./web/bundler/vite/vite.ts"},
		Target:        esbuild.ES2022,
		Format:        esbuild.FormatESModule,
		Platform:      esbuild.PlatformNode,
		LogLevel:      esbuild.LogLevelWarning,
		TreeShaking:   esbuild.TreeShakingTrue,
		Sourcemap:     esbuild.SourceMapLinked,
		Drop:          esbuild.DropDebugger,
		Metafile:      false,
		Splitting:     false,
		Define: map[string]string{
			"BLDR_IS_NODE": "true",
			"NO_COLOR":     "1",
		},
		Plugins: []esbuild.Plugin{
			bldr_esbuild_build.ExternalNodeModulesPlugin(),
			bldr_esbuild_build.GoVendorTsResolverPlugin(sourcePath),
		},
		External: []string{"starpc", "vite"},
		Bundle:   true,
		Write:    true,
	}
	result := esbuild.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return errors.Wrap(err, "compile vite script")
	}
	defer os.Remove(viteScriptPath)
	defer os.Remove(viteScriptPath + ".map")

	// Set up the IPC pipe.
	pipeListener, err := pipesock.BuildPipeListener(le, workingPath, pipeUuid)
	if err != nil {
		return err
	}
	defer pipeListener.Close()

	smc := singleton_muxed_conn.NewSingletonMuxedConn(ctx, true)
	go smc.AcceptPump(pipeListener)
	defer smc.Close()

	// Derive bun state directory.
	bunStateDir := filepath.Join(workingPath, "..", "..", "bun")

	// Start the bun process.
	cmd, err := bun.BunExec(ctx, le, bunStateDir, viteScriptPath, "--bundle-id", bundleID, "--pipe-uuid", pipeUuid)
	if err != nil {
		return err
	}
	cmd.Env = slices.Clone(os.Environ())
	cmd.Dir = filepath.Dir(viteScriptPath)
	cmd.Stdout = le.WriterLevel(logrus.DebugLevel)
	cmd.Stderr = le.WriterLevel(logrus.DebugLevel)

	// Env vars
	cmd.Env = append(cmd.Env, "NO_COLOR=1", "NODE_DISABLE_COLORS=1", "FORCE_COLOR=0")

	if ctx.Err() != nil {
		return context.Canceled
	}

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "start vite process")
	}

	// Wait for vite to connect.
	timeoutCtx, timeoutCancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.New("timeout waiting for vite to connect"))
	defer timeoutCancel()

	le.Debug("waiting for vite oneshot to connect")
	_, err = smc.WaitConn(timeoutCtx)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	// Create the SRPC client.
	srpcClient := srpc.NewClientWithMuxedConn(smc)
	client := bldr_vite.NewSRPCViteBundlerClient(srpcClient)

	le.Debug("vite oneshot connected")

	// Run the caller's function.
	fnErr := fn(ctx, client)

	// Tear down the process.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	return fnErr
}
