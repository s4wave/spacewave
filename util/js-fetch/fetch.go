//go:build js

package fetch

import (
	"context"
	"errors"
	"io"
	"syscall/js"

	"golang.org/x/exp/maps"
)

// Opts are the options for Fetch.
type Opts struct {
	// Method is the http verb (constants are copied from net/http to avoid import)
	Method string

	// Headers is a map of http headers to send.
	Headers map[string]string

	// Body is the body request
	Body io.Reader

	// Mode docs https://developer.mozilla.org/en-US/docs/Web/API/Request/mode
	Mode string

	// Credentials docs https://developer.mozilla.org/en-US/docs/Web/API/Request/credentials
	Credentials string

	// Cache docs https://developer.mozilla.org/en-US/docs/Web/API/Request/cache
	Cache string

	// Redirect docs https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/fetch
	Redirect string

	// Referrer docs https://developer.mozilla.org/en-US/docs/Web/API/Request/referrer
	Referrer string

	// ReferrerPolicy docs https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/fetch
	ReferrerPolicy string

	// Integrity docs https://developer.mozilla.org/en-US/docs/Web/Security/Subresource_Integrity
	Integrity string

	// KeepAlive docs https://developer.mozilla.org/en-US/docs/Web/API/WindowOrWorkerGlobalScope/fetch
	KeepAlive *bool

	// Signal docs https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal
	Signal context.Context
}

// Clone clones the opts, excluding the Body field.
func (o *Opts) Clone() *Opts {
	if o == nil {
		return nil
	}

	return &Opts{
		Method:         o.Method,
		Headers:        maps.Clone(o.Headers),
		Mode:           o.Mode,
		Credentials:    o.Credentials,
		Cache:          o.Cache,
		Redirect:       o.Redirect,
		Referrer:       o.Referrer,
		ReferrerPolicy: o.ReferrerPolicy,
		Integrity:      o.Integrity,
		KeepAlive:      o.KeepAlive,
		Signal:         o.Signal,
	}
}

// Response is the response that returns from the fetch promise.
type Response struct {
	Headers    Header
	OK         bool
	Redirected bool
	Status     int
	StatusText string
	Type       string
	URL        string
	Body       []byte
	BodyUsed   bool
}

// Fetch uses the JS Fetch API to make requests.
func Fetch(url string, opts *Opts) (*Response, error) {
	optsMap, err := mapOpts(opts)
	if err != nil {
		return nil, err
	}

	type fetchResponse struct {
		r *Response
		b js.Value
		e error
	}
	ch := make(chan *fetchResponse)
	if opts.Signal != nil {
		controller := js.Global().Get("AbortController").New()
		signal := controller.Get("signal")
		optsMap["signal"] = signal
		abort := func() {
			controller.Call("abort")
		}
		defer abort()
		defer context.AfterFunc(opts.Signal, abort)()
	}

	success := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		r := new(Response)
		resp := args[0]
		headersIt := resp.Get("headers").Call("entries")
		headers := Header{}
		for {
			n := headersIt.Call("next")
			if n.Get("done").Bool() {
				break
			}
			pair := n.Get("value")
			key, value := pair.Index(0).String(), pair.Index(1).String()
			headers.Add(key, value)
		}
		r.Headers = headers
		r.OK = resp.Get("ok").Bool()
		r.Redirected = resp.Get("redirected").Bool()
		r.Status = resp.Get("status").Int()
		r.StatusText = resp.Get("statusText").String()
		r.Type = resp.Get("type").String()
		r.URL = resp.Get("url").String()
		r.BodyUsed = resp.Get("bodyUsed").Bool()

		ch <- &fetchResponse{r: r, b: resp}
		return nil
	})

	defer success.Release()

	failure := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		msg := args[0].Get("message").String()
		ch <- &fetchResponse{e: errors.New(msg)}
		return nil
	})
	defer failure.Release()

	go js.Global().Call("fetch", url, optsMap).Call("then", success).Call("catch", failure)

	r := <-ch
	if r.e != nil {
		return nil, r.e
	}

	successBody := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		// Wrap the input ArrayBuffer with a Uint8Array
		uint8arrayWrapper := js.Global().Get("Uint8Array").New(args[0])
		r.r.Body = make([]byte, uint8arrayWrapper.Get("byteLength").Int())
		js.CopyBytesToGo(r.r.Body, uint8arrayWrapper)
		ch <- r
		return nil
	})
	defer successBody.Release()

	failureBody := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		// Assumes it's a TypeError. See
		// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/TypeError
		// for more information on this type.
		// See https://fetch.spec.whatwg.org/#concept-body-consume-body for error causes.
		msg := args[0].Get("message").String()
		ch <- &fetchResponse{e: errors.New(msg)}
		return nil
	})
	defer failureBody.Release()

	go r.b.Call("arrayBuffer").Call("then", successBody, failureBody)

	r = <-ch
	return r.r, r.e
}

// oof.
func mapOpts(opts *Opts) (map[string]interface{}, error) {
	mp := map[string]interface{}{}

	if opts.Method != "" {
		mp["method"] = opts.Method
	}
	if opts.Headers != nil {
		mp["headers"] = mapHeaders(opts.Headers)
	}
	if opts.Mode != "" {
		mp["mode"] = opts.Mode
	}
	if opts.Credentials != "" {
		mp["credentials"] = opts.Credentials
	}
	if opts.Cache != "" {
		mp["cache"] = opts.Cache
	}
	if opts.Redirect != "" {
		mp["redirect"] = opts.Redirect
	}
	if opts.Referrer != "" {
		mp["referrer"] = opts.Referrer
	}
	if opts.ReferrerPolicy != "" {
		mp["referrerPolicy"] = opts.ReferrerPolicy
	}
	if opts.Integrity != "" {
		mp["integrity"] = opts.Integrity
	}
	if opts.KeepAlive != nil {
		mp["keepalive"] = *opts.KeepAlive
	}

	if opts.Body != nil {
		bts, err := io.ReadAll(opts.Body)
		if err != nil {
			return nil, err
		}

		mp["body"] = string(bts)
	}

	return mp, nil
}

func mapHeaders(mp map[string]string) map[string]interface{} {
	newMap := map[string]interface{}{}
	for k, v := range mp {
		newMap[k] = v
	}
	return newMap
}
