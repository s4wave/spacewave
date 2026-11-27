//go:build !js

package goscriptbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"testing"
)

func TestProjectedImageFixtureDeterministic(t *testing.T) {
	// Generate the fixture twice and require identical bytes and metadata.
	firstData, firstFixture, err := GenerateProjectedImageFixture()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Generate the fixture a second time for comparison.
	secondData, secondFixture, err := GenerateProjectedImageFixture()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Require identical fixture bytes and metadata across generations.
	if !bytes.Equal(firstData, secondData) {
		t.Fatal("fixture bytes changed between generations")
	}
	if firstFixture != secondFixture {
		t.Fatalf("fixture metadata changed: %+v != %+v", firstFixture, secondFixture)
	}

	// Require the fixture dimensions to match the fixed workload.
	if firstFixture.Width != ProjectedImageWidth || firstFixture.Height != ProjectedImageHeight {
		t.Fatalf("fixture dimensions = %dx%d", firstFixture.Width, firstFixture.Height)
	}

	// Require the source pixel identity independently of the PNG encoder version.
	decoded, err := png.Decode(bytes.NewReader(firstData))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(decoded.(*image.NRGBA).Pix)
	if hex.EncodeToString(digest[:]) != "5301749e2d65c4622c3f12e63057827a4ebcb1ffce167e6fd494fe5fb893b478" {
		t.Fatalf("fixture pixel SHA-256 = %x", digest)
	}

}

func TestProjectedImageFixtureRejectsCorruption(t *testing.T) {
	// Reject a fixture whose bytes were corrupted.
	data, fixture, err := GenerateProjectedImageFixture()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Flip one byte and require validation to fail.
	corrupt := bytes.Clone(data)
	corrupt[len(corrupt)/2] ^= 1
	if err := ValidateProjectedImageFixture(corrupt, fixture); err == nil {
		t.Fatal("corrupted fixture validated")
	}

	// Require a dimensionally inconsistent fixture to fail validation.
	wrongDimensions := fixture
	wrongDimensions.Width++
	if err := ValidateProjectedImageFixture(data, wrongDimensions); err == nil {
		t.Fatal("dimensionally inconsistent fixture validated")
	}
}
