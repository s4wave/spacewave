package provider_spacewave

import (
	"bytes"
	"context"
	"encoding/base64"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/fastjson"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

func TestGetFriendDM(t *testing.T) {
	priv, peerID := generateTestKeypair(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/account/friends/acct-b/dm" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"sharedObjectId":"01frienddm",
			"ready":true,
			"ownerAccountId":"acct-b",
			"ownerType":"account",
			"accounts":[
				{"accountId":"acct-a","entityUuid":"entity-a","epoch":3,"sessions":[{"peerId":"peer-a"}],"recoveryKeypairs":[{"peerId":"recovery-a"}]},
				{"accountId":"acct-b","entityUuid":"entity-b","epoch":4,"sessions":[{"peerId":"peer-b"}],"recoveryKeypairs":[{"peerId":"recovery-b"}]}
			]
		}`))
	}))
	defer srv.Close()

	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, peerID.String())
	got, err := cli.GetFriendDM(context.Background(), "acct-b")
	if err != nil {
		t.Fatalf("GetFriendDM: %v", err)
	}
	if !got.Ready || got.SharedObjectID != "01frienddm" {
		t.Fatalf("unexpected bootstrap: %+v", got)
	}
	if got.OwnerAccountID != "acct-b" || got.OwnerType != "account" {
		t.Fatalf("unexpected owner: %+v", got)
	}
	if len(got.Accounts) != 2 || got.Accounts[1].Sessions[0].PeerID != "peer-b" {
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
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q, want application/json", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		for _, want := range []string{
			`"displayName":"Friend DM"`,
			`"objectType":"space"`,
			`"ownerType":"account"`,
			`"ownerId":"acct-a"`,
			`"accountPrivate":true`,
		} {
			if !bytes.Contains(body, []byte(want)) {
				t.Fatalf("request body = %s, missing %s", body, want)
			}
		}
		var parser fastjson.Parser
		value, err := parser.ParseBytes(body)
		if err != nil {
			t.Fatalf("parse request body: %v", err)
		}
		encodedConfig := string(value.GetStringBytes("configState"))
		postedConfigData, err := base64.StdEncoding.DecodeString(encodedConfig)
		if err != nil {
			t.Fatalf("decode config state: %v", err)
		}
		postedConfig := &api.PostConfigStateRequest{}
		if err := postedConfig.UnmarshalVT(postedConfigData); err != nil {
			t.Fatalf("unmarshal config state wrapper: %v", err)
		}
		if string(postedConfig.GetConfigChange()) != "config" ||
			postedConfig.GetKeyEpoch() == nil ||
			postedConfig.GetKeyEpoch().GetEpoch() != 0 {
			t.Fatalf("unexpected config state wrapper: %+v", postedConfig)
		}
		encodedRoot := string(value.GetStringBytes("rootState"))
		postedRootData, err := base64.StdEncoding.DecodeString(encodedRoot)
		if err != nil {
			t.Fatalf("decode root state: %v", err)
		}
		postedRoot := &api.PostRootRequest{}
		if err := postedRoot.UnmarshalVT(postedRootData); err != nil {
			t.Fatalf("unmarshal root state wrapper: %v", err)
		}
		if postedRoot.GetRoot() == nil ||
			postedRoot.GetRoot().GetInnerSeqno() != 1 ||
			string(postedRoot.GetRoot().GetInner()) != "root" {
			t.Fatalf("unexpected root state wrapper: %+v", postedRoot)
		}
		_, _ = w.Write([]byte(`{
			"sharedObjectId":"01frienddm",
			"ready":true,
			"ownerAccountId":"acct-a",
			"ownerType":"account",
			"accounts":[
				{"accountId":"acct-a","entityUuid":"entity-a","epoch":3,"sessions":[{"peerId":"peer-a"}],"recoveryKeypairs":[{"peerId":"recovery-a"}]},
				{"accountId":"acct-b","entityUuid":"entity-b","epoch":4,"sessions":[{"peerId":"peer-b"}],"recoveryKeypairs":[{"peerId":"recovery-b"}]}
			]
		}`))
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
	if !got.Ready || got.OwnerAccountID != "acct-a" {
		t.Fatalf("unexpected bootstrap: %+v", got)
	}
}

func TestGetFriendDMHidesUnauthorizedTarget(t *testing.T) {
	priv, peerID := generateTestKeypair(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"found":false}`))
	}))
	defer srv.Close()

	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, peerID.String())
	_, err := cli.GetFriendDM(context.Background(), "acct-b")
	if !stderrors.Is(err, ErrFriendDmNotFound) {
		t.Fatalf("error = %v, want ErrFriendDmNotFound", err)
	}
}

func TestParseFriendDmBootstrapRejectsDuplicateAccounts(t *testing.T) {
	_, err := parseFriendDmBootstrap([]byte(`{
		"sharedObjectId":"01frienddm",
		"ready":false,
		"ownerAccountId":"acct-a",
		"ownerType":"account",
		"accounts":[
			{"accountId":"acct-a","entityUuid":"entity-a","epoch":1,"sessions":[{"peerId":"peer-1"}]},
			{"accountId":"acct-a","entityUuid":"entity-b","epoch":1,"sessions":[{"peerId":"peer-2"}]}
		]
	}`))
	if err == nil {
		t.Fatal("expected duplicate account error")
	}
}

func TestDeriveFriendDmSharedObjectIDVector(t *testing.T) {
	first := FriendDmAccount{AccountID: "acct-a", EntityUUID: "acct-a-uuid", Epoch: 2}
	second := FriendDmAccount{AccountID: "acct-b", EntityUUID: "acct-b-uuid", Epoch: 2}
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

func TestParseFriendDmBootstrapAllowsEpochZero(t *testing.T) {
	got, err := parseFriendDmBootstrap([]byte(`{
		"sharedObjectId":"01frienddm",
		"ready":false,
		"ownerAccountId":"acct-a",
		"ownerType":"account",
		"accounts":[
			{"accountId":"acct-a","entityUuid":"entity-a","epoch":0,"sessions":[],"recoveryKeypairs":[{"peerId":"recovery-a"}]},
			{"accountId":"acct-b","entityUuid":"entity-b","epoch":0,"sessions":[],"recoveryKeypairs":[{"peerId":"recovery-b"}]}
		]
	}`))
	if err != nil {
		t.Fatalf("parse epoch-zero bootstrap: %v", err)
	}
	if got.Accounts[0].Epoch != 0 || got.Accounts[1].Epoch != 0 {
		t.Fatalf("unexpected epochs: %+v", got.Accounts)
	}
}

func TestParseFriendDmBootstrapAllowsEmptySessions(t *testing.T) {
	got, err := parseFriendDmBootstrap([]byte(`{
		"sharedObjectId":"01frienddm",
		"ready":false,
		"ownerAccountId":"acct-a",
		"ownerType":"account",
		"accounts":[
			{"accountId":"acct-a","entityUuid":"entity-a","epoch":1,"sessions":[],"recoveryKeypairs":[{"peerId":"recovery-a"}]},
			{"accountId":"acct-b","entityUuid":"entity-b","epoch":1,"sessions":[],"recoveryKeypairs":[{"peerId":"recovery-b"}]}
		]
	}`))
	if err != nil {
		t.Fatalf("parseFriendDmBootstrap: %v", err)
	}
	if len(got.Accounts) != 2 || len(got.Accounts[0].Sessions) != 0 {
		t.Fatalf("unexpected empty sessions: %+v", got.Accounts)
	}
}

func TestParseFriendDmBootstrapRejectsDuplicateRecoveryPeers(t *testing.T) {
	_, err := parseFriendDmBootstrap([]byte(`{
		"sharedObjectId":"01frienddm",
		"ready":false,
		"ownerAccountId":"acct-a",
		"ownerType":"account",
		"accounts":[
			{"accountId":"acct-a","entityUuid":"entity-a","epoch":0,"sessions":[],"recoveryKeypairs":[{"peerId":"recovery-a"},{"peerId":"recovery-a"}]},
			{"accountId":"acct-b","entityUuid":"entity-b","epoch":0,"sessions":[],"recoveryKeypairs":[{"peerId":"recovery-b"}]}
		]
	}`))
	if err == nil {
		t.Fatal("expected duplicate recovery peer error")
	}
}

func TestParseFriendDmBootstrapRejectsPeerAcrossAccounts(t *testing.T) {
	_, err := parseFriendDmBootstrap([]byte(`{
		"sharedObjectId":"01frienddm",
		"ready":false,
		"ownerAccountId":"acct-a",
		"ownerType":"account",
		"accounts":[
			{"accountId":"acct-a","entityUuid":"entity-a","epoch":1,"sessions":[{"peerId":"peer-1"}],"recoveryKeypairs":[{"peerId":"recovery-a"}]},
			{"accountId":"acct-b","entityUuid":"entity-b","epoch":1,"sessions":[{"peerId":"peer-1"}],"recoveryKeypairs":[{"peerId":"recovery-b"}]}
		]
	}`))
	if err == nil {
		t.Fatal("expected duplicate session peer error")
	}
}
