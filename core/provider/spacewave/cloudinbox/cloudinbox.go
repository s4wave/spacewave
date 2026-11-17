// Package cloudinbox is the spacewave/cloud-inbox event source: an object
// whose resource serves a Spacewave Cloud inbox to a Flowgraph standing run.
//
// The inbox holds the external events, such as GitHub check deliveries, that
// the Cloud appended for the account that owns the Space. Source reads them
// after the run's cursor, names each event's port from its kind, reports shed
// events as one gap event, and acknowledges what the run stored so the Cloud
// drops it. The Spacewave provider supplies the Transport, which signs every
// request with the account's session key.
package cloudinbox

import (
	"strings"

	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
)

// TypeID is the object type of a cloud inbox event source.
const TypeID = "spacewave/cloud-inbox"

// The output ports of a cloud inbox. A kind port carries InboxEvent payloads
// and the gap port carries InboxGap payloads, so a filter is a connection.
const (
	// PortCheckSuite carries check_suite events.
	PortCheckSuite = "check_suite"
	// PortCheckRun carries check_run events.
	PortCheckRun = "check_run"
	// PortWorkflowRun carries workflow_run events.
	PortWorkflowRun = "workflow_run"
	// PortGap carries one event for each run of events the inbox shed.
	PortGap = "gap"
	// PortOther carries every event of another kind.
	PortOther = "other"
)

// kindPorts are the ports named for an event kind's first segment.
var kindPorts = []string{PortCheckSuite, PortCheckRun, PortWorkflowRun}

// KindPort returns the port an event of a normalized kind, such as
// check_suite.completed, leaves by.
func KindPort(kind string) string {
	family, _, _ := strings.Cut(kind, ".")
	for _, port := range kindPorts {
		if family == port {
			return port
		}
	}
	return PortOther
}

// Metadata returns the object type metadata of a cloud inbox, with the output
// ports a Flowgraph binding reads.
func Metadata() *s4wave_objecttype_registry.ObjectTypeMetadata {
	ports := []string{PortCheckSuite, PortCheckRun, PortWorkflowRun, PortGap, PortOther}
	metadata := &s4wave_objecttype_registry.ObjectTypeMetadata{
		DisplayName: "Cloud inbox",
		Description: "Events the Spacewave Cloud receives for this account, such as CI results.",
		Visibility:  s4wave_objecttype_registry.ObjectTypeVisibility_OBJECT_TYPE_VISIBILITY_VISIBLE,
	}
	for _, port := range ports {
		metadata.Ports = append(metadata.Ports, &s4wave_objecttype_registry.ObjectTypePort{
			Name:      port,
			Direction: s4wave_objecttype_registry.ObjectTypePortDirection_OBJECT_TYPE_PORT_DIRECTION_OUTPUT,
			TypeId:    "activation",
		})
	}
	return metadata
}
