package cli

import (
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// parseDialerAddrs parses a dialer map from a string slice
func parseDialerAddrs(ss cli.StringSlice) (map[string]*dialer.DialerOpts, error) {
	m := make(map[string]*dialer.DialerOpts)
	for _, s := range ss.Value() {
		pair := strings.Split(s, "@")
		if len(pair) < 2 {
			continue
		}
		pid, err := confparse.ParsePeerID(strings.TrimSpace(pair[0]))
		if err != nil {
			return nil, err
		}
		if pid == peer.ID("") {
			continue
		}
		m[pid.String()] = &dialer.DialerOpts{
			Address: strings.TrimSpace(pair[1]),
		}
	}
	return m, nil
}
