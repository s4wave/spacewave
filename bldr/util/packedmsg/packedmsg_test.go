package packedmsg

import (
	"bytes"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/aperturerobotics/util/prng"
	"github.com/s4wave/spacewave/net/util/randstring"
)

var testMessage = []byte("Science isn't about WHY, it's about WHY NOT!")

func TestChecksum(t *testing.T) {
	body := testMessage
	in := wrapChecksum(body)
	out, ok := unwrapChecksum(in)
	if !ok || !bytes.Equal(out, body) {
		t.Fail()
	}
}

func TestPackedMessage(t *testing.T) {
	// Round-trip a packed message after adding surrounding whitespace.
	body := testMessage
	encoded := EncodePackedMessage(body)
	t.Log(encoded)
	encoded = "\t\n        " + encoded + "\t\t\t\t\n"
	decoded, ok := DecodePackedMessage(encoded)
	if !ok || !bytes.Equal(decoded, body) {
		t.Fail()
	}
}

func TestFindPackedMessages(t *testing.T) {
	// Seed the source of reproducible messages for the finder test.
	rng := prng.BuildSeededRand([]byte("los amantes"))
	rdr := prng.SourceToReader(rng)
	srcMessages := make([][]byte, 2048)

	// Generate random payloads for the packed-message collection.
	for i := range srcMessages {
		srcMessages[i] = make([]byte, rng.Uint64()%4096)
		_, _ = io.ReadFull(rdr, srcMessages[i])
	}

	// Encode every generated payload for the text body.
	encMessages := make([]string, len(srcMessages))
	for i, msg := range srcMessages {
		encMessages[i] = EncodePackedMessage(msg)
	}

	// Build a text body containing encoded messages and random separators.
	var out strings.Builder

	// Write each encoded message with a separator into the body.
	for i, msg := range encMessages {
		if i != 0 {
			out.WriteString(" ")
			out.WriteString(randstring.RandString(rand.New(rng), int(rng.Uint64()%48))) //nolint:gosec
			out.WriteString(" ")
		}
		out.WriteString(msg)
		out.WriteString("\n")
	}

	// Extract packed messages from the generated text body.
	outBody := out.String()
	outMessages, _ := FindPackedMessages(outBody)

	// Verify that every source message was found.
	if len(outMessages) != len(srcMessages) {
		t.Fail()
	}

	// Compare each extracted payload with its original message.
	for i := range outMessages {
		if !bytes.Equal(srcMessages[i], outMessages[i]) {
			t.Fail()
		}
	}
}
