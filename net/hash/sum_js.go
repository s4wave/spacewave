//go:build js && !goscript

package hash

import (
	"crypto/sha1" //nolint:gosec // Git object storage requires SHA-1.
	"crypto/sha256"
	"runtime"
	"sync"
	"syscall/js"

	"github.com/zeebo/blake3"
)

// subtleCryptoDigestMinSize is the measured browser wasm crossover where
// crypto.subtle.digest starts matching or beating wasm SHA code.
const subtleCryptoDigestMinSize = 12 * 1024

func sumHashType(h HashType, data []byte) ([]byte, error) {
	switch h {
	case HashType_HashType_SHA256:
		if runtime.Compiler == "tinygo" || len(data) < subtleCryptoDigestMinSize {
			h := sha256.Sum256(data)
			return h[:], nil
		}
		return subtleCryptoDigest("SHA-256", data)
	case HashType_HashType_SHA1:
		if runtime.Compiler == "tinygo" || len(data) < subtleCryptoDigestMinSize {
			h := sha1.Sum(data) //nolint:gosec
			return h[:], nil
		}
		return subtleCryptoDigest("SHA-1", data)
	case HashType_HashType_BLAKE3:
		h := blake3.Sum256(data)
		return h[:], nil
	default:
		return nil, newUnsupportedHashTypeError(h, "hash type unknown: "+h.String())
	}
}

func subtleCryptoDigest(name string, data []byte) ([]byte, error) {
	crypto := js.Global().Get("crypto")
	if crypto.IsUndefined() || crypto.IsNull() {
		return nil, errors.New("SubtleCrypto is unavailable")
	}
	subtle := crypto.Get("subtle")
	if subtle.IsUndefined() || subtle.IsNull() {
		return nil, errors.New("SubtleCrypto is unavailable")
	}

	input := js.Global().Get("Uint8Array").New(len(data))
	if n := js.CopyBytesToJS(input, data); n != len(data) {
		return nil, errors.Errorf("copied %d of %d bytes to js", n, len(data))
	}

	digest, err := awaitPromise(subtle.Call("digest", name, input))
	if err != nil {
		return nil, err
	}

	output := js.Global().Get("Uint8Array").New(digest)
	out := make([]byte, output.Get("length").Int())
	if n := js.CopyBytesToGo(out, output); n != len(out) {
		return nil, errors.Errorf("copied %d of %d bytes from js", n, len(out))
	}
	return out, nil
}

func awaitPromise(promise js.Value) (js.Value, error) {
	ch := make(chan struct{})
	var once sync.Once
	var result js.Value
	var jsErr error

	resolveCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			result = args[0]
		} else {
			result = js.Undefined()
		}
		once.Do(func() { close(ch) })
		return nil
	})
	defer resolveCb.Release()

	rejectCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			jsErr = errors.Errorf("SubtleCrypto digest rejected: %s", args[0].Call("toString").String())
		} else {
			jsErr = errors.New("SubtleCrypto digest rejected")
		}
		once.Do(func() { close(ch) })
		return nil
	})
	defer rejectCb.Release()

	promise.Call("then", resolveCb).Call("catch", rejectCb)
	<-ch

	return result, jsErr
}
