//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

const debugTraceStopTimeout = 10 * time.Second

// newDebugCommand builds daemon debugging commands.
func newDebugCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "debug",
		Usage: "debug a running daemon",
		Subcommands: []*cli.Command{
			newDebugTraceCommand(),
			newDebugCPUProfileCommand(),
		},
	}
}

func newDebugTraceCommand() *cli.Command {
	var statePath string
	var socketPath string
	var outputPath string
	var label string
	var duration time.Duration
	return &cli.Command{
		Name:  "trace",
		Usage: "capture a Go runtime trace from the running daemon",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:        "socket-path",
				Usage:       "connect to an existing daemon socket at this exact path",
				EnvVars:     socketPathEnvVars,
				Destination: &socketPath,
			},
			&cli.StringFlag{
				Name:        "out",
				Aliases:     []string{"o"},
				Usage:       "trace output path",
				Destination: &outputPath,
			},
			&cli.DurationFlag{
				Name:        "duration",
				Aliases:     []string{"d"},
				Usage:       "trace capture duration",
				Value:       30 * time.Second,
				Destination: &duration,
			},
			&cli.StringFlag{
				Name:        "label",
				Usage:       "runtime trace label",
				Value:       "spacewave-cli-debug-trace",
				Destination: &label,
			},
		},
		Action: func(c *cli.Context) error {
			return runDebugTrace(c, statePath, socketPath, outputPath, duration, label, c.String("output"))
		},
	}
}

func newDebugCPUProfileCommand() *cli.Command {
	var statePath string
	var socketPath string
	var outputPath string
	var label string
	var duration time.Duration
	return &cli.Command{
		Name:    "cpu-profile",
		Aliases: []string{"cpu", "cpu-trace"},
		Usage:   "capture a Go CPU profile from the running daemon",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:        "socket-path",
				Usage:       "connect to an existing daemon socket at this exact path",
				EnvVars:     socketPathEnvVars,
				Destination: &socketPath,
			},
			&cli.StringFlag{
				Name:        "out",
				Aliases:     []string{"o"},
				Usage:       "CPU profile output path",
				Destination: &outputPath,
			},
			&cli.DurationFlag{
				Name:        "duration",
				Aliases:     []string{"d"},
				Usage:       "CPU profile capture duration",
				Value:       30 * time.Second,
				Destination: &duration,
			},
			&cli.StringFlag{
				Name:        "label",
				Usage:       "CPU profile label",
				Value:       "spacewave-cli-cpu-profile",
				Destination: &label,
			},
		},
		Action: func(c *cli.Context) error {
			return runDebugCPUProfile(c, statePath, socketPath, outputPath, duration, label, c.String("output"))
		},
	}
}

func runDebugTrace(
	c *cli.Context,
	statePath string,
	socketPath string,
	outputPath string,
	duration time.Duration,
	label string,
	outputFormat string,
) error {
	if duration <= 0 {
		return errors.New("duration must be greater than zero")
	}
	if outputPath == "" {
		outputPath = defaultDebugTraceOutputPath(time.Now())
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return errors.Wrap(err, "create trace output directory")
	}

	client, err := connectDebugTraceDaemon(c.Context, c, statePath, socketPath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()

	f, err := os.Create(outputPath)
	if err != nil {
		return errors.Wrap(err, "create trace output")
	}
	defer f.Close()

	traceClient := s4wave_trace.NewSRPCTraceServiceClient(client.srpc)
	byteCount, err := captureDaemonRuntimeTrace(c.Context, traceClient, f, duration, label)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close trace output")
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var first bool
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("path")
		ms.WriteString(outputPath)
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("duration")
		ms.WriteString(duration.String())
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("bytes")
		ms.WriteInt64(byteCount)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	writeFields(os.Stdout, [][2]string{
		{"Trace", outputPath},
		{"Duration", duration.String()},
		{"Bytes", strconv.FormatInt(byteCount, 10)},
	})
	return nil
}

func runDebugCPUProfile(
	c *cli.Context,
	statePath string,
	socketPath string,
	outputPath string,
	duration time.Duration,
	label string,
	outputFormat string,
) error {
	if duration <= 0 {
		return errors.New("duration must be greater than zero")
	}
	if outputPath == "" {
		outputPath = defaultDebugCPUProfileOutputPath(time.Now())
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return errors.Wrap(err, "create CPU profile output directory")
	}

	client, err := connectDebugTraceDaemon(c.Context, c, statePath, socketPath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()

	f, err := os.Create(outputPath)
	if err != nil {
		return errors.Wrap(err, "create CPU profile output")
	}
	defer f.Close()

	traceClient := s4wave_trace.NewSRPCTraceServiceClient(client.srpc)
	byteCount, err := captureDaemonCPUProfile(c.Context, traceClient, f, duration, label)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close CPU profile output")
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var first bool
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("path")
		ms.WriteString(outputPath)
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("duration")
		ms.WriteString(duration.String())
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("bytes")
		ms.WriteInt64(byteCount)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	writeFields(os.Stdout, [][2]string{
		{"CPU Profile", outputPath},
		{"Duration", duration.String()},
		{"Bytes", strconv.FormatInt(byteCount, 10)},
	})
	return nil
}

func connectDebugTraceDaemon(
	ctx context.Context,
	c *cli.Context,
	statePath string,
	socketPath string,
) (*sdkClient, error) {
	if socketPath != "" {
		return connectDaemonAtSocket(ctx, socketPath)
	}
	resolved, err := resolveStatePathFromContext(c, statePath)
	if err != nil {
		return nil, err
	}
	return connectDaemon(ctx, resolved)
}

func defaultDebugTraceOutputPath(now time.Time) string {
	return filepath.Join(".tmp", "spacewave-daemon-"+now.Format("20060102-150405")+".trace")
}

func defaultDebugCPUProfileOutputPath(now time.Time) string {
	return filepath.Join(".tmp", "spacewave-daemon-"+now.Format("20060102-150405")+".pprof")
}

func captureDaemonRuntimeTrace(
	ctx context.Context,
	traceClient s4wave_trace.SRPCTraceServiceClient,
	out io.Writer,
	duration time.Duration,
	label string,
) (int64, error) {
	if _, err := traceClient.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: label}); err != nil {
		return 0, errors.Wrap(err, "start daemon trace")
	}

	timer := time.NewTimer(duration)
	var waitErr error
	select {
	case <-ctx.Done():
		waitErr = ctx.Err()
	case <-timer.C:
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}

	stopCtx := ctx
	var cancel context.CancelFunc
	if waitErr != nil {
		stopCtx, cancel = context.WithTimeout(context.Background(), debugTraceStopTimeout)
		defer cancel()
	}
	byteCount, err := stopDaemonRuntimeTrace(stopCtx, traceClient, out)
	if err != nil {
		return byteCount, errors.Wrap(err, "stop daemon trace")
	}
	return byteCount, waitErr
}

func captureDaemonCPUProfile(
	ctx context.Context,
	traceClient s4wave_trace.SRPCTraceServiceClient,
	out io.Writer,
	duration time.Duration,
	label string,
) (int64, error) {
	if duration <= 0 {
		return 0, errors.New("duration must be greater than zero")
	}
	durationMillis := uint32(duration / time.Millisecond)
	if durationMillis == 0 {
		durationMillis = 1
	}
	strm, err := traceClient.CaptureCPUProfile(ctx, &s4wave_trace.CaptureCPUProfileRequest{
		DurationMillis: durationMillis,
		Label:          label,
	})
	if err != nil {
		return 0, errors.Wrap(err, "capture daemon CPU profile")
	}
	defer strm.Close()

	var byteCount int64
	for {
		resp, err := strm.Recv()
		if err == io.EOF {
			return byteCount, nil
		}
		if err != nil {
			return byteCount, err
		}
		data := resp.GetData()
		if len(data) == 0 {
			continue
		}
		n, err := out.Write(data)
		byteCount += int64(n)
		if err != nil {
			return byteCount, err
		}
		if n != len(data) {
			return byteCount, io.ErrShortWrite
		}
	}
}

func stopDaemonRuntimeTrace(
	ctx context.Context,
	traceClient s4wave_trace.SRPCTraceServiceClient,
	out io.Writer,
) (int64, error) {
	strm, err := traceClient.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
	if err != nil {
		return 0, err
	}
	defer strm.Close()

	var byteCount int64
	for {
		resp, err := strm.Recv()
		if err == io.EOF {
			return byteCount, nil
		}
		if err != nil {
			return byteCount, err
		}
		data := resp.GetData()
		if len(data) == 0 {
			continue
		}
		n, err := out.Write(data)
		byteCount += int64(n)
		if err != nil {
			return byteCount, err
		}
		if n != len(data) {
			return byteCount, io.ErrShortWrite
		}
	}
}
