//go:build !js

package storage_volume

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// reopenTimeout bounds the second open. The loader retry becomes ready within
// this window; a caller holding the failed controller waits out the context.
const reopenTimeout = 4 * time.Second

// openInProcessHook reports the first log entry for an in-process database open.
type openInProcessHook struct {
	// seen is closed when a log entry reports the in-process open failure.
	seen chan struct{}
}

// Levels returns every level so a warning from the loader or the controller is observed.
func (h *openInProcessHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire closes seen when entry reports that the database is already open.
func (h *openInProcessHook) Fire(entry *logrus.Entry) error {
	// Ignore unrelated entries.
	if !entryReportsOpenInProcess(entry) {
		return nil
	}

	// Close seen once. The logger serializes hooks, so a second fire finds it closed.
	select {
	case <-h.seen:
	default:
		close(h.seen)
	}
	return nil
}

// entryReportsOpenInProcess reports whether entry carries the in-process open error.
func entryReportsOpenInProcess(entry *logrus.Entry) bool {
	text := entry.Message
	if err, ok := entry.Data[logrus.ErrorKey]; ok {
		text += " " + fmt.Sprint(err)
	}
	return strings.Contains(text, s4db.ErrOpenInProcess.Error())
}

// TestExecVolumeControllerReopenAfterInProcessOpen builds, releases, and rebuilds
// one retained volume through ExecVolumeController and GetVolume. The second open
// fails once because the database is still claimed, then the claim clears and the
// retry becomes ready. GetVolume must return that ready volume instead of waiting
// out the caller's context.
func TestExecVolumeControllerReopenAfterInProcessOpen(t *testing.T) {
	// Log controller warnings so the open failure can release the retained claim.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	hook := &openInProcessHook{seen: make(chan struct{})}
	log.AddHook(hook)
	le := logrus.NewEntry(log)
	tmpDir := t.TempDir()
	const volumeID = "retained"

	// Build the retained volume through the production controller APIs.
	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	firstCtrl, firstRef, err := openStorageVolume(firstCtx, t, le, tmpDir, volumeID)
	if err != nil {
		t.Fatal(err)
	}

	// Record the volume id, then release the controller so the file can be claimed.
	firstVol, err := firstCtrl.GetVolume(firstCtx)
	if err != nil {
		firstRef.Release()
		t.Fatal(err)
	}
	firstID := firstVol.GetID()
	firstRef.Release()
	firstCancel()

	// Hold the retained file so the next open fails once, then let the retry proceed.
	hold := claimRetainedVolume(t, tmpDir, volumeID)
	go func() {
		<-hook.seen
		_ = hold.Close()
	}()

	// Rebuild through the same APIs. A lost readiness transition expires this context.
	ctx, cancel := context.WithTimeout(context.Background(), reopenTimeout)
	defer cancel()
	ctrl, ref, err := openStorageVolume(ctx, t, le, tmpDir, volumeID)
	if err != nil {
		t.Fatalf("reopen controller: %v", err)
	}
	defer ref.Release()
	vol, err := ctrl.GetVolume(ctx)
	if err != nil {
		t.Fatalf("reopen after in-process open: %v", err)
	}
	if vol.GetID() != firstID {
		t.Fatalf("reopened volume id %q, want retained id %q", vol.GetID(), firstID)
	}
}

// openStorageVolume starts a bus and returns the running storage volume controller.
func openStorageVolume(
	ctx context.Context,
	t *testing.T,
	le *logrus.Entry,
	rootDir, volumeID string,
) (volume.Controller, directive.Reference, error) {
	t.Helper()

	// Start the core bus and register the storage volume controller factory.
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, nil, err
	}
	sr.AddFactory(NewFactory(b))

	// Attach the default storage controller for the retained directory.
	storageCtrl := default_storage.NewController(default_storage.StorageID, b, rootDir)
	relStorage, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		return nil, nil, err
	}
	t.Cleanup(relStorage)
	for _, method := range storageCtrl.GetStorage() {
		method.AddFactories(b, sr)
	}

	// Wait for the volume controller the devtool bus acquires.
	return ExecVolumeController(ctx, b, &Config{
		StorageId:       default_storage.StorageID,
		StorageVolumeId: volumeID,
	})
}

// claimRetainedVolume opens the retained volume file, waiting out the previous close.
func claimRetainedVolume(t *testing.T, rootDir, volumeID string) *s4db.DB {
	t.Helper()

	// Resolve the retained volume file.
	path, err := storage_native.VolumePath(rootDir, volumeID)
	if err != nil {
		t.Fatal(err)
	}

	// Take the file once the first controller has released it.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		db, err := s4db.Open(path, s4db.Options{})
		if err == nil {
			return db
		}
		if !errors.Is(err, s4db.ErrOpenInProcess) {
			t.Fatal(err)
		}
		waiter, ok := err.(interface {
			Wait(context.Context) error
		})
		if !ok {
			t.Fatalf("in-process open error %T cannot be waited out", err)
		}
		if err := waiter.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
