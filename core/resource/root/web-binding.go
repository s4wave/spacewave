//go:build !tinygo

package resource_root

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	space "github.com/s4wave/spacewave/core/space"
	s4wave_command_registry "github.com/s4wave/spacewave/sdk/command/registry"
	s4wave_configtype_registry "github.com/s4wave/spacewave/sdk/configtype/registry"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
	"github.com/sirupsen/logrus"
)

// errWebBindingDenied is returned for a call outside a web binding.
var errWebBindingDenied = errors.New("not available to a bound web listener")

// webBinding limits a web listener's Resource service to reading one Space
// in one session. It is the InvokerFilter for every Resource the listener
// serves, so a call reaches a Resource only through webBindingMethods.
type webBinding struct {
	// le logs denied calls.
	le *logrus.Entry
	// sessionIdx is the bound session index.
	sessionIdx uint32
	// spaceID is the bound Space shared object ID.
	spaceID string
}

// webBindingCheck admits one request message for a binding.
type webBindingCheck func(b *webBinding, msg srpc.Message) bool

// webBindingMethods lists the read-only methods a bound listener serves, keyed
// by service and method ID. A nil check admits every request; any method
// absent from the list is denied.
var webBindingMethods = map[string]webBindingCheck{
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "MountSessionByIdx"): func(b *webBinding, msg srpc.Message) bool {
		req, ok := msg.(*s4wave_root.MountSessionByIdxRequest)
		return ok && req.GetSessionIdx() == b.sessionIdx
	},
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "WatchSessionMetadata"): func(b *webBinding, msg srpc.Message) bool {
		req, ok := msg.(*s4wave_root.WatchSessionMetadataRequest)
		return ok && req.GetSessionIdx() == b.sessionIdx
	},
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "WatchListenerStatus"): nil,
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "MarshalHash"):         nil,

	webMethod(s4wave_viewer_registry.SRPCViewerRegistryResourceServiceServiceID, "ListViewers"):              nil,
	webMethod(s4wave_viewer_registry.SRPCViewerRegistryResourceServiceServiceID, "WatchViewers"):             nil,
	webMethod(s4wave_configtype_registry.SRPCConfigTypeRegistryResourceServiceServiceID, "WatchConfigTypes"): nil,
	webMethod(s4wave_objecttype_registry.SRPCObjectTypeRegistryResourceServiceServiceID, "WatchObjectTypes"): nil,
	webMethod(s4wave_quickstart_registry.SRPCQuickstartRegistryResourceServiceServiceID, "WatchQuickstarts"): nil,
	webMethod(s4wave_wizard.SRPCObjectWizardRegistryResourceServiceServiceID, "WatchWizards"):                nil,

	// Command registrations belong to the client that made them: a client
	// watches and invokes only its own commands, whose handlers run in its tab.
	webMethod(s4wave_command_registry.SRPCCommandRegistryResourceServiceServiceID, "RegisterCommand"): nil,
	webMethod(s4wave_command_registry.SRPCCommandRegistryResourceServiceServiceID, "SetActive"):       nil,
	webMethod(s4wave_command_registry.SRPCCommandRegistryResourceServiceServiceID, "SetEnabled"):      nil,
	webMethod(s4wave_command_registry.SRPCCommandRegistryResourceServiceServiceID, "WatchCommands"):   nil,
	webMethod(s4wave_command_registry.SRPCCommandRegistryResourceServiceServiceID, "GetSubItems"):     nil,
	webMethod(s4wave_command_registry.SRPCCommandRegistryResourceServiceServiceID, "InvokeCommand"):   nil,

	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "GetSessionInfo"):     nil,
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "WatchSyncStatus"):    nil,
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "WatchStorageStats"):  nil,
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "WatchLockState"):     nil,
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "WatchResourcesList"): nil,
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "MountSharedObject"): func(b *webBinding, msg srpc.Message) bool {
		req, ok := msg.(*s4wave_session.MountSharedObjectRequest)
		return ok && req.GetSharedObjectId() == b.spaceID
	},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "WatchSharedObjectHealth"): func(b *webBinding, msg srpc.Message) bool {
		req, ok := msg.(*s4wave_session.WatchSharedObjectHealthRequest)
		return ok && req.GetSharedObjectId() == b.spaceID
	},

	webMethod(s4wave_sobject.SRPCSharedObjectResourceServiceServiceID, "WatchSharedObjectHealth"): nil,
	webMethod(s4wave_sobject.SRPCSharedObjectResourceServiceServiceID, "MountSharedObjectBody"):   nil,

	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "WatchSpaceState"):             nil,
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "WatchSpaceSharingState"):      nil,
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "AccessWorld"):                 nil,
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "MountSpaceContents"):          nil,
	webMethod(s4wave_space.SRPCSpaceContentsResourceServiceServiceID, "WatchState"):          nil,
	webMethod(s4wave_world.SRPCEngineResourceServiceServiceID, "GetEngineInfo"):              nil,
	webMethod(s4wave_world.SRPCEngineResourceServiceServiceID, "GetWorldRootSnapshot"):       nil,
	webMethod(s4wave_world.SRPCEngineResourceServiceServiceID, "WatchWorldRootSnapshots"):    nil,
	webMethod(s4wave_world.SRPCEngineResourceServiceServiceID, "GetSeqno"):                   nil,
	webMethod(s4wave_world.SRPCEngineResourceServiceServiceID, "WaitSeqno"):                  nil,
	webMethod(s4wave_world.SRPCEngineResourceServiceServiceID, "AccessWorldState"):           nil,
	webMethod(s4wave_world.SRPCEngineResourceServiceServiceID, "NewTransaction"):             webBindingReadTransaction,
	webMethod(s4wave_world.SRPCTxResourceServiceServiceID, "Discard"):                        nil,
	webMethod(s4wave_world.SRPCWatchWorldStateResourceServiceServiceID, "WatchWorldState"):   nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "CompareObjectRecords"):   nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "GetReadOnly"):            nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "GetSeqno"):               nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "WaitSeqno"):              nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "AccessWorldState"):       nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "GetObject"):              nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "IterateObjects"):         nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "ListObjects"):            nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "LookupGraphQuads"):       nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "LookupGraphQuadsBatch"):  nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "ListGraphEdgeBuckets"):   nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "ListObjectsWithType"):    nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "GetObjectRootRefsBatch"): nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "GetObjectMetadataBatch"): nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "GetObjectBodiesBatch"):   nil,
	webMethod(s4wave_world.SRPCWorldStateResourceServiceServiceID, "QueryGraphPath"):         nil,
	webMethod(s4wave_world.SRPCObjectIteratorResourceServiceServiceID, "Err"):                nil,
	webMethod(s4wave_world.SRPCObjectIteratorResourceServiceServiceID, "Valid"):              nil,
	webMethod(s4wave_world.SRPCObjectIteratorResourceServiceServiceID, "Key"):                nil,
	webMethod(s4wave_world.SRPCObjectIteratorResourceServiceServiceID, "Next"):               nil,
	webMethod(s4wave_world.SRPCObjectIteratorResourceServiceServiceID, "Seek"):               nil,
	webMethod(s4wave_world.SRPCObjectIteratorResourceServiceServiceID, "Close"):              nil,
	webMethod(s4wave_world.SRPCGraphPathQueryResourceServiceServiceID, "Next"):               nil,
	webMethod(s4wave_world.SRPCGraphPathQueryResourceServiceServiceID, "Close"):              nil,
	webMethod(s4wave_world.SRPCObjectStateResourceServiceServiceID, "GetKey"):                nil,
	webMethod(s4wave_world.SRPCObjectStateResourceServiceServiceID, "GetRootRef"):            nil,
	webMethod(s4wave_world.SRPCObjectStateResourceServiceServiceID, "AccessWorldState"):      nil,
	webMethod(s4wave_world.SRPCObjectStateResourceServiceServiceID, "WaitRev"):               nil,
	webMethod(s4wave_world.SRPCTypedObjectResourceServiceServiceID, "AccessTypedObject"):     nil,
	webMethod(s4wave_world.SRPCTypedObjectResourceServiceServiceID, "WatchTypedObject"):      nil,
}

// webBindingResponses narrows the responses of admitted methods that describe
// more than the bound Space, keyed like webBindingMethods.
var webBindingResponses = map[string]func(b *webBinding, msg srpc.Message){
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "WatchResourcesList"): webBindingSpacesList,
}

// webBindingSpacesList keeps only the bound Space in a Space list.
func webBindingSpacesList(b *webBinding, msg srpc.Message) {
	// Pass other messages through unchanged.
	resp, ok := msg.(*s4wave_session.WatchResourcesListResponse)
	if !ok {
		return
	}

	// Build a new slice: the sender may still hold the original.
	var spaces []*space.SpaceSoListEntry
	for _, entry := range resp.GetSpacesList() {
		if entry.GetEntry().GetRef().GetProviderResourceRef().GetId() == b.spaceID {
			spaces = append(spaces, entry)
		}
	}
	resp.SpacesList = spaces
}

// webMethod joins a service and method ID into a webBindingMethods key.
func webMethod(serviceID, methodID string) string {
	return serviceID + "/" + methodID
}

// webBindingReadTransaction admits read transactions only.
func webBindingReadTransaction(_ *webBinding, msg srpc.Message) bool {
	req, ok := msg.(*s4wave_world.NewTransactionRequest)
	return ok && !req.GetWrite()
}

// filter wraps a Resource invoker with the binding's method policy.
func (b *webBinding) filter(inv srpc.Invoker) srpc.Invoker {
	return &webBindingInvoker{binding: b, inv: inv}
}

// webBindingInvoker serves only the methods a webBinding admits.
type webBindingInvoker struct {
	binding *webBinding
	inv     srpc.Invoker
}

// InvokeMethod denies methods outside webBindingMethods, checks the request
// messages of the methods that need it, and narrows their responses.
func (i *webBindingInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Deny methods outside the list, then route the call through its check
	// and response filter.
	key := webMethod(serviceID, methodID)
	check, ok := webBindingMethods[key]
	if !ok {
		i.binding.le.WithField("method", key).Info("web binding denied method")
		return true, errors.Wrap(errWebBindingDenied, key)
	}
	narrow := webBindingResponses[key]
	if check != nil || narrow != nil {
		strm = &webBindingStream{
			Stream:  strm,
			binding: i.binding,
			method:  key,
			check:   check,
			narrow:  narrow,
		}
	}
	return i.inv.InvokeMethod(serviceID, methodID, strm)
}

// webBindingStream checks each request message it receives and narrows each
// response it sends.
type webBindingStream struct {
	srpc.Stream
	binding *webBinding
	method  string
	check   webBindingCheck
	narrow  func(b *webBinding, msg srpc.Message)
}

// MsgRecv receives a request message and rejects it unless the check admits it.
func (s *webBindingStream) MsgRecv(msg srpc.Message) error {
	if err := s.Stream.MsgRecv(msg); err != nil {
		return err
	}
	if s.check != nil && !s.check(s.binding, msg) {
		s.binding.le.WithField("method", s.method).Info("web binding denied request")
		return errors.Wrap(errWebBindingDenied, s.method)
	}
	return nil
}

// MsgSend narrows a response message and sends it.
func (s *webBindingStream) MsgSend(msg srpc.Message) error {
	if s.narrow != nil {
		s.narrow(s.binding, msg)
	}
	return s.Stream.MsgSend(msg)
}

// _ is a type assertion
var (
	_ srpc.Invoker = (*webBindingInvoker)(nil)
	_ srpc.Stream  = (*webBindingStream)(nil)
)
