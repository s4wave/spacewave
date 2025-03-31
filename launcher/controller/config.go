package bldr_launcher_controller

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	bldr_launcher "github.com/aperturerobotics/bldr/launcher"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// Validate checks the config.
func (c *Config) Validate() error {
	projectID := c.GetProjectId()
	if projectID == "" {
		return errors.New("project_id: cannot be empty")
	}
	if _, _, err := c.ParseEndpointURLs(); err != nil {
		return errors.Wrap(err, "endpoints")
	}
	if _, err := c.ParseRefetchDur(); err != nil {
		return errors.Wrap(err, "refetch_dur")
	}
	distPeerIDs, err := c.ParseDistPeerIds()
	if err != nil {
		return errors.Wrap(err, "dist_peer_ids")
	}
	if len(distPeerIDs) == 0 {
		return errors.New("dist_peer_ids: cannot be empty")
	}
	if _, _, _, err := c.ParseInitDistConfig(projectID, distPeerIDs); err != nil {
		return errors.Wrap(err, "init_dist_config")
	}
	return nil
}

// ParseInitDistConfig parses the init dist config field.
//
// distConfig is the parsed dist config of nil if none
// confMsg is the packedmsg containing distConfig, or empty if none
// confPeerID is the peer id that signed distConfig
// err is any error parsing
func (c *Config) ParseInitDistConfig(projectID string, signerPeerIDs []peer.ID) (distConfig *bldr_launcher.DistConfig, confMsg string, confPeerID peer.ID, err error) {
	initDistConfTxt := c.GetInitDistConfig()
	if len(initDistConfTxt) == 0 {
		return nil, "", "", nil
	}
	return bldr_launcher.ParseDistConfigPackedMsg(nil, []byte(initDistConfTxt), signerPeerIDs, projectID)
}

// CloneSortEndpoints returns a copy of endpoints compacted + sorted.
func (c *Config) CloneSortEndpoints() []*HttpEndpoint {
	endps := slices.Clone(c.GetEndpoints())
	slices.SortFunc(endps, func(a, b *HttpEndpoint) int {
		return strings.Compare(a.GetUrl(), b.GetUrl())
	})
	return endps
}

// ParseEndpointURLs deduplicates and parses the endpoint URLs.
func (c *Config) ParseEndpointURLs() ([]*url.URL, []*HttpEndpoint, error) {
	endps := c.CloneSortEndpoints()
	endpsUrls := make([]string, len(endps))
	for i, endp := range endps {
		endpsUrls[i] = endp.GetUrl()
	}
	urls, err := confparse.ParseURLs(endpsUrls, true)
	if err != nil {
		return nil, endps, err
	}
	return urls, endps, nil
}

// ParseRefetchDur parses the refetch duration.
func (c *Config) ParseRefetchDur() (time.Duration, error) {
	return confparse.ParseDuration(c.GetRefetchDur())
}

// ParseDistPeerIds returns the list of distribution peer ids.
func (c *Config) ParseDistPeerIds() ([]peer.ID, error) {
	return confparse.ParsePeerIDs(c.GetDistPeerIds(), false)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
