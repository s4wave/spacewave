//go:build !js && !wasip1

package provider_local

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// MeasureDeleteStateEnv names the environment variable that, set to 1, runs
// TestMeasureDeleteStateWriterHold.
const MeasureDeleteStateEnv = "SPACEWAVE_MEASURE_DELETE_STATE"

// measureDeleteMembers is the member count of the measured config.
const measureDeleteMembers = 400

// TestMeasureDeleteStateWriterHold measures how long the delete of an object's
// stored state holds the account store's writer on the desktop's on-disk
// volume, for objects with 500 and 5,000 config-history entries, each holding
// a full config of measureDeleteMembers members. The entries are written,
// made durable, and the volume reopened before the delete, as for a
// long-lived object. It runs only when MeasureDeleteStateEnv is 1.
func TestMeasureDeleteStateWriterHold(t *testing.T) {
	if os.Getenv(MeasureDeleteStateEnv) != "1" {
		t.Skip("set " + MeasureDeleteStateEnv + "=1 to measure")
	}
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	conf := &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "volume.s4wave")}
	counts := []int{500, 5000}

	// Fill one object per count with its entries, durably.
	value := measureConfigChange(t, 0, nil)
	t.Logf("config-history entry: %d bytes for %d members", len(value), measureDeleteMembers)
	vol, store, release := openMeasureStore(ctx, t, le, conf)
	for _, n := range counts {
		fillMeasureHistory(ctx, t, store, fmt.Sprintf("measure%d", n), n)
	}
	if err := volume.WaitDurable(ctx, vol); err != nil {
		t.Fatal(err)
	}
	release()

	// Reopen the volume and time each delete transaction from open to commit.
	_, store, release = openMeasureStore(ctx, t, le, conf)
	defer release()
	for _, n := range counts {
		prefix := SobjectObjectStorePrefix(fmt.Sprintf("measure%d", n))
		start := time.Now()
		err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			return deleteKeysWithPrefix(ctx, tx, prefix)
		})
		held := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("delete of %d config-history entries held the writer %v", n, held)
	}
}

// openMeasureStore opens the on-disk volume at conf and an object store in it.
func openMeasureStore(ctx context.Context, t *testing.T, le *logrus.Entry, conf *volume_s4db.Config) (volume.Volume, object.ObjectStore, func()) {
	t.Helper()
	vol, err := volume_s4db.NewVolume(ctx, le, conf)
	if err != nil {
		t.Fatal(err)
	}
	store, rel, err := vol.AccessObjectStore(ctx, "measure", nil)
	if err != nil {
		_ = vol.Close()
		t.Fatal(err)
	}
	return vol, store, func() {
		rel()
		if err := vol.Close(); err != nil {
			t.Error(err)
		}
	}
}

// fillMeasureHistory writes n config-history entries for id, in batches.
func fillMeasureHistory(ctx context.Context, t *testing.T, store object.ObjectStore, id string, n int) {
	t.Helper()
	config := measureConfig(t)
	const batch = 500
	for first := 0; first < n; first += batch {
		err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			for i := first; i < min(first+batch, n); i++ {
				sum := sha256.Sum256(fmt.Appendf(nil, "%s/%d", id, i))
				value := measureConfigChange(t, uint64(i), config)
				if err := tx.Set(ctx, SOConfigHistoryEntryKey(id, sum[:]), value); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// measureConfig builds a config of measureDeleteMembers members with fake
// identities.
func measureConfig(t *testing.T) *sobject.SharedObjectConfig {
	t.Helper()
	config := &sobject.SharedObjectConfig{}
	for i := range measureDeleteMembers {
		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pid, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			t.Fatal(err)
		}
		config.Participants = append(config.Participants, &sobject.SOParticipantConfig{
			PeerId:       pid.String(),
			Role:         sobject.SOParticipantRole_SOParticipantRole_WRITER,
			EntityId:     fmt.Sprintf("fake-entity-%04d", i),
			Username:     fmt.Sprintf("fake-member-%04d", i),
			VotingWeight: 1,
		})
	}
	return config
}

// measureConfigChange returns a serialized config change at seqno carrying
// config and one signature, building the config when it is nil.
func measureConfigChange(t *testing.T, seqno uint64, config *sobject.SharedObjectConfig) []byte {
	t.Helper()
	if config == nil {
		config = measureConfig(t)
	}
	prev := sha256.Sum256(fmt.Appendf(nil, "fake-previous-%d", seqno))
	change := &sobject.SOConfigChange{
		ConfigSeqno:    seqno,
		Config:         config,
		PreviousHash:   prev[:],
		ChangeType:     sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
		SharedObjectId: "fake-object",
		Signatures: []*peer.Signature{{
			PubKey:   make([]byte, 32),
			HashType: hash.HashType_HashType_SHA256,
			SigData:  make([]byte, 64),
		}},
	}
	data, err := change.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return data
}
