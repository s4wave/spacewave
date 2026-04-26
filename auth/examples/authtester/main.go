package main

import (
	"context"
	"os"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/auth/examples/common"
	auth_method "github.com/s4wave/spacewave/auth/method"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	"github.com/s4wave/spacewave/net/peer"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"github.com/aperturerobotics/cli"
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
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// the root command starts interactive authentication.
	username, password, err := common.RunLoginPrompt()
	if err != nil {
		return err
	}

	le.Info("scrypt...")
	var handler auth_method.Handler // TODO
	authMethod, err := auth_method_password.NewMethod(ctx, le, handler)
	if err != nil {
		return err
	}
	params, _, err := auth_method_password.BuildParametersWithUsernamePassword(username, []byte(password))
	if err != nil {
		return err
	}
	privKey, err := authMethod.Authenticate(
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
	// aperture domain uuid for v0
	domainUUID, _ := uuid.FromString("1e4a7ac8-d1d9-4172-8d73-601e501f2382")
	entityUUID := uuid.NewV5(domainUUID, username)

	authParamsDat, err := params.MarshalVT()
	if err != nil {
		return err
	}
	le.Infof("encoded auth parameters: %s", b58.Encode(authParamsDat))
	le.
		WithField("peer-id", peerID.String()).
		WithField("entity-uuid", entityUUID).
		Info("authenticated and derived private key")

	dat, err := privKey.Sign([]byte(peerID.String()))
	if err != nil {
		return err
	}
	le.Infof("signed data: %s", b58.Encode(dat))

	return nil
}
