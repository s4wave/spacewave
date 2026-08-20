package plugin_space

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/sirupsen/logrus"
)

const sourceManifestID = "cold-plugin"

type sourceManifestController struct {
	started  chan struct{}
	released chan struct{}
}

func (c *sourceManifestController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/manifest-source", controller.MustParseVersion("0.0.1"), "serves a cold manifest")
}

func (c *sourceManifestController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *sourceManifestController) Close() error { return nil }

func (c *sourceManifestController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_manifest.FetchManifest)
	if !ok || dir.GetManifestId() != sourceManifestID {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		close(c.started)
		defer close(c.released)
		_, _ = handler.AddValue(&bldr_manifest.FetchManifestValue{ManifestRefs: []*bldr_manifest.ManifestRef{{}}})
		handler.MarkIdle(true)
		<-ctx.Done()
		return ctx.Err()
	}), nil)
}

func TestFetchManifestSourceRequiresSpaceApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	parent, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	source := &sourceManifestController{started: make(chan struct{}), released: make(chan struct{})}
	parentRef, err := parent.AddController(ctx, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parentRef()

	child, resolver, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	resolver.AddFactory(NewFactory(child, WithManifestSource(parent)))
	ctrl, _, ctrlRef, err := StartControllerWithConfig(ctx, child, &Config{EngineId: "test-engine"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRef.Release()

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	values := make(chan []*bldr_manifest.ManifestRef, 2)
	_, release, err := bus.ExecCollectValuesWatch[*bldr_manifest.FetchManifestValue](
		watchCtx,
		child,
		bldr_manifest.NewFetchManifest(sourceManifestID, nil, nil, 0),
		true,
		func(_ []error, vals []*bldr_manifest.FetchManifestValue) error {
			refs := make([]*bldr_manifest.ManifestRef, 0)
			for _, val := range vals {
				refs = append(refs, val.GetManifestRefs()...)
			}
			select {
			case values <- refs:
			default:
			}
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	select {
	case <-source.started:
		t.Fatal("unapproved manifest fetched from parent")
	default:
	}
	setSourcePluginIDs(ctrl, []string{sourceManifestID})
	for {
		select {
		case refs := <-values:
			if len(refs) == 1 {
				goto resolved
			}
		case <-ctx.Done():
			t.Fatal("approved manifest did not resolve from parent")
		}
	}

resolved:
	select {
	case <-source.started:
	case <-ctx.Done():
		t.Fatal("approved manifest did not demand the parent source")
	}

	setSourcePluginIDs(ctrl, nil)
	select {
	case <-source.released:
	case <-ctx.Done():
		t.Fatal("removing approval did not release parent manifest demand")
	}
}

func setSourcePluginIDs(c *Controller, ids []string) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.pluginIDs = ids
		broadcast()
	})
}
