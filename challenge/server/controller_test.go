package auth_challenge_server

import (
	"context"
	"testing"

	challenge_client "github.com/aperturerobotics/auth/challenge/client"
	client "github.com/aperturerobotics/auth/challenge/client"
	auth_method "github.com/aperturerobotics/auth/method"
	auth_method_triplesec_password "github.com/aperturerobotics/auth/method/triplesec-password"
	auth_static "github.com/aperturerobotics/auth/static"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/testbed"
	"github.com/aperturerobotics/bifrost/transport/common/dialer"
	transport_controller "github.com/aperturerobotics/bifrost/transport/controller"
	inproc "github.com/aperturerobotics/bifrost/transport/inproc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/identity"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

// TestLoginChallenge_TripleSec tests the auth challenge server with KeyBase triplesec.
func TestLoginChallenge_TripleSec(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := runTripleSecTest(ctx, le); err != nil {
		t.Fatal(err.Error())
	}
}

func runTripleSecTest(ctx context.Context, le *logrus.Entry) error {
	var handler auth_method.Handler // TODO
	authMethod, err := auth_method_triplesec_password.NewMethod(ctx, le, handler)
	if err != nil {
		return err
	}

	entityID := "testuser"
	domainID := "aperturerobotics.com"
	hardcodedPassword := "testpassword"

	// generate the user private key with the password in advance
	paramsSrc, userPrivKey, err := auth_method_triplesec_password.BuildParametersWithUsernamePassword(
		4,
		entityID,
		[]byte(hardcodedPassword),
	)
	if err != nil {
		return err
	}
	userPubKey := userPrivKey.GetPublic()

	kp1, err := identity.NewKeypair(userPubKey)
	if err != nil {
		return err
	}
	kp1.AuthMethodId = auth_method_triplesec_password.MethodID
	kp1.AuthMethodParams, err = proto.Marshal(paramsSrc)
	if err != nil {
		return err
	}
	targetEntitySrc := &identity.Entity{
		EntityId:   entityID,
		EntityUuid: uuid.NewV4().String(),
		DomainId:   domainID,
		Epoch:      1,
		Keypairs: []*identity.Keypair{
			kp1,
		},
	}
	serverPeerIDs := []string{} // set below

	// buildTestbed constructs a testbed with the necessary factories.
	buildTestbed := func(le *logrus.Entry) (*testbed.Testbed, error) {
		tb, err := testbed.NewTestbed(
			ctx, le, testbed.TestbedOpts{})
		if err != nil {
			return nil, err
		}
		b, sr := tb.Bus, tb.StaticResolver
		for _, ft := range []controller.Factory{
			(inproc.NewFactory(b)),
			(challenge_client.NewFactory(b)),
			(NewFactory(b)),
			(auth_static.NewFactory(b)),
			(auth_method_triplesec_password.NewFactory(b)),
		} {
			sr.AddFactory(ft)
		}
		return tb, nil
	}

	// Build testbed
	tb, err := buildTestbed(le.WithField("testbed", "auth-client"))
	if err != nil {
		return err
	}

	privKey := tb.PrivKey
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}

	// Build testbed for the "auth server"
	tbServer, err := buildTestbed(le.WithField("testbed", "auth-server"))
	if err != nil {
		return err
	}

	// Start the auth server.
	serverPrivKey := tbServer.PrivKey
	serverPeerID, err := peer.IDFromPrivateKey(serverPrivKey)
	if err != nil {
		return err
	}
	_, _, serverRef, err := loader.WaitExecControllerRunning(
		ctx,
		tbServer.Bus,
		resolver.NewLoadControllerWithConfig(&Config{
			PeerId: serverPeerID.Pretty(),
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer serverRef.Release()
	serverPeerIDs = append(serverPeerIDs, serverPeerID.Pretty())

	// Static auth list (simulating a auth database)
	_, _, staticRef, err := loader.WaitExecControllerRunning(
		ctx,
		tbServer.Bus,
		resolver.NewLoadControllerWithConfig(&auth_static.Config{
			Domains: []string{
				domainID,
			},
			Entities: []*identity.Entity{
				targetEntitySrc,
			},
			SilentNotFound: false,
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer staticRef.Release()

	tp2i, _, tp2Ref, err := loader.WaitExecControllerRunning(
		ctx,
		tbServer.Bus,
		resolver.NewLoadControllerWithConfig(&inproc.Config{
			TransportPeerId: serverPeerID.Pretty(),
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer tp2Ref.Release()
	tpc2 := tp2i.(*transport_controller.Controller)
	tp2k, err := tpc2.GetTransport(ctx)
	if err != nil {
		return err
	}
	tp2 := tp2k.(*inproc.Inproc)

	tp1i, _, tp1Ref, err := loader.WaitExecControllerRunning(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&inproc.Config{
			TransportPeerId: peerID.Pretty(),
			Dialers: map[string]*dialer.DialerOpts{
				serverPeerID.Pretty(): {
					Address: tp2.LocalAddr().String(),
				},
			},
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer tp1Ref.Release()
	tpc1 := tp1i.(*transport_controller.Controller)
	tp1k, err := tpc1.GetTransport(ctx)
	if err != nil {
		return err
	}
	tp1 := tp1k.(*inproc.Inproc)

	tp2.ConnectToInproc(ctx, tp1)
	tp1.ConnectToInproc(ctx, tp2)

	// Execute the client.
	_, clientRef, err := bus.ExecOneOff(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&client.Config{
			PeerId:        peerID.Pretty(),
			ServerPeerIds: serverPeerIDs,
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer clientRef.Release()

	entityRecordInter, di, err := bus.ExecOneOff(
		ctx,
		tb.Bus,
		identity.NewIdentityLookupEntity(entityID, domainID),
		nil,
	)
	if err != nil {
		return err
	}
	di.Release()

	entityRecordValue := entityRecordInter.GetValue().(identity.IdentityLookupEntityValue)
	if entityRecordValue.IsNotFound() {
		return errors.New("authentication failed: entity not found")
	}
	if err := entityRecordValue.GetError(); err != nil {
		return err
	}
	entity := entityRecordValue.GetEntity()
	le.Infof("got authentication entity with uuid %s", entity.GetEntityUuid())

	// TODO: select the authentication method from the user record.

	// 3. authenticate against the record
	var selectedKeypair *identity.Keypair
	for _, kp := range entity.GetKeypairs() {
		if kp.GetAuthMethodId() == authMethod.GetMethodID() {
			selectedKeypair = kp
			break
		}
	}
	if selectedKeypair == nil {
		return errors.New("no keypairs match auth method")
	}

	selectedParams, err := authMethod.UnmarshalParameters(selectedKeypair.GetAuthMethodParams())
	if err != nil {
		return err
	}

	derivedPrivKey, err := authMethod.Authenticate(selectedParams, []byte(hardcodedPassword))
	if err != nil {
		return err
	}
	derivedPeerID, err := peer.IDFromPrivateKey(derivedPrivKey)
	if err != nil {
		return err
	}
	derivedPeerIDStr := derivedPeerID.Pretty()
	if derivedPeerIDStr != selectedKeypair.GetPeerId() {
		return errors.Errorf(
			"password incorrect, expected peer id %s but got %s",
			selectedKeypair.GetPeerId(),
			derivedPeerIDStr,
		)
	}

	le.Infof("successfully derived private key for peer id %s", derivedPeerIDStr)
	expectedDerivedPeerID := "12D3KooWCLtgZtLwnrAo6hWpLsju9D68NAhkAs3jy6Xz4NEqtHnU"
	if derivedPeerIDStr != expectedDerivedPeerID {
		return errors.Errorf("expected peer id %s but got %s, must be a inconsistency in generation", expectedDerivedPeerID, derivedPeerIDStr)
	}
	return nil
}
