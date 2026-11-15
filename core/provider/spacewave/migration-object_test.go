package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// TestCloudMigrationFlushBeforeTransfer keeps pending writes under their source
// authority until the cloud has accepted them, including a failed flush.
func TestCloudMigrationFlushBeforeTransfer(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "published"
		if fail {
			name = "flush-failed"
		}
		t.Run(name, func(t *testing.T) {
			var flushed, transferred atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/resource/transfer" || r.Method != http.MethodPost {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if !flushed.Load() {
					t.Error("resource authority changed before pending writes were published")
				}
				transferred.Store(true)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			source := NewTestProviderAccount(t, server.URL)
			destination := NewTestProviderAccount(t, server.URL)
			flushErr := errors.New("cloud upload interrupted")
			object := &SharedObject{tkr: &sobjectTracker{id: "pending-space"}, blkStore: &BlockStore{forceSync: func(context.Context) error {
				if fail {
					return flushErr
				}
				flushed.Store(true)
				return nil
			}}}
			err := destination.ImportMigrationObject(t.Context(), source, object, nil, nil)
			if fail {
				if !errors.Is(err, flushErr) || transferred.Load() {
					t.Fatalf("failed flush changed resource authority: transferred=%v err=%v", transferred.Load(), err)
				}
			} else if err != nil || !transferred.Load() {
				t.Fatalf("published resource did not transfer: transferred=%v err=%v", transferred.Load(), err)
			}
		})
	}
}

// TestCloudMigrationHeldBlockFails checks that a same-cloud migration whose
// source store holds a dirty block that reads corrupt fails before the
// transfer: ForceSync reports the held block, no checkpoint is published, the
// account epoch does not move and the source keeps the block dirty.
func TestCloudMigrationHeldBlockFails(t *testing.T) {
	var transferred, checkpoints atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/resource/transfer":
			transferred.Add(1)
		case strings.HasSuffix(r.URL.Path, "/checkpoint"):
			checkpoints.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Read the source's only dirty block as corrupt before the flush.
	_, key := newTestGenesisState(t)
	syncer := newDirtySyncExecuteTestController(t, newTestPublicationClient(server, key, mustPeerID(t, key)), nil)
	ref, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	syncer.upper = &corruptUpperStore{StoreOps: syncer.upper, bad: ref.GetHash()}
	source := NewTestProviderAccount(t, server.URL)
	destination := NewTestProviderAccount(t, server.URL)
	epoch := destination.state.epoch
	object := &SharedObject{tkr: &sobjectTracker{id: "held-space"}, blkStore: &BlockStore{forceSync: syncer.FlushNowUnordered}}

	err = destination.ImportMigrationObject(t.Context(), source, object, nil, nil)
	if !errors.Is(err, block.ErrBlockCorrupt) {
		t.Fatalf("migration = %v; want the held block error", err)
	}
	if transferred.Load() != 0 || checkpoints.Load() != 0 || destination.state.epoch != epoch {
		t.Fatalf("transferred = %d, checkpoints = %d, epoch %d -> %d; want nothing moved",
			transferred.Load(), checkpoints.Load(), epoch, destination.state.epoch)
	}
	if pending, err := syncer.scanDirtyCandidates(t.Context()); err != nil || len(pending) != 1 {
		t.Fatalf("source dirty = %d blocks, %v; want the held block", len(pending), err)
	}
}

// repeatedImportObject is a local source object whose signed history is one
// change.
type repeatedImportObject struct {
	sobject.SharedObject
}

func (repeatedImportObject) GetSharedObjectID() string { return "repeated-space" }

func (repeatedImportObject) ReadSharedObjectFullConfigHistory(context.Context, *sobject.SharedObjectConfig) ([]*sobject.SOConfigChange, error) {
	return []*sobject.SOConfigChange{{ConfigSeqno: 1}}, nil
}

// TestCloudMigrationRepeatFails checks that an import into a cloud that holds
// the object under another config, as after an interrupted import and a config
// change, fails with ErrMigrationTargetExists and posts nothing more, since the
// held copy lacks that change.
func TestCloudMigrationRepeatFails(t *testing.T) {
	var after atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path.Base(r.URL.Path) != "create-with-state" {
			after.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"initialization_conflict","message":"Shared object generation or ownership conflicts with the request"}`))
	}))
	defer server.Close()
	destination := NewTestProviderAccount(t, server.URL)
	entry := &sobject.SharedObjectListEntry{Meta: &sobject.SharedObjectMeta{BodyType: "notes"}}
	err := destination.ImportMigrationObject(t.Context(), nil, repeatedImportObject{}, entry, &sobject.SOState{Checkpoint: &sobject.SOCheckpoint{Inner: []byte("held")}})
	if !errors.Is(err, ErrMigrationTargetExists) || after.Load() != 0 {
		t.Fatalf("repeated import returned %v after %d more requests, want ErrMigrationTargetExists and none", err, after.Load())
	}
}
