//go:build !js && !wasip1

package world_recovery

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"

	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// beginTraversal starts closure propagation only after every typed node is indexed.
func (s *Scanner) beginTraversal() error {
	return s.db.Update(func(tx *scratchTx) error {
		// Commit this graph slice and its exact active-node continuation.
		s.result.Phase = Phase_PHASE_TRAVERSE
		s.result.Cursor = nil
		s.result.Progress.Pass = Pass_PASS_BAD
		return s.save(tx)
	})
}

// traverse executes one bounded graph, root, witness or count step.
func (s *Scanner) traverse(ctx context.Context) error {
	// Each stage commits its own cursor together with its aggregate effects.
	switch s.result.GetProgress().GetPass() {
	case Pass_PASS_BAD:
		return s.propagate(false)
	case Pass_PASS_ROOTS:
		return s.seedRoot()
	case Pass_PASS_REACH:
		return s.propagate(true)
	case Pass_PASS_CURRENT:
		return s.propagateCurrent()
	case Pass_PASS_WITNESSES:
		return s.witness(ctx)
	case Pass_PASS_VERSIONS:
		return s.countVersion()
	case Pass_PASS_PHYSICAL:
		return s.countPhysical()
	default:
		return errors.New("invalid traversal subphase")
	}
}

// advanceStage changes the indexed pass and clears its continuation atomically.
func (s *Scanner) advanceStage(tx *scratchTx, stage Pass) error {
	s.result.Cursor = nil
	s.result.Progress.Pass = stage
	return s.save(tx)
}

// propagate follows at most 128 indexed edges per scratch transaction.
func (s *Scanner) propagate(reach bool) error {
	// Keep wide frontiers and high fan-in on disk, including the active edge cursor.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit this graph slice and its exact active-node continuation.
		queue, relation, visited := "badqueue", "parents", "bad"
		if reach {
			queue, relation, visited = "reachqueue", "edges", "reach"
		}
		q := tx.Bucket([]byte(queue))
		key, _ := q.Cursor().First()
		if key == nil {
			if reach {
				return s.advanceStage(tx, Pass_PASS_CURRENT)
			}
			return s.advanceStage(tx, Pass_PASS_ROOTS)
		}

		// Keep the active node stable while new queue entries sort before it.
		if active := s.result.GetProgress().GetActiveNode(); active != nil {
			key = active
		}
		key = bytes.Clone(key)
		s.result.Progress.ActiveNode = bytes.Clone(key)
		node := &Node{}
		if err := node.UnmarshalVT(tx.Bucket([]byte("nodes")).Get(key)); err != nil {
			return err
		}

		// Record physical reachability and missing descendants once per full ref.
		if reach {
			ref, err := node.GetRef().MarshalKey()
			if err != nil {
				return err
			}
			if err := tx.Bucket([]byte("reachblocks")).Put(ref, []byte{1}); err != nil {
				return err
			}
			physical := tx.Bucket([]byte("data")).Get(store_kvkey.NewDefaultKVKey().GetBlockKey(ref))
			if physical == nil {
				if err := s.putHole(tx, ref); err != nil {
					return err
				}
			}
		}

		// Resume the current node's edge stream after a crash or a wide adjacency.
		cursor := tx.Bucket([]byte(relation)).Cursor()
		after := s.result.GetProgress().GetEdgeAfter()
		edge, _ := cursor.Seek(key)
		if len(after) != 0 {
			edge, _ = cursor.Seek(after)
			if bytes.Equal(edge, after) {
				edge, _ = cursor.Next()
			}
		}
		for n := 0; n < 128 && bytes.HasPrefix(edge, key); n++ {
			child := bytes.Clone(edge[len(key):])
			if tx.Bucket([]byte(visited)).Get(child) == nil {
				if err := tx.Bucket([]byte(visited)).Put(child, []byte{1}); err != nil {
					return err
				}
				if err := q.Put(child, []byte{1}); err != nil {
					return err
				}
			}
			s.result.Progress.EdgeAfter = bytes.Clone(edge)
			edge, _ = cursor.Next()
		}
		if !bytes.HasPrefix(edge, key) {
			if err := q.Delete(key); err != nil {
				return err
			}
			s.result.Progress.EdgeAfter = nil
			s.result.Progress.ActiveNode = nil
		}
		return s.save(tx)
	})
}

// seedRoot finds typed interpretations for one captured held root.
func (s *Scanner) seedRoot() error {
	// A root without a complete interpretation keeps non-reachability unknown.
	ref, _, err := nextRow(s.db, "roots", s.result.GetCursor())
	if err != nil {
		return err
	}

	// Seed a bounded alias slice without trusting stored child lists.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit this graph slice and its exact active-node continuation.
		if ref == nil {
			return s.advanceStage(tx, Pass_PASS_REACH)
		}

		// Visit known interpretations after inventory has checked native bucket context.
		cursor := tx.Bucket([]byte("byref")).Cursor()
		found := false
		key, value := cursor.Seek(ref)
		after := s.result.GetProgress().GetRootAfter()
		if len(after) != 0 {
			key, value = cursor.Seek(after)
			if bytes.Equal(key, after) {
				key, value = cursor.Next()
			}
		}

		// Add at most one slice of interpretations for this held root.
		for n := 0; n < 128 && bytes.HasPrefix(key, ref); n++ {
			found = true
			if tx.Bucket([]byte("bad")).Get(value) != nil {
				if err := tx.Bucket([]byte("state")).Put([]byte("reach-incomplete"), []byte{1}); err != nil {
					return err
				}
			}
			if err := tx.Bucket([]byte("reach")).Put(value, []byte{1}); err != nil {
				return err
			}
			if err := tx.Bucket([]byte("reachqueue")).Put(value, []byte{1}); err != nil {
				return err
			}
			s.result.Progress.RootAfter = bytes.Clone(key)
			key, value = cursor.Next()
		}

		// Preserve unfinished alias scans and mark unsupported roots uncertain.
		if bytes.HasPrefix(key, ref) {
			return s.save(tx)
		}
		if len(after) != 0 {
			found = true
		}
		s.result.Progress.RootAfter = nil
		if !found {
			if err := tx.Bucket([]byte("state")).Put([]byte("reach-incomplete"), []byte{1}); err != nil {
				return err
			}
			key := store_kvkey.NewDefaultKVKey().GetBlockKey(ref)
			if tx.Bucket([]byte("data")).Get(key) == nil {
				if err := s.putHole(tx, ref); err != nil {
					return err
				}
			}
		}
		s.result.Cursor = ref
		return s.save(tx)
	})
}

// putHole counts each missing full reference once across all held paths.
func (s *Scanner) putHole(tx *scratchTx, ref []byte) error {
	if tx.Bucket([]byte("holes")).Get(ref) != nil {
		return nil
	}
	if err := tx.Bucket([]byte("holes")).Put(ref, []byte{1}); err != nil {
		return err
	}
	s.result.Counts.LiveHoles++
	return nil
}

// countVersion compares one attributed exact version with its captured current World.
func (s *Scanner) countVersion() error {
	// Read only one normalized version from the disk index.
	key, data, err := nextRow(s.db, "versions", s.result.GetCursor())
	if err != nil {
		return err
	}

	// Compare this exact normalized version in its captured Space.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit this graph slice and its exact active-node continuation.
		if key == nil {
			return s.advanceStage(tx, Pass_PASS_PHYSICAL)
		}
		version := &Version{}
		if err := version.UnmarshalVT(data); err != nil {
			return err
		}
		s.result.Counts.ObjectVersions++
		objectKey := append([]byte(version.GetSpaceId()+"\x00"), version.GetObject().GetKey()...)
		current := tx.Bucket([]byte("current")).Get(objectKey)

		// Only a complete declared current World proves exact-version absence.
		currentKnown := false
		for _, w := range s.source.GetWorlds() {
			if w.GetSpaceId() == version.GetSpaceId() && w.GetCurrent() {
				currentKnown = true
			}
		}
		currentKnown = currentKnown && tx.Bucket([]byte("state")).Get([]byte("current-incomplete/"+version.GetSpaceId())) == nil

		// Count candidate keys, closure quality and causal subsets in separate units.
		if currentKnown && !bytes.Equal(current, key) {
			s.result.Counts.AttributableVersions++
			if tx.Bucket([]byte("keys")).Get(objectKey) == nil {
				if err := tx.Bucket([]byte("keys")).Put(objectKey, []byte{1}); err != nil {
					return err
				}
				s.result.Counts.CandidateKeys++
				if current != nil {
					s.result.Counts.ChangedKeys++
				}
			}
			bodyNode := tx.Bucket([]byte("nodeversion")).Get(key)
			if tx.Bucket([]byte("ambiguousversions")).Get(key) != nil || bodyNode != nil && tx.Bucket([]byte("bad")).Get(bodyNode) != nil {
				s.result.Counts.PartialVersions++
			} else {
				s.result.Counts.RecoverableVersions++
			}
			if tx.Bucket([]byte("confirmed")).Get(key) != nil {
				s.result.Counts.ConfirmedRefusalLosses++
			}
			if tx.Bucket([]byte("deletes")).Get(objectKey) != nil {
				s.result.Counts.IntentionalDeletes++
			}
		}
		s.result.Cursor = key
		return s.save(tx)
	})
}

// countPhysical aggregates distinct bytes only after held-root propagation finishes.
func (s *Scanner) countPhysical() error {
	// Keep physical count work to one stored block per durable step.
	key, data, err := nextRow(s.db, "data", s.result.GetCursor())
	if err != nil {
		return err
	}

	// Publish completed coverage or aggregate one distinct physical block.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit this graph slice and its exact active-node continuation.
		if key == nil {
			ambiguous, _ := tx.Bucket([]byte("ambiguousversions")).Cursor().First()
			if ambiguous != nil {
				s.result.Gaps = append(s.result.Gaps, "historical body interpretations disagree; affected versions remain partial")
			}
			if tx.Bucket([]byte("state")).Get([]byte("reach-incomplete")) != nil {
				s.result.Gaps = append(s.result.Gaps, "held-root closure is incomplete; non-reachability is unknown")
			}
			s.result.Gaps = append(s.result.Gaps, "versions without agreeing native causal witnesses remain cause unknown")
			for _, w := range s.source.GetWorlds() {
				hasCurrent := false
				for _, current := range s.source.GetWorlds() {
					if current.GetCurrent() && current.GetSpaceId() == w.GetSpaceId() {
						hasCurrent = true
					}
				}
				if !hasCurrent {
					s.result.Gaps = append(s.result.Gaps, "current World unavailable for Space "+w.GetSpaceId()+"; version absence is unknown")
					break
				}
			}

			// Publish the terminal count only after preserving every uncertainty class.
			s.result.Phase = Phase_PHASE_COUNTED
			s.result.ResumePhase = Phase_PHASE_UNKNOWN
			s.result.Failure = ""
			s.result.Cursor = nil
			return s.save(tx)
		}
		ref := bytes.TrimPrefix(key, store_kvkey.NewDefaultKVKey().GetBlockFullPrefix())
		if tx.Bucket([]byte("reachblocks")).Get(ref) == nil {
			if tx.Bucket([]byte("state")).Get([]byte("reach-incomplete")) != nil {
				s.result.Counts.UnknownReachability++
			} else {
				s.result.Counts.OrphanBlocks++
				s.result.Counts.OrphanBytes += uint64(len(data))
			}
		}
		if tx.Bucket([]byte("typed")).Get(ref) == nil {
			s.result.Counts.UnassignedBlocks++
		}
		if tx.Bucket([]byte("unbound")).Get(ref) != nil && tx.Bucket([]byte("bound")).Get(ref) == nil {
			s.result.Counts.StructuralCandidates++
		}
		s.result.Cursor = key
		return s.save(tx)
	})
}

// propagateCurrent follows one current World edge slice and records its intentional deletes.
func (s *Scanner) propagateCurrent() error {
	// Persist a separate provenance frontier for each bounded current World context.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit this graph slice and its exact active-node continuation.
		queue := tx.Bucket([]byte("currentqueue"))
		item, _ := queue.Cursor().First()
		if item == nil {
			return s.advanceStage(tx, Pass_PASS_WITNESSES)
		}

		// Keep the current provenance item stable across bounded edge slices.
		if active := s.result.GetProgress().GetActiveNode(); active != nil {
			item = active
		}
		item = bytes.Clone(item)
		s.result.Progress.ActiveNode = bytes.Clone(item)

		// Attribute node-local native delete facts to this captured current Space.
		parent, index := item[:32], binary.BigEndian.Uint32(item[32:])
		space := s.source.GetWorlds()[index].GetSpaceId()
		facts := tx.Bucket([]byte("deletefacts")).Cursor()
		for key, value := facts.Seek(parent); bytes.HasPrefix(key, parent); key, value = facts.Next() {
			if err := tx.Bucket([]byte("deletes")).Put(append([]byte(space+"\x00"), value...), []byte{1}); err != nil {
				return err
			}
		}

		// Bound edge fan-out while preserving the active node across queue insertions.
		cursor := tx.Bucket([]byte("edges")).Cursor()
		edge, _ := cursor.Seek(parent)
		after := s.result.GetProgress().GetEdgeAfter()
		if len(after) != 0 {
			edge, _ = cursor.Seek(after)
			if bytes.Equal(edge, after) {
				edge, _ = cursor.Next()
			}
		}
		for n := 0; n < 128 && bytes.HasPrefix(edge, parent); n++ {
			child := append(bytes.Clone(edge[32:]), item[32:]...)
			if tx.Bucket([]byte("currentreach")).Get(child) == nil {
				if err := tx.Bucket([]byte("currentreach")).Put(child, []byte{1}); err != nil {
					return err
				}
				if err := queue.Put(child, []byte{1}); err != nil {
					return err
				}
			}
			s.result.Progress.EdgeAfter = bytes.Clone(edge)
			edge, _ = cursor.Next()
		}
		if !bytes.HasPrefix(edge, parent) {
			if err := queue.Delete(item); err != nil {
				return err
			}
			s.result.Progress.EdgeAfter = nil
			s.result.Progress.ActiveNode = nil
		}
		return s.save(tx)
	})
}
