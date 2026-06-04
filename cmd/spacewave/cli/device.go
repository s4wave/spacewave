//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	session_pb "github.com/s4wave/spacewave/core/session"
)

const deviceDockerStatePath = "/var/lib/spacewave"

// newDeviceCommand builds the managed Device command group.
func newDeviceCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:    "device",
		Aliases: []string{"devices"},
		Usage:   "manage Spacewave-managed devices",
		Flags:   clientFlags(&statePath, &sessionIdx),
		Subcommands: []*cli.Command{
			newDeviceStatusCommand(&statePath, &sessionIdx),
			newDeviceSetupCommand(&statePath, &sessionIdx),
		},
	}
}

func newDeviceStatusCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "show managed-device setup status",
		Flags: clientFlags(statePath, sessionIdx),
		Action: func(c *cli.Context) error {
			report, err := buildDeviceStatusReport(c, *statePath)
			if err != nil {
				return err
			}
			return writeDeviceStatusReport(report, c.String("output"))
		},
	}
}

func newDeviceSetupCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:  "setup",
		Usage: "prepare managed-device setup",
		Subcommands: []*cli.Command{
			newDeviceSetupDockerCommand(statePath, sessionIdx),
		},
	}
}

func newDeviceSetupDockerCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var label string
	return &cli.Command{
		Name:  "docker",
		Usage: "show the Docker daemon setup seed",
		Flags: append(clientFlags(statePath, sessionIdx),
			&cli.StringFlag{
				Name:        "label",
				Usage:       "managed device label",
				Required:    true,
				Destination: &label,
			},
		),
		Action: func(c *cli.Context) error {
			report, err := buildDeviceDockerSetupReport(c, *statePath, label)
			if err != nil {
				return err
			}
			return writeDeviceDockerSetupReport(report, c.String("output"))
		},
	}
}

type deviceRuntimePaths struct {
	statePath string
	socket    string
}

func resolveDeviceRuntimePaths(c *cli.Context, statePath string) (*deviceRuntimePaths, error) {
	resolved, err := resolveStatePathFromContext(c, statePath)
	if err != nil {
		return nil, err
	}
	sockPath := effectiveSocketPath(c, "")
	if sockPath == "" {
		sockPath = filepath.Join(resolved, socketName)
	}
	return &deviceRuntimePaths{
		statePath: resolved,
		socket:    sockPath,
	}, nil
}

type deviceStatusReport struct {
	status        string
	setup         string
	enrollment    string
	statePath     string
	socket        string
	sessionType   string
	requestedRole string
	completion    string
}

func buildDeviceStatusReport(c *cli.Context, statePath string) (*deviceStatusReport, error) {
	paths, err := resolveDeviceRuntimePaths(c, statePath)
	if err != nil {
		return nil, err
	}
	return &deviceStatusReport{
		status:        "unconfigured",
		setup:         "not started",
		enrollment:    "not linked",
		statePath:     paths.statePath,
		socket:        paths.socket,
		sessionType:   session_pb.SessionType_SESSION_TYPE_DEVICE.String(),
		requestedRole: "WRITER",
		completion:    "cli-mediated",
	}, nil
}

type deviceDockerSetupReport struct {
	label              string
	statePath          string
	socket             string
	containerStatePath string
	sessionType        string
	requestedRole      string
	completion         string
	enrollment         string
	ticket             string
}

func buildDeviceDockerSetupReport(c *cli.Context, statePath string, label string) (*deviceDockerSetupReport, error) {
	if label == "" {
		return nil, errors.New("device label required")
	}
	paths, err := resolveDeviceRuntimePaths(c, statePath)
	if err != nil {
		return nil, err
	}
	return &deviceDockerSetupReport{
		label:              label,
		statePath:          paths.statePath,
		socket:             paths.socket,
		containerStatePath: deviceDockerStatePath,
		sessionType:        session_pb.SessionType_SESSION_TYPE_DEVICE.String(),
		requestedRole:      "WRITER",
		completion:         "cli-mediated",
		enrollment:         "not started",
		ticket:             "not generated",
	}, nil
}

func writeDeviceStatusReport(report *deviceStatusReport, outputFormat string) error {
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var more bool
		writeJSONStringField(ms, &more, "status", report.status)
		writeJSONStringField(ms, &more, "setup", report.setup)
		writeJSONStringField(ms, &more, "enrollment", report.enrollment)
		writeJSONStringField(ms, &more, "statePath", report.statePath)
		writeJSONStringField(ms, &more, "socket", report.socket)
		writeJSONStringField(ms, &more, "sessionType", report.sessionType)
		writeJSONStringField(ms, &more, "requestedRole", report.requestedRole)
		writeJSONStringField(ms, &more, "completion", report.completion)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}
	if outputFormat != "text" && outputFormat != "table" {
		return formatOutput(nil, outputFormat)
	}
	writeFields(os.Stdout, [][2]string{
		{"Status", report.status},
		{"Setup", report.setup},
		{"Enrollment", report.enrollment},
		{"State Path", report.statePath},
		{"Socket", report.socket},
		{"Session Type", report.sessionType},
		{"Requested Role", report.requestedRole},
		{"Completion", report.completion},
	})
	return nil
}

func writeDeviceDockerSetupReport(report *deviceDockerSetupReport, outputFormat string) error {
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var more bool
		writeJSONStringField(ms, &more, "label", report.label)
		writeJSONStringField(ms, &more, "statePath", report.statePath)
		writeJSONStringField(ms, &more, "socket", report.socket)
		writeJSONStringField(ms, &more, "containerStatePath", report.containerStatePath)
		writeJSONStringField(ms, &more, "sessionType", report.sessionType)
		writeJSONStringField(ms, &more, "requestedRole", report.requestedRole)
		writeJSONStringField(ms, &more, "completion", report.completion)
		writeJSONStringField(ms, &more, "enrollment", report.enrollment)
		writeJSONStringField(ms, &more, "ticket", report.ticket)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}
	if outputFormat != "text" && outputFormat != "table" {
		return formatOutput(nil, outputFormat)
	}
	writeFields(os.Stdout, [][2]string{
		{"Label", report.label},
		{"State Path", report.statePath},
		{"Socket", report.socket},
		{"Container State Path", report.containerStatePath},
		{"Session Type", report.sessionType},
		{"Requested Role", report.requestedRole},
		{"Completion", report.completion},
		{"Enrollment", report.enrollment},
		{"Ticket", report.ticket},
	})
	return nil
}
