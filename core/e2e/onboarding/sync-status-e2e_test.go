//go:build e2e

package onboarding_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

type syncStatusWatchStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_session.WatchSyncStatusResponse
}

func newSyncStatusWatchStream(ctx context.Context) *syncStatusWatchStream {
	return &syncStatusWatchStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_session.WatchSyncStatusResponse, 8),
	}
}

func (s *syncStatusWatchStream) Context() context.Context {
	return s.ctx
}

func (s *syncStatusWatchStream) Send(resp *s4wave_session.WatchSyncStatusResponse) error {
	select {
	case s.msgs <- resp:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *syncStatusWatchStream) SendAndClose(resp *s4wave_session.WatchSyncStatusResponse) error {
	return s.Send(resp)
}

func (s *syncStatusWatchStream) MsgRecv(_ srpc.Message) error {
	return nil
}

func (s *syncStatusWatchStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (s *syncStatusWatchStream) CloseSend() error {
	return nil
}

func (s *syncStatusWatchStream) Close() error {
	return nil
}

func TestCloudSyncStatusUploadLifecycle(t *testing.T) {
	// Bound the upload lifecycle.
	ctx, cancel := context.WithTimeout(env.ctx, 75*time.Second)
	defer cancel()

	// Open a cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Access the session's account through the spacewave provider.
	prov, provRef, err := provider.ExLookupProvider(ctx, env.tb.Bus, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	// Subscribe the account and wait for the session to see it active.
	subscribeCloudAccount(ctx, t, cloudAccountID)
	swAcc.BumpLocalEpoch()
	status, err := waitForSubscriptionStatus(ctx, swAcc, "active")
	if err != nil {
		t.Fatalf("waiting for active subscription: %v", err)
	}
	if status != "active" {
		t.Fatalf("subscription status = %q, want active", status)
	}

	// Create the Space whose block store the test uploads to.
	soID := ulid.NewULID()
	if err := swAcc.GetSessionClient().CreateSharedObject(ctx, soID, "Sync Status", "space", "", "", false); err != nil {
		t.Fatal(err)
	}

	// Watch the session's sync status until the test ends.
	cloudResource, _, relCloudResource := mountSessionResource(ctx, t, cloudEntry)
	defer relCloudResource()
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	strm := newSyncStatusWatchStream(watchCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- cloudResource.WatchSyncStatus(&s4wave_session.WatchSyncStatusRequest{}, strm)
	}()
	defer func() {
		watchCancel()
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("WatchSyncStatus returned %v", err)
		}
	}()

	// The new session settles to synced before the upload.
	waitSyncStatusSynced(ctx, t, strm.msgs, swAcc, nil)

	// Mount the Space's cloud block store.
	bstoreID := provider_local.SobjectBlockStoreID(soID)
	bstoreRef := provider_spacewave.NewBlockStoreRef(
		swAcc.GetProviderID(),
		swAcc.GetAccountID(),
		bstoreID,
	)
	bs, relBstore, err := swAcc.MountBlockStore(ctx, bstoreRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relBstore()
	cloudStore, ok := bs.(*provider_spacewave.BlockStore)
	if !ok {
		t.Fatalf("block store type = %T, want *provider_spacewave.BlockStore", bs)
	}

	// Put a new block into the store.
	blockRef, existed, err := cloudStore.PutBlock(ctx, []byte("sync status upload "+ulid.NewULID()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Fatalf("block %s unexpectedly existed before upload", blockRef.MarshalString())
	}

	// The status reports the upload, then settles to synced again.
	uploading := recvSyncStatusUntil(t, strm.msgs, func(resp *s4wave_session.WatchSyncStatusResponse) bool {
		return resp.GetState() == s4wave_session.SyncStatusState_SyncStatusState_ACTIVE &&
			resp.GetDirection() == s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD &&
			resp.GetPendingUploadBytes() > 0
	})
	waitSyncStatusSynced(ctx, t, strm.msgs, swAcc, uploading)
}

// waitSyncStatusSynced flushes every block store the status reports with
// pending work until the session reads synced with no error. The session's
// own Space publishes its state on the checkpoint interval, which is longer
// than the test waits. The watch sends only changes, so the caller passes the
// last status it read, or nil before the first.
func waitSyncStatusSynced(
	ctx context.Context,
	t *testing.T,
	msgs <-chan *s4wave_session.WatchSyncStatusResponse,
	acc *provider_spacewave.ProviderAccount,
	last *s4wave_session.WatchSyncStatusResponse,
) {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		if last.GetState() == s4wave_session.SyncStatusState_SyncStatusState_SYNCED &&
			last.GetDirection() == s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE &&
			last.GetPendingUploadBytes() == 0 &&
			last.GetLastError() == "" {
			return
		}
		if last.GetPendingUploadCount() != 0 {
			for _, store := range last.GetBlockStores() {
				forceSyncBlockStore(ctx, t, acc, store.GetBlockStoreId())
			}
		}
		select {
		case last = <-msgs:
		case <-deadline:
			t.Fatalf("timed out waiting for synced status, last: %v", last)
		}
	}
}

// forceSyncBlockStore flushes the pending blocks and publications of one of
// the account's cloud block stores.
func forceSyncBlockStore(ctx context.Context, t *testing.T, acc *provider_spacewave.ProviderAccount, bstoreID string) {
	// Mount the block store.
	t.Helper()
	ref := provider_spacewave.NewBlockStoreRef(acc.GetProviderID(), acc.GetAccountID(), bstoreID)
	bs, rel, err := acc.MountBlockStore(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Flush its pending blocks and publications.
	if err := bs.(*provider_spacewave.BlockStore).ForceSync(ctx); err != nil {
		t.Fatal(err)
	}
}

// setTestEmailVerified gives the account email as its verified address.
func setTestEmailVerified(t *testing.T, ctx context.Context, accountID, email string) {
	// Set the email through the coordinator test helper.
	t.Helper()
	var a fastjson.Arena
	body := a.NewObject()
	body.Set("account_id", a.NewString(accountID))
	body.Set("email", a.NewString(email))
	body.Set("verified", a.NewTrue())
	postTestHelper(ctx, t, "/api/test/set-email", body)
}

func recvSyncStatusUntil(
	t *testing.T,
	msgs <-chan *s4wave_session.WatchSyncStatusResponse,
	match func(*s4wave_session.WatchSyncStatusResponse) bool,
) *s4wave_session.WatchSyncStatusResponse {
	t.Helper()
	deadline := time.After(20 * time.Second)
	var last *s4wave_session.WatchSyncStatusResponse
	for {
		select {
		case resp := <-msgs:
			if match(resp) {
				return resp
			}
			last = resp
		case <-deadline:
			t.Fatalf("timed out waiting for sync status response, last: %v", last)
		}
	}
}

// _ is a type assertion
var _ s4wave_session.SRPCSessionResourceService_WatchSyncStatusStream = (*syncStatusWatchStream)(nil)
