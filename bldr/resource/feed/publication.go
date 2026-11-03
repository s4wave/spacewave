package feed

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	"github.com/s4wave/spacewave/db/world/keyedview"
	"google.golang.org/protobuf/encoding/protowire"
)

// ViewField is the field number of the resource.ViewVersion each versioned
// view message carries after its own fields, and of the held_version each of
// their requests carries.
const ViewField = 1000

// Publication is one publication of a feed: a view shared by every watcher, or
// the error that ended the feed. A watcher that is behind skips to the latest
// publication, so a slow watcher never holds back the feed or another
// watcher.
type Publication[T any] struct {
	// Value is the published view; shared, so read-only. Zero before the
	// first view and when err is set.
	Value T
	// Revision is a World revision the view reflects every write up to.
	Revision uint64
	// View names the view's records and its version, or is nil for a feed
	// without them.
	View *keyedview.View

	// published reports a view, false before the first and when err is set.
	published bool
	// encode encodes Value, or is nil for a feed that does not.
	encode func(T) ([]byte, error)

	// history remembers the feed's recent Views.
	history *keyedview.History
	// once encodes Value at the first Encoded call.
	once sync.Once
	// data is Value's encoding once encoded.
	data []byte
	// encodeErr is the error encoding Value returned.
	encodeErr error
	// whole is the encoding followed by the ViewVersion, and unchanged the
	// ViewVersion alone marked unchanged, each built at its first Frame.
	whole, unchanged frame
	// err ends every watcher of the feed.
	err error
	// next closes when a newer publication replaces this one.
	next chan struct{}
}

// Version returns the name of the published view's content, or empty for a
// feed without versions.
func (p *Publication[T]) Version() string {
	if p.View == nil {
		return ""
	}
	return p.View.Version
}

// Since returns what a watcher holding version lacks to reach this
// publication: nothing when it holds this version, the changed and removed
// records when the feed remembers it, and every record otherwise. A feed
// without versions always has a reader take everything.
func (p *Publication[T]) Since(version string) keyedview.Changes {
	if p.View == nil {
		return keyedview.Changes{Full: true}
	}
	return p.history.Since(version, p.View)
}

// Encoded returns the published view's encoding, encoded once for every
// watcher, which must not modify it.
func (p *Publication[T]) Encoded() ([]byte, error) {
	if p.encode == nil {
		return nil, errors.New("feed does not encode its views")
	}
	p.once.Do(func() {
		p.data, p.encodeErr = p.encode(p.Value)
	})
	return p.data, p.encodeErr
}

// Frame returns the message a watcher holding version held receives: the
// ViewVersion alone, marked unchanged, when it holds this publication's
// version, and otherwise the whole view followed by its ViewVersion. Each
// answer is built once for every watcher, which must not modify it. A feed
// without versions sends its encoding alone.
func (p *Publication[T]) Frame(held string) ([]byte, error) {
	// Answer with the version alone when the watcher holds it.
	data, err := p.Encoded()
	if err != nil || p.View == nil {
		return data, err
	}
	if held == p.View.Version {
		return p.unchanged.build(func() ([]byte, error) { return p.appendVersion(nil, true) })
	}

	// Otherwise send the whole view, copied so the shared encoding keeps its
	// length.
	return p.whole.build(func() ([]byte, error) {
		return p.appendVersion(append(make([]byte, 0, len(data)+32), data...), false)
	})
}

// appendVersion appends the publication's ViewVersion to data as field
// ViewField. Protobuf merges a message's fields, so the result decodes as the
// view with its version set.
func (p *Publication[T]) appendVersion(data []byte, unchanged bool) ([]byte, error) {
	version, err := (&resource.ViewVersion{Version: p.View.Version, Revision: p.Revision, Unchanged: unchanged}).MarshalVT()
	if err != nil {
		return nil, err
	}
	data = protowire.AppendTag(data, ViewField, protowire.BytesType)
	return protowire.AppendBytes(data, version), nil
}

// frame is one answer of a publication, built once.
type frame struct {
	once sync.Once
	data []byte
	err  error
}

// build returns the answer, building it with encode at the first call.
func (f *frame) build(encode func() ([]byte, error)) ([]byte, error) {
	f.once.Do(func() { f.data, f.err = encode() })
	return f.data, f.err
}

// rawRecord is an encoded record.
type rawRecord []byte

// MarshalVT returns the encoding.
func (r rawRecord) MarshalVT() ([]byte, error) {
	return r, nil
}
