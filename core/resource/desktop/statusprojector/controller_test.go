package statusprojector

import (
	"testing"

	controller_api "github.com/aperturerobotics/controllerbus/controller"
	controllerbus "github.com/aperturerobotics/controllerbus/core"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/sirupsen/logrus"
)

// TestFactoryListenerStatus checks the hosted-plugin factory path and explicit
// listener injection used by a process that owns its resource socket.
func TestFactoryListenerStatus(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	b, _, err := controllerbus.NewCoreBus(t.Context(), le)
	if err != nil {
		t.Fatal(err)
	}
	shared := resource_listener.NewStatusBroker()
	for _, test := range []struct {
		name string
		opts []Option
	}{
		{name: "hosted plugin"},
		{name: "shared listener", opts: []Option{WithListenerStatusBroker(shared)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl, err := NewFactory(b, test.opts...).Construct(t.Context(), &Config{}, controller_api.ConstructOpts{Logger: le})
			if err != nil {
				t.Fatal(err)
			}
			defer ctrl.Close()
			projector := ctrl.(*Controller)
			if projector.statusBroker == nil {
				t.Fatal("hosted plugin has no listener status broker")
			}
			if len(test.opts) != 0 && projector.statusBroker != shared {
				t.Fatal("factory replaced the injected listener status broker")
			}
		})
	}
}
