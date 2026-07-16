package web_runtime

import (
	"errors"
	"strings"
)

// IsWebRuntimeClientClosed reports whether err is a known web runtime client
// close notification that should end a configset operation without error.
func IsWebRuntimeClientClosed(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "WebRuntimeClientInstance closed:") {
		return true
	}
	return IsNormalWebRuntimeClientClose(err)
}

// IsNormalWebRuntimeClientClose reports whether err is a normal-close web
// runtime client generation teardown rather than an unexpected failure.
func IsNormalWebRuntimeClientClose(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	const clientPrefix = "WebRuntimeClient: "
	clientIdx := strings.Index(msg, clientPrefix)
	if clientIdx < 0 {
		return false
	}
	msg = msg[clientIdx+len(clientPrefix):]

	const generationMarker = ": runtime client generation "
	_, after, ok := strings.Cut(msg, generationMarker)
	if !ok {
		return false
	}
	generation := after

	const normalCloseSuffix = " closed: normal-close"
	if !strings.HasSuffix(generation, normalCloseSuffix) {
		return false
	}
	generation = strings.TrimSuffix(generation, normalCloseSuffix)
	if generation == "" {
		return false
	}
	for _, r := range generation {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ErrEmptyWebRuntimeID is returned if the web runtime ID was empty.
var ErrEmptyWebRuntimeID = errors.New("web runtime id cannot be empty")
