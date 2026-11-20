package unixfs_billy

import (
	"sync"

	"github.com/go-git/go-billy/v6"
)

// billyFSState shares the backend and mutation order of a cursor tree.
type billyFSState struct {
	// bfs is the Billy filesystem.
	bfs billy.Basic
	// native indicates that the OS owns directory and file handle concurrency.
	native bool
	// mtx serializes mutations, including check and create.
	mtx sync.RWMutex
	// readMtx shares mtx for native reads and takes it exclusively for memfs
	// and other backends without a concurrency contract.
	readMtx sync.Locker
	// version reads the token from an open native file when one is requested.
	version func(billy.File) ([]byte, error)
}
