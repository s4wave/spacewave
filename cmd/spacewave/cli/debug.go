//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	trace_capture "github.com/s4wave/spacewave/core/trace/capture"
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
			newDebugMemoryProfileCommand(),
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

func newDebugMemoryProfileCommand() *cli.Command {
	var statePath string
	var socketPath string
	var outputPath string
	var profile string
	var gc bool
	var debug int
	return &cli.Command{
		Name:    "mem-profile",
		Aliases: []string{"mem"},
		Usage:   "capture a Go memory profile from the running daemon",
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
				Usage:       "memory profile output path",
				Destination: &outputPath,
			},
			&cli.StringFlag{
				Name:        "profile",
				Usage:       "runtime/pprof memory profile: allocs or heap",
				Value:       "allocs",
				Destination: &profile,
			},
			&cli.BoolFlag{
				Name:        "gc",
				Usage:       "force a garbage collection before capture",
				Destination: &gc,
			},
			&cli.IntFlag{
				Name:        "debug",
				Usage:       "pprof output debug level",
				Destination: &debug,
			},
		},
		Action: func(c *cli.Context) error {
			return runDebugMemoryProfile(c, statePath, socketPath, outputPath, profile, gc, debug, c.String("output"))
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
	byteCount, err := trace_capture.CaptureRuntimeTrace(c.Context, traceClient, f, trace_capture.RuntimeTraceArgs{
		Duration:    duration,
		Label:       label,
		StopTimeout: debugTraceStopTimeout,
	})
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
	byteCount, err := trace_capture.CaptureCPUProfile(c.Context, traceClient, f, trace_capture.CPUProfileArgs{
		Duration: duration,
		Label:    label,
	})
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

func runDebugMemoryProfile(
	c *cli.Context,
	statePath string,
	socketPath string,
	outputPath string,
	profile string,
	gc bool,
	debug int,
	outputFormat string,
) error {
	switch profile {
	case "":
		profile = "allocs"
	case "heap", "allocs":
	default:
		return errors.Errorf("memory profile must be heap or allocs, got %q", profile)
	}
	if debug < 0 {
		return errors.New("debug must be greater than or equal to zero")
	}
	if outputPath == "" {
		outputPath = defaultDebugMemoryProfileOutputPath(time.Now(), profile)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return errors.Wrap(err, "create memory profile output directory")
	}

	client, err := connectDebugTraceDaemon(c.Context, c, statePath, socketPath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()

	f, err := os.Create(outputPath)
	if err != nil {
		return errors.Wrap(err, "create memory profile output")
	}
	defer f.Close()

	traceClient := s4wave_trace.NewSRPCTraceServiceClient(client.srpc)
	byteCount, err := trace_capture.CaptureMemoryProfile(c.Context, traceClient, f, trace_capture.MemoryProfileArgs{
		Profile: profile,
		GC:      gc,
		Debug:   int32(debug),
	})
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close memory profile output")
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var first bool
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("path")
		ms.WriteString(outputPath)
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("profile")
		ms.WriteString(profile)
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("gc")
		ms.WriteBool(gc)
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("debug")
		ms.WriteInt64(int64(debug))
		ms.WriteMoreIf(&first)
		ms.WriteObjectField("bytes")
		ms.WriteInt64(byteCount)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	writeFields(os.Stdout, [][2]string{
		{"Memory Profile", outputPath},
		{"Profile", profile},
		{"GC", strconv.FormatBool(gc)},
		{"Debug", strconv.Itoa(debug)},
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

func defaultDebugMemoryProfileOutputPath(now time.Time, profile string) string {
	if profile == "" {
		profile = "memory"
	}
	return filepath.Join(".tmp", "spacewave-daemon-"+now.Format("20060102-150405")+"-"+profile+".pprof")
}
