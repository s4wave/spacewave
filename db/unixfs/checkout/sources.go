package unixfs_checkout

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// sources reads each block from the first source that serves it intact: the
// seed, then the cache, then each remote in order, keeping each block a
// remote serves in the cache. Every block is checked against its hash, so no
// source needs trust, and a source that fails or serves a damaged block
// passes the read to the next. Writes are refused.
type sources struct {
	// ReadOnlyStore serves the first source's hash type and refuses writes.
	*block.ReadOnlyStore
	// le logs failed sources.
	le *logrus.Entry
	// cache keeps the blocks remotes served.
	cache *blockCache
	// list holds the seed, when set, then the remotes.
	list []block.StoreOps
	// remote is the index in list of the first remote.
	remote int
	// yield runs before a read reaches the remotes when set and may hold
	// the read.
	yield func(context.Context) error
	// served counts the blocks and bytes each kind of source served.
	served [sourceKinds]struct{ blocks, bytes atomic.Uint64 }
}

// sourceKind is where sources found a block.
type sourceKind int

const (
	// fromSeed is the previous checkout beside the destination.
	fromSeed sourceKind = iota
	// fromCache is the blocks an interrupted attempt fetched.
	fromCache
	// fromRemote is the backend or the CDN.
	fromRemote
	// sourceKinds counts the kinds.
	sourceKinds
)

// count records a block of n bytes served by kind.
func (s *sources) count(kind sourceKind, n uint64) {
	s.served[kind].blocks.Add(1)
	s.served[kind].bytes.Add(n)
}

// fields describes what each kind of source served, for the checkout's log.
func (s *sources) fields() logrus.Fields {
	fields := logrus.Fields{}
	for kind, name := range [sourceKinds]string{"seed", "cache", "remote"} {
		fields[name+"_blocks"] = s.served[kind].blocks.Load()
		fields[name+"_bytes"] = s.served[kind].bytes.Load()
	}
	return fields
}

// newSources returns the blocks of seed, when set, then of cache, then of
// remotes.
func newSources(le *logrus.Entry, seed block.StoreOps, cache *blockCache, remotes []block.StoreOps) *sources {
	list := remotes
	if seed != nil {
		list = append([]block.StoreOps{seed}, remotes...)
	}
	return &sources{ReadOnlyStore: block.NewReadOnlyStore(list[0]), le: le, cache: cache, list: list, remote: len(list) - len(remotes)}
}

// BeginReadOperation returns the store itself.
func (s *sources) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// GetBlock reads ref from the first source holding it intact.
func (s *sources) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	var failed error
	for i, source := range s.list {
		// The cache answers between the seed and the remotes.
		if i == s.remote {
			if data, found := s.cache.GetBlock(ref); found && ref.VerifyData(data, false) == nil {
				s.count(fromCache, uint64(len(data)))
				return data, true, nil
			}
			if s.yield != nil {
				if err := s.yield(ctx); err != nil {
					return nil, false, err
				}
			}
		}

		// Keep each block a remote serves, so a retry reads it here. The seed
		// rebuilds blocks from files on disk and does not verify, so every
		// source keeps this check.
		data, found, err := source.GetBlock(ctx, ref)
		if err == nil && found {
			err = ref.VerifyData(data, false)
			if err == nil {
				if i < s.remote {
					s.count(fromSeed, uint64(len(data)))
				} else {
					s.count(fromRemote, uint64(len(data)))
					if err := s.cache.put(ref, data); err != nil {
						s.le.WithError(err).Debug("fetching the content block again on a retry: it could not be kept")
					}
				}
				return data, true, nil
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			s.le.WithError(err).Debug("reading a content block from the next source")
			failed = err
		}
	}
	return nil, false, failed
}

// GetStoredBlock reads ref as GetBlock does, without its references.
func (s *sources) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, s, ref)
}

// GetBlockExists reports whether any source holds ref.
func (s *sources) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if _, err := os.Stat(s.cache.path(ref)); err == nil {
		return true, nil
	}
	var failed error
	for _, source := range s.list {
		found, err := source.GetBlockExists(ctx, ref)
		if found {
			return true, nil
		}
		if err != nil {
			failed = err
		}
	}
	return false, failed
}

// GetBlockExistsBatch reports GetBlockExists for each ref.
func (s *sources) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	found := make([]bool, len(refs))
	for i, ref := range refs {
		var err error
		if found[i], err = s.GetBlockExists(ctx, ref); err != nil {
			return nil, err
		}
	}
	return found, nil
}

// seed returns the blocks of the checkout at directory, which a rewrite
// replaces, or else of the most recently used checkout beside it, hashed like
// root, or nil when there is none or it cannot be read: a checkout without a
// seed fetches every block.
func seed(ctx context.Context, le *logrus.Entry, codec Codec, directory string, root *block.BlockRef) block.StoreOps {
	// Prefer the checkout being replaced, then the most recently used sibling.
	previous := directory
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		previous = latestSibling(filepath.Dir(directory))
	}
	if previous == "" {
		return nil
	}

	// Store its files as the backend stored them.
	blocks, err := codec.Seed(ctx, le, os.DirFS(previous), root.GetHash().GetHashType())
	if err != nil {
		le.WithError(err).WithField("checkout", previous).Warn("fetching every block: the previous checkout could not seed this one")
		return nil
	}
	return blocks
}

// latestSibling returns the most recently used checkout in parent, or "".
func latestSibling(parent string) string {
	// Keep the visible directory used last.
	siblings, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}
	var previous string
	var used time.Time
	for _, sibling := range siblings {
		info, err := sibling.Info()
		if err != nil || !info.IsDir() || strings.HasPrefix(sibling.Name(), ".") || info.ModTime().Before(used) {
			continue
		}
		previous, used = filepath.Join(parent, sibling.Name()), info.ModTime()
	}
	return previous
}

// write bounds a source-backed write by arriving progress, excluding time
// spent yielding remote reads to foreground work.
func (s *sources) write(ctx context.Context, yield func(context.Context) error, write func(context.Context, func()) error) error {
	// Give up once nothing arrives for stallTimeout, so a source that
	// stops answering fails the checkout, which its caller retries, instead
	// of holding it at the same percent.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var held atomic.Int32
	var stall *time.Timer
	stall = time.AfterFunc(math.MaxInt64, func() {
		if held.Load() != 0 {
			stall.Reset(stallTimeout)
			return
		}
		cancel(errStalled)
	})
	stall.Reset(stallTimeout)
	defer stall.Stop()

	// Exclude time yielding to foreground work from the stall deadline.
	s.yield = func(ctx context.Context) error {
		held.Add(1)
		defer held.Add(-1)
		return yield(ctx)
	}
	err := write(ctx, func() { stall.Reset(stallTimeout) })
	if cause := context.Cause(ctx); err != nil && errors.Is(cause, errStalled) {
		return cause
	}
	return err
}

// _ is a type assertion
var _ block.StoreOps = (*sources)(nil)
