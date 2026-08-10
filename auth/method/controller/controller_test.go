package auth_method_controller

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	auth_method "github.com/s4wave/spacewave/auth/method"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

type testMethod struct {
	closed atomic.Int32
}

func (*testMethod) GetMethodID() string                                        { return "test" }
func (*testMethod) Execute(context.Context) error                              { return nil }
func (*testMethod) UnmarshalParameters([]byte) (auth_method.Parameters, error) { return nil, nil }
func (*testMethod) Authenticate(context.Context, auth_method.Parameters, []byte) (crypto.PrivKey, error) {
	return nil, nil
}
func (m *testMethod) Close() { m.closed.Add(1) }

func TestControllerPublishesOneMethodForItsLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	method := &testMethod{}
	var constructions atomic.Int32
	ctrl := NewController(
		logrus.NewEntry(logrus.New()),
		nil,
		func(context.Context, *logrus.Entry, auth_method.Handler) (auth_method.Method, error) {
			constructions.Add(1)
			return method, nil
		},
		"test",
		controller.MustParseVersion("0.1.0"),
	)
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()

	first, err := ctrl.GetAuthMethod(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ctrl.GetAuthMethod(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first != method || second != method {
		t.Fatal("controller published different method instances")
	}
	if constructions.Load() != 1 {
		t.Fatalf("constructed %d methods, want 1", constructions.Load())
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if method.closed.Load() != 1 {
		t.Fatalf("closed method %d times, want 1", method.closed.Load())
	}
}
