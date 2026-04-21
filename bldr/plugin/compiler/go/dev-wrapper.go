package bldr_plugin_compiler_go

import (
	"embed"
	"regexp"

	"github.com/pkg/errors"
)

// DevWrapperFs contains the dev wrapper go code.
//
//go:embed dev-wrapper
var DevWrapperFs embed.FS

// GetDevWrapper is a Go program to launch the plugin in dev mode.
func GetDevWrapper() (string, error) {
	src, err := DevWrapperFs.ReadFile("dev-wrapper/main.go")
	if err != nil {
		return "", err
	}
	return string(src), nil
}

// delveAddrRe is a regexp of allowed characters for dlv_addr
var delveAddrRe = regexp.MustCompile(`^((([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5]))?:[0-9]+$`)

// ValidateDelveAddr validates a Dlv listen address.
//
// basic checks
func ValidateDelveAddr(addr string) error {
	if addr == "" {
		return errors.New("delve listen address is empty")
	}
	if addr == "wait" {
		return nil
	}
	if !delveAddrRe.MatchString(addr) {
		return errors.Errorf("invalid listen address: %s", addr)
	}
	return nil
}
