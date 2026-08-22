// Package esphome implements the ESPHome Native API client subset used by
// Device-owned sensor connections: framing, hello, authentication, entity
// enumeration, state subscription, ping keepalive, and graceful disconnect.
package esphome

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Default options mirror the working Hemi-V client behavior.
const (
	// DefaultConnectTimeout bounds dial, hello, authentication, and enumeration.
	DefaultConnectTimeout = 5 * time.Second
	// DefaultKeepaliveInterval is how often the client sends a ping.
	DefaultKeepaliveInterval = 20 * time.Second
	// DefaultKeepaliveTimeout bounds silence after an unanswered ping.
	DefaultKeepaliveTimeout = 90 * time.Second
)

// ClientInfo identifies this client in device logs.
const ClientInfo = "spacewave-device"

// SupportedAPIVersionMajor is the only supported breaking-protocol version.
const SupportedAPIVersionMajor = 1

// Sentinel failures classify connection outcomes without exposing raw error
// text, which can carry endpoint addresses or credentials.
var (
	// ErrKeepaliveTimeout reports unanswered ping liveness. Consumers
	// distinguish degraded liveness from transport loss with errors.Is.
	ErrKeepaliveTimeout = errors.New("esphome keepalive timed out")
	// ErrAuthRejected reports the endpoint refused the configured password.
	ErrAuthRejected = errors.New("esphome rejected the api password")
	// ErrPeerDisconnect reports the endpoint asked to end the connection.
	ErrPeerDisconnect = errors.New("esphome peer requested disconnect")
	// ErrUnsupportedAPIVersion reports an incompatible Native API version.
	ErrUnsupportedAPIVersion = errors.New("esphome native api version not supported")
)

// EntityKind classifies an enumerated entity.
type EntityKind int

// Entity kinds published by the Native API subset.
const (
	// EntityBinary is a boolean binary sensor.
	EntityBinary EntityKind = iota
	// EntityNumeric is a float sensor with an optional unit.
	EntityNumeric
	// EntityLight is a controllable light.
	EntityLight
)

// String returns the stable entity kind name.
func (k EntityKind) String() string {
	switch k {
	case EntityBinary:
		return "binary_sensor"
	case EntityNumeric:
		return "sensor"
	case EntityLight:
		return "light"
	default:
		return "unknown"
	}
}

// Entity is one enumerated endpoint entity.
type Entity struct {
	// Key joins entity metadata with state packets for this connection.
	Key uint32
	// ObjectID is the stable configured entity identifier.
	ObjectID string
	// Name is the human-readable configured entity name.
	Name string
	// Kind classifies the observable value.
	Kind EntityKind
	// Unit describes numeric state units. Empty when unitless.
	Unit string
	// AccuracyDecimals is the display precision of a numeric value.
	AccuracyDecimals int32
	// DeviceClass is the endpoint-declared semantic class, when present.
	DeviceClass string
	// SupportedColorModes lists light channel combinations.
	SupportedColorModes []ColorMode
	// Effects lists light effect names accepted by the firmware.
	Effects []string
}

// LightState is the complete published light state.
type LightState struct {
	State            bool
	Brightness       float32
	ColorBrightness  float32
	ColorMode        ColorMode
	Red              float32
	Green            float32
	Blue             float32
	White            float32
	ColorTemperature float32
	ColdWhite        float32
	WarmWhite        float32
	Effect           string
}

// State is one published entity state. Missing reports that the component has
// not produced a valid value yet; consumers must never read Missing as zero.
type State struct {
	// Key identifies the entity from enumeration.
	Key uint32
	// Kind classifies the state payload.
	Kind EntityKind
	// Missing is true before the component produced a valid value.
	Missing bool
	// Binary is the boolean value for EntityBinary.
	Binary bool
	// Numeric is the float value for EntityNumeric.
	Numeric float32
	// Light is the complete state for EntityLight.
	Light LightState
}

// DeviceInfo is the negotiated endpoint identity.
type DeviceInfo struct {
	ApiVersionMajor uint32
	ApiVersionMinor uint32
	ServerInfo      string
	Name            string
}

// Options configures one client connection.
type Options struct {
	// Address is the endpoint host:port address.
	Address string
	// Password is the legacy Native API password, empty when disabled.
	Password string
	// ConnectTimeout bounds dial, hello, authentication, and enumeration.
	ConnectTimeout time.Duration
	// KeepaliveInterval is how often the client sends a ping.
	KeepaliveInterval time.Duration
	// KeepaliveTimeout bounds silence after an unanswered ping.
	KeepaliveTimeout time.Duration
	// OnState receives every published entity state.
	OnState func(State)
	// OnError receives the terminal connection error.
	OnError func(error)
	// Dial dials the endpoint connection. Tests inject fakes; nil uses net.Dial.
	Dial func(ctx context.Context, address string) (net.Conn, error)
}

func (o *Options) fillDefaults() error {
	if o.ConnectTimeout <= 0 {
		o.ConnectTimeout = DefaultConnectTimeout
	}
	if o.KeepaliveInterval <= 0 {
		o.KeepaliveInterval = DefaultKeepaliveInterval
	}
	if o.KeepaliveTimeout <= 0 {
		o.KeepaliveTimeout = DefaultKeepaliveTimeout
	}
	if o.Dial == nil {
		dialer := &net.Dialer{Timeout: o.ConnectTimeout}
		o.Dial = func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		}
	}
	return nil
}

// Client owns one ESPHome Native API connection and its entity-key map.
type Client struct {
	opts Options

	writeMu sync.Mutex
	conn    net.Conn

	mtx      sync.Mutex
	pending  map[uint32][]chan []byte
	entities []Entity
	byKey    map[uint32]Entity
	device   DeviceInfo
	failure  error
	closed   bool

	// lastPong is the liveness deadline: initialized when the protocol
	// connection succeeds and refreshed by every received pong.
	lastPong time.Time

	doneCh chan struct{}
}

// Connect dials the endpoint, negotiates hello and authentication, enumerates
// entities, subscribes to state publication, and starts ping keepalive.
func Connect(ctx context.Context, opts Options) (*Client, error) {
	if err := opts.fillDefaults(); err != nil {
		return nil, err
	}
	if opts.Address == "" {
		return nil, errors.New("esphome endpoint address is required")
	}
	dialCtx, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancel()
	conn, err := opts.Dial(dialCtx, opts.Address)
	if err != nil {
		return nil, errors.Wrap(err, "dial esphome endpoint")
	}
	c := &Client{
		opts:    opts,
		conn:    conn,
		pending: make(map[uint32][]chan []byte),
		byKey:   make(map[uint32]Entity),
		doneCh:  make(chan struct{}),
	}
	// The read loop must run during negotiation: it resolves every pending
	// response waiter the handshake blocks on.
	go c.readLoop()
	if err := c.negotiate(dialCtx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	// The liveness clock starts here: a peer that completed negotiation but
	// never answers pings degrades after KeepaliveTimeout.
	c.mtx.Lock()
	c.lastPong = time.Now()
	c.mtx.Unlock()
	go c.keepaliveLoop()
	return c, nil
}

// Device returns the negotiated endpoint identity.
func (c *Client) Device() DeviceInfo {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.device
}

// Entities returns the enumerated entities in arrival order.
func (c *Client) Entities() []Entity {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	out := make([]Entity, len(c.entities))
	copy(out, c.entities)
	return out
}

// FindEntity returns the enumerated entity with the requested key.
func (c *Client) FindEntity(key uint32) (Entity, bool) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	entity, ok := c.byKey[key]
	return entity, ok
}

// Done closes when the connection has terminated.
func (c *Client) Done() <-chan struct{} {
	return c.doneCh
}

// Err returns the terminal connection error, if any.
func (c *Client) Err() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.failure
}

// Close performs a graceful disconnect and closes the socket.
func (c *Client) Close(ctx context.Context) error {
	c.mtx.Lock()
	closed := c.closed
	c.mtx.Unlock()
	if !closed {
		waitCtx, cancel := context.WithTimeout(ctx, c.opts.ConnectTimeout)
		response := c.waitFor(EspHomeMessageId_ESP_HOME_MESSAGE_ID_DISCONNECT_RESPONSE)
		disconnect, err := (&DisconnectRequest{}).MarshalVT()
		if err == nil {
			_ = c.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_DISCONNECT_REQUEST, disconnect)
		}
		select {
		case <-response:
		case <-waitCtx.Done():
		case <-c.doneCh:
		}
		cancel()
	}
	c.terminate(nil)
	return nil
}

// negotiate runs hello, authentication, enumeration, and subscription.
func (c *Client) negotiate(ctx context.Context) error {
	helloResponse := c.waitFor(EspHomeMessageId_ESP_HOME_MESSAGE_ID_HELLO_RESPONSE)
	helloReq, err := (&HelloRequest{
		ClientInfo:      ClientInfo,
		ApiVersionMajor: SupportedAPIVersionMajor,
		ApiVersionMinor: 10,
	}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal hello request")
	}
	if err := c.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_HELLO_REQUEST, helloReq); err != nil {
		return err
	}
	payload, err := c.await(ctx, helloResponse, "hello")
	if err != nil {
		return err
	}
	hello := &HelloResponse{}
	if err := hello.UnmarshalVT(payload); err != nil {
		return errors.Wrap(err, "parse hello response")
	}
	if hello.GetApiVersionMajor() == 0 || hello.GetServerInfo() == "" || hello.GetName() == "" {
		return errors.New("esphome omitted required hello fields")
	}
	if hello.GetApiVersionMajor() != SupportedAPIVersionMajor {
		return errors.Wrapf(
			ErrUnsupportedAPIVersion,
			"endpoint reported %d.%d",
			hello.GetApiVersionMajor(),
			hello.GetApiVersionMinor(),
		)
	}
	c.mtx.Lock()
	c.device = DeviceInfo{
		ApiVersionMajor: hello.GetApiVersionMajor(),
		ApiVersionMinor: hello.GetApiVersionMinor(),
		ServerInfo:      hello.GetServerInfo(),
		Name:            hello.GetName(),
	}
	c.mtx.Unlock()

	authResponse := c.waitFor(EspHomeMessageId_ESP_HOME_MESSAGE_ID_AUTHENTICATION_RESPONSE)
	authReq, err := (&AuthenticationRequest{Password: c.opts.Password}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal authentication request")
	}
	if err := c.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_AUTHENTICATION_REQUEST, authReq); err != nil {
		return err
	}
	payload, err = c.await(ctx, authResponse, "authentication")
	if err != nil {
		return err
	}
	auth := &AuthenticationResponse{}
	if err := auth.UnmarshalVT(payload); err != nil {
		return errors.Wrap(err, "parse authentication response")
	}
	if auth.GetInvalidPassword() {
		return ErrAuthRejected
	}

	listDone := c.waitFor(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_DONE_RESPONSE)
	listReq, err := (&ListEntitiesRequest{}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal list entities request")
	}
	if err := c.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_REQUEST, listReq); err != nil {
		return err
	}
	if _, err := c.await(ctx, listDone, "entity enumeration"); err != nil {
		return err
	}
	subscribeReq, err := (&SubscribeStatesRequest{}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal subscribe states request")
	}
	return c.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_SUBSCRIBE_STATES_REQUEST, subscribeReq)
}

// await waits for one pending response or the deadline.
func (c *Client) await(ctx context.Context, response <-chan []byte, what string) ([]byte, error) {
	select {
	case <-c.doneCh:
		if err := c.Err(); err != nil {
			return nil, err
		}
		return nil, errors.Errorf("esphome %s interrupted", what)
	default:
	}
	select {
	case payload := <-response:
		return payload, nil
	case <-ctx.Done():
		return nil, errors.Errorf("esphome %s timed out", what)
	case <-c.doneCh:
		if err := c.Err(); err != nil {
			return nil, err
		}
		return nil, errors.Errorf("esphome %s interrupted", what)
	}
}

// waitFor registers a one-shot response waiter for the message ID.
func (c *Client) waitFor(messageID EspHomeMessageId) <-chan []byte {
	ch := make(chan []byte, 1)
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.pending[uint32(messageID)] = append(c.pending[uint32(messageID)], ch)
	return ch
}

// send encodes and writes one frame.
func (c *Client) send(messageID EspHomeMessageId, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mtx.Lock()
	failure := c.failure
	closed := c.closed
	c.mtx.Unlock()
	if failure != nil || closed {
		if failure == nil {
			failure = errors.New("esphome connection is closed")
		}
		return failure
	}
	frame := EncodeFrame(uint32(messageID), payload)
	_, err := c.conn.Write(frame)
	return err
}

// readLoop decodes frames until the connection terminates.
func (c *Client) readLoop() {
	decoder := NewFrameDecoder()
	buf := make([]byte, 4096)
	for {
		n, err := c.conn.Read(buf)
		if n > 0 {
			frames, ferr := decoder.Push(buf[:n])
			if ferr != nil {
				c.terminate(ferr)
				return
			}
			for _, frame := range frames {
				if herr := c.handleFrame(frame); herr != nil {
					c.terminate(herr)
					return
				}
			}
		}
		if err != nil {
			c.terminate(err)
			return
		}
	}
}

// handleFrame dispatches one decoded frame.
func (c *Client) handleFrame(frame Frame) error {
	switch frame.MessageID {
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_PING_RESPONSE):
		if err := (&PingResponse{}).UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse ping response")
		}
		c.mtx.Lock()
		c.lastPong = time.Now()
		c.mtx.Unlock()
		return nil
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_PING_REQUEST):
		if err := (&PingRequest{}).UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse ping request")
		}
		pingResp, err := (&PingResponse{}).MarshalVT()
		if err != nil {
			return errors.Wrap(err, "marshal ping response")
		}
		return c.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_PING_RESPONSE, pingResp)
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_DISCONNECT_REQUEST):
		if err := (&DisconnectRequest{}).UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse disconnect request")
		}
		disconnectResp, merr := (&DisconnectResponse{}).MarshalVT()
		if merr != nil {
			c.terminate(ErrPeerDisconnect)
			return errors.Wrap(merr, "marshal disconnect response")
		}
		serr := c.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_DISCONNECT_RESPONSE, disconnectResp)
		c.terminate(ErrPeerDisconnect)
		return serr
	}
	if c.resolvePending(frame.MessageID, frame.Payload) {
		return nil
	}
	switch frame.MessageID {
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_BINARY_SENSOR_RESPONSE):
		msg := &ListEntitiesBinarySensorResponse{}
		if err := msg.UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse binary sensor entity")
		}
		c.addEntity(Entity{
			Key:         msg.GetKey(),
			ObjectID:    msg.GetObjectId(),
			Name:        msg.GetName(),
			Kind:        EntityBinary,
			DeviceClass: msg.GetDeviceClass(),
		})
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_SENSOR_RESPONSE):
		msg := &ListEntitiesSensorResponse{}
		if err := msg.UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse sensor entity")
		}
		c.addEntity(Entity{
			Key:              msg.GetKey(),
			ObjectID:         msg.GetObjectId(),
			Name:             msg.GetName(),
			Kind:             EntityNumeric,
			Unit:             msg.GetUnitOfMeasurement(),
			AccuracyDecimals: msg.GetAccuracyDecimals(),
			DeviceClass:      msg.GetDeviceClass(),
		})
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_LIGHT_RESPONSE):
		msg := &ListEntitiesLightResponse{}
		if err := msg.UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse light entity")
		}
		c.addEntity(Entity{
			Key:                 msg.GetKey(),
			ObjectID:            msg.GetObjectId(),
			Name:                msg.GetName(),
			Kind:                EntityLight,
			SupportedColorModes: msg.GetSupportedColorModes(),
			Effects:             msg.GetEffects(),
		})
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_BINARY_SENSOR_STATE_RESPONSE):
		msg := &BinarySensorStateResponse{}
		if err := msg.UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse binary sensor state")
		}
		c.publish(State{
			Key:     msg.GetKey(),
			Kind:    EntityBinary,
			Missing: msg.GetMissingState(),
			Binary:  msg.GetState(),
		})
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_SENSOR_STATE_RESPONSE):
		msg := &SensorStateResponse{}
		if err := msg.UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse sensor state")
		}
		c.publish(State{
			Key:     msg.GetKey(),
			Kind:    EntityNumeric,
			Missing: msg.GetMissingState(),
			Numeric: msg.GetState(),
		})
	case uint32(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIGHT_STATE_RESPONSE):
		msg := &LightStateResponse{}
		if err := msg.UnmarshalVT(frame.Payload); err != nil {
			return errors.Wrap(err, "parse light state")
		}
		c.publish(State{
			Key:     msg.GetKey(),
			Kind:    EntityLight,
			Missing: false,
			Light: LightState{
				State:            msg.GetState(),
				Brightness:       msg.GetBrightness(),
				ColorBrightness:  msg.GetColorBrightness(),
				ColorMode:        msg.GetColorMode(),
				Red:              msg.GetRed(),
				Green:            msg.GetGreen(),
				Blue:             msg.GetBlue(),
				White:            msg.GetWhite(),
				ColorTemperature: msg.GetColorTemperature(),
				ColdWhite:        msg.GetColdWhite(),
				WarmWhite:        msg.GetWarmWhite(),
				Effect:           msg.GetEffect(),
			},
		})
	default:
		// A well-framed message ID this client does not know is a protocol
		// extension from newer firmware. Ignore it; the connection stays up.
		return nil
	}
	return nil
}

// addEntity records one enumerated entity.
func (c *Client) addEntity(entity Entity) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.entities = append(c.entities, entity)
	c.byKey[entity.Key] = entity
}

// publish delivers one state to the OnState callback.
func (c *Client) publish(state State) {
	if c.opts.OnState != nil {
		c.opts.OnState(state)
	}
}

// resolvePending completes one registered waiter.
func (c *Client) resolvePending(messageID uint32, payload []byte) bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	queue := c.pending[messageID]
	if len(queue) == 0 {
		return false
	}
	ch := queue[0]
	if len(queue) == 1 {
		delete(c.pending, messageID)
	} else {
		c.pending[messageID] = queue[1:]
	}
	ch <- payload
	return true
}

// keepaliveLoop pings on the interval and terminates on stale liveness.
func (c *Client) keepaliveLoop() {
	ticker := time.NewTicker(c.opts.KeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.doneCh:
			return
		case <-ticker.C:
		}
		c.mtx.Lock()
		stale := time.Since(c.lastPong) > c.opts.KeepaliveTimeout
		c.mtx.Unlock()
		if stale {
			c.terminate(ErrKeepaliveTimeout)
			return
		}
		pingReq, err := (&PingRequest{}).MarshalVT()
		if err != nil {
			c.terminate(err)
			return
		}
		if err := c.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_PING_REQUEST, pingReq); err != nil {
			c.terminate(err)
			return
		}
	}
}

// terminate records the terminal failure, fails pending waiters, and closes
// the socket exactly once.
func (c *Client) terminate(err error) {
	c.mtx.Lock()
	if c.failure == nil && err != nil {
		c.failure = err
	}
	terminated := c.closed
	c.closed = true
	pending := c.pending
	c.pending = make(map[uint32][]chan []byte)
	failure := c.failure
	c.mtx.Unlock()
	if terminated {
		return
	}
	for _, queue := range pending {
		for _, ch := range queue {
			close(ch)
		}
	}
	_ = c.conn.Close()
	if c.opts.OnError != nil && failure != nil {
		c.opts.OnError(failure)
	}
	close(c.doneCh)
}
