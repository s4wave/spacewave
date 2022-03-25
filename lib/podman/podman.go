package forge_lib_podman

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	podman_pod "github.com/aperturerobotics/forge/lib/podman/pod"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(podman_pod.NewFactory(b))
}
