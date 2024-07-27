package bldr_launcher

import (
	"context"
	io "io"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr/util/http"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FetchDistConfig fetches the dist configuration from an HTTP endpoint.
// Returns the config, the encoded config, the peer that signed the config, and any error.
// distPeerIDs are the peer ids that we accept signed DistConfig from.
func FetchDistConfig(
	ctx context.Context,
	le *logrus.Entry,
	reqURL string,
	headers map[string]string,
	projectID string,
	distPeerIDs []peer.ID,
) (*DistConfig, string, peer.ID, error) {
	le.Debugf("looking up dist config: %s", reqURL)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, "", "", err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	setFetchDistConfigHttpOpts(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", "", err
	}

	if resp.StatusCode != 200 {
		_ = resp.Body.Close()
		return nil, "", "", errors.Errorf("unsuccessful status code: %v: %s", resp.StatusCode, resp.Status)
	}

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", err
	}

	conf, confMsg, confPeer, err := ParseDistConfigPackedMsg(le, respData, distPeerIDs, projectID)
	if err == nil {
		le.WithFields(logrus.Fields{
			"rev":    conf.GetRev(),
			"signer": confPeer.String(),
		}).Info("found valid packed config")
	} else {
		le.WithError(err).Warn("no valid config found")
	}

	return conf, confMsg, confPeer, err
}
