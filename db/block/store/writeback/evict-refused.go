package block_store_writeback

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// keptRefusal returns the label kept for pack id when its remote objects
// still have identity ident, else "", and whether the pack has a kept row at
// all. An empty ident matches nothing. Costs one point read of the meta
// store.
func (s *Store) keptRefusal(ctx context.Context, id, ident string) (string, bool, error) {
	// Read the pack's row.
	tx, err := s.markers.NewTransaction(ctx, false)
	if err != nil {
		return "", false, err
	}
	defer tx.Discard()
	value, found, err := tx.Get(ctx, []byte(evictRefusedPrefix+id))
	if err != nil || !found {
		return "", false, errors.Wrap(err, "read kept refusal")
	}

	// Match the identity it was refused at.
	label, kept, ok := strings.Cut(string(value), "\n")
	if !ok || ident == "" || kept != ident || !evictLastingRefusals[label] {
		return "", true, nil
	}
	return label, true, nil
}

// keepRefusal records that pack id was refused with label while its remote
// objects had identity ident, when the label is lasting and the identity
// names something; otherwise it deletes the pack's row when found says it
// has one, as after the pack verifies. Writes at most one row.
func (s *Store) keepRefusal(ctx context.Context, id, label, ident string, found bool) error {
	// Choose the write.
	keep := ident != "" && evictLastingRefusals[label]
	if !keep && !found {
		return nil
	}

	// Write or delete the row.
	key := []byte(evictRefusedPrefix + id)
	err := retryBusy(ctx, func() error {
		return kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
			if !keep {
				return tx.Delete(ctx, key)
			}
			return tx.Set(ctx, key, []byte(label+"\n"+ident))
		}, kvtx.RetryInvalidSnapshot)
	})
	return errors.Wrap(err, "keep pack refusal")
}
