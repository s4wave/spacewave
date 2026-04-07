package coord

import (
	"context"
	"os"
	"strconv"
	"syscall"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
)

// Registry manages participant records in bbolt.
type Registry struct {
	db  *bdb.DB
	pid uint32
}

// NewRegistry creates a new participant registry.
func NewRegistry(db *bdb.DB) *Registry {
	return &Registry{
		db:  db,
		pid: uint32(os.Getpid()),
	}
}

// Register writes the local participant record to the registry.
// Cleans up stale records from dead processes as a side effect.
func (r *Registry) Register(role ParticipantRole, caps []string, socketPath string) error {
	now := time.Now().UnixNano()
	rec := &ParticipantRecord{
		Pid:                r.pid,
		Role:               role,
		StartTimeNanos:     now,
		Capabilities:       caps,
		SrpcSocketPath:     socketPath,
		LastHeartbeatNanos: now,
	}
	return r.db.Update(func(tx *bdb.Tx) error {
		// Clean stale records first.
		if err := cleanStaleParticipants(tx, r.pid); err != nil {
			return err
		}
		return PutParticipant(tx, rec)
	})
}

// Deregister removes the local participant record from the registry.
func (r *Registry) Deregister() error {
	return r.db.Update(func(tx *bdb.Tx) error {
		return DeleteParticipant(tx, r.pid)
	})
}

// cleanStaleParticipants removes records for dead processes.
// Skips the caller's own PID.
func cleanStaleParticipants(tx *bdb.Tx, selfPid uint32) error {
	records, err := ListParticipants(tx)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if rec.GetPid() == selfPid {
			continue
		}
		if !isProcessAlive(int(rec.GetPid())) {
			if err := DeleteParticipant(tx, rec.GetPid()); err != nil {
				return errors.Wrap(err, "delete stale participant "+strconv.Itoa(int(rec.GetPid())))
			}
		}
	}
	return nil
}

// Heartbeat updates the local participant's last_heartbeat_nanos field.
func (r *Registry) Heartbeat() error {
	now := time.Now().UnixNano()
	return r.db.Update(func(tx *bdb.Tx) error {
		rec, err := GetParticipant(tx, r.pid)
		if err != nil {
			return err
		}
		if rec == nil {
			return errors.New("participant record not found for heartbeat")
		}
		rec.LastHeartbeatNanos = now
		return PutParticipant(tx, rec)
	})
}

// RunHeartbeat runs periodic heartbeats every interval until ctx is cancelled.
func (r *Registry) RunHeartbeat(ctx context.Context, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			_ = r.Heartbeat()
		}
	}
}

// isProcessAlive checks if a process with the given PID is alive
// by sending signal 0.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
