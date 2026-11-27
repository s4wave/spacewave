//go:build js

package main

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"strings"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// pipeReadResult contains the bytes drained and terminal error from a pipe
// reader.
type pipeReadResult struct {
	// n counts the bytes drained before termination.
	n int
	// err is the terminal pipe error, excluding normal EOF.
	err error
}

// runPipeWriteLoop checks deterministic streaming through a Go pipe.
func runPipeWriteLoop(c *config) error {
	// Drain the pipe in the background, counting the bytes read.
	totalSize := c.iterations
	if totalSize <= 0 {
		totalSize = 4 * 1024 * 1024
	}
	pr, pw := io.Pipe()
	done := make(chan pipeReadResult, 1)
	go func() {
		buf := make([]byte, 32*1024)
		var total int
		for {
			n, err := pr.Read(buf)
			total += n
			if err == io.EOF {
				done <- pipeReadResult{n: total}
				return
			}
			if err != nil {
				done <- pipeReadResult{n: total, err: err}
				return
			}
		}
	}()

	// Stream deterministic chunks into the pipe, reporting progress.
	postProgress(c, "pipe-write-start", 0, totalSize)
	const chunkSize = 64 * 1024
	const progressEvery = 1024 * 1024
	for offset := 0; offset < totalSize; offset += chunkSize {
		n := min(chunkSize, totalSize-offset)
		written, err := pw.Write(deterministicLargeWindow(offset, n, 0))
		if err != nil {
			return errors.Wrapf(err, "pipe write offset=%d", offset)
		}
		if written != n {
			return errors.Errorf("pipe write offset=%d wrote=%d want=%d", offset, written, n)
		}
		next := offset + n
		if next == totalSize || next%progressEvery == 0 {
			postProgress(c, "pipe-write-stream", next, totalSize)
		}
	}

	// Close the writer and check the reader drained every byte.
	postProgress(c, "pipe-close-start", totalSize, totalSize)
	if err := pw.Close(); err != nil {
		return errors.Wrap(err, "pipe close")
	}
	res := <-done
	if res.err != nil {
		return errors.Wrap(res.err, "pipe read")
	}
	if res.n != totalSize {
		return errors.Errorf("pipe read=%d want=%d", res.n, totalSize)
	}
	postProgress(c, "pipe-close-complete", totalSize, totalSize)
	return nil
}

// runSRPCEchoLoop checks the echo contract over a real multiplexed connection.
func runSRPCEchoLoop(ctx context.Context, c *config) error {
	// Serve the echo service over an in-memory multiplexed connection.
	serverMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(serverMux); err != nil {
		return errors.Wrap(err, "register echo server")
	}
	conn, cleanup, err := serveMuxedPipe(ctx, serverMux)
	if err != nil {
		return err
	}

	// Run the echo loop against the served service, then join the server.
	client := echo.NewSRPCEchoerClient(conn)
	return stderrors.Join(runEchoClientLoop(ctx, c, client, "srpc-echo-loop"), cleanup())
}

// runSRPCRpcStreamEchoLoop checks echo calls through a nested RPC stream.
func runSRPCRpcStreamEchoLoop(ctx context.Context, c *config) error {
	// Serve an echo service whose RPC stream forwards to an inner echo service.
	innerMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(innerMux); err != nil {
		return errors.Wrap(err, "register inner echo server")
	}
	serverMux := srpc.NewMux()
	if err := echo.NewEchoServer(innerMux).Register(serverMux); err != nil {
		return errors.Wrap(err, "register outer echo server")
	}
	conn, cleanup, err := serveMuxedPipe(ctx, serverMux)
	if err != nil {
		return err
	}

	// Run the echo loop against the inner service through the outer stream,
	// then join the server.
	outerClient := echo.NewSRPCEchoerClient(conn)
	nestedClient := rpcstream.NewRpcStreamClient(
		func(ctx context.Context) (echo.SRPCEchoer_RpcStreamClient, error) {
			return outerClient.RpcStream(ctx)
		},
		"echo",
		true,
	)
	client := echo.NewSRPCEchoerClient(nestedClient)
	return stderrors.Join(runEchoClientLoop(ctx, c, client, "srpc-rpcstream-echo-loop"), cleanup())
}

// serveMuxedPipe serves mux on one end of an in-memory multiplexed connection
// and returns a client for the other end. The cleanup stops the server, closes
// both ends, and returns the server's error unless it is a normal close.
func serveMuxedPipe(ctx context.Context, mux srpc.Mux) (srpc.Client, func() error, error) {
	// Open both multiplexed ends of the pipe.
	clientPipe, serverPipe := net.Pipe()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
		return nil, nil, errors.Wrap(err, "open client muxed conn")
	}
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		_ = clientMp.Close()
		_ = serverPipe.Close()
		return nil, nil, errors.Wrap(err, "open server muxed conn")
	}

	// Serve the mux until the cleanup cancels it.
	serverCtx, cancelServer := context.WithCancel(ctx)
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srpc.NewServer(mux).AcceptMuxedConn(serverCtx, serverMp)
	}()
	cleanup := func() error {
		// Stop the server and join it.
		cancelServer()
		_ = clientMp.Close()
		_ = serverMp.Close()
		if err := <-serverErrCh; err != nil && !isExpectedMuxCloseError(err) {
			return errors.Wrap(err, "server mux")
		}
		return nil
	}
	return srpc.NewClientWithMuxedConn(clientMp), cleanup, nil
}

// runEchoClientLoop verifies repeated echo responses and reports stream progress.
func runEchoClientLoop(
	ctx context.Context,
	c *config,
	client echo.SRPCEchoerClient,
	phase string,
) error {
	// Default the call count and payload size.
	iterations := c.iterations
	if iterations <= 0 {
		iterations = 128
	}
	payloadSize := c.batch
	if payloadSize <= 0 {
		payloadSize = 4096
	}

	// Echo the payload repeatedly and check each response.
	body := strings.Repeat("x", payloadSize)
	postProgress(c, phase+"-start", 0, iterations)
	for i := range iterations {
		resp, err := client.Echo(ctx, &echo.EchoMsg{Body: body})
		if err != nil {
			return errors.Wrapf(err, "echo call %d", i)
		}
		if resp.GetBody() != body {
			return errors.Errorf("echo call %d body len=%d want=%d", i, len(resp.GetBody()), len(body))
		}
		next := i + 1
		if next == 1 || next == iterations || next%16 == 0 {
			postProgress(c, phase+"-stream", next, iterations)
		}
	}
	postProgress(c, phase+"-complete", iterations, iterations)
	return nil
}

// runResourceEchoLoop checks echo calls through the resource reference lifecycle.
func runResourceEchoLoop(ctx context.Context, c *config) error {
	// Serve the echo service as the root resource.
	rootMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(rootMux); err != nil {
		return errors.Wrap(err, "register root echo server")
	}
	resClient, cleanup, err := openResourceClient(ctx, rootMux)
	if err != nil {
		return err
	}
	defer cleanup()

	// Run the echo loop through a reference to the root resource.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get root resource client")
	}
	client := echo.NewSRPCEchoerClient(rootClient)
	return runEchoClientLoop(ctx, c, client, "resource-echo-loop")
}

// openResourceClient returns a live resource client for rootMux and a cleanup
// that joins its server.
func openResourceClient(
	ctx context.Context,
	rootMux srpc.Mux,
) (*resource_client.Client, func() error, error) {
	// Serve the resource service over an in-memory connection.
	serverMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux, "").Register(serverMux); err != nil {
		return nil, nil, errors.Wrap(err, "register resource server")
	}
	conn, closeConn, err := serveMuxedPipe(ctx, serverMux)
	if err != nil {
		return nil, nil, err
	}

	// Open the resource client on the connection.
	resClient, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(conn))
	if err != nil {
		_ = closeConn()
		return nil, nil, errors.Wrap(err, "open resource client")
	}
	return resClient, func() error {
		resClient.Release()
		return closeConn()
	}, nil
}

// isExpectedMuxCloseError recognizes the normal transport termination errors.
func isExpectedMuxCloseError(err error) bool {
	return stderrors.Is(err, context.Canceled) ||
		stderrors.Is(err, io.EOF) ||
		stderrors.Is(err, io.ErrClosedPipe) ||
		stderrors.Is(err, net.ErrClosed)
}
