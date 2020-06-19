package main

import (
	"context"
	"os"

	client "github.com/aperturerobotics/auth/challenge/client"
	server "github.com/aperturerobotics/auth/challenge/server"
	"github.com/aperturerobotics/auth/core"
	auth_method "github.com/aperturerobotics/auth/method"
	auth_method_triplesec_password "github.com/aperturerobotics/auth/method/triplesec-password"
	auth_static "github.com/aperturerobotics/auth/static"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/testbed"
	"github.com/aperturerobotics/bifrost/transport/common/dialer"
	transport_controller "github.com/aperturerobotics/bifrost/transport/controller"
	inproc "github.com/aperturerobotics/bifrost/transport/inproc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/identity"
	"github.com/golang/protobuf/proto"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var (
	username, password string
)

func main() {
	app := cli.NewApp()
	app.Name = "logintester"
	app.Usage = "test authentication against a network domain"
	app.HideVersion = true
	app.Action = runAuthTester
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "username",
			Usage:       "username to use, will prompt if not set",
			Destination: &username,
		},
		cli.StringFlag{
			Name:        "password",
			Usage:       "password to use, will prompt if not set",
			Destination: &password,
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err.Error())
	}
}

func runAuthTester(c *cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	var handler auth_method.Handler // TODO
	authMethod, err := auth_method_triplesec_password.NewMethod(ctx, le, handler)
	if err != nil {
		return err
	}

	entityID := "testuser"
	domainID := "aperturerobotics.com"
	hardcodedPassword := "testpassword"

	// generate the user private key with the password in advance
	// tsp := authMethod.(*auth_method_triplesec_password.TriplesecPassword)
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

	// Build testbed
	tb, err := testbed.NewTestbed(
		ctx,
		le.WithField("testbed", "auth-client"),
		testbed.TestbedOpts{},
	)
	if err != nil {
		return err
	}
	core.AddFactories(tb.Bus, tb.StaticResolver)
	tb.StaticResolver.AddFactory(inproc.NewFactory(tb.Bus))

	privKey := tb.PrivKey
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}

	// Build testbed for the "auth server"
	tbServer, err := testbed.NewTestbed(
		ctx,
		le.WithField("testbed", "auth-server"),
		testbed.TestbedOpts{},
	)
	if err != nil {
		return err
	}
	core.AddFactories(tbServer.Bus, tbServer.StaticResolver)
	tbServer.StaticResolver.AddFactory(inproc.NewFactory(tbServer.Bus))

	// Start the auth server.
	serverPrivKey := tbServer.PrivKey
	serverPeerID, err := peer.IDFromPrivateKey(serverPrivKey)
	if err != nil {
		return err
	}
	_, serverRef, err := bus.ExecOneOff(
		ctx,
		tbServer.Bus,
		resolver.NewLoadControllerWithConfig(&server.Config{
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
	_, staticRef, err := bus.ExecOneOff(
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

	// Build the inproc transports
	tp2i, tp2Ref, err := bus.ExecOneOff(
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
	tpc2 := tp2i.GetValue().(*transport_controller.Controller)
	tp2k, err := tpc2.GetTransport(ctx)
	if err != nil {
		return err
	}
	tp2 := tp2k.(*inproc.Inproc)

	tp1i, tp1Ref, err := bus.ExecOneOff(
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
	tpc1 := tp1i.GetValue().(*transport_controller.Controller)
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

	// 1. Input username
	if username == "" {
		username, err = (&promptui.Prompt{Label: "Username"}).Run()
		if err != nil {
			return err
		}
	}
	if username == "" {
		return errors.New("username cannot be empty")
	}

	// 2. Lookup username auth record from active domain.
	entityRecordInter, di, err := bus.ExecOneOff(
		ctx,
		tb.Bus,
		identity.NewIdentityLookupEntity(username, domainID),
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

	// Gather the password for the username/password method.
	if password == "" {
		password, err = (&promptui.Prompt{Label: "Password", Mask: '*'}).Run()
		if err != nil {
			return err
		}
	}
	if password == "" {
		return errors.New("password cannot be empty")
	}
	if password[len(password)-1] == '\n' {
		password = password[:len(password)-1]
	}
	le.Debugf("%q", password)

	selectedParams, err := authMethod.UnmarshalParameters(selectedKeypair.GetAuthMethodParams())
	if err != nil {
		return err
	}

	derivedPrivKey, err := authMethod.Authenticate(selectedParams, []byte(password))
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
