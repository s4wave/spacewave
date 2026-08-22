package esphome

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pkg/errors"
)

// fakeServer speaks just enough Native API for negotiation, answers frames
// through the test-supplied behavior, and delivers frames pushed by the test.
// fakeWriter sends framed messages and raw unframed bytes.
type fakeWriter struct {
	send func(id EspHomeMessageId, payload []byte)
	raw  func(data []byte)
}

type fakeServer struct {
	listener net.Listener
	push     chan func(w fakeWriter)
	done     chan struct{}
}

func startFakeServer(
	t *testing.T,
	behavior func(id EspHomeMessageId, w fakeWriter),
) *fakeServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	server := &fakeServer{
		listener: listener,
		push:     make(chan func(w fakeWriter), 16),
		done:     make(chan struct{}),
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveConn(server.push, server.done, conn, behavior)
		}
	}()
	t.Cleanup(func() {
		close(server.done)
		_ = listener.Close()
	})
	return server
}

// Push runs fn with the active connection's writer.
func (s *fakeServer) Push(t *testing.T, fn func(w fakeWriter)) {
	t.Helper()
	select {
	case s.push <- fn:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not accept the pushed frame")
	}
}

func serveConn(
	push <-chan func(w fakeWriter),
	done <-chan struct{},
	conn net.Conn,
	behavior func(id EspHomeMessageId, w fakeWriter),
) {
	defer conn.Close()
	writeQ := make(chan fakeFrame, 32)
	defer close(writeQ)
	go func() {
		for frame := range writeQ {
			data := frame.payload
			if !frame.raw {
				data = EncodeFrame(uint32(frame.id), frame.payload)
			}
			if _, err := conn.Write(data); err != nil {
				return
			}
		}
	}()
	// send blocks until the frame is queued or the test ends, so a full
	// buffer surfaces as a visible hang instead of a silent drop.
	enqueue := func(frame fakeFrame) {
		select {
		case writeQ <- frame:
		case <-done:
		}
	}
	w := fakeWriter{
		send: func(id EspHomeMessageId, payload []byte) {
			enqueue(fakeFrame{id: id, payload: payload})
		},
		raw: func(data []byte) {
			enqueue(fakeFrame{payload: data, raw: true})
		},
	}
	go func() {
		for {
			select {
			case fn := <-push:
				fn(w)
			case <-done:
				return
			}
		}
	}()

	decoder := NewFrameDecoder()
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			frames, ferr := decoder.Push(buf[:n])
			if ferr != nil {
				return
			}
			for _, frame := range frames {
				behavior(EspHomeMessageId(frame.MessageID), w)
			}
		}
		if err != nil {
			return
		}
	}
}

type fakeFrame struct {
	id      EspHomeMessageId
	payload []byte
	raw     bool
}

// negotiateBehavior completes hello, authentication, and enumeration with
// minimal responses, then hands further frames to next.
func negotiateBehavior(
	t *testing.T,
	next func(id EspHomeMessageId, w fakeWriter),
) func(id EspHomeMessageId, w fakeWriter) {
	t.Helper()
	// The behavior runs on fake-server goroutines: record marshal failures
	// with t.Errorf, which is safe off the test goroutine.
	marshal := func(msg interface{ MarshalVT() ([]byte, error) }) []byte {
		data, err := msg.MarshalVT()
		if err != nil {
			t.Errorf("marshal fake response: %v", err)
			return nil
		}
		return data
	}
	return func(id EspHomeMessageId, w fakeWriter) {
		switch id {
		case EspHomeMessageId_ESP_HOME_MESSAGE_ID_HELLO_REQUEST:
			w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_HELLO_RESPONSE,
				marshal(&HelloResponse{
					ApiVersionMajor: SupportedAPIVersionMajor,
					ServerInfo:      "fake-esphome-test-server",
					Name:            "fake-node",
				}))
		case EspHomeMessageId_ESP_HOME_MESSAGE_ID_AUTHENTICATION_REQUEST:
			w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_AUTHENTICATION_RESPONSE, nil)
		case EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_REQUEST:
			if next != nil {
				next(id, w)
				return
			}
			w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_DONE_RESPONSE, nil)
		default:
			if next != nil {
				next(id, w)
			}
		}
	}
}

// TestSilentPeerDegradesAfterKeepaliveTimeout pins that liveness starts at
// protocol connection: a peer that completes negotiation but never returns a
// pong terminates the client with ErrKeepaliveTimeout.
func TestSilentPeerDegradesAfterKeepaliveTimeout(t *testing.T) {
	server := startFakeServer(t, negotiateBehavior(t, nil))

	client, err := Connect(context.Background(), Options{
		Address:           server.listener.Addr().String(),
		ConnectTimeout:    time.Second,
		KeepaliveInterval: 20 * time.Millisecond,
		KeepaliveTimeout:  60 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close(context.Background())
	select {
	case <-client.Done():
		if !errors.Is(client.Err(), ErrKeepaliveTimeout) {
			t.Fatalf("Err() = %v, want ErrKeepaliveTimeout", client.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("silent peer did not degrade within the keepalive deadline")
	}
}

// TestUnhandledMessageIDAndMixedEntityDomains pins that well-framed unknown
// message IDs never fail the connection while binary, numeric, and light
// entities all enumerate and publish their state domains.
func TestUnhandledMessageIDAndMixedEntityDomains(t *testing.T) {
	states := make(chan State, 8)

	behavior := negotiateBehavior(t, func(id EspHomeMessageId, w fakeWriter) {
		if id != EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_REQUEST {
			return
		}
		marshal := func(msg interface{ MarshalVT() ([]byte, error) }) []byte {
			data, err := msg.MarshalVT()
			if err != nil {
				t.Errorf("marshal entity: %v", err)
				return nil
			}
			return data
		}
		// A well-framed unknown message ID rides along with enumeration.
		w.send(EspHomeMessageId(9999), []byte{0x01})
		w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_BINARY_SENSOR_RESPONSE,
			marshal(&ListEntitiesBinarySensorResponse{ObjectId: "motion", Key: 1, Name: "Motion"}))
		w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_SENSOR_RESPONSE,
			marshal(&ListEntitiesSensorResponse{
				ObjectId:          "temperature",
				Key:               2,
				Name:              "Temperature",
				UnitOfMeasurement: "°C",
				AccuracyDecimals:  1,
			}))
		w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_LIGHT_RESPONSE,
			marshal(&ListEntitiesLightResponse{ObjectId: "desk-light", Key: 3, Name: "Desk Light"}))
		w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_DONE_RESPONSE, nil)
	})
	server := startFakeServer(t, behavior)

	client, err := Connect(context.Background(), Options{
		Address:        server.listener.Addr().String(),
		ConnectTimeout: time.Second,
		OnState:        func(state State) { states <- state },
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close(context.Background())

	waitEntity := func(objectID string, kind EntityKind) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			for _, entity := range client.Entities() {
				if entity.ObjectID != objectID {
					continue
				}
				if entity.Kind != kind {
					t.Fatalf("entity %q kind = %v, want %v", objectID, entity.Kind, kind)
				}
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("entity %q not enumerated; have %+v", objectID, client.Entities())
	}
	waitEntity("motion", EntityBinary)
	waitEntity("temperature", EntityNumeric)
	waitEntity("desk-light", EntityLight)

	// The connection must have survived the unknown message ID frames.
	select {
	case <-client.Done():
		t.Fatalf("connection terminated early: %v", client.Err())
	default:
	}

	// One published state per domain arrives through OnState.
	server.Push(t, func(w fakeWriter) {
		binaryState, _ := (&BinarySensorStateResponse{Key: 1, State: true}).MarshalVT()
		numericState, _ := (&SensorStateResponse{Key: 2, State: 21.5}).MarshalVT()
		lightState, _ := (&LightStateResponse{Key: 3, State: true, Brightness: 0.5}).MarshalVT()
		w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_BINARY_SENSOR_STATE_RESPONSE, binaryState)
		w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_SENSOR_STATE_RESPONSE, numericState)
		w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIGHT_STATE_RESPONSE, lightState)
		w.send(EspHomeMessageId(9998), []byte{0x02})
	})
	for want := range 3 {
		select {
		case state := <-states:
			if state.Missing {
				t.Fatalf("state %+v unexpectedly missing", state)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of 3 states published", want)
		}
	}
	select {
	case <-client.Done():
		t.Fatalf("connection terminated during states: %v", client.Err())
	default:
	}
}

// TestMalformedFrameTerminatesThroughReadLoop pins that a malformed frame
// arriving mid-connection terminates the client through the real read loop
// with the frame-format failure delivered to OnError.
func TestMalformedFrameTerminatesThroughReadLoop(t *testing.T) {
	errorCh := make(chan error, 2)
	behavior := negotiateBehavior(t, func(id EspHomeMessageId, w fakeWriter) {
		switch id {
		case EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_REQUEST:
			w.send(EspHomeMessageId_ESP_HOME_MESSAGE_ID_LIST_ENTITIES_DONE_RESPONSE, nil)
		case EspHomeMessageId_ESP_HOME_MESSAGE_ID_SUBSCRIBE_STATES_REQUEST:
			// A lone invalid preamble byte: the decoder must reject this
			// through the live read loop, not a direct decoder call.
			w.raw([]byte{0xff})
		}
	})
	server := startFakeServer(t, behavior)

	client, err := Connect(context.Background(), Options{
		Address:        server.listener.Addr().String(),
		ConnectTimeout: time.Second,
		OnError:        func(err error) { errorCh <- err },
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close(context.Background())

	select {
	case <-client.Done():
		var formatErr *ErrFrameFormat
		if !errors.As(client.Err(), &formatErr) {
			t.Fatalf("Err() = %v, want an ErrFrameFormat", client.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("malformed frame did not terminate the connection")
	}

	select {
	case reported := <-errorCh:
		var formatErr *ErrFrameFormat
		if !errors.As(reported, &formatErr) {
			t.Fatalf("OnError = %v, want an ErrFrameFormat", reported)
		}
	case <-time.After(time.Second):
		t.Fatal("OnError never received the frame-format failure")
	}
}
