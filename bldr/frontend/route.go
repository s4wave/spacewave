package bldr_frontend

import (
	"encoding/base64"
	"strings"

	"github.com/pkg/errors"
)

// AttachedServicePrefix identifies services with a retained authoring Resource.
const AttachedServicePrefix = "frontend/"

// ServiceRoutePrefix gives an attached frontend service its same-origin namespace.
func ServiceRoutePrefix(serviceID string) string {
	return "/b/fe/rpc/" + base64.RawURLEncoding.EncodeToString([]byte(serviceID)) + "/"
}

// RouteService selects a frontend service from a browser module path.
// Ordinary development URLs use the document's existing devtool connection.
func RouteService(requestPath string) (string, error) {
	encoded, routed := strings.CutPrefix(requestPath, "/b/fe/rpc/")
	if !routed {
		return "devtool/" + SRPCFrontendServiceID, nil
	}
	encoded, rest, ok := strings.Cut(encoded, "/")
	if !ok || rest == "" || len(encoded) > 2048 {
		return "", errors.New("invalid frontend service route")
	}
	serviceID, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || !strings.HasSuffix(string(serviceID), "/"+SRPCFrontendServiceID) {
		return "", errors.New("invalid frontend service route")
	}
	return string(serviceID), nil
}
