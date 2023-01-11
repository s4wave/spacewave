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
