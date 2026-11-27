package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
)

// LookupSharedObject leaves local discovery to the existing catalog.
func (a *ProviderAccount) LookupSharedObject(context.Context, string) (*sobject.SharedObjectListEntry, error) {
	return nil, sobject.ErrSharedObjectLookupUnsupported
}
