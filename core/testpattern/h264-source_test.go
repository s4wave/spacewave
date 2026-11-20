package testpattern

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/frametrack"
)

// TestH264SourceLoop checks AVC framing, the keyframe cadence, and playback
// across the clip boundary.
func TestH264SourceLoop(t *testing.T) {
	// Open the embedded source and require one second between keyframes.
	source := NewH264Source()
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})
	if source.CanRequestKeyframe() {
		t.Fatal("an embedded clip cannot make an early keyframe")
	}

	var first []byte
	var previous time.Time
	for n := range 61 {
		// Read through both parts of the clip and its first repeated picture.
		header, frame, err := source.ReadFrame(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if want := n%30 == 0; frame.GetKeyframe() != want || (header != nil) != want {
			t.Fatalf("frame %d keyframe = %v, header = %v, want keyframe = %v", n, frame.GetKeyframe(), header, want)
		}
		captured := frame.GetCapturedAt().AsTime()
		if !captured.After(previous) {
			t.Fatalf("frame %d did not advance capture time", n)
		}
		previous = captured

		// Check the description on each independently decoded group.
		if header != nil {
			desc := header.GetDecoderDescription()
			if header.GetCodec() != frametrack.Codec_CODEC_H264_AVC ||
				header.GetWidth() != Width || header.GetHeight() != Height ||
				len(desc) < 7 || desc[0] != 1 || desc[4]&3 != 3 || desc[5]&31 == 0 {
				t.Fatalf("frame %d has invalid AVC configuration: %v", n, header)
			}
		}

		// Length-prefixed AVC has no start codes between its NAL units.
		data := frame.GetData()
		for remaining := data; len(remaining) != 0; {
			if len(remaining) < 4 {
				t.Fatalf("frame %d ends inside a NAL length field", n)
			}
			size := int(binary.BigEndian.Uint32(remaining[:4]))
			if size == 0 || size > len(remaining)-4 {
				t.Fatalf("frame %d has a NAL of size %d in %d bytes", n, size, len(remaining)-4)
			}
			remaining = remaining[4+size:]
		}
		if len(data) == 0 {
			t.Fatalf("frame %d has no AVC data", n)
		}

		// The loop starts at the exact same immutable access unit.
		if n == 0 {
			first = bytes.Clone(data)
		}
		if n == 60 && !bytes.Equal(data, first) {
			t.Fatal("clip did not wrap to its opening keyframe")
		}
	}

	// A closed source ends without waiting for another tick.
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := source.ReadFrame(t.Context()); err != io.EOF {
		t.Fatalf("closed source returned %v, want EOF", err)
	}
}
