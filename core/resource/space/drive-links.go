package resource_space

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	unixfs_linkedfolder "github.com/s4wave/spacewave/sdk/unixfs/linkedfolder"
)

// WatchDriveLinks streams the folders linked into a Drive. Each linking
// Device's entry carries the node state its Device record holds, which the
// Device writes only on a transition; the entry of a link running on this
// Device also carries its live counts, which no World record holds, and each
// entry carries whether this session reaches its Device now. The response
// counts the other peers the session reaches, which may also serve the
// Drive's files.
//
// Each World change to a Flowgraph or a linking Device rereads the Flowgraphs
// of the Space; a count or link change rereads nothing.
func (r *SpaceResource) WatchDriveLinks(
	req *s4wave_space.WatchDriveLinksRequest,
	strm s4wave_space.SRPCSpaceResourceService_WatchDriveLinksStream,
) error {
	// Require the Drive.
	ctx := strm.Context()
	objectKey := req.GetObjectKey()
	if objectKey == "" {
		return world.ErrEmptyObjectKey
	}

	// Follow the links running on this Device.
	var live driveLinksLive
	engineID := r.space.GetWorldEngineID()
	release, err := unixfs_linkedfolder.WatchLinkedFolders(ctx, r.b, engineID, objectKey, func(links []unixfs_linkedfolder.RunningLink) {
		live.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			live.running = links
			broadcast()
		})
	})
	if err != nil {
		return err
	}
	defer release()

	// Follow the session's transport, which knows the Devices it reaches. A
	// session without a peer observes no Device, so each reads unknown.
	sessionPeer, err := peer.IDB58Decode(r.sessionPeerID)
	live.observed = err == nil
	if live.observed {
		_, ref, err := bus.ExecOneOffWatchCb(ctx, func(value directive.TypedAttachedValue[*transport.SessionTransport]) bool {
			live.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				live.transport = nil
				if value != nil {
					live.transport = value.GetValue()
				}
				broadcast()
			})
			return true
		}, r.b, transport.NewLookupSessionTransport(sessionPeer))
		if err != nil {
			return err
		}
		defer ref.Release()
	}

	// Read the linking nodes, then send after each change of the running
	// links or their counts, until the World changes what the read saw.
	engine := r.space.GetWorldEngine()
	var prev *s4wave_space.WatchDriveLinksResponse
	for {
		links, reads, seqno, err := readDriveLinks(ctx, engine, objectKey)
		if err != nil {
			return err
		}
		if err := r.sendDriveLinks(ctx, strm, links, reads, seqno, &live, &prev); err != nil {
			return err
		}
	}
}

// driveLinksLive is what a Drive's links show besides the World: the links
// running on this Device and the session's transport. The broadcast guards
// running and transport.
type driveLinksLive struct {
	bcast     broadcast.Broadcast
	running   []unixfs_linkedfolder.RunningLink
	transport *transport.SessionTransport
	// observed is set when the session has a peer to reach Devices from.
	observed bool
}

// sendDriveLinks sends the links joined with what live holds after each
// change of either, and returns once the World changes what reads saw.
func (r *SpaceResource) sendDriveLinks(
	ctx context.Context,
	strm s4wave_space.SRPCSpaceResourceService_WatchDriveLinksStream,
	links []driveLink,
	reads *world.ReadSet,
	seqno uint64,
	live *driveLinksLive,
	prev **s4wave_space.WatchDriveLinksResponse,
) error {
	// Watch the World in the background; worldDone closes at its change.
	worldCtx, stopWorld := context.WithCancel(ctx)
	defer stopWorld()
	worldDone := make(chan struct{})
	var worldErr error
	go func() {
		_, worldErr = reads.WaitChange(worldCtx, r.space.GetWorldEngine(), seqno)
		close(worldDone)
	}()
	for {
		// Join the running links' counts and the Devices' liveness.
		var running []unixfs_linkedfolder.RunningLink
		var st *transport.SessionTransport
		var liveWait <-chan struct{}
		live.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			running, st, liveWait = live.running, live.transport, getWaitCh()
		})
		resp, waits := joinLiveLinks(links, running, st, live.observed)
		if !resp.EqualVT(*prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			*prev = resp
		}

		// Wait for the World, the running set, a count, or a link.
		if err := broadcast.WaitAny(ctx, append(waits, liveWait, worldDone)...); err != nil {
			return err
		}
		select {
		case <-worldDone:
			return worldErr
		default:
		}
	}
}

// driveLink is a linking node's entry with the node parameters that match it
// to a running link.
type driveLink struct {
	// link is the entry without its local fields.
	link *s4wave_space.DriveLink
	// folder is the node's folder parameter.
	folder string
	// peerID is the linking Device's peer.
	peerID string
}

// readDriveLinks reads the linked-folder nodes that write the Drive at
// objectKey, one entry per Device each is placed on, through a read set that
// names what the read depends on.
func readDriveLinks(ctx context.Context, engine world.Engine, objectKey string) ([]driveLink, *world.ReadSet, uint64, error) {
	// Open the read transaction.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, 0, err
	}
	defer tx.Discard()
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	reads := world.NewReadSet()
	ws := reads.Track(tx)

	// Walk the Flowgraphs for the nodes linking a folder into the Drive. No
	// edge joins a node to the object it binds, so this lists the Space's
	// Flowgraphs, which number in the tens.
	keys, err := world_types.ListObjectsWithType(ctx, ws, s4wave_flowgraph.FlowgraphTypeID)
	if err != nil {
		return nil, nil, 0, err
	}
	slices.Sort(keys)
	devices := make(map[string]*s4wave_device.Device)
	var links []driveLink
	for _, key := range keys {
		snapshot, err := s4wave_flowgraph.ReadFlowgraph(ctx, ws, key)
		if err != nil {
			return nil, nil, 0, err
		}
		nodes := snapshot.GetState().GetNodes()
		for _, nodeID := range slices.Sorted(maps.Keys(nodes)) {
			node := nodes[nodeID]
			if node.GetTypeId() != unixfs_linkedfolder.NodeTypeID || node.GetObjectKey() != objectKey {
				continue
			}
			for _, placement := range snapshot.GetPlacements()[nodeID].GetCopies() {
				if placement.GetKind() != s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE {
					continue
				}
				deviceKey := placement.GetObjectKey()
				device, ok := devices[deviceKey]
				if !ok {
					device, err = readLinkingDevice(ctx, ws, deviceKey)
					if err != nil {
						return nil, nil, 0, err
					}
					devices[deviceKey] = device
				}
				link := &s4wave_space.DriveLink{
					DeviceKey:   deviceKey,
					Path:        node.GetParameters()[unixfs_linkedfolder.ParamPath],
					DeviceLabel: device.GetLabel(),
				}
				capabilityID := s4wave_flowgraph.NodeCapabilityID(key, nodeID)
				for _, capability := range device.GetCapabilities() {
					if capability.GetId() == capabilityID {
						link.State = driveLinkState(capability)
						link.Detail = capability.GetDetail()
						break
					}
				}
				links = append(links, driveLink{
					link:   link,
					folder: node.GetParameters()[unixfs_linkedfolder.ParamFolder],
					peerID: device.GetPeerId(),
				})
			}
		}
	}
	return links, reads, seqno, nil
}

// readLinkingDevice reads the Device at key, or an empty Device when the
// placement names a missing object.
func readLinkingDevice(ctx context.Context, ws world.WorldState, key string) (*s4wave_device.Device, error) {
	found, err := ws.HasObject(ctx, key)
	if err != nil || !found {
		return &s4wave_device.Device{}, err
	}
	return world.LookupObjectBody[s4wave_device.Device](ctx, ws, key)
}

// driveLinkState maps a linked-folder node's capability to its link state.
func driveLinkState(capability *s4wave_device.DeviceCapability) s4wave_space.DriveLinkState {
	switch capability.GetState() {
	case s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED:
		return s4wave_space.DriveLinkState_DriveLinkState_REJECTED
	case s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE:
	default:
		return s4wave_space.DriveLinkState_DriveLinkState_STARTING
	}
	switch {
	case capability.GetDetail() == unixfs_linkedfolder.StatusIndexing:
		return s4wave_space.DriveLinkState_DriveLinkState_INDEXING
	case strings.HasPrefix(capability.GetDetail(), unixfs_linkedfolder.StatusWatching):
		return s4wave_space.DriveLinkState_DriveLinkState_LINKED
	default:
		return s4wave_space.DriveLinkState_DriveLinkState_STARTING
	}
}

// joinLiveLinks builds the response from the linking nodes, setting each
// entry's liveness from the session's transport st and counting the other
// peers it reaches, and adding to each entry of a link running on this Device
// its folder and counts. observed is unset when the session has no peer, so
// liveness is unknown. It returns the change channels of the links and counts
// it read.
func joinLiveLinks(
	links []driveLink,
	running []unixfs_linkedfolder.RunningLink,
	st *transport.SessionTransport,
	observed bool,
) (*s4wave_space.WatchDriveLinksResponse, []<-chan struct{}) {
	// Snapshot the peers the session reaches.
	reached := make(map[peer.ID]struct{})
	var waits []<-chan struct{}
	if st != nil {
		var snaps []transport_controller.LinkSnapshot
		snaps, waits = st.GetLinkSnapshotsWithWait()
		for _, s := range snaps {
			reached[s.RemotePeerID] = struct{}{}
		}
	}

	// Join each entry's liveness, then its running link.
	resp := &s4wave_space.WatchDriveLinksResponse{Links: make([]*s4wave_space.DriveLink, 0, len(links))}
	others := maps.Clone(reached)
	for _, l := range links {
		link := l.link.CloneVT()
		link.Availability = linkAvailability(l.peerID, observed, reached)
		if id, err := peer.IDB58Decode(l.peerID); err == nil {
			delete(others, id)
		}
		for _, rl := range running {
			conf := rl.GetConfig()
			if conf.GetDevicePeerId() != l.peerID || conf.GetFolder() != l.folder || conf.GetPath() != link.GetPath() {
				continue
			}
			progress, wait := rl.GetLinkProgress()
			waits = append(waits, wait)
			link.Availability = s4wave_device.DeviceAvailabilityState_DEVICE_AVAILABILITY_STATE_ONLINE
			link.Local = &s4wave_space.DriveLinkLocal{
				Folder: conf.GetFolder(),
				Files:  progress.Files,
				Bytes:  progress.Bytes,
			}
			break
		}
		resp.Links = append(resp.Links, link)
	}
	resp.OtherPeersReached = uint32(len(others)) //nolint:gosec // bounded by the session transport's linked peers.
	return resp, waits
}

// linkAvailability is whether the session reaches the Device with peerID, as
// WatchDeviceAvailability projects it: unknown without an observer or a valid
// peer, online when linked, offline otherwise.
func linkAvailability(peerID string, observed bool, reached map[peer.ID]struct{}) s4wave_device.DeviceAvailabilityState {
	id, err := peer.IDB58Decode(peerID)
	if err != nil || !observed {
		return s4wave_device.DeviceAvailabilityState_DEVICE_AVAILABILITY_STATE_UNKNOWN
	}
	if _, ok := reached[id]; ok {
		return s4wave_device.DeviceAvailabilityState_DEVICE_AVAILABILITY_STATE_ONLINE
	}
	return s4wave_device.DeviceAvailabilityState_DEVICE_AVAILABILITY_STATE_OFFLINE
}
