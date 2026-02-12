package web_runtime

import (
	"os"
	"strings"

	"github.com/pkg/errors"
)

// WebRendererEnvVar is the environment variable for selecting the web renderer.
const WebRendererEnvVar = "BLDR_WEB_RENDERER"

// DefaultWebRenderer is the default web renderer for native applications.
const DefaultWebRenderer = WebRenderer_WEB_RENDERER_SAUCER

// ParseWebRenderer parses a web renderer string.
// Returns an error if the renderer is invalid.
func ParseWebRenderer(s string) (WebRenderer, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return WebRenderer_WEB_RENDERER_DEFAULT, nil
	}
	switch s {
	case "ELECTRON", "WEB_RENDERER_ELECTRON":
		return WebRenderer_WEB_RENDERER_ELECTRON, nil
	case "SAUCER", "WEB_RENDERER_SAUCER":
		return WebRenderer_WEB_RENDERER_SAUCER, nil
	case "DEFAULT", "WEB_RENDERER_DEFAULT":
		return WebRenderer_WEB_RENDERER_DEFAULT, nil
	default:
		return WebRenderer_WEB_RENDERER_DEFAULT, errors.Errorf(
			"invalid web renderer: %q (valid: ELECTRON, SAUCER)", s,
		)
	}
}

// GetWebRendererFromEnv reads the web renderer from the environment variable.
// Returns WEB_RENDERER_DEFAULT if not set or empty.
func GetWebRendererFromEnv() WebRenderer {
	s := os.Getenv(WebRendererEnvVar)
	r, err := ParseWebRenderer(s)
	if err != nil {
		return WebRenderer_WEB_RENDERER_DEFAULT
	}
	return r
}

// Resolve resolves the web renderer, defaulting to saucer if DEFAULT.
func (r WebRenderer) Resolve() WebRenderer {
	if r == WebRenderer_WEB_RENDERER_DEFAULT {
		return DefaultWebRenderer
	}
	return r
}

// Validate validates the web renderer.
func (r WebRenderer) Validate() error {
	switch r {
	case WebRenderer_WEB_RENDERER_DEFAULT,
		WebRenderer_WEB_RENDERER_ELECTRON,
		WebRenderer_WEB_RENDERER_SAUCER:
		return nil
	default:
		return errors.Errorf("invalid web renderer: %v", r)
	}
}
