//go:build !js && !wasip1

package world_recovery

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// inventory copies one bounded source record and advances its cursor atomically.
func (s *Scanner) inventory(ctx context.Context) error {
	// Finish physical inventory before any typed reader opens scratch blocks.
	if int(s.result.GetTable()) == len(s.readers) {
		return s.db.Update(func(tx *scratchTx) error {
			// Commit the physical record, occurrence and matching inventory cursor.
			for i, w := range s.source.GetWorlds() {
				node := &Node{Ref: w.GetRef().GetRootRef(), World: uint32(i), Constructor: "world", Context: w.GetRef()}
				if err := putNode(tx, node); err != nil {
					return err
				}
				if w.GetCurrent() {
					ref, err := w.GetRef().GetRootRef().MarshalKey()
					if err != nil {
						return err
					}
					if err := tx.Bucket([]byte("roots")).Put(ref, []byte{1}); err != nil {
						return err
					}
					currentKey := append(keyNode(node), binary.BigEndian.AppendUint32(nil, uint32(i))...)
					if err := tx.Bucket([]byte("currentreach")).Put(currentKey, []byte{1}); err != nil {
						return err
					}
					if err := tx.Bucket([]byte("currentqueue")).Put(currentKey, []byte{1}); err != nil {
						return err
					}
				}
			}
			s.result.Phase = Phase_PHASE_INTERPRET
			s.result.Cursor = nil
			return s.save(tx)
		})
	}

	// Reject unsupported store formats without invoking their upgrade paths.
	reader := s.readers[s.result.GetTable()]
	if s.opts.BeforeRead != nil {
		if err := s.opts.BeforeRead(); err != nil {
			return err
		}
	}
	if s.result.GetCursor() == nil {
		data, found, err := reader.Get(ctx, store_kvkey.NewDefaultKVKey().GetFormatVersionKey())
		if err != nil {
			return err
		}
		format, parseErr := store_kvkey.ParseFormatVersion(data)
		if !found || parseErr != nil || format < 7 || format > 9 {
			return errors.New("source table is not a supported Volume format (7 through 9)")
		}
	}
	key, value, err := reader.Next(ctx, s.result.GetCursor())
	if err != nil {
		return err
	}
	if key != nil && len(key) == 0 {
		return errors.New("source Volume has an unsupported empty record key")
	}

	// Commit a record, its location and its continuation in one scratch write.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit the physical record, occurrence and matching inventory cursor.
		if key == nil {
			s.result.Table++
			s.result.Cursor = nil
			return s.save(tx)
		}

		// Verify ordinary content before recording its location and cursor.
		prefix := store_kvkey.NewDefaultKVKey().GetBlockFullPrefix()
		if encoded, ok := bytes.CutPrefix(key, prefix); ok {
			ref := &block.BlockRef{}
			if err := ref.UnmarshalVT(encoded); err != nil {
				return err
			}
			if err := ref.Validate(false); err != nil {
				return err
			}
			if err := block.VerifyRead(ref, value, "recovery inventory"); err != nil {
				return err
			}
			canonical, err := ref.MarshalKey()
			if err != nil {
				return err
			}
			if tx.Bucket([]byte("data")).Get(key) == nil {
				if err := tx.Bucket([]byte("data")).Put(key, value); err != nil {
					return err
				}
				s.result.Counts.PhysicalBlocks++
				s.result.Counts.PhysicalBytes += uint64(len(value))
				obj := &world_block.Object{}
				if obj.UnmarshalVT(value) == nil && obj.GetKey() != "" && obj.GetRootRef().Validate() == nil && !obj.GetRootRef().GetRootRef().GetEmpty() {
					if err := tx.Bucket([]byte("unbound")).Put(canonical, nil); err != nil {
						return err
					}
				}
			}
			location := append(binary.BigEndian.AppendUint32(nil, s.result.GetTable()), canonical...)
			if tx.Bucket([]byte("locations")).Get(location) == nil {
				if err := tx.Bucket([]byte("locations")).Put(location, []byte{1}); err != nil {
					return err
				}
				s.result.Counts.PhysicalLocations++
			}
		} else {
			if err := tx.Bucket([]byte("meta")).Put(append(binary.BigEndian.AppendUint32(nil, s.result.GetTable()), key...), value); err != nil {
				return err
			}
			if bytes.HasPrefix(key, []byte{2, 'R'}) {
				root := &block_gc.RootEntry{}
				if err := root.UnmarshalVT(value); err != nil {
					return err
				}
				ref, rest, err := block_gc.ParseBlockKey(root.GetRef())
				if err != nil || len(rest) != 0 {
					return errors.New("invalid captured named root")
				}
				encoded, err := ref.MarshalKey()
				if err != nil {
					return err
				}
				bucketID, _, err := block_gc.ParseNamedKey(key[2:])
				if err != nil {
					return err
				}
				typedRoot := false
				for _, input := range s.source.GetWorlds() {
					if input.GetRef().GetBucketId() == bucketID && input.GetRef().GetRootRef().EqualsRef(ref) {
						typedRoot = true
						break
					}
				}
				if !typedRoot {
					if err := tx.Bucket([]byte("state")).Put([]byte("reach-incomplete"), []byte{1}); err != nil {
						return err
					}
				}
				if err := tx.Bucket([]byte("roots")).Put(encoded, []byte{1}); err != nil {
					return err
				}
			}
		}
		s.result.Cursor = bytes.Clone(key)
		return s.save(tx)
	})
}
