package pagestore

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestLegacyPageFixtureRemainsWireCompatible(t *testing.T) {
	const legacyHex = "" +
		"020001f5967b3300000300056b657976616c7565000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000"
	legacy, err := hex.DecodeString(legacyHex)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := DecodeLeafPage(legacy)
	if err != nil {
		t.Fatalf("decode legacy page: %v", err)
	}
	if len(entries) != 1 || string(entries[0].Key) != "key" || string(entries[0].Value) != "value" {
		t.Fatalf("legacy entries = %#v", entries)
	}

	current := make([]byte, len(legacy))
	if n := EncodeLeafPage(current, []LeafEntry{{Key: []byte("key"), Value: []byte("value")}}); n != 1 {
		t.Fatalf("encoded entries = %d, want 1", n)
	}
	if !bytes.Equal(current, legacy) {
		t.Fatalf("current page encoding changed:\n got %x\nwant %x", current, legacy)
	}

	corrupt := bytes.Clone(legacy)
	corrupt[PageHeaderSize+4] ^= 0xff
	if err := ValidatePage(corrupt); err == nil {
		t.Fatal("corrupt page body accepted")
	}
	corrupt = bytes.Clone(legacy)
	corrupt[3] ^= 0xff
	if err := ValidatePage(corrupt); err == nil {
		t.Fatal("corrupt page checksum accepted")
	}
}

func TestLegacySuperblockFixtureRemainsWireCompatible(t *testing.T) {
	const legacyHex = "5053534200010000000000000000002a0000000a000000140000006456447967"
	legacy, err := hex.DecodeString(legacyHex)
	if err != nil {
		t.Fatal(err)
	}

	sb, err := DecodeSuperblock(legacy)
	if err != nil {
		t.Fatalf("decode legacy superblock: %v", err)
	}
	if sb.Generation != 42 || sb.RootPage != 10 || sb.FreelistPage != 20 || sb.PageCount != 100 {
		t.Fatalf("legacy superblock = %#v", sb)
	}

	current := make([]byte, SuperblockSize)
	EncodeSuperblock(current, sb)
	if !bytes.Equal(current, legacy) {
		t.Fatalf("current superblock encoding changed:\n got %x\nwant %x", current, legacy)
	}

	corrupt := bytes.Clone(legacy)
	corrupt[15] ^= 0xff
	if _, err := DecodeSuperblock(corrupt); err == nil {
		t.Fatal("corrupt superblock accepted")
	}
}
