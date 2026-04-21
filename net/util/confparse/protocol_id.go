package confparse

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/protocol"
)

// ParseProtocolID parses the peer ID if it is not empty.
func ParseProtocolID(protocolID string, allowEmpty bool) (protocol.ID, error) {
	if allowEmpty && protocolID == "" {
		return "", nil
	}

	id := protocol.ID(protocolID)
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

// ParseProtocolIDs parses a list of peer IDs.
func ParseProtocolIDs(ids []string, allowEmpty bool) ([]protocol.ID, error) {
	pids := make([]protocol.ID, 0, len(ids))
	for i, pidStr := range ids {
		pid, err := ParseProtocolID(pidStr, allowEmpty)
		if err != nil {
			return nil, errors.Wrapf(err, "protocol_ids[%d]", i)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// ParseProtocolIDsUnique parses a list of peer IDs and dedupes.
func ParseProtocolIDsUnique(ids []string, allowEmpty bool) ([]protocol.ID, error) {
	m := make(map[protocol.ID]struct{})
	o := make([]protocol.ID, 0, len(ids))
	for i, s := range ids {
		pid, err := ParseProtocolID(s, allowEmpty)
		if err != nil {
			return nil, errors.Wrapf(err, "protocol_ids[%d]", i)
		}
		if _, ok := m[pid]; ok {
			continue
		}
		m[pid] = struct{}{}
		o = append(o, pid)
	}
	return o, nil
}

// ValidateProtocolID checks if a peer ID is valid.
func ValidateProtocolID(id string, allowEmpty bool) error {
	_, err := ParseProtocolID(id, allowEmpty)
	return err
}
