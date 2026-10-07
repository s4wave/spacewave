//go:build !js && !wasip1

package volume_s4db_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// holderPathEnv names the file a test process holds open until its standard
// input closes.
const holderPathEnv = "SPACEWAVE_S4DB_VOLUME_HOLDER"

// TestMigrateWaitsForOtherProcesses checks that a Volume file in an old
// format is not migrated while another process has it open, and is migrated
// once that process closes it.
func TestMigrateWaitsForOtherProcesses(t *testing.T) {
	// Hold the file open when started by the parent test.
	if path := os.Getenv(holderPathEnv); path != "" {
		holdOpen(t, path)
		return
	}

	// Write a version 0 file holding one block keyed in base58.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	data := []byte("version 0 block")
	ref := writeBase58Block(t, path, data)

	// Prepare a process holding the file.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMigrateWaitsForOtherProcesses$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), holderPathEnv+"="+path)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Start it and wait until it has the file open.
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
		t.Fatalf("wait for the holder: %v", err)
	}

	// The open refuses to migrate while the file is shared.
	conf := &volume_s4db.Config{Path: path}
	if _, err := volume_s4db.NewVolume(ctx, le, conf); !errors.Is(err, volume_s4db.ErrMigrateShared) {
		t.Fatalf("NewVolume while shared = %v, want ErrMigrateShared", err)
	}

	// Release the holder, then the open migrates and reads the block.
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("holder exited: %v", err)
	}
	vol, err := volume_s4db.NewVolume(ctx, le, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	got, found, err := vol.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("migrated block found %v data %q err %v", found, got, err)
	}
}

// writeBase58Block creates the s4db file at path holding data under its
// version 0 block key, and returns the block ref.
func writeBase58Block(t *testing.T, path string, data []byte) *block.BlockRef {
	// Build the version 0 key of the block.
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	rm, err := ref.MarshalKey()
	if err != nil {
		t.Fatal(err)
	}
	key := append(store_kvkey.NewDefaultKVKey().GetBlockFullPrefix(), b58.Encode(rm)...)

	// Open the file, closed on return.
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Write the block.
	tx, err := db.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(t.Context(), key, data); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return ref
}

// holdOpen opens the file at path, reports it on standard output, and closes
// it when standard input closes.
func holdOpen(t *testing.T, path string) {
	// Open the file and hold it until standard input closes.
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fmt.Println("open")
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
}
