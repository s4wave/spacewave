package block_store_ftps

import (
	"os"
	"strings"

	"github.com/pkg/errors"
)

// Login is a user name and password on the server.
type Login struct {
	// User is the login name.
	User string
	// Password is the login password.
	Password string
}

// ReadNetrc returns every login of machine host in the netrc file at path, in
// file order, or the default entry's login when no machine entry matches. A
// server that limits connections per user takes one login per entry to
// multiply its connections. Errors name the file and host, never the
// credentials. Macro definitions are not supported.
func ReadNetrc(path, host string) ([]Login, error) {
	// Read the netrc file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Walk the tokens, collecting the fields of each entry. An entry starts at
	// machine or default and runs to the next one.
	type entry struct {
		machine, isDefault bool
		host               string
		login              Login
	}
	var entries []*entry
	var cur *entry
	tokens := strings.Fields(string(data))
	for i := 0; i < len(tokens); i++ {
		// Every keyword but default takes the next token as its value.
		keyword, value := tokens[i], ""
		if keyword != "default" && i+1 < len(tokens) {
			i++
			value = tokens[i]
		}

		switch keyword {
		case "machine":
			cur = &entry{machine: true, host: value}
			entries = append(entries, cur)
		case "default":
			cur = &entry{isDefault: true}
			entries = append(entries, cur)
		case "login":
			if cur != nil {
				cur.login.User = value
			}
		case "password":
			if cur != nil {
				cur.login.Password = value
			}
		}
	}

	// Take the machine entries of the host, else the default entry.
	var logins, defaults []Login
	for _, e := range entries {
		switch {
		case e.machine && e.host == host && e.login.User != "":
			logins = append(logins, e.login)
		case e.isDefault && e.login.User != "":
			defaults = append(defaults, e.login)
		}
	}
	if len(logins) == 0 && len(defaults) != 0 {
		logins = defaults[:1]
	}
	if len(logins) == 0 {
		return nil, errors.Errorf("%s: no login for %s", path, host)
	}
	return logins, nil
}
