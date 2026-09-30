//go:build !js

package cli_entrypoint

import (
	"context"
	"errors"
	"testing"

	bbolt_errors "github.com/aperturerobotics/bbolt/errors"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/sirupsen/logrus"
)

// TestReleaseClosesStorage checks that caller cleanup runs after storage closes.
func TestReleaseClosesStorage(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		name := "active parent"
		if cancelParent {
			name = "canceled parent"
		}
		t.Run(name, func(t *testing.T) {
			// Build the CLI bus and resolve its Bolt database.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			b, err := BuildCliBus(ctx, logrus.NewEntry(logrus.New()), "test", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer b.Release()
			db := volume_bolt.GetBoltDB(b.GetVolume())
			if db == nil {
				t.Fatal("CLI storage did not provide a Bolt database")
			}

			// Verify on release that caller cleanup runs after storage closes.
			b.AddRelease(func() {
				if b.GetContext().Err() == nil {
					t.Error("caller cleanup ran before bus cancellation")
				}
				tx, err := db.Begin(false)
				if tx != nil {
					_ = tx.Rollback()
				}
				if !errors.Is(err, bbolt_errors.ErrDatabaseNotOpen) {
					t.Errorf("caller cleanup ran before database close: %v", err)
				}
			})
			if cancelParent {
				cancel()
			}
			b.Release()
		})
	}
}
