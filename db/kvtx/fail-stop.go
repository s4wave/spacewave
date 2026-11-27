package kvtx

import (
	"errors"
	"os"
)

// FailStopEnv names the switch for fail-stop in the stores. Set to "off", a
// store's failed flush fails only its own call: a later flush that succeeds
// marks every commit before it durable, a cancelled durable commit returns at
// once, and the store keeps serving transactions, as before fail-stop.
const FailStopEnv = "SPACEWAVE_STORE_FAIL_STOP"

// FailStopOff turns fail-stop off for every store opened in this process.
// It is read from FailStopEnv at start, and tests set it before opening.
var FailStopOff = os.Getenv(FailStopEnv) == "off"

// ErrFlushFailed is returned, joined with the cause, by every flush, commit
// and new transaction of a store after one of its durable flushes failed.
// The store's outcome for the commits since its last durable flush is
// unknown, not absent: close the store and open it again, and recovery shows
// what the file holds.
var ErrFlushFailed = errors.New("an earlier flush of the store failed")
