//go:build e2e

package onboarding_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

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
	ctx, cancel := context.WithTimeout(env.ctx, 75*time.Second)
	defer cancel()

	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

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

	setTestSubscriptionStatus(t, cloudAccountID, "active")
	setTestEmailVerified(t, ctx, cloudAccountID, "sync-status-"+ulid.NewULID()+"@example.com")
	swAcc.BumpLocalEpoch()
	status, err := waitForSubscriptionStatus(ctx, swAcc, "active")
	if err != nil {
		t.Fatalf("waiting for active subscription: %v", err)
	}
	if status != "active" {
		t.Fatalf("subscription status = %q, want active", status)
	}

	soID := ulid.NewULID()
	if err := swAcc.GetSessionClient().CreateSharedObject(ctx, soID, "Sync Status", "space", "", "", false); err != nil {
		t.Fatal(err)
	}

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

	recvSyncStatusUntil(t, strm.msgs, func(resp *s4wave_session.WatchSyncStatusResponse) bool {
		return resp.GetState() == s4wave_session.SyncStatusState_SyncStatusState_SYNCED &&
			resp.GetDirection() == s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE
	})

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

	blockRef, existed, err := cloudStore.PutBlock(ctx, []byte("sync status upload "+ulid.NewULID()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Fatalf("block %s unexpectedly existed before upload", blockRef.MarshalString())
	}

	recvSyncStatusUntil(t, strm.msgs, func(resp *s4wave_session.WatchSyncStatusResponse) bool {
		return resp.GetState() == s4wave_session.SyncStatusState_SyncStatusState_ACTIVE &&
			resp.GetDirection() == s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD &&
			resp.GetPendingUploadBytes() > 0
	})

	if err := cloudStore.ForceSync(ctx); err != nil {
		t.Fatal(err)
	}

	recvSyncStatusUntil(t, strm.msgs, func(resp *s4wave_session.WatchSyncStatusResponse) bool {
		return resp.GetState() == s4wave_session.SyncStatusState_SyncStatusState_SYNCED &&
			resp.GetDirection() == s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE &&
			resp.GetPendingUploadBytes() == 0 &&
			resp.GetLastError() == ""
	})
}

func setTestEmailVerified(t *testing.T, ctx context.Context, accountID, email string) {
	t.Helper()

	body := `{"account_id":"` + accountID + `","email":"` + email + `","verified":true}`
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		env.cloudURL+"/api/test/set-email",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set verified email returned %d", resp.StatusCode)
	}
}

func recvSyncStatusUntil(
	t *testing.T,
	msgs <-chan *s4wave_session.WatchSyncStatusResponse,
	match func(*s4wave_session.WatchSyncStatusResponse) bool,
) *s4wave_session.WatchSyncStatusResponse {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case resp := <-msgs:
			if match(resp) {
				return resp
			}
		case <-deadline:
			t.Fatal("timed out waiting for sync status response")
		}
	}
}

// _ is a type assertion
var _ s4wave_session.SRPCSessionResourceService_WatchSyncStatusStream = ((*syncStatusWatchStream)(nil))
