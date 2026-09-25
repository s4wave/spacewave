package block

import "context"

// SupportsRootRetention reports the destination's root ownership capability.
func (s *BufferedStore) SupportsRootRetention() bool { return SupportsRootRetention(s.inner) }

// SetRetainedRoot fences prepared blocks before publishing their durable root.
func (s *BufferedStore) SetRetainedRoot(ctx context.Context, name string, ref *BlockRef) error {
	if _, err := s.Sync(ctx); err != nil {
		return err
	}
	return SetRetainedRoot(ctx, s.inner, name, ref)
}

// PinRoot fences a buffered root before acquiring its physical retention pin.
func (s *BufferedStore) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	if pending, err := s.getPending(ref); err != nil {
		return nil, err
	} else if pending != nil {
		if _, err := s.Sync(ctx); err != nil {
			return nil, err
		}
	}
	return PinRoot(ctx, s.inner, ref)
}

// MarkRootsComplete forwards the durable World proof.
func (s *BufferedStore) MarkRootsComplete(ctx context.Context, roots []*BlockRef) error {
	return MarkRootsComplete(ctx, s.inner, roots)
}

// RootComplete checks the underlying World proof.
func (s *BufferedStore) RootComplete(ctx context.Context, ref *BlockRef) (bool, error) {
	return RootComplete(ctx, s.inner, ref)
}
