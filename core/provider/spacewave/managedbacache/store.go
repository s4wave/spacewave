package managedbacache

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// Fetcher fetches the encoded managed billing account list.
type Fetcher interface {
	ListManagedBillingAccounts(ctx context.Context) ([]byte, error)
}

// Store caches billing accounts managed by the current caller.
type Store struct {
	mut sync.Mutex
	rc  *refcount.RefCount[*snapshot]

	opts *refcount.Options
}

type snapshot struct {
	accounts []*s4wave_provider_spacewave.ManagedBillingAccount
}

// NewStore builds a managed billing account cache.
func NewStore(opts *refcount.Options) *Store {
	return &Store{opts: opts}
}

// Invalidate invalidates the cached account list.
func (s *Store) Invalidate() {
	if s == nil {
		return
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	if s.rc != nil {
		s.rc.Invalidate()
	}
}

// Get returns cached managed billing accounts, fetching on cache miss.
func (s *Store) Get(
	ctx context.Context,
	fetcher Fetcher,
) ([]*s4wave_provider_spacewave.ManagedBillingAccount, error) {
	if fetcher == nil {
		return nil, errors.New("session client not available")
	}
	if s == nil {
		return nil, errors.New("managed billing account cache not available")
	}

	rc := s.refCount(fetcher)
	snap, rel, err := rc.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()
	if snap == nil {
		return nil, nil
	}
	accounts := make([]*s4wave_provider_spacewave.ManagedBillingAccount, 0, len(snap.accounts))
	for _, account := range snap.accounts {
		if account == nil {
			accounts = append(accounts, nil)
			continue
		}
		accounts = append(accounts, account.CloneVT())
	}
	return accounts, nil
}

func (s *Store) refCount(fetcher Fetcher) *refcount.RefCount[*snapshot] {
	s.mut.Lock()
	defer s.mut.Unlock()
	if s.rc == nil {
		s.rc = refcount.NewRefCountWithOptions(
			context.Background(),
			true,
			nil,
			nil,
			func(ctx context.Context, _ func()) (*snapshot, func(), error) {
				return resolve(ctx, fetcher)
			},
			s.opts,
		)
	}
	return s.rc
}

func resolve(
	ctx context.Context,
	fetcher Fetcher,
) (*snapshot, func(), error) {
	data, err := fetcher.ListManagedBillingAccounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	resp := &s4wave_provider_spacewave.ListManagedBillingAccountsResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal managed BA list")
	}
	return &snapshot{accounts: resp.GetAccounts()}, nil, nil
}
