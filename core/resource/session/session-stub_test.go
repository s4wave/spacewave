package resource_session

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// testSyncStatusSession is a Session with only a provider account.
type testSyncStatusSession struct {
	acc provider.ProviderAccount
}

func (s *testSyncStatusSession) GetBus() bus.Bus {
	return nil
}

func (s *testSyncStatusSession) GetSessionRef() *session.SessionRef {
	return nil
}

func (s *testSyncStatusSession) GetPeerId() peer.ID {
	return ""
}

func (s *testSyncStatusSession) GetPrivKey() crypto.PrivKey {
	return nil
}

func (s *testSyncStatusSession) GetProviderAccount() provider.ProviderAccount {
	return s.acc
}

func (s *testSyncStatusSession) AccessStateAtomStore(
	context.Context,
	string,
) (resource_state.StateAtomStore, error) {
	return nil, errors.New("not implemented")
}

func (s *testSyncStatusSession) SnapshotStateAtomStoreIDs(context.Context) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (s *testSyncStatusSession) WatchStateAtomStoreIDs(
	context.Context,
	func(storeIDs []string) error,
) error {
	return errors.New("not implemented")
}

func (s *testSyncStatusSession) GetLockState(
	context.Context,
) (session.SessionLockMode, bool, error) {
	return session.SessionLockMode_SESSION_LOCK_MODE_AUTO_UNLOCK, false, errors.New("not implemented")
}

func (s *testSyncStatusSession) WatchLockState(
	context.Context,
	func(mode session.SessionLockMode, locked bool),
) error {
	return errors.New("not implemented")
}

func (s *testSyncStatusSession) UnlockSession(context.Context, []byte) error {
	return errors.New("not implemented")
}

func (s *testSyncStatusSession) SetLockMode(
	context.Context,
	session.SessionLockMode,
	[]byte,
) error {
	return errors.New("not implemented")
}

func (s *testSyncStatusSession) LockSession(context.Context) error {
	return errors.New("not implemented")
}

func testWaitChannels(count int, sourceIdx int) ([]<-chan struct{}, func()) {
	// Build wait channels and a closer for one of them.
	chans := make([]chan struct{}, count)
	waitChs := make([]<-chan struct{}, 0, count+2)
	waitChs = append(waitChs, nil)
	for i := range chans {
		chans[i] = make(chan struct{})
		waitChs = append(waitChs, chans[i])
	}
	waitChs = append(waitChs, nil)
	return waitChs, func() {
		close(chans[sourceIdx])
	}
}

// _ is a type assertion
var _ session.Session = (*testSyncStatusSession)(nil)
