package bldr_manifest

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/bldr/util/valuelist"
	"github.com/sirupsen/logrus"
)

// manifestFetchViaBus implements the ManifestFetch service.
type manifestFetchViaBus struct {
	le *logrus.Entry
	b  bus.Bus
}

// NewManifestFetchViaBus serves manifest fetch requests through the bus.
func NewManifestFetchViaBus(le *logrus.Entry, b bus.Bus) SRPCManifestFetchServer {
	return &manifestFetchViaBus{le: le, b: b}
}

// FetchManifest fetches a manifest by metadata.
func (f *manifestFetchViaBus) FetchManifest(
	req *FetchManifestRequest,
	strm SRPCManifestFetch_FetchManifestStream,
) error {
	// Validate the request before fetching.
	if err := req.validate(); err != nil {
		return err
	}

	// Log the fetch and stream each matching manifest response.
	manifestID := req.GetManifestId()
	f.le.Debugf("host is fetching manifest: %s", manifestID)
	defer f.le.Debugf("exited host is fetching manifest: %s", manifestID)

	return valuelist.WatchDirective(
		strm.Context(),
		f.b,
		req.ToDirective(),
		func() *FetchManifestResponse { return &FetchManifestResponse{} },
		func(msg *FetchManifestResponse) error {
			dat, err := msg.MarshalJSON()
			if err != nil {
				return err
			}
			f.le.Debugf("host is fetching manifest: %s => sending %s", manifestID, string(dat))
			return strm.Send(msg)
		},
		nil,
	)
}

// _ is a type assertion
var _ SRPCManifestFetchServer = (*manifestFetchViaBus)(nil)
