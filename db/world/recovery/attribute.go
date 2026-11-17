//go:build !js && !wasip1

package world_recovery

import (
	"bytes"
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// attribute advances one native World membership at a time, separately from shared decoding.
func (s *Scanner) attribute(ctx context.Context) error {
	// Recover the per-World membership continuation from scratch.
	index := s.result.GetProgress().GetMembershipWorld()
	after := s.result.GetProgress().GetMembershipAfter()
	if int(index) == len(s.source.GetWorlds()) {
		return s.linkObject()
	}
	input := s.source.GetWorlds()[index]
	if err := validateContext(input.GetRef()); err != nil {
		return s.skipWorld(index, err)
	}

	// Finish native reads before opening the scratch writer.
	version, node, err := func() (*Version, *Node, error) {
		// Read native metadata directly, without the Engine's guarded transaction handles.
		wt, release, err := s.openWorld(ctx, index)
		if err != nil {
			return nil, nil, err
		}
		defer release()

		// Seek directly beyond the last committed object key.
		iterator := wt.IterateObjects(ctx, "", false)
		defer iterator.Close()
		if after == "" {
			iterator.Next()
		} else {
			if err := iterator.Seek(after); err != nil {
				return nil, nil, err
			}
			if iterator.Valid() && iterator.Key() == after {
				iterator.Next()
			}
		}
		if err := iterator.Err(); err != nil {
			return nil, nil, err
		}
		if !iterator.Valid() {
			return nil, nil, nil
		}

		// Read the exact object and release its native handle.
		key := iterator.Key()
		object, found, err := wt.GetObject(ctx, key)
		defer world.ReleaseObjectState(object)
		if err != nil {
			return nil, nil, err
		}

		// Require native metadata whose key agrees with the typed tree.
		typed, ok := object.(*world_block.ObjectState)
		if !found || !ok {
			return nil, nil, errors.New("typed World iterator returned an unavailable object")
		}
		metadata, err := typed.GetRoot(ctx)
		if err != nil {
			return nil, nil, err
		}
		if metadata.GetKey() != key {
			return nil, nil, errors.New("object metadata disagrees with its tree key")
		}

		// Preserve metadata when the historical type graph is unavailable.
		normalized := metadata.CloneVT()
		normalized.RootRef = resolveContext(input.GetRef(), metadata.GetRootRef())
		typeID, typeErr := readObjectType(ctx, wt, key)
		ctor := "unknown"
		if typeErr == nil && typeID != "" {
			ctor = "type:" + typeID
		}

		// Return owned metadata after the scoped native readers close.
		node := &Node{Ref: normalized.GetRootRef().GetRootRef(), World: index, Context: normalized.GetRootRef(), Constructor: ctor}
		version := &Version{SpaceId: input.GetSpaceId(), Object: normalized, World: index}
		return version, node, nil
	}()
	if err != nil || version == nil {
		return s.skipWorld(index, err)
	}
	key := version.GetObject().GetKey()
	root := &Node{Ref: input.GetRef().GetRootRef(), Context: input.GetRef(), Constructor: "world"}

	// Attach provenance to the shared body closure without retaining World-wide maps.
	return s.db.Update(func(tx *scratchTx) error {
		// Preserve one native membership or its bounded alias continuation.
		if !node.GetRef().GetEmpty() {
			node.Parent = keyNode(root)
			if err := putNode(tx, node); err != nil {
				return err
			}
			if err := recordBodyInterpretation(tx, "nodeversion", versionKey(version), keyNode(node), "ambiguousversions"); err != nil {
				return err
			}
			if err := recordBodyInterpretation(tx, "objectstatus", objectIdentity(version.GetObject(), input.GetRef()), keyNode(node), "ambiguousobjects"); err != nil {
				return err
			}
		}
		if err := s.putVersion(tx, node, version); err != nil {
			return err
		}
		s.result.Progress.MembershipAfter = key
		return s.save(tx)
	})
}

// linkObject connects one metadata interpretation to its proven typed body closure.
func (s *Scanner) linkObject() error {
	// Scan the disk relation once after all World memberships have supplied context.
	key, _, err := nextRow(s.db, "objectlinks", s.result.GetCursor())
	if err != nil {
		return err
	}
	if key == nil {
		return s.beginTraversal()
	}

	// Resolve the metadata alias with completed membership evidence.
	return s.db.Update(func(tx *scratchTx) error {
		// Preserve one native membership or its bounded alias continuation.
		parent := key[32:]
		child := tx.Bucket([]byte("objectstatus")).Get(key[:32])
		if tx.Bucket([]byte("ambiguousobjects")).Get(key[:32]) != nil {
			if err := tx.Bucket([]byte("bad")).Put(parent, []byte("historical body interpretations disagree")); err != nil {
				return err
			}
			if err := tx.Bucket([]byte("badqueue")).Put(parent, []byte{1}); err != nil {
				return err
			}
		}
		if len(child) == 32 {
			if err := tx.Bucket([]byte("edges")).Put(append(bytes.Clone(parent), child...), []byte{1}); err != nil {
				return err
			}
			if err := tx.Bucket([]byte("parents")).Put(append(bytes.Clone(child), parent...), []byte{1}); err != nil {
				return err
			}
		} else if child == nil {
			if err := tx.Bucket([]byte("bad")).Put(parent, []byte("body type unavailable in every captured World")); err != nil {
				return err
			}
			if err := tx.Bucket([]byte("badqueue")).Put(parent, []byte{1}); err != nil {
				return err
			}
		}
		s.result.Cursor = key
		return s.save(tx)
	})
}

// recordBodyInterpretation retains disagreement instead of selecting the last World's constructor.
func recordBodyInterpretation(tx *scratchTx, index string, key, body []byte, ambiguity string) error {
	// An exact version or metadata alias remains ambiguous when its typed contexts disagree.
	prior := tx.Bucket([]byte(index)).Get(key)
	if prior == nil {
		return tx.Bucket([]byte(index)).Put(key, body)
	}
	if !bytes.Equal(prior, body) {
		return tx.Bucket([]byte(ambiguity)).Put(key, []byte{1})
	}
	return nil
}

// skipWorld preserves an unavailable membership as uncertainty before continuing.
func (s *Scanner) skipWorld(index uint32, cause error) error {
	// A failed current World cannot certify absence of one of its object versions.
	return s.db.Update(func(tx *scratchTx) error {
		// Preserve one native membership or its bounded alias continuation.
		state := tx.Bucket([]byte("state"))
		if cause != nil {
			if err := state.Put(worldKey(index, "membership-incomplete"), []byte{1}); err != nil {
				return err
			}
			s.result.Gaps = append(s.result.Gaps, "World membership incomplete: "+s.source.GetWorlds()[index].GetSpaceId()+": "+cause.Error())
			if s.source.GetWorlds()[index].GetCurrent() {
				if err := state.Put([]byte("current-incomplete/"+s.source.GetWorlds()[index].GetSpaceId()), []byte{1}); err != nil {
					return err
				}
			}
			node := &Node{Ref: s.source.GetWorlds()[index].GetRef().GetRootRef(), Context: s.source.GetWorlds()[index].GetRef(), Constructor: "world"}
			key := keyNode(node)
			if err := tx.Bucket([]byte("bad")).Put(key, []byte(cause.Error())); err != nil {
				return err
			}
			if err := tx.Bucket([]byte("badqueue")).Put(key, []byte{1}); err != nil {
				return err
			}
		}

		// Advance only this World after preserving incomplete membership.
		s.result.Progress.MembershipWorld = index + 1
		s.result.Progress.MembershipAfter = ""
		return s.save(tx)
	})
}
