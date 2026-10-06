//go:build !js && !wasip1

package volume_s4db_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/volume"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	volume_test "github.com/s4wave/spacewave/db/volume/test"
	"github.com/sirupsen/logrus"
)

// TestVolume builds the volume from its configuration on a controller bus and
// runs the volume checks.
func TestVolume(t *testing.T) {
	// Register the factory on a test bus.
	ctx := t.Context()
	b, sr, err := core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	sr.AddFactory(volume_s4db.NewFactory(b))

	// Load the volume controller and run the checks.
	conf := &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "volume.s4wave")}
	ctrl, _, ref, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx, b, resolver.NewLoadControllerWithConfig(conf), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	vol, err := ctrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := volume_test.CheckVolume(ctx, vol); err != nil {
		t.Fatal(err)
	}
	if err := volume_test.CheckStorageStatsNonZero(ctx, vol); err != nil {
		t.Fatal(err)
	}
}

// TestReopen checks that a reopened volume keeps its identity and blocks.
func TestReopen(t *testing.T) {
	// Create the volume.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	conf := &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "volume.s4wave")}
	vol, err := volume_s4db.NewVolume(ctx, le, conf)
	if err != nil {
		t.Fatal(err)
	}

	// Store a block and close.
	id := vol.GetID()
	data := writerBlock("reopen", 0)
	ref, _, err := vol.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the file and check the identity.
	vol, err = volume_s4db.NewVolume(ctx, le, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	if got := vol.GetID(); got != id {
		t.Fatalf("reopened volume id %q, want %q", got, id)
	}

	// Read the block back.
	got, found, err := vol.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, data) {
		t.Fatalf("reopened block found %v", found)
	}
}

// multiprocessEnv names the environment that runs a test process as a block
// writer: its value is the writer number, and multiprocessPathEnv the file.
const (
	multiprocessEnv     = "SPACEWAVE_S4DB_VOLUME_WRITER"
	multiprocessPathEnv = "SPACEWAVE_S4DB_VOLUME_PATH"
)

// writerBlocks is the number of blocks each writer process puts.
const writerBlocks = 20

// TestMultiprocessBlocks runs block writers in three processes at once on one
// volume file and checks that a fresh open reads every block.
func TestMultiprocessBlocks(t *testing.T) {
	// Run as a writer when started by the parent test.
	if n := os.Getenv(multiprocessEnv); n != "" {
		writeBlocks(t, os.Getenv(multiprocessPathEnv), n)
		return
	}

	// Create the volume.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	conf := &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "volume.s4wave")}
	vol, err := volume_s4db.NewVolume(ctx, le, conf)
	if err != nil {
		t.Fatal(err)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Start the writers and wait for them.
	const writers = 3
	cmds := make([]*exec.Cmd, writers)
	for i := range cmds {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMultiprocessBlocks$", "-test.timeout=60s")
		cmd.Env = append(os.Environ(), multiprocessEnv+"="+strconv.Itoa(i), multiprocessPathEnv+"="+conf.GetPath())
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds[i] = cmd
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}

	// Read every block from a fresh open.
	vol, err = volume_s4db.NewVolume(ctx, le, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	for w := range writers {
		for i := range writerBlocks {
			data := writerBlock(strconv.Itoa(w), i)
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, found, err := vol.GetBlock(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			if !found || !bytes.Equal(got, data) {
				t.Fatalf("writer %d block %d: found %v", w, i, found)
			}
		}
	}
}

// writeBlocks puts writer n's blocks into the volume at path.
func writeBlocks(t *testing.T, path, n string) {
	// Open the volume.
	ctx := context.Background()
	vol, err := volume_s4db.NewVolume(ctx, logrus.NewEntry(logrus.New()), &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()

	// Put the blocks, each durable on return.
	for i := range writerBlocks {
		if _, _, err := vol.PutBlock(ctx, writerBlock(n, i), nil); err != nil {
			t.Fatal(err)
		}
	}
}

// writerBlock returns block i of writer n, large enough to live outside the
// index.
func writerBlock(n string, i int) []byte {
	b := []byte("writer " + n + " block " + strconv.Itoa(i) + " ")
	for len(b) < 4096 {
		b = append(b, b...)
	}
	return b
}
