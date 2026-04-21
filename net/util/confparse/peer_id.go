package confparse

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// ParsePeerID parses the peer ID if it is not empty.
func ParsePeerID(peerID string) (peer.ID, error) {
	if peerID == "" {
		return "", nil
	}

	return peer.IDB58Decode(peerID)
}

// ParsePeerIDs parses a list of peer IDs.
func ParsePeerIDs(ids []string, allowEmpty bool) ([]peer.ID, error) {
	pids := make([]peer.ID, 0, len(ids))
	for i, pidStr := range ids {
		if len(pidStr) == 0 {
			if allowEmpty {
				continue
			}
			return nil, errors.Wrapf(peer.ErrEmptyPeerID, "peer_ids[%d]", i)
		}
		v, err := peer.IDB58Decode(pidStr)
		if err != nil {
			return nil, errors.Wrapf(err, "peer_ids[%d]", i)
		}
		pids = append(pids, v)
	}
	return pids, nil
}

// ParsePeerIDsUnique parses a list of peer IDs and dedupes.
func ParsePeerIDsUnique(ids []string, allowEmpty bool) ([]peer.ID, error) {
	m := make(map[peer.ID]struct{})
	o := make([]peer.ID, 0, len(ids))
	for i, s := range ids {
		pid, err := ParsePeerID(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		if pid == peer.ID("") {
			if !allowEmpty {
				return nil, errors.Wrapf(peer.ErrEmptyPeerID, "peer_ids[%d]", i)
			}
			continue
		}
		if _, ok := m[pid]; ok {
			continue
		}
		m[pid] = struct{}{}
		o = append(o, pid)
	}
	return o, nil
}

// ValidatePeerID checks if a peer ID is valid and set.
func ValidatePeerID(id string) error {
	pid, err := ParsePeerID(id)
	if err == nil && len(pid) == 0 {
		err = peer.ErrEmptyPeerID
	}
	return err
}
