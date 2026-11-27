package confparse

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/protocol"
)

// ParseProtocolID parses the protocol ID if it is not empty.
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

// ParseProtocolIDs parses a list of protocol IDs.
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
