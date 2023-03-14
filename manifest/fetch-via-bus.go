package bldr_manifest

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ManifestFetchViaBusControllerID is the controller ID used for ManifestFetchViaBus.
const ManifestFetchViaBusControllerID = "bldr/manifest/fetch-via-bus"

// ManifestFetchViaBusVersion is the controller version used for ManifestFetchViaBus.
var ManifestFetchViaBusVersion = semver.MustParse("0.0.1")

// ManifestFetchViaBus implements the ManifestFetch service.
type ManifestFetchViaBus struct {
	le *logrus.Entry
	b  bus.Bus
}

// NewManifestFetchViaBus constructs a new ManifestFetchViaBus implementation.
func NewManifestFetchViaBus(le *logrus.Entry, b bus.Bus) *ManifestFetchViaBus {
	return &ManifestFetchViaBus{le: le, b: b}
}

// NewManifestFetchViaBusController constructs a new controller resolving
// LookupRpcService with the FetchManifestViaBus service.
func NewManifestFetchViaBusController(le *logrus.Entry, b bus.Bus) *bifrost_rpc.InvokerController {
	mux := srpc.NewMux()
	f := NewManifestFetchViaBus(le, b)
	_ = SRPCRegisterManifestFetch(mux, f)
	return bifrost_rpc.NewInvokerController(
		le,
		b,
		controller.NewInfo(
			ManifestFetchViaBusControllerID,
			ManifestFetchViaBusVersion,
			"FetchManifest rpc to directive",
		),
		mux,
		nil,
	)
}

// FetchManifest fetches a manifest by metadata.
func (f *ManifestFetchViaBus) FetchManifest(
	ctx context.Context,
	req *FetchManifestRequest,
) (*FetchManifestResponse, error) {
	if err := req.Validate(false); err != nil {
		return nil, err
	}

	meta := req.GetManifestMeta()
	manifestID := meta.GetManifestId()
	f.le.Infof("host requests fetching manifest: %s", manifestID)

	return ExFetchManifest(ctx, f.b, meta, false)
}

// _ is a type assertion
var _ SRPCManifestFetchServer = ((*ManifestFetchViaBus)(nil))
