//go:build !tinygo

package resource_root

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	s4wave_configtype_registry "github.com/s4wave/spacewave/sdk/configtype/registry"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
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
	webMethod(s4wave_viewer_registry.SRPCViewerRegistryResourceServiceServiceID, "ListViewers"):              nil,
	webMethod(s4wave_viewer_registry.SRPCViewerRegistryResourceServiceServiceID, "WatchViewers"):             nil,
	webMethod(s4wave_configtype_registry.SRPCConfigTypeRegistryResourceServiceServiceID, "WatchConfigTypes"): nil,

	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "GetSessionInfo"): nil,
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "MountSharedObject"): func(b *webBinding, msg srpc.Message) bool {
		req, ok := msg.(*s4wave_session.MountSharedObjectRequest)
		return ok && req.GetSharedObjectId() == b.spaceID
	},

	webMethod(s4wave_sobject.SRPCSharedObjectResourceServiceServiceID, "WatchSharedObjectHealth"): nil,
	webMethod(s4wave_sobject.SRPCSharedObjectResourceServiceServiceID, "MountSharedObjectBody"):   nil,

	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "WatchSpaceState"):             nil,
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

// InvokeMethod denies methods outside webBindingMethods and checks the
// request messages of the methods that need it.
func (i *webBindingInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Deny methods outside the list, then route the call through its check.
	key := webMethod(serviceID, methodID)
	check, ok := webBindingMethods[key]
	if !ok {
		i.binding.le.WithField("method", key).Info("web binding denied method")
		return true, errors.Wrap(errWebBindingDenied, key)
	}
	if check != nil {
		strm = &webBindingStream{Stream: strm, binding: i.binding, method: key, check: check}
	}
	return i.inv.InvokeMethod(serviceID, methodID, strm)
}

// webBindingStream checks each request message it receives.
type webBindingStream struct {
	srpc.Stream
	binding *webBinding
	method  string
	check   webBindingCheck
}

// MsgRecv receives a request message and rejects it unless the check admits it.
func (s *webBindingStream) MsgRecv(msg srpc.Message) error {
	if err := s.Stream.MsgRecv(msg); err != nil {
		return err
	}
	if !s.check(s.binding, msg) {
		s.binding.le.WithField("method", s.method).Info("web binding denied request")
		return errors.Wrap(errWebBindingDenied, s.method)
	}
	return nil
}

// _ is a type assertion
var (
	_ srpc.Invoker = (*webBindingInvoker)(nil)
	_ srpc.Stream  = (*webBindingStream)(nil)
)
