package resource_session_test

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	cloudapi "github.com/aperturerobotics/spacewave-cloud/api"
	"github.com/aperturerobotics/util/ccontainer"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestMountSharedObjectDuringCatalogLoad mounts real local data before the
// catalog's first value, resolves a loaded miss and keeps offline hits local.
func TestMountSharedObjectDuringCatalogLoad(t *testing.T) {
	// Build durable local Space data and its real Session controllers.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	env := setupTestEnv(ctx, t)
	sessRef, _ := env.createSession(ctx, t)
	account := env.accessAccount(ctx, t, sessRef)
	createSpaceWithIndexObjectType(ctx, t, env, account, "01j00000000000000000000000", "Lookup", "index", "test/lookup")
	catalog, releaseCatalog, err := account.AccessSharedObjectList(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCatalog()
	var entry *sobject.SharedObjectListEntry
	for _, candidate := range catalog.GetValue().GetSharedObjects() {
		if candidate.GetRef().GetProviderResourceRef().GetId() == "01j00000000000000000000000" {
			entry = candidate
			break
		}
	}
	if entry == nil {
		t.Fatal("real local Space has no descriptor")
	}
	sess, sessionRef, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessionRef.Release()

	// Hold discovery at loading, loaded-miss and offline-hit snapshots in turn.
	for _, state := range []string{"loading", "miss", "cached"} {
		// Replace only discovery; actual SharedObject mounts use the real bus.
		list := ccontainer.NewCContainer[*sobject.SharedObjectList](nil)
		if state == "miss" {
			list.SetValue(&sobject.SharedObjectList{})
		}
		if state == "cached" {
			list.SetValue(&sobject.SharedObjectList{SharedObjects: []*sobject.SharedObjectListEntry{entry}})
		}
		// Serve a real signed client request beside a gated catalog fetch.
		var requests atomic.Int32
		catalogStarted, catalogRelease := make(chan struct{}), make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/sobject/list" {
				close(catalogStarted)
				select {
				case <-catalogRelease:
				case <-r.Context().Done():
				}
				return
			}
			if r.Method != http.MethodGet || r.URL.Path != "/api/sobject/01j00000000000000000000000/lookup" || r.Header.Get("X-Signature") == "" {
				t.Errorf("unexpected signed lookup: %s %s", r.Method, r.URL.Path)
			}
			requests.Add(1)
			data, _ := (&cloudapi.LookupSObjectResponse{Entry: entry}).MarshalVT()
			_, _ = w.Write(data)
		}))
		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pid, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			t.Fatal(err)
		}
		cli := provider_spacewave.NewSessionClient(http.DefaultClient, srv.URL, "lookup-test", priv, pid.String())
		var catalogDone chan error
		if state == "loading" {
			catalogDone = make(chan error, 1)
			go func() { _, err := cli.ListSharedObjects(ctx); catalogDone <- err }()
			<-catalogStarted
		}
		defer func() {
			close(catalogRelease)
			if catalogDone != nil {
				<-catalogDone
			}
			srv.Close()
		}()
		if state == "cached" {
			srv.Close()
		}
		wrapped := &lookupMountAccount{ProviderAccount: account, SharedObjectProvider: account, list: list, lookup: cli.LookupSObject}
		resource := resource_session.NewSessionResource(env.tb.Logger, env.tb.Bus, lookupMountSession{Session: sess, account: wrapped})
		resources := &lookupMountResources{}
		mountCtx := resource_server.WithResourceClientContext(ctx, resources)

		// Mount before any catalog release, using the durable local object.
		resp, err := resource.MountSharedObject(mountCtx, &s4wave_session.MountSharedObjectRequest{SharedObjectId: "01j00000000000000000000000"})
		if resources.release != nil {
			resources.release()
		}
		if err != nil || resp.GetResourceId() == 0 || resp.GetSharedObjectId() != "01j00000000000000000000000" {
			t.Fatalf("%s mount: %v %v", state, resp, err)
		}
		want := 1
		if state == "cached" {
			want = 0
		}
		if wrapped.lookups != want || wrapped.refreshes != 0 || requests.Load() != int32(want) {
			t.Fatalf("%s: lookups=%d refreshes=%d", state, wrapped.lookups, wrapped.refreshes)
		}
	}
}
