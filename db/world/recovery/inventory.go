//go:build !js && !wasip1

package world_recovery

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"strconv"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// inventory copies one bounded source record and advances its cursor atomically.
func (s *Scanner) inventory(ctx context.Context) error {
	// Read the pack tiers after the tables, then finish physical inventory
	// before any typed reader opens scratch blocks.
	if int(s.result.GetTable()) == len(s.readers) && int(s.result.GetPackDir()) < len(s.source.GetPacks()) {
		return s.inventoryPack(ctx)
	}
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
	var legacy uint64
	if s.result.GetCursor() == nil {
		data, found, err := reader.Get(ctx, store_kvkey.NewDefaultKVKey().GetFormatVersionKey())
		if err != nil {
			return err
		}
		format, parseErr := store_kvkey.ParseFormatVersion(data)
		if !found {
			// A store without a version key predates versioning: format 0.
			format, parseErr = 0, nil
		}
		if parseErr != nil || format > store_kvkey.FormatVersion {
			return errors.New("source table is not a supported Volume format (0 through " + strconv.Itoa(store_kvkey.FormatVersion) + ")")
		}
		if format < 7 {
			legacy = format + 1
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
		if legacy != 0 {
			if err := tx.Bucket([]byte("state")).Put([]byte("reach-incomplete"), []byte{1}); err != nil {
				return err
			}
			s.addGap("store format " + strconv.FormatUint(legacy-1, 10) + " keeps its named roots in legacy graph records, which are not read; non-reachability is unknown")
		}
		if key == nil {
			s.result.Table++
			s.result.Cursor = nil
			return s.save(tx)
		}

		// Verify ordinary content before recording its location and cursor.
		prefix := store_kvkey.NewDefaultKVKey().GetBlockFullPrefix()
		if encoded, ok := bytes.CutPrefix(key, prefix); ok {
			// Format 0 keyed a block by the base58 text of its ref bytes.
			if len(encoded) != 0 && encoded[0] != binaryBlockRefTag {
				decoded, err := b58.Decode(string(encoded))
				if err != nil {
					return err
				}
				encoded = decoded
			}
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
			if err := s.recordBlock(tx, canonical, value, append(binary.BigEndian.AppendUint32(nil, s.result.GetTable()), canonical...)); err != nil {
				return err
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

// binaryBlockRefTag begins every marshaled BlockRef: the tag of its hash
// field. Base58 text never contains it.
const binaryBlockRefTag = 0x0a

// recordBlock records one verified occurrence of a block: its bytes once per
// canonical reference, an unbound Object probe, and its physical location.
func (s *Scanner) recordBlock(tx *scratchTx, canonical, value, location []byte) error {
	// Store distinct bytes once under the key the typed readers use.
	key := store_kvkey.NewDefaultKVKey().GetBlockKey(canonical)
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

	// Count each distinct location of the same bytes.
	if tx.Bucket([]byte("locations")).Get(location) == nil {
		if err := tx.Bucket([]byte("locations")).Put(location, []byte{1}); err != nil {
			return err
		}
		s.result.Counts.PhysicalLocations++
	}
	return nil
}

// addGap appends a coverage gap once.
func (s *Scanner) addGap(gap string) {
	if !slices.Contains(s.result.GetGaps(), gap) {
		s.result.Gaps = append(s.result.Gaps, gap)
	}
}
