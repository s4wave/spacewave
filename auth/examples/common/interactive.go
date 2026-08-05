//go:build !js

package common

import (
	"errors"

	"github.com/manifoldco/promptui"
)

// RunLoginPrompt executes the username:password prompt.
func RunLoginPrompt() (
	username string,
	password string,
	err error,
) {
	// Prompt for the username.
	username, err = (&promptui.Prompt{Label: "Username"}).Run()
	if err != nil {
		return
	}

	// Prompt for the password.
	password, err = (&promptui.Prompt{Label: "Password", Mask: '*'}).Run()
	if err != nil {
		return
	}

	// Reject empty credentials.
	if username == "" || password == "" {
		err = errors.New("username and password cannot be empty")
	}

	return
}
