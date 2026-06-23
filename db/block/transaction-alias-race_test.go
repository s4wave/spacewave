package block

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/hash"
)

func TestWriteAtRootWaitsForInlineMarshalAliasSubtree(t *testing.T) {
	ctx := context.Background()

	oldEncodeConcurrency := maxEncodeConcurrency
	maxEncodeConcurrency = 0
	defer func() {
		maxEncodeConcurrency = oldEncodeConcurrency
	}()

	shared := &aliasRaceShared{
		start: make(chan struct{}),
	}

	dirent := &aliasRaceDirent{
		shared: shared,
	}
	tx, rootCursor := NewTransaction(aliasRaceStore{}, nil, nil, nil)
	rootCursor.SetBlock(&aliasRaceRoot{
		dirents: &aliasRaceDirentSlice{
			dirent: dirent,
		},
	}, true)

	inlineCursor := rootCursor.FollowRef(1, nil)
	inlineCursor.SetBlock(&aliasRaceInline{
		shared: shared,
		dirent: dirent,
	}, true)
	direntCursor := rootCursor.FollowSubBlock(2).FollowSubBlock(0)
	leafCursor := direntCursor.FollowRef(2, nil)
	leafCursor.SetBlock(&aliasRaceLeaf{}, true)

	if _, _, err := tx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}
}

type aliasRaceShared struct {
	ref       *BlockRef
	ready     atomic.Int32
	start     chan struct{}
	startOnce sync.Once
}

func (s *aliasRaceShared) waitStart() {
	if s.ready.Add(1) == 2 {
		s.startOnce.Do(func() {
			close(s.start)
		})
	}
	select {
	case <-s.start:
	case <-time.After(50 * time.Millisecond):
	}
}

type aliasRaceRoot struct {
	alias     AliasIdentityToken
	inlineRef *BlockRef
	dirents   *aliasRaceDirentSlice
}

func (r *aliasRaceRoot) BlockAliasIdentity() *AliasIdentityToken {
	return &r.alias
}

func (r *aliasRaceRoot) MarshalBlock() ([]byte, error) {
	return []byte{1}, nil
}

func (r *aliasRaceRoot) UnmarshalBlock([]byte) error {
	return nil
}

func (r *aliasRaceRoot) ApplySubBlock(id uint32, next SubBlock) error {
	if id == 2 {
		r.dirents, _ = next.(*aliasRaceDirentSlice)
	}
	return nil
}

func (r *aliasRaceRoot) GetSubBlocks() map[uint32]SubBlock {
	return map[uint32]SubBlock{
		2: r.dirents,
	}
}

func (r *aliasRaceRoot) GetSubBlockCtor(id uint32) SubBlockCtor {
	if id == 2 {
		return func(bool) SubBlock {
			return r.dirents
		}
	}
	return nil
}

func (r *aliasRaceRoot) ApplyBlockRef(id uint32, ptr *BlockRef) error {
	if id == 1 {
		r.inlineRef = ptr.Clone()
	}
	return nil
}

func (r *aliasRaceRoot) GetBlockRefs() (map[uint32]*BlockRef, error) {
	return map[uint32]*BlockRef{
		1: r.inlineRef,
	}, nil
}

func (r *aliasRaceRoot) GetBlockRefCtor(uint32) Ctor {
	return func() Block {
		return &aliasRaceInline{}
	}
}

type aliasRaceInline struct {
	alias  AliasIdentityToken
	shared *aliasRaceShared
	dirent *aliasRaceDirent
}

func (i *aliasRaceInline) BlockAliasIdentity() *AliasIdentityToken {
	return &i.alias
}

func (i *aliasRaceInline) IsNil() bool {
	return i == nil
}

func (i *aliasRaceInline) MarshalBlock() ([]byte, error) {
	i.shared.waitStart()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		aliasRaceRefSink = i.shared.ref
		runtime.Gosched()
	}
	return []byte{2}, nil
}

func (i *aliasRaceInline) UnmarshalBlock([]byte) error {
	return nil
}

func (i *aliasRaceInline) GetSubBlocks() map[uint32]SubBlock {
	return map[uint32]SubBlock{
		1: i.dirent,
	}
}

func (i *aliasRaceInline) GetSubBlockCtor(uint32) SubBlockCtor {
	return nil
}

func (i *aliasRaceInline) ApplySubBlock(uint32, SubBlock) error {
	return nil
}

type aliasRaceDirentSlice struct {
	alias  AliasIdentityToken
	dirent *aliasRaceDirent
}

func (s *aliasRaceDirentSlice) BlockAliasIdentity() *AliasIdentityToken {
	return &s.alias
}

func (s *aliasRaceDirentSlice) IsNil() bool {
	return s == nil
}

func (s *aliasRaceDirentSlice) ApplySubBlock(id uint32, next SubBlock) error {
	if id == 0 {
		s.dirent, _ = next.(*aliasRaceDirent)
	}
	return nil
}

func (s *aliasRaceDirentSlice) GetSubBlocks() map[uint32]SubBlock {
	return map[uint32]SubBlock{
		0: s.dirent,
	}
}

func (s *aliasRaceDirentSlice) GetSubBlockCtor(uint32) SubBlockCtor {
	return func(bool) SubBlock {
		return s.dirent
	}
}

type aliasRaceDirent struct {
	alias  AliasIdentityToken
	shared *aliasRaceShared
}

func (d *aliasRaceDirent) BlockAliasIdentity() *AliasIdentityToken {
	return &d.alias
}

func (d *aliasRaceDirent) IsNil() bool {
	return d == nil
}

func (d *aliasRaceDirent) ApplyBlockRef(id uint32, ptr *BlockRef) error {
	if id == 2 {
		d.shared.waitStart()
		deadline := time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) {
			d.shared.ref = ptr.Clone()
			runtime.Gosched()
		}
	}
	return nil
}

func (d *aliasRaceDirent) GetBlockRefs() (map[uint32]*BlockRef, error) {
	return map[uint32]*BlockRef{
		2: d.shared.ref,
	}, nil
}

func (d *aliasRaceDirent) GetBlockRefCtor(uint32) Ctor {
	return func() Block {
		return &aliasRaceLeaf{}
	}
}

type aliasRaceLeaf struct{}

func (l *aliasRaceLeaf) MarshalBlock() ([]byte, error) {
	return []byte{3}, nil
}

func (l *aliasRaceLeaf) UnmarshalBlock([]byte) error {
	return nil
}

type aliasRaceStore struct {
	NopStoreOps
}

var aliasRaceRefSink *BlockRef

func (aliasRaceStore) GetHashType() hash.HashType {
	return DefaultHashType
}

func (aliasRaceStore) PutBlock(_ context.Context, _ []byte, opts *PutOpts) (*BlockRef, bool, error) {
	return opts.GetForceBlockRef().Clone(), false, nil
}

func (s aliasRaceStore) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone {
			continue
		}
		if _, _, err := s.PutBlock(ctx, entry.Data, &PutOpts{
			ForceBlockRef: entry.Ref,
			Refs:          entry.Refs,
		}); err != nil {
			return err
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ Block              = ((*aliasRaceRoot)(nil))
	_ BlockWithRefs      = ((*aliasRaceRoot)(nil))
	_ BlockWithSubBlocks = ((*aliasRaceRoot)(nil))
	_ Block              = ((*aliasRaceInline)(nil))
	_ BlockWithSubBlocks = ((*aliasRaceInline)(nil))
	_ BlockWithSubBlocks = ((*aliasRaceDirentSlice)(nil))
	_ SubBlock           = ((*aliasRaceDirentSlice)(nil))
	_ BlockWithRefs      = ((*aliasRaceDirent)(nil))
	_ SubBlock           = ((*aliasRaceDirent)(nil))
	_ Block              = ((*aliasRaceLeaf)(nil))
)
