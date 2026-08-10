//go:build !js

package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/auth/examples/common"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	"github.com/s4wave/spacewave/net/peer"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

func main() {
	app := cli.NewApp()
	app.Name = "logintester"
	app.Usage = "networked login testing"
	app.HideVersion = true
	app.Action = runAuthTester
	app.Flags = []cli.Flag{}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err.Error())
	}
}

func runAuthTester(c *cli.Context) error {
	// Initialize the authentication test context and logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Collect credentials through the shared login prompt.
	// The root command starts interactive authentication.
	username, password, err := common.RunLoginPrompt()
	if err != nil {
		return err
	}

	// Construct the password method and derive its parameters.
	le.Info("scrypt...")

	authMethod := auth_method_password.NewPasswordMethod(ctx)
	defer authMethod.Close()
	params, _, err := authMethod.BuildParametersWithUsernamePassword(ctx, username, []byte(password))
	if err != nil {
		return err
	}

	// Authenticate and derive the peer identity.
	privKey, err := authMethod.Authenticate(
		ctx,
		params,
		[]byte(password),
	)
	if err != nil {
		return err
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}

	// Derive the test entity identifier and serialize its parameters.
	// aperture domain uuid for v0
	domainUUID, _ := uuid.FromString("1e4a7ac8-d1d9-4172-8d73-601e501f2382")
	entityUUID := uuid.NewV5(domainUUID, username)

	// Serialize and report the authentication parameters.
	authParamsDat, err := params.MarshalVT()
	if err != nil {
		return err
	}
	le.Infof("encoded auth parameters: %s", b58.Encode(authParamsDat))
	le.
		WithField("peer-id", peerID.String()).
		WithField("entity-uuid", entityUUID).
		Info("authenticated and derived private key")

	// Sign the derived peer identifier as the final authentication check.
	dat, err := privKey.Sign([]byte(peerID.String()))
	if err != nil {
		return err
	}
	le.Infof("signed data: %s", b58.Encode(dat))

	return nil
}
