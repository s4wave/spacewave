package plugin_host_scheduler

import (
	"testing"

	bldr_frontend "github.com/s4wave/spacewave/bldr/frontend"
	"github.com/s4wave/spacewave/db/testbed"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// TestPVISProbe tries service-ID shapes the author's tests do not use.
func TestPVISProbe(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	hostSvc := "probe.HostService"
	worldSvc := sdk_world_engine.SpaceWorldServiceID
	attached := bldr_frontend.AttachedServicePrefix + "grant-1/probe.Attached"
	probeIDs := []string{
		hostSvc, attached, worldSvc,
		"/" + hostSvc, hostSvc + "/", "Probe.HostService",
		"/" + worldSvc, worldSvc + "/",
	}

	appTb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer appTb.Release()
	app := newFallbackBed(ctx, t, le, appTb.Bus, "app", &Config{HostExportPluginIds: []string{"spacewave-core"}}, probeIDs)

	spTb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer spTb.Release()
	space := newFallbackBed(ctx, t, le, spTb.Bus, "space-a", &Config{InstanceKey: "space-a", AllowedHostServiceIds: []string{worldSvc}}, probeIDs)

	shapes := []string{
		"plugin/spacewave-core/resource.ResourceService",
		"plugin/spacewave-core//resource.ResourceService",
		"plugin/spacewave-core/plugin-host/" + hostSvc,
		"plugin/spacewave-core/frontend/grant-1/probe.Attached",
		"plugin//" + hostSvc,
		"plugin/" + hostSvc,
		"plugin/../" + hostSvc,
		"PLUGIN/x/" + hostSvc,
		"plugin/Spacewave-Core/svc",
		"/" + hostSvc,
		hostSvc + "/",
		"Probe.HostService",
		attached,
		"/" + worldSvc,
		worldSvc + "/",
		"plugin-host//" + hostSvc,
		"plugin-host/plugin/spacewave-core/svc",
	}
	for _, sh := range shapes {
		t.Logf("PVIS app attacker@space-a %q: %s", sh, app.call(ctx, "attacker", "space-a", sh))
		t.Logf("PVIS app attacker@\"\" %q: %s", sh, app.call(ctx, "attacker", "", sh))
		t.Logf("PVIS space plugin@space-a %q: %s", sh, space.call(ctx, "space-plugin", "space-a", sh))
	}
}
