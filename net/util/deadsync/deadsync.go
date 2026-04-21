package sync

import (
	ssync "sync"

	"github.com/sasha-s/go-deadlock"
)

// Pool is the sync.Pool.
type Pool = ssync.Pool

// Mutex is the deadlock mutex.
type Mutex = deadlock.Mutex

// RWMutex is the deadlock mutex.
type RWMutex = deadlock.RWMutex
