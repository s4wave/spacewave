package packedmsg

import (
	"bytes"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/aperturerobotics/util/prng"
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
	rng := prng.BuildSeededRand([]byte("los amantes"))
	rdr := prng.SourceToReader(rng)
	srcMessages := make([][]byte, 2048)

	for i := range srcMessages {
		srcMessages[i] = make([]byte, rng.Uint64()%4096)
		_, _ = io.ReadFull(rdr, srcMessages[i])
	}

	encMessages := make([]string, len(srcMessages))
	for i, msg := range srcMessages {
		encMessages[i] = EncodePackedMessage(msg)
	}

	var out strings.Builder

	for i, msg := range encMessages {
		if i != 0 {
			out.WriteString(" ")
			out.WriteString(randstring.RandString(rand.New(rng), int(rng.Uint64()%48))) //nolint:gosec
			out.WriteString(" ")
		}
		out.WriteString(msg)
		out.WriteString("\n")
	}

	outBody := out.String()
	outMessages, _ := FindPackedMessages(outBody)

	if len(outMessages) != len(srcMessages) {
		t.Fail()
	}

	for i := range outMessages {
		if !bytes.Equal(srcMessages[i], outMessages[i]) {
			t.Fail()
		}
	}
}
