//go:build !js

package goscriptbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"runtime"

	"github.com/pkg/errors"
)

const (
	// ProjectedImageWidth is the fixed fixture width in pixels.
	ProjectedImageWidth = 1024
	// ProjectedImageHeight is the fixed fixture height in pixels.
	ProjectedImageHeight = 1024
	// ProjectedImageFixturePath is the fixed UnixFS path for the benchmark image.
	ProjectedImageFixturePath = "goscriptbench-projected-image-v1.png"

	projectedImageGenerator         = "xorshift64star-nrgba"
	projectedImageGeneratorRevision = "1"
	projectedImageSeed              = uint64(0x6a09e667f3bcc909)
	projectedImagePixelBytes        = ProjectedImageWidth * ProjectedImageHeight * 4
)

// GenerateProjectedImageFixture creates the fixed low-compressibility PNG and its identity.
func GenerateProjectedImageFixture() ([]byte, Fixture, error) {
	// Generate the fixed four-channel pixel sequence.
	pixels := image.NewNRGBA(image.Rect(0, 0, ProjectedImageWidth, ProjectedImageHeight))
	fillProjectedImagePixels(pixels.Pix)

	// Encode the pixels with the standard PNG encoder.
	buffer := bytes.NewBuffer(make([]byte, 0, projectedImagePixelBytes+64*1024))
	if err := png.Encode(buffer, pixels); err != nil {
		return nil, Fixture{}, errors.Wrap(err, "encode projected-image fixture")
	}

	// Record the encoded byte and generator identity.
	data := buffer.Bytes()
	digest := sha256.Sum256(data)
	fixture := Fixture{
		Generator:          projectedImageGenerator,
		GeneratorRevision:  projectedImageGeneratorRevision,
		Encoder:            "image/png.Encode",
		EncoderEnvironment: runtime.Version() + "/" + runtime.GOOS + "/" + runtime.GOARCH,
		SHA256:             hex.EncodeToString(digest[:]),
		EncodedBytes:       int64(len(data)),
		Width:              ProjectedImageWidth,
		Height:             ProjectedImageHeight,
		ColorModel:         "NRGBA",
		Path:               ProjectedImageFixturePath,
	}

	// Validate the generated fixture before returning it to browser setup.
	if err := ValidateProjectedImageFixture(data, fixture); err != nil {
		return nil, Fixture{}, err
	}
	return data, fixture, nil
}

// ValidateProjectedImageFixture checks the encoded bytes against their recorded identity.
func ValidateProjectedImageFixture(data []byte, fixture Fixture) error {
	// Validate the recorded metadata and encoded byte identity.
	if err := fixture.Validate(); err != nil {
		return err
	}
	if int64(len(data)) != fixture.EncodedBytes {
		return errors.Errorf("fixture byte count %d differs from recorded %d", len(data), fixture.EncodedBytes)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != fixture.SHA256 {
		return errors.New("fixture SHA-256 differs from recorded identity")
	}

	// Decode the PNG and verify its image identity.
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "decode projected-image fixture")
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != fixture.Width || bounds.Dy() != fixture.Height {
		return errors.New("decoded fixture dimensions differ from recorded identity")
	}
	if fixture.ColorModel != "NRGBA" {
		return errors.New("recorded fixture color model must be NRGBA")
	}
	if _, ok := decoded.(*image.NRGBA); !ok {
		return errors.New("decoded fixture color model differs from recorded identity")
	}
	return nil
}

func fillProjectedImagePixels(pixels []byte) {
	state := projectedImageSeed
	for offset := 0; offset < len(pixels); offset += 8 {
		state ^= state >> 12
		state ^= state << 25
		state ^= state >> 27
		value := state * 0x2545f4914f6cdd1d
		for idx := range 8 {
			pixels[offset+idx] = byte(value >> (idx * 8))
		}
	}
}
