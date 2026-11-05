package block

import "fmt"

// DeadlistRootPrefix prefixes the named root of each deadlist record.
const DeadlistRootPrefix = "deadlist/"

// DeadlistDueRootPrefix prefixes the named root of each due deadlist record,
// which the collector handles and releases.
const DeadlistDueRootPrefix = "deadlist-due/"

// DeadlistRootName returns the named root of the pending record at seq. The
// sequence is zero padded so the names sort in sequence order.
func DeadlistRootName(seq uint64) string {
	return fmt.Sprintf("%s%020d", DeadlistRootPrefix, seq)
}

// DeadlistDueRootName returns the named root of the due record at seq.
func DeadlistDueRootName(seq uint64) string {
	return fmt.Sprintf("%s%020d", DeadlistDueRootPrefix, seq)
}
