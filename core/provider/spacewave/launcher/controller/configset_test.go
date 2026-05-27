package spacewave_launcher_controller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/sirupsen/logrus"
)

const launcherConfigSetTestKey = "launcher-config-test"

func TestApplyDistConfigSetSwapsAndReleasesSignedLauncherConfigSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	started := make(chan struct{}, 8)
	stopped := make(chan struct{}, 8)
	factory := &launcherConfigSetTestFactory{
		started: started,
		stopped: stopped,
	}
	le := logrus.NewEntry(logrus.New())
	b, sr, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(factory)

	_, _, configSetRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer configSetRef.Release()

	ctrl := &Controller{
		le:  le,
		bus: b,
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				DistConfig: launcherConfigSetTestDistConfig(1),
			},
		),
	}
	applyCtx, applyCancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.applyDistConfigSet(applyCtx)
	}()
	waitStartedController(t, ctx, started)
	waitLauncherConfigSetRev(t, ctx, b, 1)

	ctrl.launcherInfoCtr.SetValue(&spacewave_launcher.LauncherInfo{
		DistConfig: launcherConfigSetTestDistConfig(2),
	})
	waitLauncherConfigSetRev(t, ctx, b, 2)

	applyCancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("applyDistConfigSet() error = %v, want context.Canceled", err)
	}
	waitActiveControllerCount(t, ctx, stopped, &factory.active, 0)
}

func launcherConfigSetTestDistConfig(rev uint64) *spacewave_launcher.DistConfig {
	return &spacewave_launcher.DistConfig{
		Rev: rev,
		LauncherConfigSet: map[string]*configset_proto.ControllerConfig{
			launcherConfigSetTestKey: {
				Id:  (&config.Placeholder{}).GetConfigID(),
				Rev: rev,
			},
		},
	}
}

func waitStartedController(t *testing.T, ctx context.Context, started <-chan struct{}) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err().Error())
	case <-started:
	}
}

func waitLauncherConfigSetRev(t *testing.T, ctx context.Context, b bus.Bus, rev uint64) {
	t.Helper()
	st, _, ref, err := bus.ExecWaitValue[configset.LookupConfigSetValue](
		ctx,
		b,
		configset.NewLookupConfigSet([]string{launcherConfigSetTestKey}),
		nil,
		nil,
		func(st configset.LookupConfigSetValue) (bool, error) {
			conf := st.GetControllerConfig()
			return conf != nil && conf.GetRev() == rev && st.GetController() != nil, st.GetError()
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ref.Release()
	if st.GetControllerConfig().GetRev() != rev {
		t.Fatalf("launcher config set rev = %d, want %d", st.GetControllerConfig().GetRev(), rev)
	}
}

func waitActiveControllerCount(
	t *testing.T,
	ctx context.Context,
	stopped <-chan struct{},
	active *atomic.Int32,
	want int32,
) {
	t.Helper()
	for active.Load() != want {
		select {
		case <-ctx.Done():
			t.Fatalf("active controller count = %d, want %d: %v", active.Load(), want, ctx.Err())
		case <-stopped:
		}
	}
}

type launcherConfigSetTestFactory struct {
	started chan<- struct{}
	stopped chan<- struct{}
	active  atomic.Int32
}

func (f *launcherConfigSetTestFactory) GetConfigID() string {
	return (&config.Placeholder{}).GetConfigID()
}

func (f *launcherConfigSetTestFactory) ConstructConfig() config.Config {
	return &config.Placeholder{}
}

func (f *launcherConfigSetTestFactory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	return &launcherConfigSetTestController{factory: f}, nil
}

func (f *launcherConfigSetTestFactory) GetVersion() controller.Version {
	return controller.MustParseVersion("0.0.1")
}

type launcherConfigSetTestController struct {
	factory *launcherConfigSetTestFactory
}

func (c *launcherConfigSetTestController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		(&config.Placeholder{}).GetConfigID(),
		controller.MustParseVersion("0.0.1"),
		"launcher config set test controller",
	)
}

func (c *launcherConfigSetTestController) Execute(ctx context.Context) error {
	c.factory.active.Add(1)
	select {
	case c.factory.started <- struct{}{}:
	default:
	}
	defer func() {
		c.factory.active.Add(-1)
		select {
		case c.factory.stopped <- struct{}{}:
		default:
		}
	}()
	<-ctx.Done()
	return ctx.Err()
}

func (c *launcherConfigSetTestController) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

func (c *launcherConfigSetTestController) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Factory = ((*launcherConfigSetTestFactory)(nil))

// _ is a type assertion
var _ controller.Controller = ((*launcherConfigSetTestController)(nil))
