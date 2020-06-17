package main

import (
	"context"
	"hash/crc64"
	"math/rand"
	"os"

	"github.com/aperturerobotics/auth/toys/common"
	"github.com/blang/semver"
	"github.com/keybase/go-triplesec"
	b58 "github.com/mr-tron/base58/base58"
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

	// build seed
	rs := rand.New(rand.NewSource(
		int64(crc64.Checksum(
			[]byte(username),
			crc64.MakeTable(crc64.ECMA),
		)),
	))
	salt := make([]byte, 16)
	_, err = rs.Read(salt)
	if err != nil {
		return err
	}

	le.
		WithField("username", username).
		WithField("password-len", len(password)).
		Info("scrypt....")
	// use username as salt for now
	cipher, err := triplesec.NewCipher(
		[]byte(password),
		salt,
		triplesec.LatestVersion,
	)
	if err != nil {
		return err
	}
	defer cipher.Scrub()

	derived, _, err := cipher.DeriveKey(0)
	if err != nil {
		return err
	}
	derivedStr := b58.Encode(derived)
	le.Infof("derived key: %s", derivedStr)

	_ = ctx
	return nil
}
