package registration

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
)

// Registry owns admission of plugin generations across the Resource registries.
// Participating registries use Broadcast for their records and visibility reads,
// so activation and a registry snapshot share one critical section.
type Registry struct {
	// bcast serializes membership, activation, and participating registry records.
	bcast broadcast.Broadcast
	// active contains the admitted generation for each family and installation.
	active map[binding]*Generation
	// members associates immutable registration pointers with their generation.
	members map[any]*Generation
	// changed contains registry-specific notifications made under bcast.
	changed []func()
}

// NewRegistry constructs an independent registration admission boundary.
func NewRegistry() *Registry {
	return &Registry{
		active:  make(map[binding]*Generation),
		members: make(map[any]*Generation),
	}
}

// Broadcast returns the lock shared by generation admission and registry records.
func (r *Registry) Broadcast() *broadcast.Broadcast {
	return &r.bcast
}

// OnChange adds a notification called under Broadcast when visibility changes.
// Install notifications during construction, before serving any requests.
func (r *Registry) OnChange(changed func()) {
	r.changed = append(r.changed, changed)
}

// BindLocked associates a comparable registration pointer with its generation.
// A nil generation is a normal immediately visible registration. A generation
// accepts members while it prepares and while it is active: a member added to
// the active generation is visible at once and hides with it, so a plugin can
// register what it can serve only after starting, such as a handler that needs
// recovery first. A closed or retired generation refuses members. The caller
// holds Broadcast and must forget the member when its Resource is released.
func (r *Registry) BindLocked(member any, generation *Generation) error {
	// Refuse a generation this registry does not hold open.
	if generation == nil {
		return nil
	}
	retired := generation.activated && r.active[generation.binding] != generation
	if generation.registry != r || generation.closed || retired {
		return errors.New("plugin registration scope is closed or retired")
	}
	r.members[member] = generation
	return nil
}

// ForgetLocked removes a released registration's generation association.
// The caller holds Broadcast.
func (r *Registry) ForgetLocked(member any) {
	delete(r.members, member)
}

// GenerationLocked returns a registration's group, or nil for an ungrouped
// registration. The caller holds Broadcast.
func (r *Registry) GenerationLocked(member any) *Generation {
	return r.members[member]
}

// InstanceKeyLocked returns the plugin instance that serves a registration,
// empty for a global registration. A scoped registration's plugin runs in its
// installation, so loading its handler uses this key. The caller holds
// Broadcast.
func (r *Registry) InstanceKeyLocked(member any) string {
	if generation := r.members[member]; generation != nil {
		return generation.instanceKey
	}
	return ""
}

// priorityLocked returns zero for hidden registrations, one for a global
// registration, and two for this installation's registration. The caller holds
// Broadcast. Scoped definitions take precedence over global defaults.
func (r *Registry) priorityLocked(member any, instanceKey string) int {
	// Rank the member by its admitted generation and installation scope.
	generation := r.members[member]
	if generation == nil {
		return 1
	}
	if generation.closed || r.active[generation.binding] != generation {
		return 0
	}
	if generation.instanceKey == "" {
		return 1
	}
	if generation.instanceKey == instanceKey {
		return 2
	}
	return 0
}

// SelectLocked selects visible registrations by semantic name for one logical
// installation. Scoped definitions override global defaults. The latest ID wins
// within one priority. Callers hold Broadcast and clone mutable wire results.
func SelectLocked[T comparable, K comparable](r *Registry, members map[uint32]T, instanceKey string, name func(T) K) []T {
	// Select the highest-priority visible registration for each semantic name.
	type candidate struct {
		member   T
		priority int
		id       uint32
	}
	selected := make(map[K]candidate)
	for id, member := range members {
		priority := r.priorityLocked(member, instanceKey)
		if priority == 0 {
			continue
		}
		key := name(member)
		current := selected[key]
		if priority > current.priority || priority == current.priority && id > current.id {
			selected[key] = candidate{member: member, priority: priority, id: id}
		}
	}

	// Collect the selected registrations for the installation.
	result := make([]T, 0, len(selected))
	for _, current := range selected {
		result = append(result, current.member)
	}
	return result
}

// CanShareNameLocked permits independent installation scopes and private
// replacement candidates of the same family. It rejects duplicates within one
// generation and competing families within one scope. The caller holds Broadcast.
func (r *Registry) CanShareNameLocked(member any, candidate *Generation) bool {
	// Compare installation scopes before checking a replacement within one family.
	previous := r.members[member]
	var previousScope, candidateScope string
	if previous != nil {
		previousScope = previous.instanceKey
	}
	if candidate != nil {
		candidateScope = candidate.instanceKey
	}
	if previousScope != candidateScope {
		return true
	}
	return previous != nil && candidate != nil && previous != candidate &&
		previous.registry == r && candidate.registry == r && previous.pluginID == candidate.pluginID
}

// Register installs preparation on root. Private scopes forward the existing
// registry services to root with their generation bound to the call context.
func (r *Registry) Register(root srpc.Mux) error {
	return sdk_registration.SRPCRegisterRegistrationService(root, &registrationService{registry: r, root: root})
}

// registrationService prepares scopes beneath a caller's existing root Resource.
type registrationService struct {
	registry *Registry
	root     srpc.Invoker
}

// Prepare constructs a private generation with the caller's Resource lifetime.
func (s *registrationService) Prepare(ctx context.Context, req *sdk_registration.PrepareRequest) (*sdk_registration.PrepareResponse, error) {
	// Resolve the existing caller capability before creating a registration scope.
	if req.GetPluginId() == "" || req.GetManifestRoot() == "" {
		return nil, errors.New("plugin generation requires a family and immutable manifest")
	}
	if err := checkPrepareCaller(ctx, req); err != nil {
		return nil, err
	}
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Releasing the scope hides its complete generation. Member Resources
	// remain independently owned by the same client and release their
	// registry records.
	generation := &Generation{
		registry:     s.registry,
		binding:      binding{pluginID: req.GetPluginId(), instanceKey: req.GetInstanceKey()},
		manifestRoot: req.GetManifestRoot(),
		root:         s.root,
	}
	generation.mux = srpc.NewMux()
	if err := sdk_registration.SRPCRegisterGenerationService(generation.mux, generation); err != nil {
		return nil, err
	}
	id, err := client.AddResource(generation, generation.Close)
	if err != nil {
		return nil, err
	}
	return &sdk_registration.PrepareResponse{ResourceId: id}, nil
}

// checkPrepareCaller refuses a Prepare from a plugin that requests a binding
// other than its own. The host stamps each plugin's calls with the plugin ID
// and binding key it bound when it started the plugin, and the registration SDK
// sends exactly those, so a mismatch is a forged or broken request. Refusing
// the request, rather than registering under the stamped binding, keeps a
// plugin from silently publishing into a scope it did not ask for. A call with
// no stamp comes from the host itself, which may select any binding.
func checkPrepareCaller(ctx context.Context, req *sdk_registration.PrepareRequest) error {
	caller := bldr_plugin.PluginCallerFromContext(ctx)
	if caller == nil {
		return nil
	}
	if req.GetPluginId() != caller.GetPluginId() || req.GetInstanceKey() != caller.GetBindingKey() {
		return errors.Errorf(
			"plugin %q may not prepare a generation for plugin %q binding %q",
			caller.GetPluginId(), req.GetPluginId(), req.GetInstanceKey(),
		)
	}
	return nil
}
