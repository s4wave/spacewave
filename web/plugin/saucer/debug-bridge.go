package saucer

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	singleton_muxed_conn "github.com/aperturerobotics/bldr/util/singleton-muxed-conn"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// debugSocketName is the socket file name within the .bldr directory.
const debugSocketName = "saucer-debug.sock"

// statementPrefixes lists keywords that indicate the code contains statements
// and should not be implicitly wrapped with return.
var statementPrefixes = []string{
	"var ", "let ", "const ", "if ", "if(", "for ", "for(",
	"while ", "while(", "do ", "do{", "switch ", "switch(",
	"function ", "class ", "try ", "try{", "throw ",
	"return ", "return;", "import ", "export ", "{", "//", "/*",
}

// isExpression returns true if code looks like a single expression
// (no semicolons, single line, no statement keywords).
func isExpression(code string) bool {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" || strings.Contains(trimmed, ";") || strings.Contains(trimmed, "\n") {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range statementPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return true
}

// wrapEvalCode wraps user code in a JavaScript async IIFE that evaluates the
// code and posts the result back to C++ via window.webkit.messageHandlers.
// The C++ side replaces the __EVAL_ID__ placeholder with a unique eval ID
// before executing the code.
func wrapEvalCode(code string) string {
	body := code
	if isExpression(code) {
		body = "return (" + code + ")"
	}
	// The wrapper:
	// 1. Evaluates the code in an async IIFE.
	// 2. Serializes the result with JSON.stringify (handles undefined -> null).
	// 3. Posts the result back to C++ via the saucer message channel.
	// The __EVAL_ID__ placeholder is replaced by C++ with a unique request ID.
	// The postMessage format is: __bldr_eval:<id>:r:<result> or __bldr_eval:<id>:e:<error>
	return `(async()=>{` +
		`var __id="__EVAL_ID__";` +
		`try{` +
		`var __r=await(async()=>{` + body + `})();` +
		`var __s=__r===undefined?"undefined":JSON.stringify(__r);` +
		`if(__s===undefined)__s=String(__r);` +
		`window.webkit.messageHandlers.saucer.postMessage("__bldr_eval:"+__id+":r:"+__s);` +
		`}catch(__e){` +
		`window.webkit.messageHandlers.saucer.postMessage("__bldr_eval:"+__id+":e:"+String(__e));` +
		`}})();`
}

// debugBridge implements the SaucerDebugService over a Unix socket.
// It forwards EvalJS calls to the C++ webview via yamux streams.
type debugBridge struct {
	le *logrus.Entry
	mc *singleton_muxed_conn.SingletonMuxedConn
}

// EvalJS evaluates JavaScript code in the webview context.
func (d *debugBridge) EvalJS(ctx context.Context, req *EvalJSRequest) (*EvalJSResponse, error) {
	code := req.GetCode()
	if code == "" {
		return &EvalJSResponse{Error: "empty code"}, nil
	}

	truncated := code
	if len(truncated) > 100 {
		truncated = truncated[:100] + "..."
	}
	d.le.Debugf("EvalJS: %s", truncated)

	result, err := d.evalViaYamux(ctx, code)
	if err != nil {
		return &EvalJSResponse{Error: err.Error()}, nil
	}
	return &EvalJSResponse{Result: result}, nil
}

// evalViaYamux opens a yamux stream to C++ and sends the wrapped JS code.
// The code is wrapped in an async IIFE that posts the result back to C++ via
// the saucer message channel. C++ waits for the result and returns it as protobuf.
func (d *debugBridge) evalViaYamux(ctx context.Context, code string) (string, error) {
	wrapped := wrapEvalCode(code)

	d.le.Infof("waiting for yamux conn (closed: %v)", d.mc.IsClosed())
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	conn, err := d.mc.WaitConn(waitCtx)
	if err != nil {
		return "", fmt.Errorf("wait yamux conn: %w", err)
	}
	d.le.Infof("got yamux conn (closed: %v), opening stream", conn.IsClosed())
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		return "", fmt.Errorf("open yamux stream: %w", err)
	}
	d.le.Info("yamux stream opened, writing request")
	defer stream.Close()

	// Marshal EvalJSRequest protobuf.
	req := &EvalJSRequest{Code: wrapped}
	reqData, err := req.MarshalVT()
	if err != nil {
		return "", fmt.Errorf("marshal eval request: %w", err)
	}
	if uint64(len(reqData)) > uint64(MaxFrameSize) {
		return "", fmt.Errorf("request too large: %d bytes (max %d)", len(reqData), MaxFrameSize)
	}

	// Write length-prefixed protobuf request.
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(reqData))) // #nosec G115 -- bounded by MaxFrameSize check above
	if _, err := stream.Write(lenBuf); err != nil {
		return "", fmt.Errorf("write request length: %w", err)
	}
	if _, err := stream.Write(reqData); err != nil {
		return "", fmt.Errorf("write request: %w", err)
	}
	d.le.Info("request written, reading response")

	// Read length-prefixed protobuf response from C++.
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		return "", fmt.Errorf("read response length: %w", err)
	}
	respLen := binary.LittleEndian.Uint32(lenBuf)
	if respLen > MaxFrameSize {
		return "", fmt.Errorf("response too large: %d", respLen)
	}
	respData := make([]byte, respLen)
	if _, err := io.ReadFull(stream, respData); err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Unmarshal the EvalJSResponse protobuf.
	var resp EvalJSResponse
	if err := resp.UnmarshalVT(respData); err != nil {
		return "", fmt.Errorf("unmarshal eval response: %w", err)
	}
	if resp.GetError() != "" {
		return "", fmt.Errorf("%s", resp.GetError())
	}
	return resp.GetResult(), nil
}

// runDebugSocket starts the debug bridge Unix socket listener.
// Blocks until ctx is canceled.
func runDebugSocket(ctx context.Context, le *logrus.Entry, mc *singleton_muxed_conn.SingletonMuxedConn, workdir string) error {
	mux := srpc.NewMux()
	svc := &debugBridge{le: le, mc: mc}
	if err := SRPCRegisterSaucerDebugService(mux, svc); err != nil {
		return err
	}

	sockDir := filepath.Join(workdir, ".bldr")
	if err := os.MkdirAll(sockDir, 0755); err != nil {
		return fmt.Errorf("create .bldr dir: %w", err)
	}
	sockPath := filepath.Join(sockDir, debugSocketName)

	// Remove stale socket.
	_ = os.Remove(sockPath)

	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix: %w", err)
	}
	defer func() {
		lis.Close()
		_ = os.Remove(sockPath)
	}()

	if err := os.Chmod(sockPath, 0600); err != nil {
		le.WithError(err).Warn("failed to chmod socket")
	}

	le.Infof("debug bridge listening on %s", sockPath)

	// Close listener when context is canceled.
	go func() {
		<-ctx.Done()
		lis.Close()
	}()

	srv := srpc.NewServer(mux)
	return srpc.AcceptMuxedListener(ctx, lis, srv, nil)
}

// _ is a type assertion
var _ SRPCSaucerDebugServiceServer = ((*debugBridge)(nil))
