package s4db

import "context"

// Lease is a named lock held across transactions and processes.
type Lease struct{}

// TryLease takes the lease name when no other holder has it.
func (db *DB) TryLease(name string) (*Lease, bool, error) {
	return nil, false, ErrUnimplemented
}

// WaitLease waits for the lease name and takes it.
func (db *DB) WaitLease(ctx context.Context, name string) (*Lease, error) {
	return nil, ErrUnimplemented
}

// Release releases the lease.
func (l *Lease) Release() error {
	return nil
}
