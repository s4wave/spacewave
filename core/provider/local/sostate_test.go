package provider_local

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// It returns the state, its genesis config change and the owner key.
func seedTestGenesisState(t *testing.T, store kvtx.Store) (*sobject.SOState, *sobject.SOConfigChange, crypto.PrivKey) {
	// Build the genesis state of a new owner.
	t.Helper()
	priv, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	state, genesis, err := sobject.BuildGenesisSOState(le, testStepFactorySet(), testSharedObjectID, priv, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Commit the encoded state and its history.
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		if err := WriteSOConfigHistory(ctx, tx, testSharedObjectID, genesis.GetConfig(), state.GetConfig(), []*sobject.SOConfigChange{genesis}); err != nil {
			return err
		}
		return tx.Set(ctx, SobjectObjectStoreHostStateKey(testSharedObjectID), data)
	}); err != nil {
		t.Fatal(err)
	}
	return state, genesis, priv
}

// newTestGenesisHost returns a host over store seeded with the genesis state
// of a shared object owned by a fresh key, and that key.
func newTestGenesisHost(t *testing.T, store kvtx.Store) (*sobject.SOHost, crypto.PrivKey) {
	// Seed a genesis store and host it.
	t.Helper()
	_, _, priv := seedTestGenesisState(t, store)
	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(t.Context(), store, "", &SOStateRuns{Reload: func() {}})
	host := sobject.NewSOHost(t.Context(), watch, lock, testSharedObjectID, syncFuncs)
	t.Cleanup(host.ClearContext)
	return host, priv
}

// testStepFactorySet returns the step factories of the default block transform.
func testStepFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	return sfs
}
