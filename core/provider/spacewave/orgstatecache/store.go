package orgstatecache

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// Fetcher fetches encoded organization snapshots.
type Fetcher interface {
	GetOrganization(ctx context.Context, orgID string) ([]byte, error)
	ListOrgInvites(ctx context.Context, orgID string) ([]byte, error)
}

// RoleLookup returns the current caller's role for an organization.
type RoleLookup func(ctx context.Context, orgID string) (string, error)

// Store caches organization detail snapshots by organization id.
type Store struct {
	mut   sync.Mutex
	opts  *refcount.Options
	cache map[string]*refcount.RefCount[*snapshot]
}

type snapshot struct {
	info    *api.GetOrgResponse
	invites *api.ListOrgInvitesResponse
	roleID  string
}

// NewStore builds an organization state cache.
func NewStore(opts *refcount.Options) *Store {
	return &Store{opts: opts}
}

// Invalidate invalidates one organization snapshot, or all snapshots when orgID
// is empty.
func (s *Store) Invalidate(orgID string) {
	if s == nil {
		return
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	if orgID == "" {
		for _, rc := range s.cache {
			rc.Invalidate()
		}
		return
	}
	rc := s.cache[orgID]
	if rc != nil {
		rc.Invalidate()
	}
}

// Get returns cached organization details and invites, fetching on cache miss.
func (s *Store) Get(
	ctx context.Context,
	orgID string,
	fetcher Fetcher,
	roleLookup RoleLookup,
) (*api.GetOrgResponse, *api.ListOrgInvitesResponse, string, error) {
	if orgID == "" {
		return nil, nil, "", errors.New("organization id is required")
	}
	if fetcher == nil {
		return nil, nil, "", errors.New("session client not available")
	}
	if s == nil {
		return nil, nil, "", errors.New("organization cache not available")
	}

	rc := s.refCount(orgID, fetcher, roleLookup)
	snap, rel, err := rc.Resolve(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	defer rel()
	if snap == nil || snap.info == nil {
		return nil, nil, "", errors.New("organization snapshot not available")
	}
	var invites *api.ListOrgInvitesResponse
	if snap.invites != nil {
		invites = snap.invites.CloneVT()
	}
	return snap.info.CloneVT(), invites, snap.roleID, nil
}

func (s *Store) refCount(
	orgID string,
	fetcher Fetcher,
	roleLookup RoleLookup,
) *refcount.RefCount[*snapshot] {
	s.mut.Lock()
	defer s.mut.Unlock()
	if s.cache == nil {
		s.cache = make(map[string]*refcount.RefCount[*snapshot])
	}
	rc := s.cache[orgID]
	if rc == nil {
		rc = refcount.NewRefCountWithOptions(
			context.Background(),
			true,
			nil,
			nil,
			func(ctx context.Context, _ func()) (*snapshot, func(), error) {
				return resolve(ctx, orgID, fetcher, roleLookup)
			},
			s.opts,
		)
		s.cache[orgID] = rc
	}
	return rc
}

func resolve(
	ctx context.Context,
	orgID string,
	fetcher Fetcher,
	roleLookup RoleLookup,
) (*snapshot, func(), error) {
	data, err := fetcher.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	info := &api.GetOrgResponse{}
	if err := info.UnmarshalVT(data); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal org info")
	}

	var roleID string
	if roleLookup != nil {
		roleID, err = roleLookup(ctx, orgID)
		if err != nil {
			return nil, nil, err
		}
	}

	var invites *api.ListOrgInvitesResponse
	if isOrganizationOwnerRole(roleID) {
		inviteData, err := fetcher.ListOrgInvites(ctx, orgID)
		if err != nil {
			return nil, nil, err
		}
		invites = &api.ListOrgInvitesResponse{}
		if err := invites.UnmarshalVT(inviteData); err != nil {
			return nil, nil, errors.Wrap(err, "unmarshal invite list")
		}
	}

	if invites == nil {
		invites = &api.ListOrgInvitesResponse{}
	}
	return &snapshot{
		info:    info,
		invites: invites,
		roleID:  roleID,
	}, nil, nil
}

func isOrganizationOwnerRole(roleID string) bool {
	return roleID == "owner" || roleID == "org:owner"
}
