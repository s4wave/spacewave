package webrtc

import (
	"slices"

	"github.com/pion/webrtc/v4"
)

// ToWebRtcConfiguration converts the WebRtcConfig into a webrtc.Configuration.
func (c *WebRtcConfig) ToWebRtcConfiguration() *webrtc.Configuration {
	conf := &webrtc.Configuration{
		ICETransportPolicy: c.GetIceTransportPolicy().ToICETransportPolicy(),
		ICEServers:         make([]webrtc.ICEServer, 0, len(c.GetIceServers())),
	}
	for _, iceServer := range c.GetIceServers() {
		conf.ICEServers = append(conf.ICEServers, iceServer.ToICEServer())
	}
	return conf
}

// ToICETransportPolicy converts the IceTransportPolicy to a webrtc.ICETransportPolicy.
func (c IceTransportPolicy) ToICETransportPolicy() webrtc.ICETransportPolicy {
	switch c {
	case IceTransportPolicy_IceTransportPolicy_ALL:
		return webrtc.ICETransportPolicyAll
	case IceTransportPolicy_IceTransportPolicy_RELAY:
		return webrtc.ICETransportPolicyRelay
	default:
		// Default to All if unknown
		return webrtc.ICETransportPolicyAll
	}
}

// ToICEServer converts the IceServerConfig to a webrtc.ICEServer.
func (c *IceServerConfig) ToICEServer() webrtc.ICEServer {
	out := webrtc.ICEServer{
		URLs:     slices.Clone(c.GetUrls()),
		Username: c.GetUsername(),
	}
	switch cred := c.GetCredential().(type) {
	case *IceServerConfig_Oauth:
		out.Credential = cred.Oauth.ToOauthCredential()
	case *IceServerConfig_Password:
		out.Credential = cred.Password
	}
	return out
}

// ToOauthCredential converts to a webrtc credential.
func (o *IceServerConfig_OauthCredential) ToOauthCredential() webrtc.OAuthCredential {
	return webrtc.OAuthCredential{
		MACKey:      o.GetMacKey(),
		AccessToken: o.GetAccessToken(),
	}
}
