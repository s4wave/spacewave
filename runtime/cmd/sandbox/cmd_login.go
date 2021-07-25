package main

import (
	"errors"

	"github.com/manifoldco/promptui"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var loginArgs struct {
	username, password string
}

func init() {
	Commands = append(Commands, cli.Command{
		Name:  "login",
		Usage: "generate local identity and verify against known node",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:        "username",
				EnvVar:      "APERTURE_USERNAME",
				Usage:       "username to auth with, interactive if empty",
				Destination: &loginArgs.username,
			},
			cli.StringFlag{
				Name:        "password",
				EnvVar:      "APERTURE_PASSWORD",
				Usage:       "password to auth with, interactive if empty",
				Destination: &loginArgs.password,
			},
		},
		Action: runLogin,
	})
}

// runLogin executes the login command.
func runLogin(c *cli.Context) error {
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

	var err error
	username := loginArgs.username
	password := loginArgs.password

	if password != "" && username == "" {
		return errors.New("username must be specified with password")
	}

	if username == "" {
		le.Debug("prompting for authentication info")
		username, password, err = runLoginPrompt()
		if err != nil {
			return err
		}
	}

	le.
		WithField("username", username).
		Info("authenticating")

	return nil
}

// runLoginPrompt executes the username:password prompt.
func runLoginPrompt() (
	username string,
	password string,
	err error,
) {
	username, err = (&promptui.Prompt{Label: "Username"}).Run()
	if err != nil {
		return
	}

	password, err = (&promptui.Prompt{Label: "Password", Mask: '*'}).Run()
	if err != nil {
		return
	}

	if username == "" || password == "" {
		err = errors.New("username and password cannot be empty")
	}

	return
}
