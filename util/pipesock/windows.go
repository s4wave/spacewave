//go:build windows
// +build windows

package pipesock

import (
	"context"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// BuildPipeListener builds the pipe listener in the directory.
func BuildPipeListener(le *logrus.Entry, rootDir, pipeUuid string) (net.Listener, error) {
	pipeName := BuildPipeName(rootDir, pipeUuid)
	le.Debugf("listening on winio pipe: %s", pipeName)
	return winio.ListenPipe(pipeName, nil)
}

// DialPipeListener connects to the pipe listener in the directory.
func DialPipeListener(ctx context.Context, le *logrus.Entry, rootDir, pipeUuid string) (net.Conn, error) {
	pipeName := BuildPipeName(rootDir, pipeUuid)
	le.Debugf("connecting to winio pipe: %s", pipeName)
	return winio.DialPipeContext(ctx, pipeName)
}

// BuildPipeName builds a unique pipe name from a path and uuid.
// uuid must be unique for rootDir
func BuildPipeName(rootDir, pipeUuid string) string {
	material := strings.Join([]string{rootDir, "uuid", pipeUuid}, "!")
	var key [32]byte
	blake3.DeriveKey("bldr pipesock windows Tue Apr 11 01:33:30 PM PDT 2023", []byte(material), key[:])
	keyStr := b58.Encode(key[:])
	return `\\.\pipe\bldr\` + keyStr
}
