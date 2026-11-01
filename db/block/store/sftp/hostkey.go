package block_store_sftp

import (
	"bytes"
	"net"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

// newHostKeyCallback returns the callback that verifies the server's host key,
// and the host key algorithms to offer, nil for the defaults. A pin is a
// "SHA256:..." fingerprint or an authorized_keys style public key line, and the
// server must show exactly that key. A fingerprint does not name the key's
// type, so the server may show another of its keys and fail the pin; a public
// key line restricts the handshake to its own type. Without a pin the callback
// trusts the first key it sees and requires the same one afterwards, so a
// reconnect cannot be redirected to another server.
func newHostKeyCallback(pin string) (ssh.HostKeyCallback, []string, error) {
	// Ignore the whitespace around a pin copied from a file.
	pin = strings.TrimSpace(pin)

	// Pin a fingerprint.
	if strings.HasPrefix(pin, "SHA256:") {
		return func(host string, _ net.Addr, key ssh.PublicKey) error {
			if got := ssh.FingerprintSHA256(key); got != pin {
				return errors.Errorf("ssh host key of %s is %s, not the pinned key", host, got)
			}
			return nil
		}, nil, nil
	}

	// Pin a public key.
	if pin != "" {
		want, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pin))
		if err != nil {
			return nil, nil, errors.Wrap(err, "parse the pinned ssh host key")
		}
		return pinned(want), algorithms(want), nil
	}

	// Trust the first key seen.
	var mu sync.Mutex
	var first ssh.PublicKey
	return func(host string, remote net.Addr, key ssh.PublicKey) error {
		mu.Lock()
		defer mu.Unlock()
		if first == nil {
			first = key
			return nil
		}
		return pinned(first)(host, remote, key)
	}, nil, nil
}

// algorithms returns the host key algorithms that sign with a key of key's
// type.
func algorithms(key ssh.PublicKey) []string {
	if key.Type() == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{key.Type()}
}

// pinned returns the callback that accepts exactly the key want.
func pinned(want ssh.PublicKey) ssh.HostKeyCallback {
	return func(host string, _ net.Addr, key ssh.PublicKey) error {
		if !bytes.Equal(want.Marshal(), key.Marshal()) {
			return errors.Errorf("ssh host key of %s is %s, not the pinned key", host, ssh.FingerprintSHA256(key))
		}
		return nil
	}
}
