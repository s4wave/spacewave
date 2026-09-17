package block

import (
	"context"
	"testing"
)

type storeRWFreshener struct {
	NopStoreOps

	scoped       *storeRWFreshener
	freshenCalls int
}

func (s *storeRWFreshener) BeginReadOperation(context.Context) (StoreOps, func(), error) {
	if s.scoped != nil {
		return s.scoped, func() {}, nil
	}
	return s, func() {}, nil
}

func (s *storeRWFreshener) EnsureDecodedBlockCacheFresh(context.Context) error {
	s.freshenCalls++
	return nil
}

func TestStoreRWForwardsDecodedBlockCacheFreshness(t *testing.T) {
	ctx := context.Background()
	scopedInner := &storeRWFreshener{}
	readInner := &storeRWFreshener{scoped: scopedInner}
	store := NewStoreRW(readInner, nil)

	if err := store.(DecodedBlockCacheFreshener).EnsureDecodedBlockCacheFresh(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if readInner.freshenCalls != 1 {
		t.Fatalf("expected one read-handle freshness call, got %d", readInner.freshenCalls)
	}

	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()
	if err := scoped.(DecodedBlockCacheFreshener).EnsureDecodedBlockCacheFresh(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if scopedInner.freshenCalls != 1 {
		t.Fatalf("expected one scoped freshness call, got %d", scopedInner.freshenCalls)
	}
}

type storeRWPublisher struct {
	NopStoreOps
	volumeID    string
	submissions int
}

func (s *storeRWPublisher) SupportsAtomicPublication() bool   { return true }
func (s *storeRWPublisher) AtomicPublicationVolumeID() string { return s.volumeID }
func (s *storeRWPublisher) SubmitAtomic(context.Context, *AtomicPublication) (*PublicationReceipt, error) {
	s.submissions++
	r := NewPublicationReceipt()
	r.Resolve(nil)
	return r, nil
}

func (s *storeRWPublisher) PublishAtomic(ctx context.Context, p *AtomicPublication) error {
	r, err := s.SubmitAtomic(ctx, p)
	if err != nil {
		return err
	}
	return r.Wait(ctx)
}

func TestStoreRWPublicationUsesOnlyWriteDomain(t *testing.T) {
	read := &storeRWPublisher{volumeID: "read"}
	write := &storeRWPublisher{volumeID: "write"}
	store := NewStoreRW(read, write)
	p := store.(AtomicPublisher)
	if !p.SupportsAtomicPublication() || p.AtomicPublicationVolumeID() != "write" {
		t.Fatal("wrong publication domain")
	}
	if err := p.PublishAtomic(t.Context(), &AtomicPublication{}); err != nil {
		t.Fatal(err)
	}
	if read.submissions != 0 || write.submissions != 1 {
		t.Fatal("publication went through lookup")
	}
	scoped, release, err := store.BeginReadOperation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if scoped.(AtomicPublisher).SupportsAtomicPublication() {
		t.Fatal("read scope exposed publication")
	}
	if _, err := scoped.(AtomicPublisher).SubmitAtomic(t.Context(), nil); err != ErrAtomicPublicationUnsupported {
		t.Fatal(err)
	}
}
