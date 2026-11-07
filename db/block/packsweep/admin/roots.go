//go:build !tinygo

package block_packsweep_admin

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// RootLister lists a store's named roots, as block.RootOps does.
type RootLister interface {
	// ListRoots calls fn with each held root whose name starts with prefix.
	ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error
}

// Pins is the registry of the refs a process holds outside its named roots,
// such as the snapshot root a reader pins while it reads. The consumer fills
// it. A pin is added before the write log sees it as a root, so a pin that
// Roots does not list has its root line after the R that Roots read first.
type Pins struct {
	// observe sees each pin as a root, nil for none.
	observe func(*hash.Hash)
	// mtx guards the fields below.
	mtx sync.Mutex
	// next numbers the next pin.
	next uint64
	// refs are the held pins by number.
	refs map[uint64]*block.BlockRef
}

// NewPins builds an empty registry. observe sees each pin as a root, such as
// writelog.Log.Root; nil observes none.
func NewPins(observe func(*hash.Hash)) *Pins {
	return &Pins{observe: observe, refs: make(map[uint64]*block.BlockRef)}
}

// Pin holds ref until the returned release is called. Releasing twice
// releases once.
func (p *Pins) Pin(ref *block.BlockRef) func() {
	// Add the pin before the log sees it.
	if ref.GetEmpty() {
		return func() {}
	}
	p.mtx.Lock()
	n := p.next
	p.next++
	p.refs[n] = ref
	p.mtx.Unlock()
	if p.observe != nil {
		p.observe(ref.GetHash())
	}

	// Release it once.
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mtx.Lock()
			delete(p.refs, n)
			p.mtx.Unlock()
		})
	}
}

// List returns the held pins, each ref once.
func (p *Pins) List() []*block.BlockRef {
	// Copy the pins out under the lock.
	p.mtx.Lock()
	refs := make([]*block.BlockRef, 0, len(p.refs))
	for _, ref := range p.refs {
		refs = append(refs, ref)
	}
	p.mtx.Unlock()

	// Drop the repeats.
	seen := make(map[string]bool, len(refs))
	out := refs[:0]
	for _, ref := range refs {
		if key := ref.MarshalString(); !seen[key] {
			seen[key] = true
			out = append(out, ref)
		}
	}
	return out
}

// Roots reads the write log's position R, then every named root, then every
// pin. A root or pin set after R is in the log after R, since the write-back
// logs a root it sets and Pins logs a pin after adding it; one set before R and
// still held is listed. So the roots and the writes since R together cover
// every block held at any point since R.
func (s *Server) Roots(ctx context.Context, _ *RootsRequest) (*RootsResponse, error) {
	// Read R first, with every line before it flushed.
	if s.opts.Roots == nil {
		return nil, errors.New("the backend lists no roots")
	}
	resp := &RootsResponse{}
	if s.opts.Log != nil {
		if pos, ok := s.opts.Log.Position(); ok {
			resp.Position = pos.String()
		}
	}

	// List the named roots, then the pins.
	err := s.opts.Roots.ListRoots(ctx, "", func(root block.NamedRoot) error {
		if !root.Ref.GetEmpty() {
			resp.Roots = append(resp.Roots, &NamedRootRef{Name: root.Name, Ref: root.Ref})
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "list the named roots")
	}
	if s.opts.Pins != nil {
		resp.Pins = s.opts.Pins.List()
	}
	return resp, nil
}

// WalkRoots reads the backend's roots and pins as one list of refs for a
// walk, with the log position R they were read at, empty while the log is off.
func (c *Client) WalkRoots(ctx context.Context) ([]*block.BlockRef, string, error) {
	// Read the roots, then list the named roots before the pins.
	resp, err := c.Roots(ctx, &RootsRequest{})
	if err != nil {
		return nil, "", err
	}
	refs := make([]*block.BlockRef, 0, len(resp.GetRoots())+len(resp.GetPins()))
	for _, root := range resp.GetRoots() {
		refs = append(refs, root.GetRef())
	}
	return append(refs, resp.GetPins()...), resp.GetPosition(), nil
}
