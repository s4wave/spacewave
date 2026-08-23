package provider_spacewave

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

func friendDmTestResponse() *api.GetFriendDmResponse {
	return &api.GetFriendDmResponse{
		Found:          true,
		SharedObjectId: "01frienddm",
		Ready:          true,
		OwnerAccountId: "acct-b",
		OwnerType:      "account",
		Accounts: []*api.FriendDmAccount{
			{
				AccountId:  "acct-a",
				EntityUuid: "entity-a",
				Epoch:      3,
				Sessions:   []*api.FriendDmSessionPeer{{PeerId: "peer-a"}},
				RecoveryKeypairs: []*api.FriendDmRecoveryPeer{
					{PeerId: "recovery-a"},
				},
			},
			{
				AccountId:  "acct-b",
				EntityUuid: "entity-b",
				Epoch:      4,
				Sessions:   []*api.FriendDmSessionPeer{{PeerId: "peer-b"}},
				RecoveryKeypairs: []*api.FriendDmRecoveryPeer{
					{PeerId: "recovery-b"},
				},
			},
		},
	}
}

func writeFriendDmResponse(t *testing.T, w http.ResponseWriter, resp *api.GetFriendDmResponse) {
	t.Helper()
	data, err := resp.MarshalVT()
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func TestGetFriendDM(t *testing.T) {
	priv, peerID := generateTestKeypair(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/account/friends/acct-b/dm" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		writeFriendDmResponse(t, w, friendDmTestResponse())
	}))
	defer srv.Close()

	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, peerID.String())
	got, err := cli.GetFriendDM(context.Background(), "acct-b")
	if err != nil {
		t.Fatalf("GetFriendDM: %v", err)
	}
	if !got.Ready || got.SharedObjectId != "01frienddm" {
		t.Fatalf("unexpected bootstrap: %+v", got)
	}
	if got.OwnerAccountId != "acct-b" || got.OwnerType != "account" {
		t.Fatalf("unexpected owner: %+v", got)
	}
	if len(got.Accounts) != 2 || got.Accounts[1].Sessions[0].PeerId != "peer-b" {
		t.Fatalf("unexpected accounts: %+v", got.Accounts)
	}
}

func TestCreateFriendDMWithState(t *testing.T) {
	priv, peerID := generateTestKeypair(t)
	configState, err := (&api.PostConfigStateRequest{
		ConfigChange: []byte("config"),
		KeyEpoch:     &sobject.SOKeyEpoch{Epoch: 0},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal config state: %v", err)
	}
	rootState, err := (&api.PostRootRequest{
		Root: &sobject.SORoot{InnerSeqno: 1, Inner: []byte("root")},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal root state: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/account/friends/acct-b/dm" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
			t.Fatalf("content type = %q, want application/octet-stream", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		req := &api.CreateWithStateRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		if req.DisplayName != "Friend DM" ||
			req.ObjectType != "space" ||
			req.OwnerType != "account" ||
			req.OwnerId != "acct-a" ||
			!req.AccountPrivate {
			t.Fatalf("unexpected request metadata: %+v", req)
		}
		postedConfig := &api.PostConfigStateRequest{}
		if err := postedConfig.UnmarshalVT(req.ConfigState); err != nil {
			t.Fatalf("unmarshal config state wrapper: %v", err)
		}
		if string(postedConfig.GetConfigChange()) != "config" ||
			postedConfig.GetKeyEpoch() == nil ||
			postedConfig.GetKeyEpoch().GetEpoch() != 0 {
			t.Fatalf("unexpected config state wrapper: %+v", postedConfig)
		}
		postedRoot := &api.PostRootRequest{}
		if err := postedRoot.UnmarshalVT(req.RootState); err != nil {
			t.Fatalf("unmarshal root state wrapper: %v", err)
		}
		if postedRoot.GetRoot() == nil ||
			postedRoot.GetRoot().GetInnerSeqno() != 1 ||
			string(postedRoot.GetRoot().GetInner()) != "root" {
			t.Fatalf("unexpected root state wrapper: %+v", postedRoot)
		}
		resp := friendDmTestResponse()
		resp.OwnerAccountId = "acct-a"
		writeFriendDmResponse(t, w, resp)
	}))
	defer srv.Close()

	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, peerID.String())
	got, err := cli.CreateFriendDMWithState(
		context.Background(),
		"acct-b",
		"acct-a",
		configState,
		rootState,
	)
	if err != nil {
		t.Fatalf("CreateFriendDMWithState: %v", err)
	}
	if !got.Ready || got.OwnerAccountId != "acct-a" {
		t.Fatalf("unexpected bootstrap: %+v", got)
	}
}

func TestGetFriendDMHidesUnauthorizedTarget(t *testing.T) {
	priv, peerID := generateTestKeypair(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFriendDmResponse(t, w, &api.GetFriendDmResponse{})
	}))
	defer srv.Close()

	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, peerID.String())
	_, err := cli.GetFriendDM(context.Background(), "acct-b")
	if !stderrors.Is(err, ErrFriendDmNotFound) {
		t.Fatalf("error = %v, want ErrFriendDmNotFound", err)
	}
}

func TestValidateFriendDmBootstrapRejectsDuplicateAccounts(t *testing.T) {
	_, peerID := generateTestKeypair(t)
	account := &api.FriendDmAccount{
		AccountId:        "acct-a",
		Sessions:         []*api.FriendDmSessionPeer{{PeerId: peerID.String()}},
		RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: peerID.String()}},
	}
	bootstrap := &api.GetFriendDmResponse{
		Found:          true,
		SharedObjectId: "01frienddm",
		OwnerAccountId: "acct-a",
		OwnerType:      "account",
		Accounts:       []*api.FriendDmAccount{account, account},
	}
	err := validateFriendDmBootstrap(bootstrap, "acct-a", "acct-b", peerID.String())
	if err == nil || err.Error() != "friend dm response contains duplicate account" {
		t.Fatalf("error = %v, want duplicate account error", err)
	}
}

func TestDeriveFriendDmSharedObjectIDVector(t *testing.T) {
	first := &api.FriendDmAccount{AccountId: "acct-a", EntityUuid: "acct-a-uuid", Epoch: 2}
	second := &api.FriendDmAccount{AccountId: "acct-b", EntityUuid: "acct-b-uuid", Epoch: 2}
	const want = "014qkjwy00eemfrmy7cx7nj2sx"
	if got := deriveFriendDmSharedObjectID(first, second); got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
	if got := deriveFriendDmSharedObjectID(second, first); got != want {
		t.Fatalf("reversed id = %q, want %q", got, want)
	}
	second.Epoch++
	if got := deriveFriendDmSharedObjectID(first, second); got == want {
		t.Fatalf("generation change kept id %q", got)
	}
}
