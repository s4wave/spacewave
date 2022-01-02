package main

import (
	"context"
	"os"

	auth_method "github.com/aperturerobotics/auth/method"
	auth_method_triplesec_password "github.com/aperturerobotics/auth/method/triplesec-password"
	"github.com/aperturerobotics/auth/toys/common"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/blang/semver"
	"github.com/golang/protobuf/proto"
	b58 "github.com/mr-tron/base58/base58"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var privKeyPath string

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

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
	//TODO
	username, password, err := common.RunLoginPrompt()
	if err != nil {
		return err
	}

	le.Info("scrypt...")
	var handler auth_method.Handler // TODO
	authMethod, err := auth_method_triplesec_password.NewMethod(ctx, le, handler)
	if err != nil {
		return err
	}
	params, _, err := auth_method_triplesec_password.BuildParametersWithUsernamePassword(4, username, []byte(password))
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

	authParamsDat, err := proto.Marshal(params)
	if err != nil {
		return err
	}
	le.Infof("encoded auth parameters: %s", b58.Encode(authParamsDat))
	le.
		WithField("peer-id", peerID.Pretty()).
		WithField("entity-uuid", entityUUID).
		Info("authenticated and derived private key")

	dat, err := privKey.Sign([]byte(peerID.Pretty()))
	if err != nil {
		return err
	}
	le.Infof("signed data: %s", b58.Encode(dat))

	return nil
}
