//go:build !tinygo

package s4wave_appconnector_world

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_appconnector "github.com/s4wave/spacewave/sdk/appconnector"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// requestTimeout bounds one read of the application's admin API.
const requestTimeout = 30 * time.Second

// connectorFactory creates the connector's PersistentExecutionService.
// Returns a nil invoker when the session peer is unknown: the fetcher reads the
// token Secret as that peer, so it cannot run without one.
func connectorFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	// Require the engine the fetcher reads and writes through.
	if engine == nil {
		return nil, nil, errors.New("world engine is required")
	}
	sessionPeerID := objecttype.SessionPeerIDFromContext(ctx)
	if len(sessionPeerID) == 0 {
		return nil, func() {}, nil
	}

	// Serve the connector's process over its resource mux.
	resource := &connectorResource{
		le:        le.WithField("app-connector", objectKey),
		b:         b,
		engine:    engine,
		objectKey: objectKey,
		peerID:    sessionPeerID,
		client:    &http.Client{},
	}
	mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_process.SRPCRegisterPersistentExecutionService(mux, resource)
	})
	return mux, func() {}, nil
}

// connectorResource implements PersistentExecutionService for an AppConnector.
type connectorResource struct {
	// le reports fetch failures.
	le *logrus.Entry
	// b mounts the token Secret's nested SharedObject.
	b bus.Bus
	// engine is the Space World the connector and its snapshot live in.
	engine world.Engine
	// objectKey identifies the AppConnector object.
	objectKey string
	// peerID is the session peer that reads the token Secret.
	peerID peer.ID
	// client performs the admin API reads.
	client *http.Client
}

// readResult is the outcome of one read in a fetch cycle.
type readResult struct {
	// read names the read.
	read *s4wave_appconnector.AppRead
	// status is the HTTP status code of a response that arrived.
	status uint32
	// contentType is the Content-Type of the response.
	contentType string
	// body is the response body.
	body []byte
	// err is the failure, or nil for a good response.
	err error
}

// Execute implements SRPCPersistentExecutionServiceServer.
// Sends RUNNING status, then fetches every read of the connector each poll
// interval and records the results in its AppSnapshot until the stream is
// canceled, which happens when the ProcessBinding is revoked.
func (r *connectorResource) Execute(
	req *s4wave_process.ExecuteRequest,
	stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream,
) error {
	// Report RUNNING before the first fetch.
	ctx := stream.Context()
	if err := stream.Send(&s4wave_process.ExecuteStatus{
		State:     s4wave_process.ExecutionState_ExecutionState_RUNNING,
		Timestamp: timestamppb.Now(),
	}); err != nil {
		return err
	}

	// Refresh the snapshot, then wait out the interval the connector currently sets.
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		conn, err := r.refresh(ctx)
		if err != nil {
			return err
		}
		timer.Reset(time.Duration(conn.GetPollIntervalMs()) * time.Millisecond)
	}
}

// refresh fetches each read and records the results in the snapshot.
// A failed read is recorded in the snapshot, not returned: only a connector
// that cannot be loaded or a snapshot that cannot be written fails the process.
func (r *connectorResource) refresh(ctx context.Context) (*s4wave_appconnector.AppConnector, error) {
	// Load the connector, which fails the process when it cannot be loaded.
	conn, token, tokenErr := r.loadInputs(ctx)
	if conn == nil {
		return nil, tokenErr
	}

	// Fetch every read, or fail every read with the token error.
	results := make([]readResult, len(conn.GetReads()))
	for i, read := range conn.GetReads() {
		results[i] = readResult{read: read, err: tokenErr}
		if tokenErr == nil {
			results[i] = r.fetch(ctx, conn, read, token)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if results[i].err != nil {
			r.le.WithError(results[i].err).WithField("read", read.GetName()).Warn("app connector read failed")
		}
	}
	return conn, r.writeSnapshot(ctx, results)
}

// loadInputs reads the connector and its API token from one World snapshot.
// Returns a nil connector with the error when the connector cannot be loaded,
// and the connector with a token error when only the token is unavailable.
func (r *connectorResource) loadInputs(ctx context.Context) (*s4wave_appconnector.AppConnector, string, error) {
	// Open one snapshot so the connector and token Secret are read together.
	tx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, "", err
	}
	defer tx.Discard()

	// Load the connector and require it to be well formed.
	conn, err := world.LookupObjectBody[*s4wave_appconnector.AppConnector](
		ctx,
		tx,
		r.objectKey,
		s4wave_appconnector.NewAppConnectorBlock,
	)
	if err != nil {
		return nil, "", errors.Wrap(err, "load app connector")
	}
	if err := conn.Validate(); err != nil {
		return nil, "", err
	}

	// Read the token as the session peer: the Secret's grants decide access.
	tokenKey := conn.GetTokenSecretObjectKey()
	if err := world_types.CheckObjectType(ctx, tx, tokenKey, s4wave_secret.SecretTypeID); err != nil {
		return conn, "", errors.Wrap(err, "token secret")
	}
	secret, err := world.LookupObjectBody[*s4wave_secret.Secret](ctx, tx, tokenKey, s4wave_secret.NewSecretBlock)
	if err != nil {
		return conn, "", errors.Wrap(err, "token secret")
	}
	payload, err := s4wave_secret.ReadSecretPayloadForPeer(
		ctx,
		r.b,
		secret,
		s4wave_secret.SecretKindAPIToken,
		r.peerID.String(),
	)
	if err != nil {
		return conn, "", errors.Wrap(err, "read token secret")
	}
	if len(payload.GetValue()) == 0 {
		return conn, "", errors.New("token secret is empty")
	}
	return conn, string(payload.GetValue()), nil
}

// fetch reads one path of the application's admin API with the bearer token.
// The failure text reaches Space members, so it never includes the token or
// the response body.
func (r *connectorResource) fetch(
	ctx context.Context,
	conn *s4wave_appconnector.AppConnector,
	read *s4wave_appconnector.AppRead,
	token string,
) readResult {
	// Bound the request and build it with the bearer token.
	result := readResult{read: read}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	// Build the request against the connector's base URL.
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimRight(conn.GetBaseUrl(), "/")+read.GetPath(),
		nil,
	)
	if err != nil {
		result.err = err
		return result
	}

	// Send the request, authenticated with the bearer token.
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := r.client.Do(req)
	if err != nil {
		result.err = errors.Wrap(err, "request")
		return result
	}
	defer resp.Body.Close()

	// Read one byte past the limit to tell an oversized body from one that fits.
	limit := int64(conn.GetMaxBodyBytes())
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		result.err = errors.Wrap(err, "read response")
		return result
	}
	result.status = uint32(resp.StatusCode)
	switch {
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		result.err = fmt.Errorf("unexpected status %d", resp.StatusCode)
	case int64(len(body)) > limit:
		result.err = fmt.Errorf("response exceeds %d bytes", limit)
	default:
		result.contentType = resp.Header.Get("Content-Type")
		result.body = body
	}
	return result
}

// writeSnapshot merges the results into the connector's AppSnapshot.
// The merge runs inside the transaction callback so a replay after another
// writer moved the World head merges into the new head.
func (r *connectorResource) writeSnapshot(ctx context.Context, results []readResult) error {
	now := time.Now()
	return world.ExecTransaction(ctx, r.engine, true, func(ctx context.Context, tx world.WorldState) error {
		// Open the snapshot linked from the connector.
		snapshotKey, err := s4wave_appconnector.LookupSnapshotKey(ctx, tx, r.objectKey)
		if err != nil {
			return err
		}
		obj, found, err := tx.GetObject(ctx, snapshotKey)
		defer world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		if !found {
			return errors.Wrap(world.ErrObjectNotFound, "app snapshot")
		}

		// Merge the results into the snapshot at the transaction's head.
		_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
			prev, err := s4wave_appconnector.UnmarshalAppSnapshot(ctx, bcs)
			if err != nil {
				return err
			}
			bcs.SetBlock(mergeSnapshot(prev, results, now), true)
			return nil
		})
		return err
	})
}

// mergeSnapshot returns the snapshot after applying a fetch cycle to prev.
// The result holds one entry per read, in read order. A good response replaces
// the entry. A failure keeps the entry's last good response and sets its error.
func mergeSnapshot(prev *s4wave_appconnector.AppSnapshot, results []readResult, now time.Time) *s4wave_appconnector.AppSnapshot {
	// Index the previous entries by read name.
	prevByName := make(map[string]*s4wave_appconnector.AppReadSnapshot, len(prev.GetReads()))
	for _, rs := range prev.GetReads() {
		prevByName[rs.GetName()] = rs
	}

	// Build one entry per result, in read order.
	next := &s4wave_appconnector.AppSnapshot{Reads: make([]*s4wave_appconnector.AppReadSnapshot, 0, len(results))}
	for _, result := range results {
		name := result.read.GetName()
		if result.err == nil {
			next.Reads = append(next.Reads, &s4wave_appconnector.AppReadSnapshot{
				Name:        name,
				Status:      result.status,
				ContentType: result.contentType,
				Body:        result.body,
				FetchedAt:   timestamppb.New(now),
			})
			continue
		}
		rs := prevByName[name].CloneVT()
		if rs == nil {
			rs = &s4wave_appconnector.AppReadSnapshot{Name: name}
		}
		rs.Error = result.err.Error()
		rs.ErrorAt = timestamppb.New(now)
		next.Reads = append(next.Reads, rs)
	}
	return next
}
