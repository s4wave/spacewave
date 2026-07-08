//go:build !goscript

package blake3_test

import (
	"bytes"
	"io"
	"testing"

	owner "github.com/s4wave/spacewave/net/crypto/blake3"
	zeebo "github.com/zeebo/blake3"
)

func TestSumFunctionsMatchZeebo(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "short", data: []byte("Spacewave BLAKE3 owner package")},
		{name: "multi-block", data: deterministicBytes(4099)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := owner.Sum256(tc.data), zeebo.Sum256(tc.data); got != want {
				t.Fatalf("Sum256() = %x, want zeebo %x", got, want)
			}
			if got, want := owner.Sum512(tc.data), zeebo.Sum512(tc.data); got != want {
				t.Fatalf("Sum512() = %x, want zeebo %x", got, want)
			}
		})
	}
}

func TestDeriveKeyVariableOutputMatchesZeebo(t *testing.T) {
	cases := []struct {
		name     string
		context  string
		material []byte
		outLen   int
	}{
		{name: "empty-output", context: "spacewave:empty", material: []byte("material"), outLen: 0},
		{name: "one-byte", context: "spacewave:one", material: []byte("material"), outLen: 1},
		{name: "digest-sized", context: "spacewave:digest", material: deterministicBytes(37), outLen: 32},
		{name: "wide-output", context: "spacewave:wide", material: deterministicBytes(257), outLen: 131},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := make([]byte, tc.outLen)
			want := make([]byte, tc.outLen)
			owner.DeriveKey(tc.context, tc.material, got)
			zeebo.DeriveKey(tc.context, tc.material, want)
			requireBytesEqual(t, got, want, "DeriveKey(%q, material, %d-byte out)", tc.context, tc.outLen)
		})
	}
}

func TestStreamingHasherMatchesZeebo(t *testing.T) {
	got := owner.New()
	want := zeebo.New()

	writeBytesToBoth(t, got, want, []byte("prefix:"))
	writeStringToBoth(t, got, want, "snowman=☃;")
	writeBytesToBoth(t, got, want, deterministicBytes(1025))

	prefix := []byte("caller-prefix:")
	requireBytesEqual(t, got.Sum(bytes.Clone(prefix)), want.Sum(bytes.Clone(prefix)), "Sum with caller prefix")

	writeStringToBoth(t, got, want, ":after-sum")
	requireBytesEqual(t, got.Sum(nil), want.Sum(nil), "Sum after prior Sum and more writes")
}

func TestDigestReadAndSeekMatchZeebo(t *testing.T) {
	gotHasher := owner.New()
	wantHasher := zeebo.New()
	writeBytesToBoth(t, gotHasher, wantHasher, []byte("digest-stream:"))
	writeStringToBoth(t, gotHasher, wantHasher, "seek/read/☃")
	writeBytesToBoth(t, gotHasher, wantHasher, deterministicBytes(2049))

	got := gotHasher.Digest()
	want := wantHasher.Digest()

	readDigestSame(t, got, want, "initial 17 bytes", 17)
	readDigestSame(t, got, want, "next 50 bytes", 50)
	seekDigestSame(t, got, want, "absolute seek", 8, io.SeekStart)
	readDigestSame(t, got, want, "from absolute offset", 64)
	seekDigestSame(t, got, want, "relative seek backward", -10, io.SeekCurrent)
	readDigestSame(t, got, want, "after relative seek", 32)
	seekDigestSame(t, got, want, "negative absolute seek", -1, io.SeekStart)
	seekDigestSame(t, got, want, "seek from end", 0, io.SeekEnd)
	seekDigestSame(t, got, want, "invalid whence", 0, 99)
}

func TestNewKeyedMatchesZeeboAndRejectsInvalidKeySizes(t *testing.T) {
	validKey := deterministicBytes(32)
	got, gotErr := owner.NewKeyed(validKey)
	want, wantErr := zeebo.NewKeyed(validKey)
	requireSameError(t, gotErr, wantErr, "NewKeyed(valid 32-byte key)")
	if got == nil || want == nil {
		t.Fatalf("NewKeyed(valid 32-byte key) returned got=%v want=%v", got, want)
	}

	writeStringToBoth(t, got, want, "keyed input")
	writeBytesToBoth(t, got, want, deterministicBytes(333))
	requireBytesEqual(t, got.Sum(nil), want.Sum(nil), "NewKeyed Sum")
	readDigestSame(t, got.Digest(), want.Digest(), "NewKeyed Digest", 96)

	for _, tc := range []struct {
		name string
		key  []byte
	}{
		{name: "empty", key: nil},
		{name: "short", key: deterministicBytes(31)},
		{name: "long", key: deterministicBytes(33)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := owner.NewKeyed(tc.key)
			want, wantErr := zeebo.NewKeyed(tc.key)
			requireSameError(t, gotErr, wantErr, "NewKeyed(%d-byte key)", len(tc.key))
			if got != nil || want != nil {
				t.Fatalf("NewKeyed(%d-byte key) returned got=%v want=%v", len(tc.key), got, want)
			}
		})
	}
}

type stringWriterHasher interface {
	Write([]byte) (int, error)
	WriteString(string) (int, error)
}

type digestReaderSeeker interface {
	Read([]byte) (int, error)
	Seek(int64, int) (int64, error)
}

func writeBytesToBoth(t *testing.T, got, want stringWriterHasher, p []byte) {
	t.Helper()
	gotN, gotErr := got.Write(p)
	wantN, wantErr := want.Write(p)
	requireSameError(t, gotErr, wantErr, "Write(%d bytes)", len(p))
	if gotN != wantN {
		t.Fatalf("Write(%d bytes) returned n=%d, want zeebo n=%d", len(p), gotN, wantN)
	}
}

func writeStringToBoth(t *testing.T, got, want stringWriterHasher, p string) {
	t.Helper()
	gotN, gotErr := got.WriteString(p)
	wantN, wantErr := want.WriteString(p)
	requireSameError(t, gotErr, wantErr, "WriteString(%q)", p)
	if gotN != wantN {
		t.Fatalf("WriteString(%q) returned n=%d, want zeebo n=%d", p, gotN, wantN)
	}
}

func readDigestSame(t *testing.T, got, want digestReaderSeeker, label string, size int) {
	t.Helper()
	gotBuf := make([]byte, size)
	wantBuf := make([]byte, size)
	gotN, gotErr := got.Read(gotBuf)
	wantN, wantErr := want.Read(wantBuf)
	requireSameError(t, gotErr, wantErr, "%s Read(%d bytes)", label, size)
	if gotN != wantN {
		t.Fatalf("%s Read(%d bytes) returned n=%d, want zeebo n=%d", label, size, gotN, wantN)
	}
	requireBytesEqual(t, gotBuf[:gotN], wantBuf[:wantN], "%s Read(%d bytes)", label, size)
}

func seekDigestSame(t *testing.T, got, want digestReaderSeeker, label string, offset int64, whence int) {
	t.Helper()
	gotOffset, gotErr := got.Seek(offset, whence)
	wantOffset, wantErr := want.Seek(offset, whence)
	requireSameError(t, gotErr, wantErr, "%s Seek(%d, %d)", label, offset, whence)
	if gotOffset != wantOffset {
		t.Fatalf("%s Seek(%d, %d) returned offset=%d, want zeebo offset=%d", label, offset, whence, gotOffset, wantOffset)
	}
}

func requireBytesEqual(t *testing.T, got, want []byte, format string, args ...any) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf(format+" = %x, want zeebo %x", append(args, got, want)...)
	}
}

func requireSameError(t *testing.T, got, want error, format string, args ...any) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil || want == nil {
		t.Fatalf(format+" error = %v, want zeebo %v", append(args, got, want)...)
	}
	if got.Error() != want.Error() {
		t.Fatalf(format+" error = %q, want zeebo %q", append(args, got.Error(), want.Error())...)
	}
}

func deterministicBytes(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte((i*37 + i/5 + 11) & 0xff)
	}
	return out
}
