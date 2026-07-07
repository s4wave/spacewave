package world_block

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/filters"
	bquad "github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
)

type changeWaiter struct {
	filter     world.ChangeFilter
	afterSeqno uint64
	bcast      broadcast.Broadcast
	seqno      uint64
}

func (w *changeWaiter) notify(seqno uint64) {
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if w.seqno == 0 || seqno < w.seqno {
			w.seqno = seqno
		}
		broadcast()
	})
}

func (w *changeWaiter) wait(ctx context.Context) (uint64, error) {
	for {
		var seqno uint64
		var waitCh <-chan struct{}
		w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			seqno = w.seqno
			if seqno == 0 {
				waitCh = getWaitCh()
			}
		})
		if seqno != 0 {
			return seqno, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-waitCh:
		}
	}
}

// WaitChange waits until a changelog entry after afterSeqno matches filter.
func (e *Engine) WaitChange(ctx context.Context, afterSeqno uint64, filter world.ChangeFilter) (uint64, error) {
	if filter.IsEmpty() {
		return e.WaitSeqno(ctx, afterSeqno+1)
	}

	waiter := &changeWaiter{filter: filter.Clone(), afterSeqno: afterSeqno}
	e.rmtx.Lock()
	if e.closed {
		e.rmtx.Unlock()
		return 0, errors.New("world block engine is closed")
	}
	if e.changeWaiters == nil {
		e.changeWaiters = make(map[*changeWaiter]struct{})
	}
	e.changeWaiters[waiter] = struct{}{}
	e.rmtx.Unlock()
	defer func() {
		e.rmtx.Lock()
		delete(e.changeWaiters, waiter)
		e.rmtx.Unlock()
	}()

	seqno, matched, err := e.findMatchingChange(ctx, afterSeqno, waiter.filter)
	if err != nil || matched {
		return seqno, err
	}
	return waiter.wait(ctx)
}

func (e *Engine) findMatchingChange(ctx context.Context, afterSeqno uint64, filter world.ChangeFilter) (uint64, bool, error) {
	entries, err := ReadChangeLogEntries(ctx, e.AccessWorldState, ChangeLogReadOptions{AfterSeqno: afterSeqno})
	if err != nil {
		return 0, false, err
	}
	for _, entry := range entries {
		if changeLogEntryMatchesFilter(entry, filter) {
			return entry.Seqno, true, nil
		}
	}
	return 0, false, nil
}

func (e *Engine) notifyChangeWaitersLocked(ctx context.Context, afterSeqno uint64) {
	if len(e.changeWaiters) == 0 {
		return
	}

	_, rootBcs := e.root.BuildTransaction(nil)
	entries, err := ReadChangeLogEntriesFromCursor(ctx, rootBcs, ChangeLogReadOptions{AfterSeqno: afterSeqno})
	if err != nil || len(entries) == 0 {
		seqno, _, seqErr := e.currentRootSeqnoLocked(ctx)
		if seqErr == nil && seqno > afterSeqno {
			for waiter := range e.changeWaiters {
				if seqno > waiter.afterSeqno {
					waiter.notify(seqno)
				}
			}
		}
		return
	}

	for waiter := range e.changeWaiters {
		for _, entry := range entries {
			if entry.Seqno <= waiter.afterSeqno {
				continue
			}
			if changeLogEntryMatchesFilter(entry, waiter.filter) {
				waiter.notify(entry.Seqno)
				break
			}
		}
	}
}

func changeLogEntryMatchesFilter(entry *ChangeLogEntry, filter world.ChangeFilter) bool {
	if entry == nil {
		return false
	}
	if filter.AnyObject && isObjectChangeType(entry.ChangeType) {
		return true
	}
	if filter.AnyGraph && isGraphChangeType(entry.ChangeType) {
		return true
	}
	if len(filter.ObjectKeys) != 0 || len(filter.ObjectKeyPrefixes) != 0 {
		if changeLogEntryMatchesObjectFilter(entry, filter.ObjectKeys, filter.ObjectKeyPrefixes) {
			return true
		}
	}
	if len(filter.GraphQuads) != 0 {
		if changeLogEntryMatchesGraphFilter(entry, filter.GraphQuads) {
			return true
		}
	}
	return false
}

func changeLogEntryMatchesObjectFilter(entry *ChangeLogEntry, keys, prefixes []string) bool {
	if !isObjectChangeType(entry.ChangeType) {
		return false
	}
	if kf := entry.KeyFilters; kf != nil && !kf.IsEmpty() {
		reader := filters.NewKeyFiltersReader(kf)
		if slices.ContainsFunc(keys, reader.TestObjectKey) {
			return true
		}
		keyPrefix := kf.GetKeyPrefix()
		for _, prefix := range prefixes {
			if prefixesOverlap(keyPrefix, prefix) {
				return true
			}
		}
		return false
	}
	for _, change := range entry.Changes {
		if worldChangeMatchesObjectFilter(change, keys, prefixes) {
			return true
		}
	}
	return false
}

func changeLogEntryMatchesGraphFilter(entry *ChangeLogEntry, quads []world.GraphQuad) bool {
	if !isGraphChangeType(entry.ChangeType) {
		return false
	}
	if kf := entry.KeyFilters; kf != nil && !kf.IsEmpty() {
		reader := filters.NewKeyFiltersReader(kf)
		for _, graphQuad := range quads {
			quadFilter := graphQuadToBlockQuad(graphQuad)
			if reader.TestQuad(quadFilter) {
				return true
			}
		}
		return false
	}
	for _, change := range entry.Changes {
		changedQuad := change.GetQuad()
		if changedQuad.IsEmpty() {
			continue
		}
		for _, graphQuad := range quads {
			if blockQuadMatchesGraphFilter(changedQuad, graphQuad) {
				return true
			}
		}
	}
	return false
}

func worldChangeMatchesObjectFilter(change *WorldChange, keys, prefixes []string) bool {
	if change == nil {
		return false
	}
	return objectKeyMatchesFilter(change.GetKey(), keys, prefixes) ||
		objectKeyMatchesFilter(change.GetNewKey(), keys, prefixes)
}

func objectKeyMatchesFilter(key string, keys, prefixes []string) bool {
	if key == "" {
		return false
	}
	if slices.Contains(keys, key) {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func prefixesOverlap(changePrefix, watchPrefix string) bool {
	if watchPrefix == "" {
		return true
	}
	if changePrefix == "" {
		return true
	}
	return strings.HasPrefix(changePrefix, watchPrefix) || strings.HasPrefix(watchPrefix, changePrefix)
}

func blockQuadMatchesGraphFilter(changed *bquad.Quad, filter world.GraphQuad) bool {
	if filter == nil {
		return true
	}
	if val := filter.GetSubject(); val != "" && changed.GetSubject() != val {
		return false
	}
	if val := filter.GetPredicate(); val != "" && changed.GetPredicate() != val {
		return false
	}
	if val := filter.GetObj(); val != "" && changed.GetObj() != val {
		return false
	}
	if val := filter.GetLabel(); val != "" && changed.GetLabel() != val {
		return false
	}
	return true
}

func graphQuadToBlockQuad(graphQuad world.GraphQuad) *bquad.Quad {
	if graphQuad == nil {
		return &bquad.Quad{}
	}
	return &bquad.Quad{
		Subject:   graphQuad.GetSubject(),
		Predicate: graphQuad.GetPredicate(),
		Obj:       graphQuad.GetObj(),
		Label:     graphQuad.GetLabel(),
	}
}

func isObjectChangeType(changeType WorldChangeType) bool {
	switch changeType {
	case WorldChangeType_WorldChange_OBJECT_SET,
		WorldChangeType_WorldChange_OBJECT_INC_REV,
		WorldChangeType_WorldChange_OBJECT_DELETE,
		WorldChangeType_WorldChange_OBJECT_RENAME:
		return true
	default:
		return false
	}
}

func isGraphChangeType(changeType WorldChangeType) bool {
	switch changeType {
	case WorldChangeType_WorldChange_GRAPH_SET,
		WorldChangeType_WorldChange_GRAPH_DELETE:
		return true
	default:
		return false
	}
}
