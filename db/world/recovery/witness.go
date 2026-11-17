//go:build !js && !wasip1

package world_recovery

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"

	"github.com/s4wave/spacewave/db/bucket"
)

// witness validates one native causal record set or one exact introduced version.
func (s *Scanner) witness(ctx context.Context) error {
	// Read one bounded witness cursor and any already validated World indices.
	index := s.result.GetProgress().GetWitnessIndex()
	pair := s.result.GetProgress().GetWitness()
	if int(index) == len(s.source.GetWitnesses()) {
		return s.db.Update(func(tx *scratchTx) error { return s.advanceStage(tx, Pass_PASS_VERSIONS) })
	}

	// Validate operation identity, successful introduction, refusal and exact author cover.
	if pair == nil {
		pair, err := s.validateWitness(ctx, s.source.GetWitnesses()[index])
		if err != nil {
			return s.finishWitness(index, err)
		}
		return s.db.Update(func(tx *scratchTx) error {
			// Commit the causal proof cursor with the versions it establishes.
			s.result.Progress.Witness = pair
			s.result.Cursor = nil
			return s.save(tx)
		})
	}

	// Stream versions of the validated accepted World with an indexed cursor.
	after, before, cover, refusal := pair.GetAccepted(), pair.GetBefore(), pair.GetCovering(), pair.GetRefused()
	prefix := worldKey(after, "")
	row, version, err := nextRow(s.db, "worldobjects", s.result.GetCursor())
	if err != nil {
		return err
	}
	if s.result.GetCursor() == nil {
		err = s.db.View(func(tx *scratchTx) error {
			k, v := tx.Bucket([]byte("worldobjects")).Cursor().Seek(prefix)
			row, version = bytes.Clone(k), bytes.Clone(v)
			return nil
		})
		if err != nil {
			return err
		}
	}
	if !bytes.HasPrefix(row, prefix) {
		return s.finishWitness(index, nil)
	}

	// Confirmation is an exact-version delta, never mere presence in an accepted World.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit the causal proof cursor with the versions it establishes.
		key := string(row[4:])
		prior := tx.Bucket([]byte("worldobjects")).Get(worldKey(before, key))
		covered := tx.Bucket([]byte("worldobjects")).Get(worldKey(cover, key))
		refusedVersion := tx.Bucket([]byte("worldobjects")).Get(worldKey(refusal, key))
		space := s.source.GetWitnesses()[index].GetSpaceId()
		objectKey := append([]byte(space+"\x00"), key...)
		current := tx.Bucket([]byte("current")).Get(objectKey)

		// Incomplete membership cannot establish exact-version disappearance.
		beforeOK := tx.Bucket([]byte("state")).Get(worldKey(before, "membership-incomplete")) == nil
		coverOK := tx.Bucket([]byte("state")).Get(worldKey(cover, "membership-incomplete")) == nil
		refusalOK := tx.Bucket([]byte("state")).Get(worldKey(refusal, "membership-incomplete")) == nil
		if beforeOK && coverOK && refusalOK && !bytes.Equal(prior, version) && !bytes.Equal(covered, version) && !bytes.Equal(refusedVersion, version) && !bytes.Equal(current, version) && tx.Bucket([]byte("deletes")).Get(objectKey) == nil {
			if err := tx.Bucket([]byte("confirmed")).Put(version, []byte{1}); err != nil {
				return err
			}
		}
		s.result.Cursor = row
		return s.save(tx)
	})
}

// validateWitness maps externally validated native evidence onto captured typed Worlds.
func (s *Scanner) validateWitness(ctx context.Context, input *WitnessInput) (*WitnessPosition, error) {
	// The database layer consumes a validated native witness through its injected registry.
	if s.opts.Witness == nil {
		return nil, errors.New("native causal witness interpreter unavailable")
	}
	roots, err := s.opts.Witness(ctx, input, func(key []byte) ([]byte, error) {
		var data []byte
		err := s.db.View(func(tx *scratchTx) error {
			data = tx.Bucket([]byte("meta")).Get(append(binary.BigEndian.AppendUint32(nil, 0), key...))
			return nil
		})
		return data, err
	})
	if err != nil {
		return nil, err
	}

	// Preserve only bounded context indices after native validation succeeds.
	if roots == nil {
		return nil, errors.New("native witness interpreter returned no evidence")
	}
	after, afterOK := s.findWorld(input.GetSpaceId(), roots.GetAcceptedAfter())
	before, beforeOK := s.findWorld(input.GetSpaceId(), roots.GetAcceptedBefore())
	cover, coverOK := s.findWorld(input.GetSpaceId(), roots.GetCovering())
	refusal, refusalOK := s.findWorld(input.GetSpaceId(), roots.GetRefusedAfter())
	if !afterOK || !beforeOK || !coverOK || !refusalOK {
		return nil, errors.New("causal witness typed context unavailable")
	}
	return &WitnessPosition{Accepted: after, Before: before, Covering: cover, Refused: refusal}, nil
}

// findWorld matches a native causal root and exact transform/bucket context.
func (s *Scanner) findWorld(space string, ref *bucket.ObjectRef) (uint32, bool) {
	for i, input := range s.source.GetWorlds() {
		candidate := input.GetRef()
		normalized := resolveContext(candidate, ref)
		if input.GetSpaceId() == space && candidate.GetRootRef().EqualsRef(ref.GetRootRef()) && bytes.Equal(contextIdentity(candidate), contextIdentity(normalized)) {
			return uint32(i), true
		}
	}
	return 0, false
}

// finishWitness preserves a rejected proof as a coverage gap and advances idempotently.
func (s *Scanner) finishWitness(index uint32, cause error) error {
	// Record this proof boundary once before moving to the next witness.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit the causal proof cursor with the versions it establishes.
		if cause != nil {
			s.result.Gaps = append(s.result.Gaps, "causal witness unavailable: "+cause.Error())
		}
		s.result.Progress.WitnessIndex = index + 1
		s.result.Progress.Witness = nil
		s.result.Cursor = nil
		return s.save(tx)
	})
}
