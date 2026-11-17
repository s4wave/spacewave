//go:build !js && !wasip1

// Package world_recovery counts surviving object evidence in an isolated capture.
package world_recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"

	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/coord"
	coord_filelock "github.com/s4wave/spacewave/db/coord/filelock"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// Scanner owns one disk-backed run and read-only source handles.
// No source write, free, rebuild, repair, pin, stage or maintenance is reachable.
// All broad StoreOps consumers receive scratch data, never a source handle.
type Scanner struct {
	// source is the bounded immutable input manifest.
	source *Source
	// readers expose only Get, Next and Close on source tables.
	readers []*common.Immutable
	// db stores work, frontier, deduplication, provenance and result atomically.
	db *scratchIndex
	// blocks serves typed readers from scratch, without source capabilities.
	blocks block.StoreOps
	// result is the last durable result.
	result *Result
	// opts supplies interpretation and transition observers.
	opts Options
	// capture binds the already opened file to its current path and mutation metadata.
	capture []byte
	// lease excludes another count process while allowing saved report reads.
	lease coord.WriteLease
}

// NewScanner opens a capture and a separate durable work index.
func NewScanner(ctx context.Context, source *Source, scratch string, opts Options) (*Scanner, error) {
	// Bound manifest-dependent memory and require one current World per Space.
	if source == nil || source.GetCaptureId() == "" || len(source.GetTables()) == 0 || len(source.GetTables()) > 32 || len(source.GetWorlds()) > 64 || source.SizeVT() > 1<<20 {
		return nil, errors.New("capture identity, bounded tables and World contexts are required")
	}
	if (opts.Lookup != nil || opts.Witness != nil) && opts.InterpretationID == "" {
		return nil, errors.New("constructor lookup requires a stable interpretation identity")
	}
	for i, w := range source.GetWorlds() {
		if w.GetSpaceId() == "" || w.GetRef().GetRootRef().GetEmpty() {
			return nil, errors.New("typed World requires a Space and nonempty root")
		}
		for j, other := range source.GetWorlds() {
			if i != j && w.GetCurrent() && other.GetCurrent() && w.GetSpaceId() == other.GetSpaceId() {
				return nil, errors.New("capture has several current Worlds for one Space")
			}
		}
	}
	for i, table := range source.GetTables() {
		for _, other := range source.GetTables()[:i] {
			if table == other {
				return nil, errors.New("capture repeats the same table location")
			}
		}
	}

	// Bind the manifest to the physical capture identity without walking it at open.
	info, err := os.Stat(source.GetPath())
	if err != nil {
		return nil, err
	}
	if other, err := os.Stat(scratch); err == nil && os.SameFile(info, other) {
		return nil, errors.New("scratch must be separate from the source")
	}
	abs, err := filepath.Abs(source.GetPath())
	if err != nil {
		return nil, err
	}

	// Bind interpretation rules and physical identity to the run.
	manifest, err := source.MarshalVT()
	if err != nil {
		return nil, err
	}
	physical, err := captureIdentity(source.GetPath())
	if err != nil {
		return nil, err
	}
	identity := sha256.Sum256(bytes.Join([][]byte{manifest, []byte(abs), physical, []byte("recovery-count-3"), []byte(opts.InterpretationID)}, []byte{0}))

	// Open only physically immutable backend handles, before opening scratch.
	s := &Scanner{source: source.CloneVT(), opts: opts, capture: physical}
	for _, table := range source.GetTables() {
		reader, err := sqlite.OpenImmutable(ctx, source.GetPath(), table, block.MaxBlockSize)
		if err != nil {
			return nil, errors.Join(err, s.Close())
		}
		s.readers = append(s.readers, reader)
	}

	// Claim the run through the existing cross-process keyed coordinator.
	absScratch, err := canonicalScratchPath(scratch)
	if err != nil {
		return nil, errors.Join(err, s.Close())
	}
	directory := filepath.Dir(absScratch)
	coordinator := coord_filelock.NewCoordinator(directory, absScratch, nil)
	scope := coord.Scope{Key: "recovery-count"}
	if err := rejectSourceAliases(info, absScratch, coordinator.LockPath(scope)); err != nil {
		return nil, errors.Join(err, s.Close())
	}

	// Exclude another active count only after rejecting every writable source alias.
	var acquired bool
	s.lease, acquired, err = coordinator.TryAcquireWriteLease(ctx, scope)
	if err != nil || !acquired {
		return nil, errors.Join(errors.New("scratch is already owned by another count"), err, s.Close())
	}

	// Open only the canonical scratch target after every source alias has been rejected.
	s.db, err = openScratch(absScratch, false)
	if err != nil {
		return nil, errors.Join(err, s.Close())
	}
	s.blocks = block.NewReadOnlyStore(block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), kvtx_prefixer.NewPrefixer(s.db.store, []byte("data\x00")), 0))

	// Initialize the indexes or retain the prior run, including a paused cursor.
	err = s.db.Update(func(tx *scratchTx) error {
		// Initialize the durable report and named indexes in the same transaction.
		s.result = &Result{Input: source.CloneVT(), Progress: &Progress{}, InputIdentity: bytes.Clone(identity[:]), Phase: Phase_PHASE_INVENTORY, Counts: &Counts{}, Gaps: []string{"s4db, external BlockDomain values, packahead, local packs, remote catalogs and uncataloged packs are not covered", "wholly absent versions cannot be counted; structural decoding does not prove Space membership", "unbound detection probes separate plain Object records only, not every declared transform context or detached World, IAVL, Okra or changelog roots; zero candidates cannot prove absence", "unsupported constructors or transformations and missing descendants keep closures partial"}}
		s.result.Input.Path = abs
		if previous := tx.Bucket([]byte("state")).Get([]byte("result")); previous != nil {
			durable := &Result{}
			if err := durable.UnmarshalVT(previous); err != nil {
				return err
			}
			s.result = durable
			if !bytes.Equal(s.result.GetInputIdentity(), identity[:]) {
				s.result.Phase = Phase_PHASE_STALE
				s.result.Failure = "capture identity changed; preserve this report and start a new scratch file"
			}
		}
		return s.save(tx)
	})
	if err != nil {
		return nil, errors.Join(err, s.Close())
	}
	return s, nil
}

// Result returns the durable report without reading the source again.
func (s *Scanner) Result() *Result {
	return s.result.CloneVT()
}

// Run continues committed work until counted, paused or stale.
func (s *Scanner) Run(ctx context.Context) (*Result, error) {
	// Resume the operation whose cursor and rows survived the interruption.
	if s.result.GetPhase() == Phase_PHASE_STALE {
		return s.Result(), errors.New(s.result.GetFailure())
	}
	if s.result.GetPhase() == Phase_PHASE_PAUSED {
		s.result.Phase = s.result.GetResumePhase()
		s.result.Failure = ""
	}

	// Execute one bounded durable operation at a time.
	for s.result.GetPhase() != Phase_PHASE_COUNTED {
		physical, identityErr := captureIdentity(s.source.GetPath())
		if identityErr != nil || !bytes.Equal(physical, s.capture) {
			s.result.Phase = Phase_PHASE_STALE
			s.result.Failure = "capture identity changed; preserve this report and start a new scratch file"
			return s.Result(), errors.Join(errors.New(s.result.GetFailure()), s.db.Update(s.save))
		}
		err := ctx.Err()
		if err == nil && s.opts.BeforeStep != nil {
			err = s.opts.BeforeStep(s.result.GetPhase())
		}
		if err == nil {
			switch s.result.GetPhase() {
			case Phase_PHASE_INVENTORY:
				err = s.inventory(ctx)
			case Phase_PHASE_INTERPRET:
				err = s.interpret(ctx)
			case Phase_PHASE_TRAVERSE:
				err = s.traverse(ctx)
			default:
				err = errors.New("invalid durable scan phase")
			}
		}
		if err != nil {
			return s.pause(err)
		}
		if s.opts.Committed != nil {
			if err := s.opts.Committed(s.Result()); err != nil {
				return s.pause(err)
			}
		}
	}
	physical, identityErr := captureIdentity(s.source.GetPath())
	if identityErr != nil || !bytes.Equal(physical, s.capture) {
		s.result.Phase = Phase_PHASE_STALE
		s.result.Failure = "capture identity changed; preserve this report and start a new scratch file"
		return s.Result(), errors.Join(errors.New(s.result.GetFailure()), s.db.Update(s.save))
	}
	return s.Result(), nil
}

// Close releases source readers and scratch; it never runs source maintenance.
func (s *Scanner) Close() error {
	// Close the bounded source pools independently of scratch durability.
	var err error
	for _, reader := range s.readers {
		err = errors.Join(err, reader.Close())
	}
	s.readers = nil

	// Flush and close the work index only.
	if s.db != nil {
		err = errors.Join(err, s.db.Close())
		s.db = nil
	}
	if s.lease != nil {
		err = errors.Join(err, s.lease.Release(context.Background()))
		s.lease = nil
	}
	return err
}

// save commits the aggregate with the operation's rows and continuation cursor.
func (s *Scanner) save(tx *scratchTx) error {
	// Prepare the aggregate beside its rows before the shared atomic commit.
	data, err := s.result.MarshalVT()
	if err != nil {
		return err
	}
	if err := tx.Bucket([]byte("state")).Put([]byte("result"), data); err != nil {
		return err
	}
	if s.opts.BeforeCommit != nil {
		return s.opts.BeforeCommit(s.Result())
	}
	return nil
}

// pause preserves the last committed report even when scratch is full.
func (s *Scanner) pause(cause error) (*Result, error) {
	// Reload the durable cursor so a rolled-back transaction never advances it.
	durable := &Result{}
	err := s.db.View(func(tx *scratchTx) error {
		return durable.UnmarshalVT(tx.Bucket([]byte("state")).Get([]byte("result")))
	})
	if err != nil {
		return s.Result(), errors.Join(cause, err)
	}
	s.result = durable
	if s.result.GetPhase() == Phase_PHASE_COUNTED {
		return s.Result(), cause
	}

	// Preserve the original resume phase when a retried pause fails again.
	if s.result.GetPhase() != Phase_PHASE_PAUSED {
		s.result.ResumePhase = s.result.GetPhase()
	}
	s.result.Phase = Phase_PHASE_PAUSED
	s.result.Failure = cause.Error()

	// A failed pause write leaves the earlier non-counted result resumable.
	err = s.db.Update(s.save)
	return s.Result(), errors.Join(cause, err)
}

// keyNode binds a work item to its full reference, constructor and World context.
func keyNode(node *Node) []byte {
	// Exclude incidental World provenance while retaining decoder constraints.
	context := normalizedContext(node.GetContext())
	data, _ := (&Node{Ref: node.GetRef(), Constructor: node.GetConstructor(), TreeKey: node.GetTreeKey(), Context: context, ExpectedSize: node.GetExpectedSize()}).MarshalVT()
	key := sha256.Sum256(data)
	return key[:]
}

// putNode inserts one idempotent interpretation and its reverse edge.
func putNode(tx *scratchTx, node *Node) error {
	// Persist each bounded interpretation once, with its disk queue entry.
	key := keyNode(node)
	if tx.Bucket([]byte("nodes")).Get(key) == nil {
		data, err := node.MarshalVT()
		if err != nil {
			return err
		}
		for _, name := range []string{"nodes", "queue"} {
			if err := tx.Bucket([]byte(name)).Put(key, data); err != nil {
				return err
			}
		}

		// Index the shared interpretation by its complete physical reference.
		ref, err := node.GetRef().MarshalKey()
		if err != nil {
			return err
		}
		if err := tx.Bucket([]byte("byref")).Put(append(ref, key...), key); err != nil {
			return err
		}
	}

	// Keep every parent relation even when several Worlds share a child.
	if len(node.GetParent()) != 0 {
		if err := tx.Bucket([]byte("edges")).Put(append(bytes.Clone(node.GetParent()), key...), nil); err != nil {
			return err
		}
		return tx.Bucket([]byte("parents")).Put(append(bytes.Clone(key), node.GetParent()...), nil)
	}
	return nil
}

// nextRow copies one indexed item outside the transaction lifetime.
func nextRow(db *scratchIndex, bucket string, after []byte) ([]byte, []byte, error) {
	// Copy at most one row before any dependent read or write scope opens.
	var key, value []byte
	err := db.View(func(tx *scratchTx) error {
		// Seek and copy one indexed row before releasing its snapshot.
		cursor := tx.Bucket([]byte(bucket)).Cursor()
		k, v := cursor.First()
		if len(after) != 0 {
			k, v = cursor.Seek(after)
			if bytes.Equal(k, after) {
				k, v = cursor.Next()
			}
		}
		key, value = bytes.Clone(k), bytes.Clone(v)
		return nil
	})
	return key, value, err
}

// worldKey encodes a fixed-width World index before an object key.
func worldKey(index uint32, key string) []byte {
	return append(binary.BigEndian.AppendUint32(nil, index), key...)
}
