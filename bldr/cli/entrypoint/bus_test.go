//go:build !js

package cli_entrypoint

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/s4db"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
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
			// Build the CLI bus and resolve its s4db database.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			b, err := BuildCliBus(ctx, logrus.NewEntry(logrus.New()), "test", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer b.Release()
			db := volume_s4db.GetDB(b.GetVolume())
			if db == nil {
				t.Fatal("CLI storage did not provide an s4db database")
			}

			// Verify on release that caller cleanup runs after storage closes.
			b.AddRelease(func() {
				if b.GetContext().Err() == nil {
					t.Error("caller cleanup ran before bus cancellation")
				}
				tx, err := db.NewTransaction(context.Background(), false)
				if tx != nil {
					tx.Discard()
				}
				if !errors.Is(err, s4db.ErrClosed) {
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
