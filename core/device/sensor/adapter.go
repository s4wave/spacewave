// Package sensor implements Device-owned sensor endpoint adapters: one
// long-lived connection per enabled daemon-local sensor endpoint, projecting
// sanitized entity metadata and connection state into the Sensor World object.
package sensor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/core/device/sensor/esphome"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

// ReconnectDelay is the fixed delay between connection attempts.
const ReconnectDelay = 5 * time.Second

// ObservationFlushInterval bounds how often receive time is flushed to the
// Sensor object while states stream in.
const ObservationFlushInterval = time.Second

// DialFunc opens the endpoint connection. Tests inject fakes.
type DialFunc func(ctx context.Context, address string) (net.Conn, error)

// ObjectKey derives the stable Sensor object key for one Device endpoint.
func ObjectKey(deviceObjectKey, endpointID string) string {
	sum := sha256.Sum256([]byte("sensors/" + deviceObjectKey + "/" + endpointID))
	return "sensors/" + hex.EncodeToString(sum[:])[:32]
}

// EndpointLabel derives the sanitized operator label from local policy. It
// never contains the raw endpoint address.
func EndpointLabel(endpointID string) string {
	fields := strings.Fields(strings.TrimSpace(endpointID))
	return strings.Join(fields, " ")
}

// Status is the live adapter state used by capability projection.
type Status struct {
	// ConnectionState is the current connection state.
	ConnectionState s4wave_device.SensorConnectionState
	// LastError is the bounded sanitized category of the last connection
	// failure, if any. It never contains raw error text.
	LastError string
}

// Manager reconciles sensor adapters with daemon-local policy. The caller
// supplies a ready World engine; adapters stop when that engine or their
// context ends, riding the existing Device readiness lifecycle.
type Manager struct {
	le              *logrus.Entry
	engine          world.Engine
	deviceObjectKey string
	dial            DialFunc
	now             func() time.Time

	mu       sync.Mutex
	adapters map[string]*Adapter
	changed  chan struct{}
	closed   bool
}

// NewManager constructs a Manager over a ready World engine. A nil dial uses
// the real network dialer.
func NewManager(
	le *logrus.Entry,
	engine world.Engine,
	deviceObjectKey string,
	dial DialFunc,
) *Manager {
	if dial == nil {
		dialer := &net.Dialer{}
		dial = func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		}
	}
	return &Manager{
		le:              le,
		engine:          engine,
		deviceObjectKey: deviceObjectKey,
		dial:            dial,
		now:             time.Now,
		adapters:        make(map[string]*Adapter),
		changed:         make(chan struct{}, 1),
	}
}

// Changed returns a channel that receives a signal after any adapter state
// change so callers can re-project capabilities.
func (m *Manager) Changed() <-chan struct{} {
	return m.changed
}

// Close stops every adapter and rejects further reconciliation. Adapters stop
// outside the lock so joining their run loops never blocks Reconcile.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	adapters := make([]*Adapter, 0, len(m.adapters))
	for _, adapter := range m.adapters {
		adapters = append(adapters, adapter)
	}
	m.adapters = make(map[string]*Adapter)
	m.mu.Unlock()

	for _, adapter := range adapters {
		adapter.stop()
	}
}

// endpointConfig is the normalized connection configuration one adapter runs.
type endpointConfig struct {
	kind    s4wave_device.SensorAdapterKind
	address string
}

// normalizeEndpoint reduces one policy endpoint to its connection
// configuration. It reports false for endpoints no adapter should serve.
func normalizeEndpoint(endpoint *device_policy.SensorEndpointPolicy) (endpointConfig, string, bool) {
	if endpoint == nil || !endpoint.GetEnabled() {
		return endpointConfig{}, "", false
	}
	id := strings.TrimSpace(endpoint.GetId())
	if id == "" {
		return endpointConfig{}, "", false
	}
	kind := endpoint.GetAdapterKind()
	if kind != s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME {
		return endpointConfig{}, id, false
	}
	return endpointConfig{kind: kind, address: strings.TrimSpace(endpoint.GetEndpoint())}, id, true
}

// Reconcile is level-triggered on the full normalized endpoint configuration:
// every enabled endpoint runs exactly one adapter whose config matches policy,
// removed or disabled endpoints stop, and an endpoint whose address or kind
// changed has its old adapter stopped outside the lock before the replacement
// starts. Unknown adapter kinds are logged and skipped.
func (m *Manager) Reconcile(ctx context.Context, endpoints []*device_policy.SensorEndpointPolicy) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	desired := make(map[string]endpointConfig, len(endpoints))
	var start []*device_policy.SensorEndpointPolicy
	var restart []*device_policy.SensorEndpointPolicy
	for _, endpoint := range endpoints {
		cfg, id, ok := normalizeEndpoint(endpoint)
		if !ok {
			if endpoint != nil && endpoint.GetEnabled() && strings.TrimSpace(endpoint.GetId()) != "" &&
				endpoint.GetAdapterKind() != s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME {
				m.le.WithField("endpoint_id", strings.TrimSpace(endpoint.GetId())).
					Warn("skipping sensor endpoint with unknown adapter kind")
			}
			continue
		}
		desired[id] = cfg
		existing, live := m.adapters[id]
		switch {
		case !live:
			start = append(start, endpoint)
		case existing.config != cfg:
			restart = append(restart, endpoint)
		}
	}
	var stopped []*Adapter
	for id, adapter := range m.adapters {
		if _, ok := desired[id]; !ok {
			stopped = append(stopped, adapter)
			delete(m.adapters, id)
		}
	}
	for _, endpoint := range restart {
		id := strings.TrimSpace(endpoint.GetId())
		stopped = append(stopped, m.adapters[id])
		delete(m.adapters, id)
	}
	for _, endpoint := range append(start, restart...) {
		adapter := newAdapter(m, ctx, endpoint)
		m.adapters[strings.TrimSpace(endpoint.GetId())] = adapter
		adapter.start()
	}
	m.mu.Unlock()

	for _, adapter := range stopped {
		adapter.stop()
	}
}

// Status returns the live connection status for one endpoint.
func (m *Manager) Status(endpointID string) (Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	adapter, ok := m.adapters[strings.TrimSpace(endpointID)]
	if !ok {
		return Status{}, false
	}
	return adapter.status(), true
}

// notify wakes capability projection without blocking.
func (m *Manager) notify() {
	select {
	case m.changed <- struct{}{}:
	default:
	}
}

// Adapter owns one sensor endpoint connection and its Sensor object writes.
type Adapter struct {
	m         *Manager
	endpoint  *device_policy.SensorEndpointPolicy
	config    endpointConfig
	sensorKey string

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu               sync.Mutex
	connectionState  s4wave_device.SensorConnectionState
	lastError        string
	lastObservation  time.Time
	observationDirty bool
}

func newAdapter(m *Manager, ctx context.Context, endpoint *device_policy.SensorEndpointPolicy) *Adapter {
	cfg, _, _ := normalizeEndpoint(endpoint)
	adapterCtx, cancel := context.WithCancel(ctx)
	return &Adapter{
		m:         m,
		endpoint:  endpoint.CloneVT(),
		config:    cfg,
		sensorKey: ObjectKey(m.deviceObjectKey, strings.TrimSpace(endpoint.GetId())),
		ctx:       adapterCtx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
}

// start launches the adapter loop.
func (a *Adapter) start() {
	go func() {
		defer close(a.done)
		a.run()
	}()
}

// stop cancels the adapter loop and waits for it to finish.
func (a *Adapter) stop() {
	a.cancel()
	<-a.done
}

// status snapshots the live connection status.
func (a *Adapter) status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Status{
		ConnectionState: a.connectionState,
		LastError:       a.lastError,
	}
}

// errConnectionClosed reports the connection ended without a failure.
var errConnectionClosed = errors.New("esphome connection closed")

// sanitizeFailure reduces a connection failure to one bounded operator-safe
// category. Raw error text can carry the endpoint address or credentials, so
// it never enters World state or capability detail.
func sanitizeFailure(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, esphome.ErrKeepaliveTimeout):
		return "keepalive timeout"
	case errors.Is(err, esphome.ErrAuthRejected):
		return "authentication rejected"
	case errors.Is(err, esphome.ErrPeerDisconnect):
		return "endpoint closed the connection"
	case errors.Is(err, errConnectionClosed):
		return "connection closed"
	case errors.Is(err, context.Canceled):
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "connection timed out"
	case errors.Is(err, esphome.ErrUnsupportedAPIVersion):
		return "unsupported protocol version"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "network error"
	}
	var frameErr *esphome.ErrFrameFormat
	if errors.As(err, &frameErr) {
		return "protocol error"
	}
	return "connection failed"
}

// record updates the live status snapshot and wakes capability projection.
// It does not write World state.
func (a *Adapter) record(state s4wave_device.SensorConnectionState, lastError string) {
	a.mu.Lock()
	a.connectionState = state
	a.lastError = lastError
	a.mu.Unlock()
	a.m.notify()
}

// transition records an actual connection-state change and persists it to the
// Sensor object with its sanitized category. Unchanged states stay unwritten:
// repeated retry attempts do not rewrite CONNECTING.
func (a *Adapter) transition(state s4wave_device.SensorConnectionState, lastError string) {
	a.mu.Lock()
	if a.connectionState == state && a.lastError == lastError {
		a.mu.Unlock()
		return
	}
	a.connectionState = state
	a.lastError = lastError
	a.mu.Unlock()
	a.m.notify()

	now := timestamppb.New(a.m.now())
	err := a.persist(func(sensor *s4wave_device.Sensor) {
		sensor.ConnectionState = state
		sensor.LastError = lastError
		sensor.TouchUpdatedAt(now)
	})
	if err != nil && a.ctx.Err() == nil && a.m.le != nil {
		a.m.le.WithError(err).WithField("sensor_object_key", a.sensorKey).
			Warn("failed to persist sensor connection state")
	}
}

// run connects, enumerates, consumes states, and reconnects until cancelled.
func (a *Adapter) run() {
	for a.ctx.Err() == nil {
		if err := a.connectOnce(); err != nil {
			if a.ctx.Err() != nil {
				return
			}
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(ReconnectDelay):
			}
		}
	}
}

// connectOnce runs one full connection attempt through cancellation. Every
// actual connection-state change persists to the Sensor object; only its
// sanitized category ever leaves this package.
func (a *Adapter) connectOnce() error {
	a.transition(s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTING, "")

	client, err := esphome.Connect(a.ctx, esphome.Options{
		Address: a.config.address,
		Dial:    a.m.dial,
		OnState: a.onState,
		OnError: func(error) {},
	})
	if err != nil {
		a.transition(s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_OFFLINE, sanitizeFailure(err))
		return err
	}

	entities := client.Entities()
	now := timestamppb.New(a.m.now())
	if err := a.persist(func(sensor *s4wave_device.Sensor) {
		sensor.Entities = mapEntities(client.Device(), entities)
		sensor.ConnectionState = s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTED
		sensor.LastError = ""
		sensor.TouchUpdatedAt(now)
	}); err != nil {
		_ = client.Close(context.Background())
		return errors.Wrap(err, "persist enumerated entities")
	}
	a.record(s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTED, "")

	flushStop := make(chan struct{})
	defer close(flushStop)
	go func() {
		ticker := time.NewTicker(ObservationFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-client.Done():
				return
			case <-flushStop:
				return
			case <-ticker.C:
				a.flushObservation(false)
			}
		}
	}()

	select {
	case <-a.ctx.Done():
		_ = client.Close(context.Background())
		return a.ctx.Err()
	case <-client.Done():
		err := client.Err()
		_ = client.Close(context.Background())
		if err == nil {
			err = errConnectionClosed
		}
		a.flushObservation(true)
		// The socket is terminated, so the endpoint is offline regardless of
		// why it ended; the sanitized category carries the liveness detail.
		a.transition(s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_OFFLINE, sanitizeFailure(err))
		return err
	}
}

// onState records observation receive time. Observed values never enter the
// Sensor object; the receive time flushes on a bounded interval.
func (a *Adapter) onState(state esphome.State) {
	a.mu.Lock()
	a.lastObservation = a.m.now()
	a.observationDirty = true
	a.mu.Unlock()
}

// flushObservation persists pending receive time at most once per interval.
func (a *Adapter) flushObservation(force bool) {
	a.mu.Lock()
	if !force && (!a.observationDirty || time.Since(a.lastObservation) < ObservationFlushInterval) {
		a.mu.Unlock()
		return
	}
	observedAt := a.lastObservation
	a.observationDirty = false
	a.mu.Unlock()
	now := timestamppb.New(observedAt)
	if err := a.persist(func(sensor *s4wave_device.Sensor) {
		sensor.LastObservationAt = now
		sensor.TouchUpdatedAt(now)
	}); err != nil && a.m.le != nil {
		a.m.le.WithError(err).WithField("sensor_object_key", a.sensorKey).
			Warn("failed to persist sensor observation time")
	}
}

// persist applies one mutation to the Sensor object in one World transaction,
// creating the object with its ObjectType when absent.
func (a *Adapter) persist(mutate func(*s4wave_device.Sensor)) error {
	tx, err := a.m.engine.NewTransaction(a.ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	objState, found, err := tx.GetObject(a.ctx, a.sensorKey)
	if err != nil {
		return err
	}
	var sensor *s4wave_device.Sensor
	if found {
		sensor, err = readSensorBlock(a.ctx, objState)
		if err != nil {
			return err
		}
	} else {
		sensor = &s4wave_device.Sensor{
			EndpointId:    strings.TrimSpace(a.endpoint.GetId()),
			AdapterKind:   a.endpoint.GetAdapterKind(),
			EndpointLabel: EndpointLabel(a.endpoint.GetId()),
			CreatedAt:     timestamppb.New(a.m.now()),
		}
	}
	if sensor == nil {
		return errors.New("sensor object state is required")
	}
	mutate(sensor)
	if err := sensor.Validate(); err != nil {
		return err
	}
	if found {
		_, _, err = world.AccessObjectState(a.ctx, objState, true, func(bcs *block.Cursor) error {
			bcs.SetBlock(sensor, true)
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		if _, _, err := world.CreateWorldObject(a.ctx, tx, a.sensorKey, func(bcs *block.Cursor) error {
			bcs.ClearAllRefs()
			bcs.SetBlock(sensor, true)
			return nil
		}); err != nil {
			return err
		}
		if err := world_types.SetObjectType(a.ctx, tx, a.sensorKey, s4wave_device.SensorTypeID); err != nil {
			return err
		}
	}
	// Reconcile the Device-to-Sensor edge on every persist: SetGraphQuad is a
	// documented no-op when the quad exists and restores it if state lost it.
	if err := tx.SetGraphQuad(a.ctx, s4wave_device.NewDeviceToSensorQuad(a.m.deviceObjectKey, a.sensorKey)); err != nil {
		return errors.Wrap(err, "link device to sensor")
	}
	return tx.Commit(a.ctx)
}

// readSensorBlock unmarshals the Sensor block from an object state.
func readSensorBlock(ctx context.Context, objState world.ObjectState) (*s4wave_device.Sensor, error) {
	var sensor *s4wave_device.Sensor
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		sensor, uerr = s4wave_device.UnmarshalSensor(ctx, bcs)
		return uerr
	})
	return sensor, err
}

// mapEntities converts enumerated ESPHome entities to sanitized SensorEntity
// metadata.
func mapEntities(device esphome.DeviceInfo, entities []esphome.Entity) []*s4wave_device.SensorEntity {
	out := make([]*s4wave_device.SensorEntity, 0, len(entities))
	for _, entity := range entities {
		valueKind := s4wave_device.SensorEntityValueKind_SENSOR_ENTITY_VALUE_KIND_UNKNOWN
		switch entity.Kind {
		case esphome.EntityBinary:
			valueKind = s4wave_device.SensorEntityValueKind_SENSOR_ENTITY_VALUE_KIND_BINARY
		case esphome.EntityNumeric:
			valueKind = s4wave_device.SensorEntityValueKind_SENSOR_ENTITY_VALUE_KIND_NUMERIC
		case esphome.EntityLight:
			valueKind = s4wave_device.SensorEntityValueKind_SENSOR_ENTITY_VALUE_KIND_LIGHT
		}
		out = append(out, &s4wave_device.SensorEntity{
			ObjectId:    entity.ObjectID,
			Key:         entity.Key,
			Name:        entity.Name,
			ValueKind:   valueKind,
			Unit:        entity.Unit,
			Precision:   uint32(max(entity.AccuracyDecimals, 0)),
			DeviceClass: entity.DeviceClass,
		})
	}
	return out
}
