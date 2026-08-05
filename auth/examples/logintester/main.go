//go:build !js

package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/auth/core"
	auth_method "github.com/s4wave/spacewave/auth/method"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	"github.com/s4wave/spacewave/identity"
	identity_domain "github.com/s4wave/spacewave/identity/domain"
	client "github.com/s4wave/spacewave/identity/domain/service/client"
	server "github.com/s4wave/spacewave/identity/domain/service/server"
	identity_static "github.com/s4wave/spacewave/identity/domain/static"
	"github.com/s4wave/spacewave/net/peer"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	inproc "github.com/s4wave/spacewave/net/transport/inproc"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

var username, password string

func main() {
	app := cli.NewApp()
	app.Name = "logintester"
	app.Usage = "test authentication against a network domain"
	app.HideVersion = true
	app.Action = runAuthTester
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "username",
			Usage:       "username to use, will prompt if not set",
			Destination: &username,
		},
		&cli.StringFlag{
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
	// Initialize the test context and logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Construct the password authentication method.
	var handler auth_method.Handler // TODO
	authMethod, err := auth_method_password.NewMethod(ctx, le, handler)
	if err != nil {
		return err
	}

	// Define the test entity and domain identifiers.
	entityID := "testuser"
	domainID := "aperture.app"
	domainName := "Aperture App"
	hardcodedPassword := "testpassword"
	entityUUID := uuid.NewV4().String()

	// Derive the test entity keypair from its password.
	paramsSrc, userPrivKey, err := auth_method_password.BuildParametersWithUsernamePassword(
		entityID,
		[]byte(hardcodedPassword),
	)
	if err != nil {
		return err
	}

	// Serialize the authentication parameters for the entity record.
	authMethodParams, err := paramsSrc.MarshalVT()
	if err != nil {
		return err
	}

	// Build the target entity record with its private key.
	targetEntitySrc, err := identity.EntityWithPrivKey(
		domainID,
		entityID, entityUUID,
		userPrivKey,
		auth_method_password.MethodID,
		authMethodParams,
	)
	if err != nil {
		return err
	}

	// Build the client testbed and register its controllers.
	tb, err := testbed.NewTestbed(
		ctx,
		le.WithField("testbed", "auth-client"),
		testbed.TestbedOpts{},
	)
	if err != nil {
		return err
	}

	// Add core and transport factories to the client testbed.
	core.AddFactories(tb.Bus, tb.StaticResolver)
	tb.StaticResolver.AddFactory(inproc.NewFactory(tb.Bus))

	// Resolve the client peer identity from its private key.
	privKey := tb.PrivKey
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}

	// Build the testbed that hosts the authentication server.
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

	// Start the authentication server controller.
	serverPrivKey := tbServer.PrivKey
	serverPeerID, err := peer.IDFromPrivateKey(serverPrivKey)
	if err != nil {
		return err
	}
	_, _, serverRef, err := loader.WaitExecControllerRunning(
		ctx,
		tbServer.Bus,
		resolver.NewLoadControllerWithConfig(&server.Config{
			Server: &stream_srpc_server.Config{
				PeerIds: []string{serverPeerID.String()},
			},
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer serverRef.Release()

	serverPeerIDs := []string{serverPeerID.String()}

	// Publish the target entity through the static authentication list.
	_, _, staticRef, err := loader.WaitExecControllerRunning(
		ctx,
		tbServer.Bus,
		resolver.NewLoadControllerWithConfig(&identity_static.Config{
			DomainInfo: &identity_domain.DomainInfo{DomainId: domainID, Name: domainName},
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

	// Start the server-side in-process transport.
	tp2i, _, tp2Ref, err := loader.WaitExecControllerRunning(
		ctx,
		tbServer.Bus,
		resolver.NewLoadControllerWithConfig(&inproc.Config{
			TransportPeerId: serverPeerID.String(),
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

	// Start the client-side in-process transport and dial the server.
	tp1i, _, tp1Ref, err := loader.WaitExecControllerRunning(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&inproc.Config{
			TransportPeerId: peerID.String(),
			Dialers: map[string]*dialer.DialerOpts{
				serverPeerID.String(): {
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

	// Connect both in-process transports.
	tp2.ConnectToInproc(ctx, tp1)
	tp1.ConnectToInproc(ctx, tp2)

	// Execute the client controller against the authentication server.
	_, _, clientRef, err := bus.ExecOneOff(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&client.Config{
			PeerId: peerID.String(),
			ClientOpts: &stream_srpc_client.Config{
				ServerPeerIds: serverPeerIDs,
			},
			DomainInfo: &identity_domain.DomainInfo{
				DomainId:    domainID,
				Name:        "Test",
				Description: "Test domain",
			},
		}),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	defer clientRef.Release()

	// Collect the username used for the authentication lookup.
	if username == "" {
		username, err = (&promptui.Prompt{Label: "Username"}).Run()
		if err != nil {
			return err
		}
	}
	if username == "" {
		return errors.New("username cannot be empty")
	}

	// Look up the username record from the active domain.
	entityRecordInter, _, di, err := bus.ExecOneOff(
		ctx,
		tb.Bus,
		identity.NewIdentityLookupEntity(domainID, username),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	di.Release()

	// Validate the lookup result and capture the entity record.
	entityRecordValue := entityRecordInter.GetValue().(identity.IdentityLookupEntityValue)
	if entityRecordValue.IsNotFound() {
		return errors.New("authentication failed: entity not found")
	}
	if err := entityRecordValue.GetError(); err != nil {
		return err
	}
	entity := entityRecordValue.GetEntity()
	le.Infof("got authentication entity with uuid %s", entity.GetEntityUuid())

	// Select the keypair registered for this authentication method.
	var selectedKeypair *identity.Keypair
	for i, kpd := range entity.GetEntityKeypairSet().GetEntityKeypairs() {
		ekp := &identity.EntityKeypair{}
		if err := ekp.UnmarshalBlock(kpd); err != nil {
			le.WithError(err).Warnf("entity_keypairs[%d]: cannot unmarshal", i)
			continue
		}
		kp := ekp.GetKeypair()
		if kp.GetAuthMethodId() == authMethod.GetMethodID() {
			selectedKeypair = kp
			break
		}
	}
	if selectedKeypair == nil {
		return errors.New("no keypairs match auth method")
	}

	// Collect and normalize the password for authentication.
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

	// Decode the selected keypair's authentication parameters.
	selectedParams, err := authMethod.UnmarshalParameters(selectedKeypair.GetAuthMethodParams())
	if err != nil {
		return err
	}

	// Authenticate the password and derive the peer identity.
	derivedPrivKey, err := authMethod.Authenticate(selectedParams, []byte(password))
	if err != nil {
		return err
	}
	derivedPeerID, err := peer.IDFromPrivateKey(derivedPrivKey)
	if err != nil {
		return err
	}

	// Verify that the derived peer matches the selected keypair.
	derivedPeerIDStr := derivedPeerID.String()
	if derivedPeerIDStr != selectedKeypair.GetPeerId() {
		return errors.Errorf(
			"password incorrect, expected peer id %s but got %s",
			selectedKeypair.GetPeerId(),
			derivedPeerIDStr,
		)
	}

	// Report successful authentication for the derived peer.
	le.Infof("successfully derived private key for peer id %s", derivedPeerIDStr)
	return nil
}
