package main

import (
	"errors"
	"os"

	"github.com/blang/semver"
	"github.com/manifoldco/promptui"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var privKeyPath string

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

func main() {
	app := cli.NewApp()
	app.Name = "logintester"
	app.Usage = "test authentication against a network domain"
	app.HideVersion = true
	app.Action = runAuthTester
	app.Flags = []cli.Flag{}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err.Error())
	}
}

func runAuthTester(c *cli.Context) error {
	// ctx := context.Background()
	// log := logrus.New()
	// log.SetLevel(logrus.DebugLevel)
	// le := logrus.NewEntry(log)

	// 1. Input username

	username, err := (&promptui.Prompt{Label: "Username"}).Run()
	if err != nil {
		return err
	}
	if username == "" {
		return errors.New("username cannot be empty")
	}

	// 2. Lookup username auth record from active domain.
	// TODO LookupIdentityEntityRecord (?)

	// TODO: select the authentication method from the user record.

	// Gather the password for the username/password method.
	password, err := (&promptui.Prompt{Label: "Password", Mask: '*'}).Run()
	if err != nil {
		return err
	}
	if password == "" {
		return errors.New("password cannot be empty")
	}

	return errors.New("TODO authenticate")
}
