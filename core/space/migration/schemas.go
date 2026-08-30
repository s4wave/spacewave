package space_migration

import (
	"context"
	"path"
	"strings"

	"github.com/pkg/errors"
	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
)

func inspectSpaceSettings(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	settings, err := world.LookupObjectBody[*space_world.SpaceSettings](ctx, object.World, object.ObjectKey, space_world.NewSpaceSettingsBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode SpaceSettings payload")
	}
	out := &Inspection{}
	if settings == nil {
		return out, nil
	}
	indexPath := strings.TrimPrefix(path.Clean("/"+settings.GetIndexPath()), "/")
	if after, ok := strings.CutPrefix(indexPath, "-/"); ok {
		indexPath = after
	}
	if objectKey, _, ok := strings.Cut(indexPath, "/-/"); ok {
		indexPath = objectKey
	} else {
		indexPath = strings.TrimSuffix(indexPath, "/-")
	}
	if indexPath != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: indexPath})
	}
	return out, nil
}

func inspectCanvas(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	state, err := s4wave_canvas.LookupCanvasState(ctx, object.World, object.ObjectKey)
	if err != nil {
		return nil, errors.Wrap(err, "decode Canvas payload")
	}
	return inspectCanvasState(ctx, object, state)
}

func inspectObjectLayout(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	layoutObject, err := world.LookupObjectBody[*s4wave_layout_world.ObjectLayout](ctx, object.World, object.ObjectKey, s4wave_layout_world.NewObjectLayoutBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode ObjectLayout payload")
	}
	return inspectObjectLayoutPayload(layoutObject)
}

func inspectLayoutRow(row *s4wave_layout.RowDef, out *Inspection) error {
	if row == nil {
		return nil
	}
	for _, child := range row.GetChildren() {
		if child == nil {
			continue
		}
		if nested := child.GetRow(); nested != nil {
			if err := inspectLayoutRow(nested, out); err != nil {
				return err
			}
		}
		if tabs := child.GetTabSet(); tabs != nil {
			if err := inspectLayoutTabs(tabs.GetChildren(), out); err != nil {
				return err
			}
		}
	}
	return nil
}

func inspectLayoutTabs(tabs []*s4wave_layout.TabDef, out *Inspection) error {
	for _, tab := range tabs {
		if tab == nil || len(tab.GetData()) == 0 {
			continue
		}
		payload := s4wave_layout_world.NewObjectLayoutTab("", nil, "")
		if err := payload.UnmarshalVT(tab.GetData()); err != nil {
			return errors.Wrap(err, "decode ObjectLayout tab payload")
		}
		if info := payload.GetObjectInfo(); info != nil {
			if worldInfo := info.GetWorldObjectInfo(); worldInfo != nil && worldInfo.GetObjectKey() != "" {
				out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: worldInfo.GetObjectKey()})
			}
		}
	}
	return nil
}

func inspectSecret(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	secret, err := world.LookupObjectBody[*s4wave_secret.Secret](ctx, object.World, object.ObjectKey, s4wave_secret.NewSecretBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode Secret envelope")
	}
	return inspectSecretPayload(secret)
}

func inspectDevice(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	device, err := world.LookupObjectBody[*s4wave_device.Device](ctx, object.World, object.ObjectKey, s4wave_device.NewDeviceBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode Device payload")
	}
	return inspectDevicePayload(device)
}

func inspectTerminal(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	terminal, err := world.LookupObjectBody[*s4wave_terminal.Terminal](ctx, object.World, object.ObjectKey, s4wave_terminal.NewTerminalBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode Terminal payload")
	}
	return inspectTerminalPayload(terminal)
}

func inspectSSHHost(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	host, err := world.LookupObjectBody[*s4wave_sshhost.SshHost](ctx, object.World, object.ObjectKey, s4wave_sshhost.NewSshHostBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode SSH Host payload")
	}
	return inspectSSHHostPayload(host)
}

func inspectChatMessage(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	message, err := world.LookupObjectBody[*s4wave_chat.ChatMessage](ctx, object.World, object.ObjectKey, s4wave_chat.NewChatMessageBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode chat message payload")
	}
	return inspectChatMessagePayload(message)
}

func inspectCanvasState(ctx context.Context, object *ObjectDescriptor, state *s4wave_canvas.CanvasState) (*Inspection, error) {
	out := &Inspection{}
	if state == nil {
		return out, nil
	}
	for id, node := range state.GetNodes() {
		if id != "" {
			out.References = append(out.References, TypedReference{Kind: ReferenceCanvasNode, Value: id})
		}
		if node != nil && node.GetObjectKey() != "" {
			out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: node.GetObjectKey()})
		}
	}
	if object == nil || object.World == nil {
		return nil, errors.New("Canvas graph references require a live World")
	}
	for _, link := range state.GetHiddenGraphLinks() {
		if link == nil {
			continue
		}
		for index, value := range []string{link.GetSubject(), link.GetPredicate(), link.GetObject(), link.GetLabel()} {
			if value == "" {
				continue
			}
			out.References = append(out.References, TypedReference{Kind: ReferenceGraphIRI, Value: value})
			if index == 1 {
				continue
			}
			key := strings.Trim(value, "<>")
			if key == "" {
				continue
			}
			objectState, found, err := object.World.GetObject(ctx, key)
			if err != nil {
				return nil, errors.Wrapf(err, "resolve Canvas graph reference %s", value)
			}
			if found {
				world.ReleaseObjectState(objectState)
				out.Dependencies = append(out.Dependencies, key)
			}
		}
	}
	return out, nil
}

func inspectObjectLayoutPayload(layoutObject *s4wave_layout_world.ObjectLayout) (*Inspection, error) {
	out := &Inspection{}
	if layoutObject == nil || layoutObject.GetLayoutModel() == nil {
		return out, nil
	}
	model := layoutObject.GetLayoutModel()
	for _, border := range model.GetBorders() {
		if border != nil {
			if err := inspectLayoutTabs(border.GetChildren(), out); err != nil {
				return nil, err
			}
		}
	}
	if err := inspectLayoutRow(model.GetLayout(), out); err != nil {
		return nil, err
	}
	return out, nil
}

func inspectSecretPayload(secret *s4wave_secret.Secret) (*Inspection, error) {
	out := &Inspection{}
	if secret == nil {
		return out, nil
	}
	if secret.GetNestedSharedObjectId() != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceNestedSharedObject, Value: secret.GetNestedSharedObjectId()})
	}
	if ref := secret.GetRef(); ref != nil {
		if ref.GetBlockStoreId() == "" {
			return nil, errors.New("Secret envelope has an empty block-store identity")
		}
		out.References = append(out.References, TypedReference{Kind: ReferenceBlockStore, Value: ref.GetBlockStoreId()})
	} else if secret.GetNestedSharedObjectId() != "" {
		return nil, errors.New("Secret envelope has no SharedObject block-store reference")
	}
	return out, nil
}

func inspectDevicePayload(device *s4wave_device.Device) (*Inspection, error) {
	out := &Inspection{}
	if device == nil {
		return out, nil
	}
	if device.GetPeerId() != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceExternal, Value: device.GetPeerId()})
	}
	for _, capability := range device.GetCapabilities() {
		if capability == nil {
			continue
		}
		if link := capability.GetLink(); link != nil {
			if link.GetObjectKey() != "" {
				out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: link.GetObjectKey()})
			}
			if link.GetProtocolId() != "" {
				out.References = append(out.References, TypedReference{Kind: ReferenceExternal, Value: link.GetProtocolId()})
			}
		}
	}
	return out, nil
}

func inspectTerminalPayload(terminal *s4wave_terminal.Terminal) (*Inspection, error) {
	out := &Inspection{}
	if terminal == nil {
		return out, nil
	}
	if terminal.GetDeviceObjectKey() != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: terminal.GetDeviceObjectKey()})
	}
	if terminal.GetSshHostObjectKey() != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: terminal.GetSshHostObjectKey()})
	}
	if terminal.GetDevicePeerId() != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceExternal, Value: terminal.GetDevicePeerId()})
	}
	return out, nil
}

func inspectSSHHostPayload(host *s4wave_sshhost.SshHost) (*Inspection, error) {
	out := &Inspection{}
	if host == nil {
		return out, nil
	}
	if endpoint := host.GetEndpoint(); endpoint != nil && endpoint.GetHost() != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceExternal, Value: endpoint.GetHost()})
	}
	if creds := host.GetCredentials(); creds != nil {
		for _, key := range []string{creds.GetPrivateKeySecretObjectKey(), creds.GetPasswordSecretObjectKey(), creds.GetPassphraseSecretObjectKey()} {
			if key != "" {
				out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: key})
			}
		}
	}
	for _, pin := range host.GetHostKeyPins() {
		if pin != nil && pin.GetAcceptedByPeerId() != "" {
			out.References = append(out.References, TypedReference{Kind: ReferenceExternal, Value: pin.GetAcceptedByPeerId()})
		}
	}
	return out, nil
}

func inspectChatMessagePayload(message *s4wave_chat.ChatMessage) (*Inspection, error) {
	out := &Inspection{}
	if message == nil {
		return out, nil
	}
	if message.GetReplyToKey() != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: message.GetReplyToKey()})
	}
	if message.GetSenderPeerId() != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceExternal, Value: message.GetSenderPeerId()})
	}
	return out, nil
}

func inspectPayload[T block.Block](ctx context.Context, object *ObjectDescriptor, ctor block.Ctor, label string) (*Inspection, error) {
	_, err := world.LookupObjectBody[T](ctx, object.World, object.ObjectKey, ctor)
	if err != nil {
		return nil, errors.Wrap(err, "decode "+label+" payload")
	}
	return &Inspection{}, nil
}

func validateDecodedPayload(payload any, label string) error {
	if payload == nil {
		return errors.Wrapf(ErrPayloadSchemaRefused, "%s payload is missing", label)
	}
	if validator, ok := payload.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return errors.Wrap(err, "validate "+label+" payload")
		}
	}
	if validator, ok := payload.(interface{ Validate(bool) error }); ok {
		if err := validator.Validate(false); err != nil {
			return errors.Wrap(err, "validate "+label+" payload")
		}
	}
	return nil
}

func rewritePayload[T block.Block](ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap, ctor block.Ctor, label string) (*RewriteResult, error) {
	payload, err := world.LookupObjectBody[T](ctx, object.World, object.ObjectKey, ctor)
	if err != nil {
		return nil, errors.Wrap(err, "decode "+label+" payload")
	}
	if any(payload) == nil {
		return nil, errors.Wrapf(ErrPayloadSchemaRefused, "%s payload is missing", label)
	}
	data, err := payload.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal "+label+" payload")
	}
	return &RewriteResult{Payload: data, GraphReferences: mappedGraphReferences(object, mapping)}, nil
}

func mappedGraphReferences(object *ObjectDescriptor, mapping *IdentityMap) []string {
	if object == nil {
		return nil
	}
	out := make([]string, 0, len(object.GraphReferences))
	for index, value := range object.GraphReferences {
		if value == "" || index%4 == 1 {
			out = append(out, value)
			continue
		}
		out = append(out, mapGraphReference(mapping, value))
	}
	return out
}

func mapGraphReference(mapping *IdentityMap, value string) string {
	if value == "" || mapping == nil {
		return value
	}
	if mapped := mapping.MapGraphIRI(value); mapped != value {
		return mapped
	}
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		if mapped := mapping.MapGraphIRI(strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">")); mapped != strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">") {
			return "<" + mapped + ">"
		}
	}
	return value
}

func remapObjectKey(mapping *IdentityMap, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if mapping == nil || mapping.ObjectKeys[value] == "" {
		return "", errors.Errorf("unmapped Space-local object key %s", value)
	}
	return mapping.ObjectKeys[value], nil
}

func remapObjectPath(mapping *IdentityMap, value string) (string, error) {
	if value == "" {
		return value, nil
	}
	trimmed := strings.TrimPrefix(value, "/")
	parts := strings.SplitN(trimmed, "/-/", 2)
	key, err := remapObjectKey(mapping, parts[0])
	if err != nil {
		return "", err
	}
	if len(parts) == 1 {
		return "/" + key, nil
	}
	return "/" + key + "/-/" + parts[1], nil
}

func rewriteSpaceSettings(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	settings, err := world.LookupObjectBody[*space_world.SpaceSettings](ctx, object.World, object.ObjectKey, space_world.NewSpaceSettingsBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode SpaceSettings payload")
	}
	if settings == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "SpaceSettings payload is missing")
	}
	if settings.GetIndexPath() != "" {
		mapped, err := remapObjectPath(mapping, settings.GetIndexPath())
		if err != nil {
			return nil, err
		}
		settings.IndexPath = mapped
	}
	data, err := settings.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal SpaceSettings payload")
	}
	return &RewriteResult{Payload: data}, nil
}

func rewriteCanvas(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	state, err := s4wave_canvas.LookupCanvasState(ctx, object.World, object.ObjectKey)
	if err != nil {
		return nil, errors.Wrap(err, "decode Canvas payload")
	}
	if state == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "Canvas payload is missing")
	}
	for _, node := range state.GetNodes() {
		if node == nil || node.GetObjectKey() == "" {
			continue
		}
		mapped, err := remapObjectKey(mapping, node.GetObjectKey())
		if err != nil {
			return nil, err
		}
		node.ObjectKey = mapped
	}

	if len(mapping.CanvasNodes) != 0 {
		nodes := make(map[string]*s4wave_canvas.CanvasNode, len(state.GetNodes()))
		for id, node := range state.GetNodes() {
			mappedID := id
			if mapped := mapping.CanvasNodes[id]; mapped != "" {
				mappedID = mapped
			}
			if node != nil {
				node.Id = mappedID
			}
			nodes[mappedID] = node
		}
		state.Nodes = nodes
		if len(state.GetLayoutMetadata()) != 0 {
			metadata := make(map[string]*s4wave_canvas.CanvasLayoutMetadata, len(state.GetLayoutMetadata()))
			for id, value := range state.GetLayoutMetadata() {
				mappedID := id
				if mapped := mapping.CanvasNodes[id]; mapped != "" {
					mappedID = mapped
				}
				metadata[mappedID] = value
			}
			state.LayoutMetadata = metadata
		}
		for _, edge := range state.GetEdges() {
			if edge == nil {
				continue
			}
			if mapped := mapping.CanvasNodes[edge.GetSourceNodeId()]; mapped != "" {
				edge.SourceNodeId = mapped
			}
			if mapped := mapping.CanvasNodes[edge.GetTargetNodeId()]; mapped != "" {
				edge.TargetNodeId = mapped
			}
		}
	}
	for _, link := range state.GetHiddenGraphLinks() {
		if link == nil {
			continue
		}
		link.Subject = mapGraphReference(mapping, link.GetSubject())
		link.Object = mapGraphReference(mapping, link.GetObject())
		link.Label = mapGraphReference(mapping, link.GetLabel())
	}
	// Migration rewrites use the logical CanvasState transport payload. Durable
	// Canvas objects use CanvasStorage and must be created through WriteCanvasState.
	data, err := state.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal Canvas payload")
	}
	inspection, err := inspectCanvasState(ctx, object, state)
	if err != nil {
		return nil, err
	}
	return &RewriteResult{Payload: data, References: inspection.References, GraphReferences: mappedGraphReferences(object, mapping)}, nil
}

func rewriteObjectLayout(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	layoutObject, err := world.LookupObjectBody[*s4wave_layout_world.ObjectLayout](ctx, object.World, object.ObjectKey, s4wave_layout_world.NewObjectLayoutBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode ObjectLayout payload")
	}
	if layoutObject == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "ObjectLayout payload is missing")
	}
	model := layoutObject.GetLayoutModel()
	if model != nil {
		for _, border := range model.GetBorders() {
			if border != nil {
				if err := rewriteLayoutTabs(border.GetChildren(), mapping); err != nil {
					return nil, err
				}
			}
		}
		if err := rewriteLayoutRow(model.GetLayout(), mapping); err != nil {
			return nil, err
		}
	}
	data, err := layoutObject.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal ObjectLayout payload")
	}
	inspection, err := inspectObjectLayoutPayload(layoutObject)
	if err != nil {
		return nil, err
	}
	return &RewriteResult{Payload: data, References: inspection.References}, nil
}

func rewriteLayoutRow(row *s4wave_layout.RowDef, mapping *IdentityMap) error {
	if row == nil {
		return nil
	}
	for _, child := range row.GetChildren() {
		if child == nil {
			continue
		}
		if nested := child.GetRow(); nested != nil {
			if err := rewriteLayoutRow(nested, mapping); err != nil {
				return err
			}
		}
		if tabs := child.GetTabSet(); tabs != nil {
			if err := rewriteLayoutTabs(tabs.GetChildren(), mapping); err != nil {
				return err
			}
		}
	}
	return nil
}

func rewriteLayoutTabs(tabs []*s4wave_layout.TabDef, mapping *IdentityMap) error {
	for _, tab := range tabs {
		if tab == nil || len(tab.GetData()) == 0 {
			continue
		}
		payload := s4wave_layout_world.NewObjectLayoutTab("", nil, "")
		if err := payload.UnmarshalVT(tab.GetData()); err != nil {
			return errors.Wrap(err, "decode ObjectLayout tab payload")
		}
		if info := payload.GetObjectInfo(); info != nil {
			if worldInfo := info.GetWorldObjectInfo(); worldInfo != nil && worldInfo.GetObjectKey() != "" {
				mapped, err := remapObjectKey(mapping, worldInfo.GetObjectKey())
				if err != nil {
					return err
				}
				worldInfo.ObjectKey = mapped
			}
		}
		data, err := payload.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "marshal ObjectLayout tab payload")
		}
		tab.Data = data
	}
	return nil
}

func rewriteSecret(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	secret, err := world.LookupObjectBody[*s4wave_secret.Secret](ctx, object.World, object.ObjectKey, s4wave_secret.NewSecretBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode Secret envelope")
	}
	if secret == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "Secret envelope is missing")
	}
	if secret.NestedSharedObjectId != "" {
		mapped := mapping.NestedSharedObjects[secret.NestedSharedObjectId]
		if mapped == "" {
			return nil, errors.Errorf("unmapped nested SharedObject identity %s", secret.NestedSharedObjectId)
		}
		secret.NestedSharedObjectId = mapped
	}
	if secret.Ref != nil {
		mapped := mapping.BlockStoreIDs[secret.Ref.GetBlockStoreId()]
		if mapped == "" {
			return nil, errors.Errorf("unmapped block-store identity %s", secret.Ref.GetBlockStoreId())
		}
		secret.Ref.BlockStoreId = mapped
	}
	data, err := secret.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal Secret envelope")
	}
	inspection, err := inspectSecretPayload(secret)
	if err != nil {
		return nil, err
	}
	return &RewriteResult{Payload: data, References: inspection.References}, nil
}

func rewriteDevice(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	device, err := world.LookupObjectBody[*s4wave_device.Device](ctx, object.World, object.ObjectKey, s4wave_device.NewDeviceBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode Device payload")
	}
	if device == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "Device payload is missing")
	}
	for _, capability := range device.GetCapabilities() {
		if capability == nil || capability.GetLink() == nil || capability.GetLink().GetObjectKey() == "" {
			continue
		}
		mapped, err := remapObjectKey(mapping, capability.GetLink().GetObjectKey())
		if err != nil {
			return nil, err
		}
		capability.Link.ObjectKey = mapped
	}
	data, err := device.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal Device payload")
	}
	inspection, err := inspectDevicePayload(device)
	if err != nil {
		return nil, err
	}
	return &RewriteResult{Payload: data, References: inspection.References}, nil
}

func rewriteTerminal(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	terminal, err := world.LookupObjectBody[*s4wave_terminal.Terminal](ctx, object.World, object.ObjectKey, s4wave_terminal.NewTerminalBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode Terminal payload")
	}
	if terminal == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "Terminal payload is missing")
	}
	if terminal.DeviceObjectKey != "" {
		terminal.DeviceObjectKey, err = remapObjectKey(mapping, terminal.DeviceObjectKey)
		if err != nil {
			return nil, err
		}
	}
	if terminal.SshHostObjectKey != "" {
		terminal.SshHostObjectKey, err = remapObjectKey(mapping, terminal.SshHostObjectKey)
		if err != nil {
			return nil, err
		}
	}
	data, err := terminal.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal Terminal payload")
	}
	inspection, err := inspectTerminalPayload(terminal)
	if err != nil {
		return nil, err
	}
	return &RewriteResult{Payload: data, References: inspection.References}, nil
}

func rewriteSSHHost(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	host, err := world.LookupObjectBody[*s4wave_sshhost.SshHost](ctx, object.World, object.ObjectKey, s4wave_sshhost.NewSshHostBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode SSH Host payload")
	}
	if host == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "SSH Host payload is missing")
	}
	if creds := host.GetCredentials(); creds != nil {
		for _, key := range []*string{&creds.PrivateKeySecretObjectKey, &creds.PasswordSecretObjectKey, &creds.PassphraseSecretObjectKey} {
			if *key == "" {
				continue
			}
			mapped, err := remapObjectKey(mapping, *key)
			if err != nil {
				return nil, err
			}
			*key = mapped
		}
	}
	data, err := host.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal SSH Host payload")
	}
	inspection, err := inspectSSHHostPayload(host)
	if err != nil {
		return nil, err
	}
	return &RewriteResult{Payload: data, References: inspection.References}, nil
}

func rewriteChatMessage(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	message, err := world.LookupObjectBody[*s4wave_chat.ChatMessage](ctx, object.World, object.ObjectKey, s4wave_chat.NewChatMessageBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode chat message payload")
	}
	if message == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "chat message payload is missing")
	}
	if message.ReplyToKey != "" {
		message.ReplyToKey, err = remapObjectKey(mapping, message.ReplyToKey)
		if err != nil {
			return nil, err
		}
	}
	data, err := message.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal chat message payload")
	}
	inspection, err := inspectChatMessagePayload(message)
	if err != nil {
		return nil, err
	}
	return &RewriteResult{Payload: data, References: inspection.References}, nil
}

func decodeForgePayload[T block.Block](ctx context.Context, object *ObjectDescriptor, ctor block.Ctor, label string) (T, error) {
	payload, err := world.LookupObjectBody[T](ctx, object.World, object.ObjectKey, ctor)
	if err != nil {
		return payload, errors.Wrap(err, "decode "+label+" payload")
	}
	if any(payload) == nil {
		return payload, errors.Wrapf(ErrPayloadSchemaRefused, "%s payload is missing", label)
	}
	return payload, nil
}

func inspectForgePayload[T block.Block](ctx context.Context, object *ObjectDescriptor, ctor block.Ctor, label string, inspect func(T, *Inspection) error) (*Inspection, error) {
	payload, err := decodeForgePayload[T](ctx, object, ctor, label)
	if err != nil {
		return nil, err
	}
	out := &Inspection{}
	if inspect != nil {
		if err := inspect(payload, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rewriteForgePayload[T block.Block](ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap, ctor block.Ctor, label string, rewrite func(T, *IdentityMap) error, inspect func(T, *Inspection) error) (*RewriteResult, error) {
	payload, err := decodeForgePayload[T](ctx, object, ctor, label)
	if err != nil {
		return nil, err
	}
	if rewrite != nil {
		if err := rewrite(payload, mapping); err != nil {
			return nil, err
		}
	}
	data, err := payload.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal "+label+" payload")
	}
	out := &Inspection{}
	if inspect != nil {
		if err := inspect(payload, out); err != nil {
			return nil, err
		}
	}
	return &RewriteResult{Payload: data, References: out.References, GraphReferences: mappedGraphReferences(object, mapping)}, nil
}

func appendForgeObjectKey(out *Inspection, value string) {
	if value != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: value})
	}
}

func appendForgeBlockStore(out *Inspection, value string) {
	if value != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceBlockStore, Value: value})
	}
}

func appendForgeExternal(out *Inspection, value string) {
	if value != "" {
		out.References = append(out.References, TypedReference{Kind: ReferenceExternal, Value: value})
	}
}

func inspectForgeValueSet(out *Inspection, set *forge_target.ValueSet) {
	if set == nil {
		return
	}
	inspectForgeValues(out, set.GetInputs())
	inspectForgeValues(out, set.GetOutputs())
}

func inspectForgeValues(out *Inspection, values []*forge_value.Value) {
	for _, value := range values {
		if value == nil {
			continue
		}
		if ref := value.GetBucketRef(); ref != nil {
			appendForgeBlockStore(out, ref.GetBucketId())
		}
		if snapshot := value.GetWorldObjectSnapshot(); snapshot != nil {
			appendForgeObjectKey(out, snapshot.GetKey())
			appendForgeObjectKey(out, snapshot.GetObjectParent())
			if root := snapshot.GetRootRef(); root != nil {
				appendForgeBlockStore(out, root.GetBucketId())
			}
		}
	}
}

func remapForgeBlockStoreID(mapping *IdentityMap, value *string) error {
	if value == nil || *value == "" {
		return nil
	}
	if mapping == nil || mapping.BlockStoreIDs[*value] == "" {
		return errors.Errorf("unmapped block-store identity %s", *value)
	}
	*value = mapping.BlockStoreIDs[*value]
	return nil
}

func rewriteForgeValueSet(set *forge_target.ValueSet, mapping *IdentityMap) error {
	if set == nil {
		return nil
	}
	if err := rewriteForgeValues(set.GetInputs(), mapping); err != nil {
		return err
	}
	return rewriteForgeValues(set.GetOutputs(), mapping)
}

func rewriteForgeValues(values []*forge_value.Value, mapping *IdentityMap) error {
	for _, value := range values {
		if value == nil {
			continue
		}
		if ref := value.GetBucketRef(); ref != nil {
			if err := remapForgeBlockStoreID(mapping, &ref.BucketId); err != nil {
				return err
			}
		}
		if snapshot := value.GetWorldObjectSnapshot(); snapshot != nil {
			if snapshot.GetKey() != "" {
				mapped, err := remapObjectKey(mapping, snapshot.GetKey())
				if err != nil {
					return err
				}
				snapshot.Key = mapped
			}
			if snapshot.GetObjectParent() != "" {
				mapped, err := remapObjectKey(mapping, snapshot.GetObjectParent())
				if err != nil {
					return err
				}
				snapshot.ObjectParent = mapped
			}
			if root := snapshot.GetRootRef(); root != nil {
				if err := remapForgeBlockStoreID(mapping, &root.BucketId); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func decodeKV(ctx context.Context, object *ObjectDescriptor) (*kvtx_block.KeyValueStore, error) {
	payload, err := world.LookupObjectBody[*kvtx_block.KeyValueStore](ctx, object.World, object.ObjectKey, kvtx_block.NewKeyValueStoreBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode KV store payload")
	}
	if err := validateDecodedPayload(payload, "KV store"); err != nil {
		return nil, err
	}
	switch payload.GetImplType() {
	case kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL, kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA:
		// Both implementations permit an empty root for a newly initialized
		// store; the decoded enum still proves this is a real KV payload.
	default:
		return nil, errors.New("KV store payload has unknown implementation")
	}
	return payload, nil
}

func inspectKV(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	_, err := decodeKV(ctx, object)
	return &Inspection{}, err
}

func rewriteKV(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	payload, err := decodeKV(ctx, object)
	if err != nil {
		return nil, err
	}
	data, err := payload.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal KV store payload")
	}
	return &RewriteResult{Payload: data, GraphReferences: mappedGraphReferences(object, mapping)}, nil
}

func inspectChatChannel(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectPayload[*s4wave_chat.ChatChannel](ctx, object, s4wave_chat.NewChatChannelBlock, "chat channel")
}

func rewriteChatChannel(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewritePayload[*s4wave_chat.ChatChannel](ctx, object, mapping, s4wave_chat.NewChatChannelBlock, "chat channel")
}

func inspectUnixFS(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectPayload[*unixfs_block.FSNode](ctx, object, unixfs_block.NewFSNodeBlock, "UnixFS")
}

func rewriteUnixFS(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewritePayload[*unixfs_block.FSNode](ctx, object, mapping, unixfs_block.NewFSNodeBlock, "UnixFS")
}

func inspectGitRepo(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectPayload[*git_block.Repo](ctx, object, git_block.NewRepoBlock, "Git repository")
}

func rewriteGitRepo(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewritePayload[*git_block.Repo](ctx, object, mapping, git_block.NewRepoBlock, "Git repository")
}

func inspectGitWorktree(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	_, err := world.LookupObjectBody[*git_world.Worktree](ctx, object.World, object.ObjectKey, git_world.NewWorktreeBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode Git worktree payload")
	}
	out := &Inspection{}
	if object == nil {
		return out, nil
	}
	for index := 0; index+3 < len(object.GraphReferences); index += 4 {
		predicate := object.GraphReferences[index+1]
		if predicate != git_world.GitRepoPred && predicate != git_world.GitWorktreeWorkdirPred {
			continue
		}
		key := strings.Trim(object.GraphReferences[index+2], "<>")
		if key != "" {
			out.References = append(out.References, TypedReference{Kind: ReferenceObjectKey, Value: key})
		}
	}
	return out, nil
}

func rewriteGitWorktree(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	payload, err := world.LookupObjectBody[*git_world.Worktree](ctx, object.World, object.ObjectKey, git_world.NewWorktreeBlock)
	if err != nil {
		return nil, errors.Wrap(err, "decode Git worktree payload")
	}
	if payload == nil {
		return nil, errors.Wrap(ErrPayloadSchemaRefused, "Git worktree payload is missing")
	}
	data, err := payload.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal Git worktree payload")
	}
	inspection, err := inspectGitWorktree(ctx, object)
	if err != nil {
		return nil, err
	}
	references := make([]TypedReference, 0, len(inspection.References))
	for _, reference := range inspection.References {
		mapped := reference.Value
		if reference.Kind == ReferenceObjectKey {
			mapped, err = remapObjectKey(mapping, reference.Value)
			if err != nil {
				return nil, err
			}
		}
		references = append(references, TypedReference{Kind: reference.Kind, Value: mapped})
	}
	return &RewriteResult{Payload: data, References: references, GraphReferences: mappedGraphReferences(object, mapping)}, nil
}

func inspectForgeClusterFields(payload *forge_cluster.Cluster, out *Inspection) error {
	appendForgeExternal(out, payload.GetPeerId())
	return nil
}

func inspectForgeJobFields(*forge_job.Job, *Inspection) error {
	return nil
}

func inspectForgeTaskFields(payload *forge_task.Task, out *Inspection) error {
	appendForgeExternal(out, payload.GetPeerId())
	inspectForgeValueSet(out, payload.GetValueSet())
	return nil
}

func inspectForgePassFields(payload *forge_pass.Pass, out *Inspection) error {
	appendForgeExternal(out, payload.GetPeerId())
	inspectForgeValueSet(out, payload.GetValueSet())
	for _, execution := range payload.GetExecStates() {
		if execution == nil {
			continue
		}
		appendForgeObjectKey(out, execution.GetObjectKey())
		appendForgeExternal(out, execution.GetPeerId())
		inspectForgeValueSet(out, execution.GetValueSet())
	}
	return nil
}

func inspectForgeExecutionFields(payload *forge_execution.Execution, out *Inspection) error {
	appendForgeExternal(out, payload.GetPeerId())
	inspectForgeValueSet(out, payload.GetValueSet())
	return nil
}

func inspectForgeWorkerFields(*forge_worker.Worker, *Inspection) error {
	return nil
}

func inspectForgeDashboardFields(*forge_dashboard.ForgeDashboard, *Inspection) error {
	return nil
}

func rewriteForgeTaskFields(payload *forge_task.Task, mapping *IdentityMap) error {
	return rewriteForgeValueSet(payload.GetValueSet(), mapping)
}

func rewriteForgePassFields(payload *forge_pass.Pass, mapping *IdentityMap) error {
	if err := rewriteForgeValueSet(payload.GetValueSet(), mapping); err != nil {
		return err
	}
	for _, execution := range payload.GetExecStates() {
		if execution == nil {
			continue
		}
		if execution.GetObjectKey() != "" {
			mapped, err := remapObjectKey(mapping, execution.GetObjectKey())
			if err != nil {
				return err
			}
			execution.ObjectKey = mapped
		}
		if err := rewriteForgeValueSet(execution.GetValueSet(), mapping); err != nil {
			return err
		}
	}
	return nil
}

func rewriteForgeExecutionFields(payload *forge_execution.Execution, mapping *IdentityMap) error {
	return rewriteForgeValueSet(payload.GetValueSet(), mapping)
}

func inspectForgeCluster(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectForgePayload[*forge_cluster.Cluster](ctx, object, forge_cluster.NewClusterBlock, "Forge cluster", inspectForgeClusterFields)
}

func inspectForgeJob(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectForgePayload[*forge_job.Job](ctx, object, forge_job.NewJobBlock, "Forge job", inspectForgeJobFields)
}

func inspectForgeTask(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectForgePayload[*forge_task.Task](ctx, object, forge_task.NewTaskBlock, "Forge task", inspectForgeTaskFields)
}

func inspectForgePass(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectForgePayload[*forge_pass.Pass](ctx, object, forge_pass.NewPassBlock, "Forge pass", inspectForgePassFields)
}

func inspectForgeExecution(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectForgePayload[*forge_execution.Execution](ctx, object, forge_execution.NewExecutionBlock, "Forge execution", inspectForgeExecutionFields)
}

func inspectForgeWorker(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectForgePayload[*forge_worker.Worker](ctx, object, forge_worker.NewWorkerBlock, "Forge worker", inspectForgeWorkerFields)
}

func inspectForgeDashboard(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	return inspectForgePayload[*forge_dashboard.ForgeDashboard](ctx, object, func() block.Block { return &forge_dashboard.ForgeDashboard{} }, "Forge dashboard", inspectForgeDashboardFields)
}

func rewriteForgeCluster(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewriteForgePayload[*forge_cluster.Cluster](ctx, object, mapping, forge_cluster.NewClusterBlock, "Forge cluster", nil, inspectForgeClusterFields)
}

func rewriteForgeJob(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewriteForgePayload[*forge_job.Job](ctx, object, mapping, forge_job.NewJobBlock, "Forge job", nil, inspectForgeJobFields)
}

func rewriteForgeTask(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewriteForgePayload[*forge_task.Task](ctx, object, mapping, forge_task.NewTaskBlock, "Forge task", rewriteForgeTaskFields, inspectForgeTaskFields)
}

func rewriteForgePass(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewriteForgePayload[*forge_pass.Pass](ctx, object, mapping, forge_pass.NewPassBlock, "Forge pass", rewriteForgePassFields, inspectForgePassFields)
}

func rewriteForgeExecution(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewriteForgePayload[*forge_execution.Execution](ctx, object, mapping, forge_execution.NewExecutionBlock, "Forge execution", rewriteForgeExecutionFields, inspectForgeExecutionFields)
}

func rewriteForgeWorker(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewriteForgePayload[*forge_worker.Worker](ctx, object, mapping, forge_worker.NewWorkerBlock, "Forge worker", nil, inspectForgeWorkerFields)
}

func rewriteForgeDashboard(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	return rewriteForgePayload[*forge_dashboard.ForgeDashboard](ctx, object, mapping, func() block.Block { return &forge_dashboard.ForgeDashboard{} }, "Forge dashboard", nil, inspectForgeDashboardFields)
}
