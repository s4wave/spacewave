package block_store_ftps

import (
	"os"
	"strings"

	"github.com/pkg/errors"
)

// ReadNetrc returns the login and password of machine host in the netrc file
// at path, or of its default entry when no machine entry matches. Errors name
// the file and host, never the credentials. Macro definitions are not
// supported.
func ReadNetrc(path, host string) (login, password string, err error) {
	// Read the netrc file.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	// Walk the tokens, keeping the fields of the matching entry.
	var match, found, isDefault bool
	var defLogin, defPassword string
	tokens := strings.Fields(string(data))
	for i := 0; i < len(tokens); i++ {
		// Every keyword but default takes the next token as its value.
		keyword, value := tokens[i], ""
		if keyword != "default" && i+1 < len(tokens) {
			i++
			value = tokens[i]
		}

		// Start an entry or record a field of the current one.
		switch {
		case keyword == "machine":
			match, isDefault = value == host && !found, false
			found = found || match
		case keyword == "default":
			match, isDefault = false, true
		case keyword == "login" && match:
			login = value
		case keyword == "password" && match:
			password = value
		case keyword == "login" && isDefault:
			defLogin = value
		case keyword == "password" && isDefault:
			defPassword = value
		}
	}

	// Fall back to the default entry.
	if !found {
		login, password = defLogin, defPassword
	}
	if login == "" {
		return "", "", errors.Errorf("%s: no login for %s", path, host)
	}
	return login, password, nil
}
