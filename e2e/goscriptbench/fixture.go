//go:build !js

package goscriptbench

import "github.com/pkg/errors"

// Fixture identifies the bytes and environment used by one benchmark run.
type Fixture struct {
	// generator names the deterministic fixture algorithm
	Generator string
	// generatorRevision identifies the generator contract
	GeneratorRevision string
	// encoder names the fixture encoder
	Encoder string
	// encoderEnvironment identifies the encoder runtime
	EncoderEnvironment string
	// SHA256 is the lowercase hexadecimal digest of the encoded fixture
	SHA256 string
	// encodedBytes is the encoded fixture size
	EncodedBytes int64
	// width is the decoded fixture width
	Width int
	// height is the decoded fixture height
	Height int
	// colorModel names the decoded fixture color model
	ColorModel string
	// path is the fixture path inside the measured storage system
	Path string
}

// Validate checks that the fixture has a complete byte identity.
func (f Fixture) Validate() error {
	if f.Generator == "" || f.GeneratorRevision == "" {
		return errors.New("fixture generator identity is incomplete")
	}
	if f.Encoder == "" || f.EncoderEnvironment == "" {
		return errors.New("fixture encoder identity is incomplete")
	}
	if len(f.SHA256) != 64 {
		return errors.New("fixture SHA-256 must contain 64 lowercase hexadecimal characters")
	}
	for _, char := range f.SHA256 {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return errors.New("fixture SHA-256 must contain 64 lowercase hexadecimal characters")
		}
	}
	if f.EncodedBytes <= 0 {
		return errors.New("fixture encoded byte count must be positive")
	}
	if f.Width <= 0 || f.Height <= 0 {
		return errors.New("fixture dimensions must be positive")
	}
	if f.ColorModel == "" || f.Path == "" {
		return errors.New("fixture color model and path are required")
	}
	return nil
}
