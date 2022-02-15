package identity_world

import (
	"context"
	"sort"
	"strings"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/identity"
	identity_domain "github.com/aperturerobotics/identity/domain"
	"github.com/cayleygraph/cayley"
	"github.com/pkg/errors"
)

const (
	// DomainInfoPrefix is the prefix applied to the domain info.
	DomainInfoPrefix = "d/"
	// DomainInfoTypeID is the type identifier for a DomainInfo.
	DomainInfoTypeID = "identity/domain"
)

// NewDomainInfoKey builds a key from a domain id.
func NewDomainInfoKey(id string) string {
	return DomainInfoPrefix + id
}

// FollowDomainInfo follows & checks a reference to a DomainInfo.
func FollowDomainInfo(
	ctx context.Context,
	accessState world.AccessWorldStateFunc,
	domainInfoRef *bucket.ObjectRef,
) (*identity_domain.DomainInfo, error) {
	var err error
	if domainInfoRef.GetEmpty() {
		return nil, errors.New("empty domain info ref")
	}
	if err := domainInfoRef.Validate(); err != nil {
		return nil, err
	}
	var domain *identity_domain.DomainInfo
	_, err = world.AccessObject(ctx, accessState, domainInfoRef, func(bcs *block.Cursor) error {
		// Confirm valid DomainInfo object.
		var err error
		domain, err = identity_domain.UnmarshalDomainInfo(bcs)
		return err
	})
	if err == nil {
		err = domain.Validate()
	}
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// LookupDomainInfoOp performs the lookup operation for the DomainInfo op types.
func LookupDomainInfoOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case DomainInfoUpdateOpId:
		return &DomainInfoUpdateOp{}, nil
	}
	return nil, nil
}

// LookupDomainInfo looks up an entity with the given key.
// returns nil, nil, nil if not found.
func LookupDomainInfo(ctx context.Context, w world.WorldState, objKey string) (*identity_domain.DomainInfo, world.ObjectState, error) {
	obj, objFound, err := w.GetObject(objKey)
	if err != nil {
		return nil, nil, err
	}
	if !objFound {
		return nil, nil, nil
	}
	var entity *identity_domain.DomainInfo
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		entity, err = identity_domain.UnmarshalDomainInfo(bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return entity, obj, nil
}

// LookupDomainInfos looks up a set of keypairs.
func LookupDomainInfos(ctx context.Context, w world.WorldState, objKeys []string) ([]*identity_domain.DomainInfo, error) {
	kps := make([]*identity_domain.DomainInfo, len(objKeys))
	for i, objKey := range objKeys {
		var err error
		kps[i], _, err = LookupDomainInfo(ctx, w, objKey)
		if err != nil {
			return nil, err
		}
	}
	return kps, nil
}

// CollectAllDomainInfos collects all DomainInfo states located in the store.
// returns list of entities and object keys
func CollectAllDomainInfos(ctx context.Context, w world.WorldState) ([]*identity_domain.DomainInfo, []string, error) {
	var objKeys []string
	ts := world_types.NewTypesState(ctx, w)
	err := ts.IterateObjectsWithType(DomainInfoTypeID, func(objKey string) (bool, error) {
		if !strings.HasPrefix(objKey, DomainInfoPrefix) {
			return true, nil
		}
		objKeys = append(objKeys, objKey)
		return true, nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(objKeys)

	// collect list
	list, err := LookupDomainInfos(ctx, w, objKeys)
	return list, objKeys, err
}

// ListDomainInfoEntities lists all Entity that link to the given domain object keys.
// returns list of object keys
func ListDomainInfoEntities(ctx context.Context, w world.WorldState, domainKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		domainKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.In(PredEntityToDomainInfo), nil
		},
	)
}

// CollectDomainInfoEntities collects all Entity linking to the domain.
// returns list of Entities and object keys
func CollectDomainInfoEntities(ctx context.Context, w world.WorldState, domainKeys ...string) ([]*identity.Entity, error) {
	objKeys, err := ListDomainInfoEntities(ctx, w, domainKeys...)
	if err != nil {
		return nil, err
	}

	return LookupEntities(ctx, w, objKeys)
}
