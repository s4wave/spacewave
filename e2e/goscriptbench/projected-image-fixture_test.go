//go:build !js

package goscriptbench

import (
	"bytes"
	"testing"
)

func TestProjectedImageFixtureDeterministic(t *testing.T) {
	firstData, firstFixture, err := GenerateProjectedImageFixture()
	if err != nil {
		t.Fatal(err.Error())
	}
	secondData, secondFixture, err := GenerateProjectedImageFixture()
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(firstData, secondData) {
		t.Fatal("fixture bytes changed between generations")
	}
	if firstFixture != secondFixture {
		t.Fatalf("fixture metadata changed: %+v != %+v", firstFixture, secondFixture)
	}
	if firstFixture.Width != ProjectedImageWidth || firstFixture.Height != ProjectedImageHeight {
		t.Fatalf("fixture dimensions = %dx%d", firstFixture.Width, firstFixture.Height)
	}
	if firstFixture.EncodedBytes != 4_198_217 {
		t.Fatalf("fixture size = %d bytes, want 4198217", firstFixture.EncodedBytes)
	}
	if firstFixture.SHA256 != "3470475e663fab4c571c4c1f3857c5bcad4902ad1058ed4cfae96b3bf5127724" {
		t.Fatalf("fixture SHA-256 = %s", firstFixture.SHA256)
	}
}

func TestProjectedImageFixtureRejectsCorruption(t *testing.T) {
	data, fixture, err := GenerateProjectedImageFixture()
	if err != nil {
		t.Fatal(err.Error())
	}
	corrupt := bytes.Clone(data)
	corrupt[len(corrupt)/2] ^= 1
	if err := ValidateProjectedImageFixture(corrupt, fixture); err == nil {
		t.Fatal("corrupted fixture validated")
	}

	wrongDimensions := fixture
	wrongDimensions.Width++
	if err := ValidateProjectedImageFixture(data, wrongDimensions); err == nil {
		t.Fatal("dimensionally inconsistent fixture validated")
	}
}
