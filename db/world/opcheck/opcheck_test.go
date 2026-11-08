package world_opcheck

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestCheckPassesPureOp checks an operation that writes only its own inputs.
func TestCheckPassesPureOp(t *testing.T) {
	Check(t, mutantCase(&mutantOp{Key: "pure/a"}))
}

// TestCheckFailsClockRead checks that an apply writing the clock fails.
func TestCheckFailsClockRead(t *testing.T) {
	if msg := checkFails(t, mutantCase(&mutantOp{Key: "clock/a", Clock: true})); msg == "" {
		t.Fatal("Check passed an operation that writes the clock")
	}
}

// TestCheckFailsNetworkCall checks that an apply calling the network fails.
func TestCheckFailsNetworkCall(t *testing.T) {
	if msg := checkFails(t, mutantCase(&mutantOp{Key: "net/a", Network: true})); msg == "" {
		t.Fatal("Check passed an operation that calls the network")
	}
}

// mutantCase returns a Case for op on an empty World.
func mutantCase(op *mutantOp) Case {
	return Case{
		Op: op,
		LookupOp: func(context.Context, string) (world.Operation, error) {
			return &mutantOp{}, nil
		},
	}
}

// checkFails runs Check on c and returns the failure it reports, or "".
func checkFails(t *testing.T, c Case) string {
	rec := &failRecorder{TB: t}
	var wg sync.WaitGroup
	wg.Go(func() { Check(rec, c) })
	wg.Wait()
	t.Logf("Check reported: %s", rec.msg)
	return rec.msg
}

// failRecorder records a Check failure instead of failing the test.
type failRecorder struct {
	testing.TB
	// msg is the recorded failure.
	msg string
}

// Fatal records the failure and stops the checking goroutine.
func (f *failRecorder) Fatal(args ...any) {
	f.msg = fmt.Sprint(args...)
	runtime.Goexit()
}

// Fatalf records the failure and stops the checking goroutine.
func (f *failRecorder) Fatalf(format string, args ...any) {
	f.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// mutantOp creates an object at Key, and with Clock set names it with the
// time, and with Network set calls the network first.
type mutantOp struct {
	// Key is the object key.
	Key string
	// Clock appends the clock to the key.
	Clock bool
	// Network calls the network.
	Network bool
}

// mutantOpTypeID is mutantOp's type.
const mutantOpTypeID = "world/opcheck/mutant"

// TypeID returns the operation type.
func (*mutantOp) TypeID() string { return mutantOpTypeID }

// MarshalVT encodes the operation.
func (o *mutantOp) MarshalVT() ([]byte, error) {
	return []byte(strconv.FormatBool(o.Clock) + "," + strconv.FormatBool(o.Network) + "," + o.Key), nil
}

// UnmarshalVT decodes the operation.
func (o *mutantOp) UnmarshalVT(data []byte) error {
	s := string(data)
	clock, rest, _ := strings.Cut(s, ",")
	network, key, _ := strings.Cut(rest, ",")
	o.Clock, o.Network, o.Key = clock == "true", network == "true", key
	return nil
}

// ApplyWorldOp applies the operation.
func (o *mutantOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	if o.Network {
		resp, err := http.Get("http://127.0.0.1:1/")
		if err == nil {
			resp.Body.Close()
		}
	}
	key := o.Key
	if o.Clock {
		key += "/" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	obj, err := ws.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	return false, err
}
