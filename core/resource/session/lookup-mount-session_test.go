package resource_session_test

import (
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
)

// lookupMountSession replaces only discovery while retaining the real Session.
type lookupMountSession struct {
	// Session supplies the real bus, keys and local durable data.
	session.Session
	// account supplies a loading catalog and targeted discovery.
	account provider.ProviderAccount
}

// GetProviderAccount returns the fixture's feature-preserving account wrapper.
func (s lookupMountSession) GetProviderAccount() provider.ProviderAccount {
	return s.account
}

var _ session.Session = lookupMountSession{}
