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

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/bun"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	bldr_pipesock "github.com/s4wave/spacewave/bldr/util/pipesock"
	singleton_muxed_conn "github.com/s4wave/spacewave/bldr/util/singleton-muxed-conn"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	"github.com/s4wave/spacewave/net/util/randstring"
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
	// Name the one-shot Vite bundle.
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

	// Compile the Vite service script through the direct internal owner.
	if err := os.MkdirAll(workingPath, 0o755); err != nil {
		return err
	}
	bunStateDir := filepath.Join(workingPath, "..", "..", "bun")
	viteScriptPath := filepath.Join(workingPath, "bldr-"+pipeUuid+".mjs")
	if _, err := bldr_vite.BuildServiceScript(
		ctx,
		le,
		bunStateDir,
		sourcePath,
		distSourcePath,
		viteScriptPath,
	); err != nil {
		return errors.Wrap(err, "compile Vite service script")
	}
	defer os.Remove(viteScriptPath)
	defer os.Remove(viteScriptPath + ".map")

	// Set up the IPC pipe.
	pipeListener, err := bldr_pipesock.Listen(le, workingPath, pipeUuid)
	if err != nil {
		return err
	}
	defer pipeListener.Close()

	// Pump accepted connections through the singleton muxed connection.
	smc := singleton_muxed_conn.NewSingletonMuxedConn(ctx, true)
	go smc.AcceptPump(pipeListener)
	defer smc.Close()

	// Start the bun process.
	cmd, err := bun.BunExec(ctx, le, bunStateDir, viteScriptPath, "--bundle-id", bundleID, "--pipe-uuid", pipeUuid, "--pipe-root", pipeListener.GetRootDir())
	if err != nil {
		return err
	}
	cmd.Env = slices.Clone(os.Environ())
	cmd.Dir = filepath.Dir(viteScriptPath)
	cmd.Stdout = le.WriterLevel(logrus.DebugLevel)
	cmd.Stderr = le.WriterLevel(logrus.DebugLevel)

	// Configure the color and root environment variables.
	cmd.Env = append(
		cmd.Env,
		"NO_COLOR=1",
		"NODE_DISABLE_COLORS=1",
		"FORCE_COLOR=0",
		"BLDR_PROJECT_ROOT="+sourcePath,
		"BLDR_DIST_ROOT="+distSourcePath,
	)

	// Stop before starting when the context is already done.
	if ctx.Err() != nil {
		return context.Canceled
	}

	// Start the bun process.
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "start vite process")
	}

	// Wait for vite to connect, killing the process on timeout.
	timeoutCtx, timeoutCancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.New("timeout waiting for vite to connect"))
	defer timeoutCancel()

	// Log the wait and block until the Vite process connects.
	le.Debug("waiting for vite oneshot to connect")
	_, err = smc.WaitConn(timeoutCtx)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	// Create the budgeted SRPC client over the muxed connection.
	srpcClient := srpc.NewClientWithMuxedConn(smc)
	client, err := bldr_vite.NewBudgetClient(bldr_vite.NewSRPCViteBundlerClient(srpcClient))
	if err != nil {
		// Failed admission setup still closes the process started above.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	// Report the connected Vite process.
	le.Debug("vite oneshot connected")

	// Run the caller's function with the budgeted Vite client.
	fnErr := fn(ctx, client)

	// Tear down the process.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	return fnErr
}
