package provider_spacewave

import (
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/accountstatus"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// canMutateCloudObjects returns true when cached account status, billing, and
// lifecycle state all permit owner-side cloud mutations.
func (a *ProviderAccount) canMutateCloudObjects() bool {
	var state *api.AccountStateResponse
	var status provider.ProviderAccountStatus
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = a.state.info
		status = a.state.status
	})
	return accountstatus.CanMutateCloudObjects(status, state)
}

// canSelfEnrollCloudObjects returns true when cached account status, billing,
// and lifecycle state permit same-entity cloud self-enrollment for export.
func (a *ProviderAccount) canSelfEnrollCloudObjects() bool {
	var state *api.AccountStateResponse
	var status provider.ProviderAccountStatus
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = a.state.info
		status = a.state.status
	})
	return accountstatus.CanSelfEnrollCloudObjects(status, state)
}
