//go:build !js

package goscriptbench

import "github.com/pkg/errors"

// Measurement groups one sample with evidence captured in the same window.
type Measurement struct {
	// sample is the browser-local timing row
	Sample Sample
	// runtimeTrace is present only for the traced diagnostic
	RuntimeTrace []byte
	// browserCPUProfile is optional Chromium-only diagnostic evidence
	BrowserCPUProfile []byte
}

// Validate checks sample trace state and diagnostic evidence custody.
func (m Measurement) Validate(request SampleRequest, metadata RunMetadata) error {
	if m.Sample.Traced != request.Trace {
		return errors.New("measurement trace state differs from its request")
	}
	if err := m.Sample.Validate(metadata); err != nil {
		return err
	}
	if !request.Trace {
		if len(m.RuntimeTrace) != 0 || len(m.BrowserCPUProfile) != 0 {
			return errors.New("untraced measurement contains diagnostic evidence")
		}
		return nil
	}
	if len(m.RuntimeTrace) == 0 {
		return errors.New("traced measurement has no runtime trace")
	}
	if len(m.BrowserCPUProfile) != 0 && metadata.Engine != "chromium" {
		return errors.New("browser CPU profile requires Chromium")
	}
	return nil
}
