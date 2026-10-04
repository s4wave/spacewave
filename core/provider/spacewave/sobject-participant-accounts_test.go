package provider_spacewave

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestFillParticipantAccounts checks that the owner records its own account
// and an admitted writer's account in one config change, and leaves a peer
// without an accepted mailbox entry blank.
func TestFillParticipantAccounts(t *testing.T) {
	// Sign a config whose owner, writer and reader have no account.
	const (
		soID          = "so-fill-accounts"
		accountID     = "test-account"
		writerAccount = "writer-account"
	)
	_, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	_, writerPID := generateTestKeypair(t)
	_, readerPID := generateTestKeypair(t)
	state, chain := buildBlankParticipantFixtures(
		t,
		soID,
		ownerPriv,
		ownerPID,
		writerPID,
		readerPID,
	)
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainData := mustMarshalVT(t, chain)

	// Admit the writer through the mailbox. Only the owner's account has
	// recovery keypairs.
	mailboxData := mustMarshalVT(t, &api.GetMailboxResponse{
		Entries: []*api.MailboxEntry{{
			Id:        1,
			PeerId:    writerPID.String(),
			Status:    "accepted",
			AccountId: writerAccount,
		}},
	})
	keypairData := mustMarshalVT(t, &api.ListSORecoveryEntityKeypairsResponse{
		Entities: []*api.SORecoveryEntityKeypairs{{
			EntityId: accountID,
			Keypairs: []*session.EntityKeypair{{PeerId: entityPID.String()}},
		}},
	})

	// Serve the Space and capture the posted config change.
	var posted *api.PostConfigStateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainData)
		case "/api/sobject/" + soID + "/invite-mailbox":
			if status := r.URL.Query().Get("status"); status != "accepted" {
				t.Errorf("mailbox status = %q, want accepted", status)
			}
			_, _ = w.Write(mailboxData)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(keypairData)
		case "/api/sobject/" + soID + "/config-state":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read config-state body: %v", err)
				return
			}
			req := &api.PostConfigStateRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Errorf("unmarshal config-state request: %v", err)
				return
			}
			posted = req
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	// Mount the Space as the owner peer.
	le := logrus.New().WithField("test", t.Name())
	acc := NewTestProviderAccount(t, srv.URL)
	acc.ReplaceSessionClient(NewSessionClient(
		http.DefaultClient,
		srv.URL,
		DefaultSigningEnvPrefix,
		ownerPriv,
		ownerPID.String(),
	))
	host := newCloudSOHost(
		le,
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(le, func() *SessionClient { return acc.sessionClient }),
		ownerPriv,
		ownerPID,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(t.Context())
	if err := host.ensureInitialState(t.Context(), SeedReasonColdSeed); err != nil {
		t.Fatalf("ensureInitialState: %v", err)
	}
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  ownerPriv,
		localPid: ownerPID,
	}

	// Fill the accounts.
	err := so.tkr.fillParticipantAccounts(t.Context(), so, acc.sessionClient)
	if err != nil {
		t.Fatalf("fillParticipantAccounts: %v", err)
	}

	// One change records the owner and writer and leaves the reader blank.
	if posted == nil {
		t.Fatal("expected config-state write")
	}
	change := &sobject.SOConfigChange{}
	if err := change.UnmarshalVT(posted.GetConfigChange()); err != nil {
		t.Fatalf("unmarshal posted config change: %v", err)
	}
	want := map[peer.ID]string{
		ownerPID:  accountID,
		writerPID: writerAccount,
		readerPID: "",
	}
	for peerID, wantAccount := range want {
		participant := participantConfigForPeer(change.GetConfig(), peerID.String())
		if got := participant.GetEntityId(); got != wantAccount {
			t.Errorf("peer %s account = %q, want %q", peerID, got, wantAccount)
		}
	}

	// Only the owner's account has keypairs to seal an envelope for.
	envs := posted.GetRecoveryEnvelopes()
	if len(envs) != 1 || envs[0].GetEntityId() != accountID {
		t.Fatalf("recovery envelopes = %v, want one for %s", envs, accountID)
	}
}

// buildBlankParticipantFixtures signs a genesis naming an owner, a writer and
// a reader without accounts, and grants the owner the Space key.
func buildBlankParticipantFixtures(
	t *testing.T,
	soID string,
	ownerPriv crypto.PrivKey,
	ownerPID, writerPID, readerPID peer.ID,
) (*sobject.SOState, *sobject.SOConfigChainResponse) {
	// Build the encrypted block transform carried by the owner grant.
	t.Helper()
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      []byte("0123456789abcdef0123456789abcdef"),
		},
	})
	if err != nil {
		t.Fatalf("build transform config: %v", err)
	}

	// Sign the genesis membership.
	cfg := &sobject.SharedObjectConfig{
		Participants: []*sobject.SOParticipantConfig{{
			PeerId: ownerPID.String(),
			Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
		}, {
			PeerId: writerPID.String(),
			Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
		}, {
			PeerId: readerPID.String(),
			Role:   sobject.SOParticipantRole_SOParticipantRole_READER,
		}},
	}
	genesis, err := sobject.BuildSOConfigChange(
		soID,
		&sobject.SharedObjectConfig{},
		cfg,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		ownerPriv,
		nil,
	)
	if err != nil {
		t.Fatalf("build genesis entry: %v", err)
	}
	genesisHash, err := sobject.HashSOConfigChange(genesis)
	if err != nil {
		t.Fatalf("hash genesis entry: %v", err)
	}
	cfg = cfg.CloneVT()
	cfg.ConfigChainSeqno = genesis.GetConfigSeqno()
	cfg.ConfigChainHash = genesisHash

	// Grant the owner the Space key and sign the genesis checkpoint.
	ownerPub, err := ownerPID.ExtractPublicKey()
	if err != nil {
		t.Fatalf("extract owner public key: %v", err)
	}
	ownerGrant, err := sobject.EncryptSOGrant(
		ownerPriv,
		ownerPub,
		soID,
		&sobject.SOGrantInner{TransformConf: transformConf},
	)
	if err != nil {
		t.Fatalf("encrypt owner grant: %v", err)
	}
	checkpoint, err := sobject.BuildGenesisSOCheckpoint(
		ownerPriv,
		soID,
		genesisHash,
		[]byte("state"),
	)
	if err != nil {
		t.Fatalf("build genesis checkpoint: %v", err)
	}

	// Return the matching state and config history.
	epochs := []*sobject.SOKeyEpoch{{
		Epoch:  1,
		Grants: []*sobject.SOGrant{ownerGrant},
	}}
	state := &sobject.SOState{
		Config:     cfg,
		Checkpoint: checkpoint,
		KeyEpochs:  epochs,
	}
	chain := &sobject.SOConfigChainResponse{
		ConfigChanges: []*sobject.SOConfigChange{genesis},
		KeyEpochs:     epochs,
	}
	return state, chain
}
