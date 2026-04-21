//go:build !js

package coord

import (
	"context"
	"sync"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// RoleChangeHandler is called when this process's role changes.
// The handler should start or stop services accordingly.
type RoleChangeHandler interface {
	// OnBecomeLeader is called when this process becomes the leader.
	// ctx is cancelled when leadership is lost.
	OnBecomeLeader(ctx context.Context) error
	// OnBecomeFollower is called when this process becomes a follower.
	// leaderSocketPath is the SRPC socket of the current leader.
	OnBecomeFollower(ctx context.Context, leaderSocketPath string) error
}

// Coordinator manages the full lifecycle: participant registry, leader
// election, heartbeat, SRPC mesh, and role change notifications.
type Coordinator struct {
	le       *logrus.Entry
	db       *bdb.DB
	dir      string
	registry *Registry
	election *Election
	watcher  *ParticipantWatcher
	mesh     *Mesh
	handler  RoleChangeHandler
	caps     []string

	bcast broadcast.Broadcast
	role  ParticipantRole
}

// NewCoordinator creates a new coordinator. The dir is used for the SRPC
// socket (coord-{pid}.sock). Call Run to start the lifecycle.
func NewCoordinator(
	le *logrus.Entry,
	db *bdb.DB,
	dir string,
	caps []string,
	handler RoleChangeHandler,
) *Coordinator {
	c := &Coordinator{
		le:       le,
		db:       db,
		dir:      dir,
		registry: NewRegistry(db),
		watcher:  NewParticipantWatcher(db),
		handler:  handler,
		caps:     caps,
		role:     ParticipantRole_ParticipantRole_UNKNOWN,
	}
	c.mesh = NewMesh(le.WithField("component", "mesh"), c.registry.pid, c.Role, caps)
	return c
}

// Role returns the current role.
func (c *Coordinator) Role() ParticipantRole {
	return c.role
}

// Election returns the election manager.
func (c *Coordinator) GetElection() *Election {
	return c.election
}

// Registry returns the participant registry.
func (c *Coordinator) GetRegistry() *Registry {
	return c.registry
}

// Watcher returns the participant watcher.
func (c *Coordinator) GetWatcher() *ParticipantWatcher {
	return c.watcher
}

// GetMesh returns the SRPC mesh for registering services and obtaining
// clients to remote participants.
func (c *Coordinator) GetMesh() *Mesh {
	return c.mesh
}

// Run executes the coordinator lifecycle. Blocks until ctx is cancelled.
func (c *Coordinator) Run(ctx context.Context) error {
	// Start SRPC mesh listener.
	if err := c.mesh.Listen(c.dir); err != nil {
		return errors.Wrap(err, "mesh listen")
	}

	// Create election with the now-known socket path.
	c.election = NewElection(c.db, c.mesh.SocketPath())

	// Register as participant.
	if err := c.registry.Register(ParticipantRole_ParticipantRole_FOLLOWER, c.caps, c.mesh.SocketPath()); err != nil {
		c.mesh.Close()
		return errors.Wrap(err, "register participant")
	}

	// Start background goroutines. On exit: close mesh (unblocks Serve),
	// cancel context (unblocks watcher + heartbeat), wait for all, then cleanup.
	var wg sync.WaitGroup
	bgCtx, bgCancel := context.WithCancel(ctx)
	defer func() {
		c.mesh.Close()
		bgCancel()
		wg.Wait()
		_ = c.election.ReleaseLease()
		_ = c.registry.Deregister()
	}()

	wg.Add(3)
	go func() { defer wg.Done(); c.mesh.Serve(bgCtx) }()
	go func() { defer wg.Done(); c.watcher.Run(bgCtx) }()
	go func() { defer wg.Done(); c.registry.RunHeartbeat(bgCtx, 500*time.Millisecond) }()

	// Try initial leader claim.
	claimed, err := c.election.TryClaimLeadership()
	if err != nil {
		return errors.Wrap(err, "initial leader claim")
	}

	if claimed {
		return c.runAsLeader(ctx)
	}
	return c.runAsFollower(ctx)
}

// runAsLeader runs the leader lifecycle.
func (c *Coordinator) runAsLeader(ctx context.Context) error {
	c.le.Info("became leader")
	c.setRole(ParticipantRole_ParticipantRole_LEADER)

	// Update participant record to reflect leader role.
	if err := c.db.Update(func(tx *bdb.Tx) error {
		rec, err := GetParticipant(tx, c.registry.pid)
		if err != nil || rec == nil {
			return err
		}
		rec.Role = ParticipantRole_ParticipantRole_LEADER
		return PutParticipant(tx, rec)
	}); err != nil {
		c.le.WithError(err).Warn("failed to update participant role to leader")
	}

	leaderCtx, leaderCancel := context.WithCancel(ctx)
	defer leaderCancel()

	// Start role handler.
	handlerDone := make(chan error, 1)
	go func() {
		handlerDone <- c.handler.OnBecomeLeader(leaderCtx)
	}()

	// Run lease renewal. Returns when ctx cancelled or lease lost.
	renewErr := c.election.RunLeaseRenewal(ctx)

	leaderCancel()
	<-handlerDone

	if renewErr != nil {
		return renewErr
	}
	if ctx.Err() != nil {
		return nil
	}

	// Lease lost (shouldn't happen, but defend). Transition to follower.
	c.le.Warn("lost leadership, transitioning to follower")
	return c.runAsFollower(ctx)
}

// runAsFollower runs the follower lifecycle.
func (c *Coordinator) runAsFollower(ctx context.Context) error {
	c.setRole(ParticipantRole_ParticipantRole_FOLLOWER)

	for {
		lease, err := c.election.CurrentLeader()
		if err != nil {
			return errors.Wrap(err, "read current leader")
		}

		if lease == nil {
			// No leader. Try to become one.
			claimed, err := c.election.TryReelect()
			if err != nil {
				return errors.Wrap(err, "re-elect")
			}
			if claimed {
				return c.runAsLeader(ctx)
			}
		}

		socketPath := ""
		if lease != nil {
			socketPath = lease.GetLeaderSocketPath()
		}

		c.le.WithField("leader-socket", socketPath).Info("following leader")

		followerCtx, followerCancel := context.WithCancel(ctx)
		handlerDone := make(chan error, 1)
		go func() {
			handlerDone <- c.handler.OnBecomeFollower(followerCtx, socketPath)
		}()

		// cancelFollower cancels the follower context and waits for the handler.
		cancelFollower := func() {
			followerCancel()
			<-handlerDone
		}

		// Wait for commit counter changes (leader lease renewal, participant changes, etc.)
		lastCounter := c.db.CommitCounter()
		restarting := false
		for {
			counter, err := c.db.WaitCommitCounter(ctx, lastCounter)
			if err != nil {
				cancelFollower()
				return err
			}
			lastCounter = counter

			// Check if leader is still valid.
			lease, err := c.election.CurrentLeader()
			if err != nil {
				cancelFollower()
				return errors.Wrap(err, "check leader")
			}

			if lease == nil || !c.election.isLeaseValid(lease) {
				// Leader gone or stale. Try to claim.
				claimed, err := c.election.TryReelect()
				if err != nil {
					cancelFollower()
					return errors.Wrap(err, "re-elect after leader loss")
				}
				cancelFollower()
				if claimed {
					return c.runAsLeader(ctx)
				}
				restarting = true
				break // restart follower loop with new leader
			}
		}
		if !restarting {
			cancelFollower()
		}
	}
}

// CountParticipants returns the number of alive participants in the registry.
// Useful after becoming leader to determine if this is a fresh session
// (only ourselves) or a re-election (other participants alive).
func (c *Coordinator) CountParticipants() (int, error) {
	var count int
	err := c.db.View(func(tx *bdb.Tx) error {
		records, err := ListParticipants(tx)
		if err != nil {
			return err
		}
		count = len(records)
		return nil
	})
	return count, err
}

// WaitRole waits until the coordinator has determined this process's role
// (leader or follower). Returns the role once known, or an error if ctx
// is cancelled before the role is determined.
func (c *Coordinator) WaitRole(ctx context.Context) (ParticipantRole, error) {
	var role ParticipantRole
	err := c.bcast.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
		role = c.role
		return role != ParticipantRole_ParticipantRole_UNKNOWN, nil
	})
	return role, err
}

// setRole updates the role and broadcasts the change.
func (c *Coordinator) setRole(role ParticipantRole) {
	c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		c.role = role
		broadcast()
	})
}
