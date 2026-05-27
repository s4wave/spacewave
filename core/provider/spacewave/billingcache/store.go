package billingcache

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// Fetcher fetches encoded billing snapshots.
type Fetcher interface {
	GetBillingState(ctx context.Context, baID string) ([]byte, error)
	GetBillingUsage(ctx context.Context, baID string) ([]byte, error)
}

// Store caches billing account state and usage by billing account id.
type Store struct {
	mut   sync.Mutex
	opts  *refcount.Options
	cache map[string]*refcount.RefCount[*snapshot]
}

type snapshot struct {
	state *api.BillingStateResponse
	usage *api.BillingUsageResponse
}

// NewStore builds a billing snapshot store.
func NewStore(opts *refcount.Options) *Store {
	return &Store{opts: opts}
}

// Invalidate invalidates one cached billing snapshot, or all snapshots when
// baID is empty.
func (s *Store) Invalidate(baID string) {
	if s == nil {
		return
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	if baID == "" {
		for _, rc := range s.cache {
			rc.Invalidate()
		}
		return
	}
	rc := s.cache[baID]
	if rc != nil {
		rc.Invalidate()
	}
}

// Get returns cached billing state and usage, fetching on cache miss.
func (s *Store) Get(
	ctx context.Context,
	baID string,
	fetcher Fetcher,
) (*api.BillingStateResponse, *api.BillingUsageResponse, error) {
	if baID == "" {
		return nil, nil, errors.New("billing account id is required")
	}
	if fetcher == nil {
		return nil, nil, errors.New("session client not available")
	}
	if s == nil {
		return nil, nil, errors.New("billing cache not available")
	}

	rc := s.refCount(baID, fetcher)
	snap, rel, err := rc.Resolve(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer rel()
	if snap == nil {
		return nil, nil, errors.New("billing snapshot not available")
	}
	var state *api.BillingStateResponse
	if snap.state != nil {
		state = snap.state.CloneVT()
	}
	var usage *api.BillingUsageResponse
	if snap.usage != nil {
		usage = snap.usage.CloneVT()
	}
	return state, usage, nil
}

func (s *Store) refCount(
	baID string,
	fetcher Fetcher,
) *refcount.RefCount[*snapshot] {
	s.mut.Lock()
	defer s.mut.Unlock()
	if s.cache == nil {
		s.cache = make(map[string]*refcount.RefCount[*snapshot])
	}
	rc := s.cache[baID]
	if rc == nil {
		rc = refcount.NewRefCountWithOptions(
			context.Background(),
			true,
			nil,
			nil,
			func(ctx context.Context, _ func()) (*snapshot, func(), error) {
				return resolve(ctx, baID, fetcher)
			},
			s.opts,
		)
		s.cache[baID] = rc
	}
	return rc
}

func resolve(
	ctx context.Context,
	baID string,
	fetcher Fetcher,
) (*snapshot, func(), error) {
	stateData, err := fetcher.GetBillingState(ctx, baID)
	if err != nil {
		return nil, nil, err
	}
	state := &api.BillingStateResponse{}
	if err := state.UnmarshalVT(stateData); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal billing state")
	}

	usageData, err := fetcher.GetBillingUsage(ctx, baID)
	if err != nil {
		return nil, nil, err
	}
	usage := &api.BillingUsageResponse{}
	if err := usage.UnmarshalVT(usageData); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal billing usage")
	}

	return &snapshot{
		state: state,
		usage: usage,
	}, nil, nil
}
