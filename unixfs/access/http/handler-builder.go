package unixfs_access_http

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	bifrost_http "github.com/aperturerobotics/bifrost/http"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/go-brotli-decoder"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/util/billyhttp"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/pkg/errors"
)

// NewHTTPHandlerBuilder constructs a HTTPHandlerBuilder function.
//
// if returnIfIdle is set and the directive becomes idle, returns ErrFsNotFound
func NewHTTPHandlerBuilder(
	b bus.Bus,
	unixFsID, unixFsPrefix string,
	httpPrefix string,
	returnIfIdle bool,
) bifrost_http.HTTPHandlerBuilder {
	return func(ctx context.Context, released func()) (http.Handler, func(), error) {
		val, valRef, err := unixfs_access.ExAccessUnixFS(ctx, b, unixFsID, returnIfIdle, released)
		if err != nil {
			return nil, nil, err
		}
		if valRef == nil {
			return nil, nil, errors.Wrap(unixfs_errors.ErrFsNotFound, unixFsID)
		}

		fsHandle, fsHandleRel, err := val(ctx, released)
		if err != nil {
			valRef.Release()
			return nil, nil, err
		}

		hfs := NewFileSystem(ctx, fsHandle, unixFsPrefix, httpPrefix)
		handler := NewFileServer(hfs)
		return handler, func() {
			fsHandleRel()
			valRef.Release()
		}, nil
	}
}

// NewFileSystem constructs a new http.FileSystem from a fsHandle.
func NewFileSystem(
	ctx context.Context,
	fsHandle *unixfs.FSHandle,
	unixFsPrefix, httpPrefix string,
) http.FileSystem {
	var billyfs billy.Filesystem = unixfs_billy.NewBillyFS(ctx, fsHandle, "", time.Time{})
	if unixFsPrefix != "" && unixFsPrefix != "/" && unixFsPrefix != "." {
		billyfs = chroot.New(billyfs, unixFsPrefix)
	}
	return billyhttp.NewFileSystem(billyfs, httpPrefix)
}

// NewFileServer builds a new http.FileServer which has extended content-type support.
func NewFileServer(hfs http.FileSystem) http.Handler {
	handler := http.FileServer(hfs)
	handlerFunc := func(rw http.ResponseWriter, req *http.Request) {
		hasSuffix := func(suffix string) bool {
			return strings.HasSuffix(req.URL.Path, suffix)
		}

		if hasSuffix(".wasm") || hasSuffix(".wasm.br") {
			rw.Header().Set("Content-Type", "application/wasm")
		}

		// fetch() does not set Accept-Encoding nor does it handle Content-Encoding.
		// https://stackoverflow.com/questions/78295701/fetch-set-accept-encoding-and-honor-content-encoding
		// If the content is brotli encoded (ends with .br) and not in Accept-Encoding let's decompress it here.
		if hasSuffix(".br") {
			acceptsBr := slices.ContainsFunc(
				strings.Split(req.Header.Get("accept-encoding"), ","),
				func(val string) bool {
					return strings.ToLower(strings.TrimSpace(val)) == "br"
				},
			)
			if !acceptsBr {
				f, err := hfs.Open(path.Clean(req.URL.Path))
				if err != nil {
					msg, code := toHTTPError(err)
					http.Error(rw, msg, code)
					return
				}

				// Omit sending the Content-Length header since we don't know the decompressed length.
				brReader := brotli.NewReader(f)
				_, err = io.Copy(rw, brReader)
				if err != nil {
					http.Error(rw, err.Error(), 500)
				}

				// done
				return
			} else {
				rw.Header().Set("Content-Encoding", "br")
			}
		}

		if rw.Header().Get("Content-Encoding") != "" {
			rw.Header().Add("Access-Control-Expose-Headers", "Content-Encoding")
			rw.Header().Add("Vary", "Content-Encoding")

			// XXX: When setting Content-Encoding, Go will drop the Content-Length header.
			// https://github.com/golang/go/issues/66735
			// https://github.com/golang/go/blob/9f136650/src/net/http/fs.go#L377
			var st fs.FileInfo
			f, err := hfs.Open(path.Clean(req.URL.Path))
			if err == nil {
				st, err = f.Stat()
			}
			if err != nil {
				msg, code := toHTTPError(err)
				http.Error(rw, msg, code)
				return
			}
			rw.Header().Add("Content-Length", strconv.Itoa(int(st.Size())))
		}

		handler.ServeHTTP(rw, req)
	}

	return http.HandlerFunc(handlerFunc)
}

// https://github.com/golang/go/blob/9f1366508/src/net/http/fs.go#L723
func toHTTPError(err error) (msg string, httpStatus int) {
	if errors.Is(err, fs.ErrNotExist) {
		return "404 page not found", http.StatusNotFound
	}
	if errors.Is(err, fs.ErrPermission) {
		return "403 Forbidden", http.StatusForbidden
	}
	// Default:
	return "500 Internal Server Error", http.StatusInternalServerError
}
