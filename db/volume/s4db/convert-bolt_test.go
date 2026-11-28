//go:build !js && !wasip1

package volume_s4db_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	bdb "github.com/aperturerobotics/bbolt"
	bdb_errors "github.com/aperturerobotics/bbolt/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// boltCrashEnv names the environment variable holding the step a child
// process of TestConvertBolt exits at, and boltCrashPathEnv the bolt file it
// converts.
const (
	boltCrashEnv     = "SPACEWAVE_TEST_BOLT_CRASH_STEP"
	boltCrashPathEnv = "SPACEWAVE_TEST_BOLT_CRASH_PATH"
)

// boltCrashExit is the exit code of a child process that stopped at its step.
const boltCrashExit = 3

// TestConvertBolt checks that opening a bolt Volume file converts it in
// place, keeping the Volume key and blocks, and that the next open uses the
// converted file. A conversion file beside the bolt file that no claim
// accounts for is kept and refuses the conversion. A bolt writer waiting on
// the conversion never commits to the replaced file, and a process killed
// after any step leaves a file the next open reads whole.
func TestConvertBolt(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	if step := os.Getenv(boltCrashEnv); step != "" {
		convertUntil(t, os.Getenv(boltCrashPathEnv), step)
		return
	}

	t.Run("convert", func(t *testing.T) {
		// Create a bolt Volume, then open it twice as an s4db Volume: once
		// converting, once reading.
		path, id, refs := newBoltVolume(t)
		for range 2 {
			vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			if got := vol.GetPeerID(); got != id {
				t.Fatalf("converted volume peer %s, want %s", got, id)
			}
			for i, ref := range refs {
				got, found, err := vol.GetBlock(ctx, ref)
				if err != nil {
					t.Fatal(err)
				}
				if !found || !bytes.Equal(got, writerBlock("bolt", i)) {
					t.Fatalf("converted block %d found %v", i, found)
				}
			}
			if err := vol.Close(); err != nil {
				t.Fatal(err)
			}
		}

		// Only the converted file remains.
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name() != filepath.Base(path) {
				t.Errorf("leftover file %q", e.Name())
			}
		}
	})

	t.Run("unowned converting", func(t *testing.T) {
		// Put a file no claim accounts for where the copy goes.
		path, _, _ := newBoltVolume(t)
		other := []byte("someone else's file")
		if err := os.WriteFile(path+".converting", other, 0o600); err != nil {
			t.Fatal(err)
		}

		// The conversion refuses permanently and keeps the file.
		_, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		if !volume.IsPermanent(err) {
			t.Fatalf("open beside an unowned file: %v, want a permanent error", err)
		}
		got, err := os.ReadFile(path + ".converting")
		if err != nil || !bytes.Equal(got, other) {
			t.Fatalf("unowned file changed: %v", err)
		}

		// Once it is moved aside, the bolt file converts.
		if err := os.Remove(path + ".converting"); err != nil {
			t.Fatal(err)
		}
		vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		if err := vol.Close(); err != nil {
			t.Fatal(err)
		}
	})

	for _, alias := range []bool{false, true} {
		t.Run(fmt.Sprintf("waiting writer alias %v", alias), func(t *testing.T) {
			// Open the Volume through a symbolic link when alias is set,
			// while the writer opens the bolt file's own name.
			path, _, _ := newBoltVolume(t)
			open := path
			if alias {
				open = filepath.Join(t.TempDir(), "link.s4wave")
				if err := os.Symlink(path, open); err != nil {
					t.Fatal(err)
				}
			}

			// Once the copy is done, start a bolt writer and wait until it
			// holds the bolt file, so it waits on the conversion's lock, and
			// open the bolt file to probe that lock. The lock must hold
			// through the rename and its directory sync, whatever the
			// scheduler does. Let the conversion return only once the
			// writer has finished.
			opened, done := make(chan struct{}), make(chan struct{})
			var writerErr error
			var probe *os.File
			defer volume_s4db.SetBoltStep(func(step string) {
				switch step {
				case "copied":
					go func() {
						writerErr = commitBolt(path, opened)
						close(done)
					}()
					<-opened
					var err error
					if probe, err = os.Open(path); err != nil {
						t.Fatal(err)
					}
				case "renamed", "installed":
					if locked, err := boltLocked(probe); err != nil || !locked {
						t.Errorf("bolt file unlocked at %s: %v", step, err)
					}
					select {
					case <-done:
						t.Errorf("bolt writer finished while the bolt file was locked: %v", writerErr)
					default:
					}
				case "unlocked":
					<-done
				}
			})()
			defer func() {
				if probe != nil {
					_ = probe.Close()
				}
			}()
			vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: open})
			if err != nil {
				t.Fatal(err)
			}
			if err := vol.Close(); err != nil {
				t.Fatal(err)
			}

			// The writer either refused the converted file or committed
			// into it.
			<-done
			if errors.Is(writerErr, bdb_errors.ErrInvalid) {
				return
			}
			if writerErr != nil {
				t.Fatal(writerErr)
			}
			db, err := s4db.Open(open, s4db.Options{ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			tx, err := db.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			if _, found, err := tx.Get(ctx, lateKey); err != nil || !found {
				t.Fatalf("bolt writer's acknowledged commit missing from the converted file: %v", err)
			}
		})
	}

	t.Run("hard link", func(t *testing.T) {
		// Leave the bolt file without a synced freelist, which a writable
		// bolt open would write.
		path, _, _ := newBoltVolume(t)
		src, err := bdb.Open(path, 0o600, &bdb.Options{NoFreelistSync: true})
		if err != nil {
			t.Fatal(err)
		}
		err = src.Update(func(tx *bdb.Tx) error {
			return tx.Bucket([]byte("hydra")).Put([]byte("unsynced freelist"), []byte("value"))
		})
		if err := errors.Join(err, src.Close()); err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		// A second name for the bolt file refuses the conversion and keeps
		// both names and every byte.
		other := filepath.Join(t.TempDir(), "other.s4wave")
		if err := os.Link(path, other); err != nil {
			t.Fatal(err)
		}
		_, err = volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		if !errors.Is(err, volume_s4db.ErrConvertLinked) || !volume.IsPermanent(err) {
			t.Fatalf("open of a linked bolt file: %v, want ErrConvertLinked", err)
		}
		a, errA := os.Stat(path)
		b, errB := os.Stat(other)
		if errA != nil || errB != nil || !os.SameFile(a, b) {
			t.Fatalf("names changed: %v %v", errA, errB)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("refused conversion changed the bolt file: %v", err)
		}
	})

	t.Run("directory sync fails", func(t *testing.T) {
		// Open the Volume through a symbolic link in another directory, and
		// fail every directory sync after the rename.
		target, id, _ := newBoltVolume(t)
		path := filepath.Join(t.TempDir(), "volume.s4wave")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		renamed := false
		restoreStep := volume_s4db.SetBoltStep(func(step string) {
			renamed = renamed || step == "renamed"
		})
		restoreSync := volume_s4db.SetSyncDir(func(dir string) error {
			if renamed {
				return errors.New("injected directory sync failure")
			}
			return nil
		})
		_, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		restoreSync()
		restoreStep()
		if !renamed {
			t.Fatal("conversion stopped before the rename")
		}
		if err == nil {
			t.Fatal("conversion passed a failed directory sync")
		}

		// The next open syncs the directory holding the renamed file, not
		// the link's, before the Volume takes writes.
		dir, err := filepath.EvalSymlinks(filepath.Dir(target))
		if err != nil {
			t.Fatal(err)
		}
		synced := false
		defer volume_s4db.SetSyncDir(func(d string) error {
			synced = synced || d == dir
			return nil
		})()
		vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		defer vol.Close()
		if !synced {
			t.Fatalf("reopen did not sync %s", dir)
		}
		if got := vol.GetPeerID(); got != id {
			t.Fatalf("reopened volume peer %s, want %s", got, id)
		}
	})

	for _, step := range []string{"copied", "renamed", "installed", "unlocked"} {
		t.Run("killed after "+step, func(t *testing.T) {
			// Kill a converting process right after the step.
			path, id, refs := newBoltVolume(t)
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConvertBolt$", "-test.timeout=60s")
			cmd.Env = append(os.Environ(), boltCrashEnv+"="+step, boltCrashPathEnv+"="+path)
			cmd.Stderr = os.Stderr
			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != boltCrashExit {
				t.Fatalf("converting process: %v, want exit %d", err, boltCrashExit)
			}

			// The next open reads every block.
			vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			defer vol.Close()
			if got := vol.GetPeerID(); got != id {
				t.Fatalf("reopened volume peer %s, want %s", got, id)
			}
			for i, ref := range refs {
				got, found, err := vol.GetBlock(ctx, ref)
				if err != nil {
					t.Fatal(err)
				}
				if !found || !bytes.Equal(got, writerBlock("bolt", i)) {
					t.Fatalf("reopened block %d found %v", i, found)
				}
			}
		})
	}
}

// lateKey is the key a bolt writer commits while the conversion holds the
// bolt file.
var lateKey = []byte("late-acknowledged")

// commitBolt opens path as a bolt writer, waiting for its lock, and commits
// lateKey. It closes opened once it holds the bolt file open.
func commitBolt(path string, opened chan struct{}) error {
	var once sync.Once
	db, err := bdb.Open(path, 0o600, &bdb.Options{
		OpenFile: func(name string, flag int, perm os.FileMode) (*os.File, error) {
			f, err := os.OpenFile(name, flag, perm) // #nosec G304 -- the test's own file.
			once.Do(func() { close(opened) })
			return f, err
		},
	})
	if err != nil {
		return err
	}
	err = db.Update(func(tx *bdb.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("hydra"))
		if err != nil {
			return err
		}
		return b.Put(lateKey, []byte("committed while converting"))
	})
	return errors.Join(err, db.Close())
}

// convertUntil converts the bolt file at path and exits the process with
// boltCrashExit right after the named step, as a kill there would.
func convertUntil(t *testing.T, path, stop string) {
	volume_s4db.SetBoltStep(func(step string) {
		if step == stop {
			os.Exit(boltCrashExit)
		}
	})
	_, err := volume_s4db.NewVolume(t.Context(), logrus.NewEntry(logrus.New()), &volume_s4db.Config{Path: path})
	t.Fatalf("conversion returned past step %s: %v", stop, err)
}

// newBoltVolume creates a bolt Volume holding ten blocks and closes it. It
// returns the file's path, the Volume's peer ID and the block refs.
func newBoltVolume(t *testing.T) (string, peer.ID, []*block.BlockRef) {
	t.Helper()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	bolt, err := volume_bolt.NewBolt(ctx, logrus.NewEntry(logrus.New()), &volume_bolt.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	id := bolt.GetPeerID()
	refs := make([]*block.BlockRef, 10)
	for i := range refs {
		refs[i], _, err = bolt.PutBlock(ctx, writerBlock("bolt", i), &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := bolt.Close(); err != nil {
		t.Fatal(err)
	}
	return path, id, refs
}
