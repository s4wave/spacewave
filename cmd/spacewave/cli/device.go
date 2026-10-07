//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

const (
	deviceSetupStateNotConfigured = "not_configured"
	deviceSetupStateLocalReady    = "local_ready"
	deviceSetupStateWaiting       = "waiting_for_completion"
	deviceSetupStateFailed        = "setup_failed"
	deviceSetupStateImported      = "completion_imported"
	deviceSetupStateSessionReady  = "device_session_ready"

	deviceSpaceLinkAuthRequestVersion = 1
	deviceSetupDefaultTicketTTL       = 15 * time.Minute
	deviceSetupNonceLength            = 16

	deviceSetupStateDir      = "device"
	deviceSetupStateFile     = "setup.json"
	deviceIdentityKeyPEMFile = "identity.pem"
	deviceDockerStatePath    = "/var/lib/spacewave"
)

// deviceSetupRecord is the Device's persisted setup state in setup.json.
type deviceSetupRecord struct {
	// SetupState is the setup step the Device has reached.
	SetupState string `json:"setupState"`
	// PeerID is the Device identity's peer ID.
	PeerID string `json:"peerId,omitempty"`
	// Label is the Device's display label.
	Label string `json:"label,omitempty"`
	// RequestedRole is the Space role the ticket requests.
	RequestedRole string `json:"requestedRole,omitempty"`
	// TargetHint names the Space the operator intends to link.
	TargetHint string `json:"targetHint,omitempty"`
	// CompletionMode is how the completion reaches the Device.
	CompletionMode string `json:"completionMode,omitempty"`
	// Completion is the imported approval completion.
	Completion string `json:"completion,omitempty"`
	// CompletionAt is when the completion was imported, in Unix seconds.
	CompletionAt int64 `json:"completionAt,omitempty"`
	// CompletionStatus is the approval's reported status.
	CompletionStatus string `json:"completionStatus,omitempty"`
	// AccountID is the provider account holding the Device session.
	AccountID string `json:"accountId,omitempty"`
	// ResourceID is the base64 linked Space ID.
	ResourceID string `json:"resourceId,omitempty"`
	// SessionID is the Device session's ID.
	SessionID string `json:"sessionId,omitempty"`
	// SessionIndex is the daemon's index for the mounted Device session.
	SessionIndex uint32 `json:"sessionIndex,omitempty"`
	// SessionPeerID is the peer ID the Device session runs as.
	SessionPeerID string `json:"sessionPeerId,omitempty"`
	// DeviceObjectKey is the World key of the projected Device object.
	DeviceObjectKey string `json:"deviceObjectKey,omitempty"`
	// FailureReason explains the last failed or pending step.
	FailureReason string `json:"failureReason,omitempty"`
	// ExpiresAt is when the ticket expires, in Unix seconds.
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	// Ticket is the Space Link ticket the operator approves.
	Ticket string `json:"ticket,omitempty"`
}

// deviceStatusOutput reports the daemon and the setup record. Fields shared
// with deviceSetupRecord mean the same; the completion is never reported.
type deviceStatusOutput struct {
	// DaemonStatus is the daemon's state.
	DaemonStatus string `json:"daemonStatus"`
	// SetupState is the setup step the Device has reached.
	SetupState string `json:"setupState"`
	// StatePath is the daemon's resolved state directory.
	StatePath string `json:"statePath"`
	// Socket is the daemon's control socket path.
	Socket string `json:"socket"`
	// PeerID is the Device identity's peer ID.
	PeerID string `json:"peerId,omitempty"`
	// Label is the Device's display label.
	Label string `json:"label,omitempty"`
	// RequestedRole is the Space role the ticket requests.
	RequestedRole string `json:"requestedRole,omitempty"`
	// TargetHint names the Space the operator intends to link.
	TargetHint string `json:"targetHint,omitempty"`
	// CompletionMode is how the completion reaches the Device.
	CompletionMode string `json:"completionMode,omitempty"`
	// CompletionAt is when the completion was imported, in Unix seconds.
	CompletionAt int64 `json:"completionAt,omitempty"`
	// CompletionStatus is the approval's reported status.
	CompletionStatus string `json:"completionStatus,omitempty"`
	// AccountID is the provider account holding the Device session.
	AccountID string `json:"accountId,omitempty"`
	// ResourceID is the base64 linked Space ID.
	ResourceID string `json:"resourceId,omitempty"`
	// SessionID is the Device session's ID.
	SessionID string `json:"sessionId,omitempty"`
	// SessionIndex is the daemon's index for the mounted Device session.
	SessionIndex uint32 `json:"sessionIndex,omitempty"`
	// SessionPeerID is the peer ID the Device session runs as.
	SessionPeerID string `json:"sessionPeerId,omitempty"`
	// DeviceObjectKey is the World key of the projected Device object.
	DeviceObjectKey string `json:"deviceObjectKey,omitempty"`
	// FailureReason explains the last failed or pending step.
	FailureReason string `json:"failureReason,omitempty"`
	// ExpiresAt is when the ticket expires, in Unix seconds.
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	// Ticket is the Space Link ticket the operator approves.
	Ticket string `json:"ticket,omitempty"`
	// IdentityCreated reports that this request created the Device identity.
	IdentityCreated bool `json:"identityCreated,omitempty"`
}

// newDeviceStatusOutput projects a setup record into the status output for
// the given daemon paths.
func newDeviceStatusOutput(record *deviceSetupRecord, resolvedStatePath, sockPath string) deviceStatusOutput {
	return deviceStatusOutput{
		DaemonStatus:     "running",
		SetupState:       record.SetupState,
		StatePath:        resolvedStatePath,
		Socket:           sockPath,
		PeerID:           record.PeerID,
		Label:            record.Label,
		RequestedRole:    record.RequestedRole,
		TargetHint:       record.TargetHint,
		CompletionMode:   record.CompletionMode,
		CompletionAt:     record.CompletionAt,
		CompletionStatus: record.CompletionStatus,
		AccountID:        record.AccountID,
		ResourceID:       record.ResourceID,
		SessionID:        record.SessionID,
		SessionIndex:     record.SessionIndex,
		SessionPeerID:    record.SessionPeerID,
		DeviceObjectKey:  record.DeviceObjectKey,
		FailureReason:    record.FailureReason,
		ExpiresAt:        record.ExpiresAt,
		Ticket:           record.Ticket,
	}
}

// deviceSetupArgs holds the flags of device setup.
type deviceSetupArgs struct {
	statePath     string
	outputFormat  string
	label         string
	targetHint    string
	requestedRole string
	expiresIn     time.Duration
}

// deviceCompleteArgs holds the flags of device complete.
type deviceCompleteArgs struct {
	statePath    string
	outputFormat string
	completion   string
}

// deviceDockerSetupReport describes the setup a Docker Device container needs.
type deviceDockerSetupReport struct {
	// Label is the Device's display label.
	Label string `json:"label"`
	// StatePath is the host state directory.
	StatePath string `json:"statePath"`
	// Socket is the daemon's control socket path.
	Socket string `json:"socket"`
	// ContainerStatePath is the state directory inside the container.
	ContainerStatePath string `json:"containerStatePath"`
	// SessionType is the session type the Device creates.
	SessionType string `json:"sessionType"`
	// RequestedRole is the Space role the Device requests.
	RequestedRole string `json:"requestedRole"`
	// Completion is how the completion reaches the Device.
	Completion string `json:"completion"`
	// Enrollment is the enrollment's progress.
	Enrollment string `json:"enrollment"`
	// Ticket is the ticket's progress.
	Ticket string `json:"ticket"`
}

var deviceMountLinkedSession = func(
	ctx context.Context,
	client *sdkClient,
	req *s4wave_provider_spacewave.MountLinkedDeviceSessionRequest,
) (*s4wave_provider_spacewave.MountLinkedDeviceSessionResponse, error) {
	prov, cleanup, err := client.lookupSpacewaveProvider(ctx, "")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return prov.MountLinkedDeviceSession(ctx, req)
}

var (
	deviceUpsertObject      = upsertLinkedDeviceObject
	deviceMountLocalSession = mountLocalDeviceSession
)

func newDeviceCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	return &cli.Command{
		Name:    "device",
		Aliases: []string{"devices"},
		Usage:   "manage Spacewave-managed Device setup",
		Flags:   daemonClientFlags(&statePath),
		Subcommands: []*cli.Command{
			newDeviceSetupCommand(),
			newDeviceApproveCommand(),
			newDeviceCompleteCommand(),
			newDevicePolicyCommand(),
			newDeviceStatusCommand(),
			newDeviceShowCommand(),
		},
	}
}

func newDeviceSetupCommand() *cli.Command {
	// Declare the setup flags and return the command.
	var statePath string
	var label string
	var targetHint string
	var requestedRole string
	var expiresIn time.Duration
	return &cli.Command{
		Name:  "setup",
		Usage: "initialize local Device setup state",
		Subcommands: []*cli.Command{
			newDeviceSetupDockerCommand(),
		},
		Flags: append(
			daemonClientFlags(&statePath),
			&cli.StringFlag{
				Name:        "label",
				Usage:       "operator-visible Device label",
				Destination: &label,
			},
			&cli.StringFlag{
				Name:        "target-hint",
				Usage:       "target Space hint to include in the SpaceLink ticket",
				Destination: &targetHint,
			},
			&cli.StringFlag{
				Name:        "role",
				Usage:       "requested Space role (reader/writer)",
				Value:       "writer",
				Destination: &requestedRole,
			},
			&cli.DurationFlag{
				Name:        "expires-in",
				Usage:       "SpaceLink ticket lifetime",
				Value:       deviceSetupDefaultTicketTTL,
				Destination: &expiresIn,
			},
			outputFlag(),
		),
		Action: func(c *cli.Context) error {
			return runDeviceSetup(c, deviceSetupArgs{
				statePath:     statePath,
				outputFormat:  c.String("output"),
				label:         label,
				targetHint:    targetHint,
				requestedRole: requestedRole,
				expiresIn:     expiresIn,
			})
		},
	}
}

func newDeviceSetupDockerCommand() *cli.Command {
	var statePath string
	var label string
	return &cli.Command{
		Name:  "docker",
		Usage: "show the Docker daemon setup seed",
		Flags: append(
			daemonClientFlags(&statePath),
			&cli.StringFlag{
				Name:        "label",
				Usage:       "managed Device label",
				Required:    true,
				Destination: &label,
			},
			outputFlag(),
		),
		Action: func(c *cli.Context) error {
			report, err := buildDeviceDockerSetupReport(c, statePath, label)
			if err != nil {
				return err
			}
			return writeDeviceDockerSetupReport(report, c.String("output"))
		},
	}
}

func newDeviceCompleteCommand() *cli.Command {
	var statePath string
	var completion string
	return &cli.Command{
		Name:  "complete",
		Usage: "import SpaceLink approval completion",
		Flags: append(
			daemonClientFlags(&statePath),
			&cli.StringFlag{
				Name:        "completion",
				Usage:       "base64 SpaceLink completion payload",
				Destination: &completion,
			},
			outputFlag(),
		),
		Action: func(c *cli.Context) error {
			completionValue := completion
			if strings.TrimSpace(completionValue) == "" && c.NArg() > 0 {
				completionValue = c.Args().First()
			}
			return runDeviceComplete(c, deviceCompleteArgs{
				statePath:    statePath,
				outputFormat: c.String("output"),
				completion:   completionValue,
			})
		},
	}
}

func newDeviceStatusCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:  "status",
		Usage: "show local Device setup status",
		Flags: append(daemonClientFlags(&statePath), outputFlag()),
		Action: func(c *cli.Context) error {
			return runDeviceStatus(c, statePath, c.String("output"))
		},
	}
}

func runDeviceSetup(c *cli.Context, args deviceSetupArgs) error {
	// Connect to the daemon for the resolved state path.
	ctx := c.Context
	resolvedStatePath, sockPath, err := resolveDeviceDaemonPaths(c, args.statePath)
	if err != nil {
		return err
	}
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolvedStatePath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()

	// Default the Device label and require a valid role and ticket lifetime.
	label := strings.TrimSpace(args.label)
	if label == "" {
		label = defaultDeviceSetupLabel()
	}
	role, roleLabel, err := parseDeviceRequestedRole(args.requestedRole)
	if err != nil {
		return err
	}
	if args.expiresIn <= 0 {
		return errors.New("expires-in must be positive")
	}

	// Load or create the device identity and build its Space Link ticket.
	priv, agentPeerID, identityCreated, err := loadOrCreateDeviceIdentity(resolvedStatePath)
	if err != nil {
		return err
	}
	ticket, expiresAt, err := buildDeviceSpaceLinkTicket(deviceSpaceLinkTicketArgs{
		priv:          priv,
		agentPeerID:   agentPeerID,
		label:         label,
		targetHint:    strings.TrimSpace(args.targetHint),
		requestedRole: role,
		expiresIn:     args.expiresIn,
		now:           time.Now(),
	})
	if err != nil {
		return err
	}

	// Save the waiting setup record and write its status.
	record := &deviceSetupRecord{
		SetupState:     deviceSetupStateWaiting,
		PeerID:         agentPeerID.String(),
		Label:          label,
		RequestedRole:  roleLabel,
		TargetHint:     strings.TrimSpace(args.targetHint),
		CompletionMode: "cli",
		SessionID:      deviceSessionID(agentPeerID),
		ExpiresAt:      expiresAt.Unix(),
		Ticket:         ticket,
	}
	if err := writeDeviceSetupRecord(resolvedStatePath, record); err != nil {
		return err
	}
	out := newDeviceStatusOutput(record, resolvedStatePath, sockPath)
	out.IdentityCreated = identityCreated
	return writeDeviceStatusOutput(out, args.outputFormat)
}

func runDeviceComplete(c *cli.Context, args deviceCompleteArgs) error {
	// Connect to the daemon for the resolved state path.
	ctx := c.Context
	resolvedStatePath, sockPath, err := resolveDeviceDaemonPaths(c, args.statePath)
	if err != nil {
		return err
	}
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolvedStatePath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()

	// Import the completion and save the updated setup record.
	record, err := readDeviceSetupRecord(resolvedStatePath)
	if err != nil {
		return err
	}
	updated, err := applyDeviceCompletion(record, args.completion, time.Now())
	if err != nil {
		return err
	}
	if err := writeDeviceSetupRecord(resolvedStatePath, updated); err != nil {
		return err
	}

	// Publish the completed Device identity to the daemon's policy host before
	// this request projects the Device into the World.
	_ = requestDevicePolicyReload(ctx, client)
	if updated.SetupState == deviceSetupStateImported {
		activated, err := openDeviceSession(ctx, client, resolvedStatePath, updated)
		if err != nil {
			updated.SetupState = deviceSetupStateImported
			updated.FailureReason = err.Error()
			if persistErr := writeDeviceSetupRecord(resolvedStatePath, updated); persistErr != nil {
				return errors.Wrapf(err, "persist Device setup failure: %v", persistErr)
			}
			return err
		}
		updated = activated
		if err := writeDeviceSetupRecord(resolvedStatePath, updated); err != nil {
			return err
		}
	}
	return writeDeviceStatusOutput(newDeviceStatusOutput(updated, resolvedStatePath, sockPath), args.outputFormat)
}

func runDeviceStatus(c *cli.Context, statePath, outputFormat string) error {
	// Connect to the daemon for the resolved state path.
	ctx := c.Context
	resolvedStatePath, sockPath, err := resolveDeviceDaemonPaths(c, statePath)
	if err != nil {
		return err
	}
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolvedStatePath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()

	// Read the setup record and write its status.
	record, err := readDeviceSetupRecord(resolvedStatePath)
	if err != nil {
		return err
	}
	return writeDeviceStatusOutput(newDeviceStatusOutput(record, resolvedStatePath, sockPath), outputFormat)
}

func buildDeviceDockerSetupReport(c *cli.Context, statePath string, label string) (*deviceDockerSetupReport, error) {
	// Require a label and resolve the daemon paths for the Docker report.
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, errors.New("device label required")
	}
	resolvedStatePath, sockPath, err := resolveDeviceDaemonPaths(c, statePath)
	if err != nil {
		return nil, err
	}
	return &deviceDockerSetupReport{
		Label:              label,
		StatePath:          resolvedStatePath,
		Socket:             sockPath,
		ContainerStatePath: deviceDockerStatePath,
		SessionType:        core_session.SessionType_SESSION_TYPE_DEVICE.String(),
		RequestedRole:      "WRITER",
		Completion:         "cli-mediated",
		Enrollment:         "not started",
		Ticket:             "not generated",
	}, nil
}

func resolveDeviceDaemonPaths(c *cli.Context, statePath string) (string, string, error) {
	// Resolve the state path and require a socket path.
	resolvedStatePath, err := resolveStatePathFromContext(c, statePath)
	if err != nil {
		return "", "", err
	}
	sockPath := effectiveSocketPath(c, "")
	if sockPath == "" {
		sockPath = filepath.Join(resolvedStatePath, socketName)
	}
	return resolvedStatePath, sockPath, nil
}

func defaultDeviceSetupLabel() string {
	hostname, err := os.Hostname()
	if err == nil && strings.TrimSpace(hostname) != "" {
		return strings.TrimSpace(hostname)
	}
	return "Spacewave Device"
}

func parseDeviceRequestedRole(raw string) (sobject.SOParticipantRole, string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "writer":
		return sobject.SOParticipantRole_SOParticipantRole_WRITER, "writer", nil
	case "reader":
		return sobject.SOParticipantRole_SOParticipantRole_READER, "reader", nil
	default:
		return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN, "", errors.New("device setup role must be reader or writer")
	}
}

func loadOrCreateDeviceIdentity(statePath string) (crypto.PrivKey, peer.ID, bool, error) {
	// Return the existing device identity when the key file is present.
	path := deviceIdentityKeyPath(statePath)
	priv, pid, _, err := loadDeviceIdentity(path)
	if err == nil {
		return priv, pid, false, nil
	}
	if !os.IsNotExist(err) {
		return nil, "", false, err
	}

	// Generate an Ed25519 device identity and marshal it.
	priv, _, err = crypto.GenerateEd25519Key(cryptorand.Reader)
	if err != nil {
		return nil, "", false, errors.Wrap(err, "generate device identity")
	}
	pemData, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return nil, "", false, errors.Wrap(err, "marshal device identity")
	}

	// Write the identity key and derive its peer ID.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, "", false, errors.Wrap(err, "create device identity directory")
	}
	if err := os.WriteFile(path, pemData, 0o600); err != nil {
		return nil, "", false, errors.Wrap(err, "write device identity")
	}
	pid, err = peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, "", false, errors.Wrap(err, "derive device peer id")
	}
	return priv, pid, true, nil
}

func loadDeviceIdentity(path string) (crypto.PrivKey, peer.ID, []byte, error) {
	// Read and parse the device identity key.
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil, err
		}
		return nil, "", nil, errors.Wrap(err, "read device identity")
	}
	priv, err := keypem.ParsePrivKeyPem(data)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "parse device identity")
	}
	if priv == nil {
		return nil, "", nil, errors.New("device identity private key is empty")
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "derive device peer id")
	}
	return priv, pid, data, nil
}

type deviceSpaceLinkTicketArgs struct {
	priv          crypto.PrivKey
	agentPeerID   peer.ID
	label         string
	targetHint    string
	requestedRole sobject.SOParticipantRole
	expiresIn     time.Duration
	now           time.Time
}

func buildDeviceSpaceLinkTicket(args deviceSpaceLinkTicketArgs) (string, time.Time, error) {
	// Require the identity, peer ID, and label.
	if args.priv == nil {
		return "", time.Time{}, errors.New("device identity private key is required")
	}
	if args.agentPeerID == "" {
		return "", time.Time{}, errors.New("device peer id is required")
	}
	if args.label == "" {
		return "", time.Time{}, errors.New("device label is required")
	}

	// Build and marshal the Space Link auth request.
	nonce := make([]byte, deviceSetupNonceLength)
	if _, err := cryptorand.Read(nonce); err != nil {
		return "", time.Time{}, errors.Wrap(err, "generate spacelink nonce")
	}
	expiresAt := args.now.Add(args.expiresIn)
	payload := &s4wave_provider_spacewave.SpaceLinkAuthRequest{
		Version:        deviceSpaceLinkAuthRequestVersion,
		SessionType:    core_session.SessionType_SESSION_TYPE_DEVICE,
		AgentPeerId:    []byte(args.agentPeerID),
		Label:          args.label,
		TargetHint:     []byte(args.targetHint),
		RequestedRole:  args.requestedRole,
		Nonce:          nonce,
		ExpiresAt:      expiresAt.Unix(),
		CompletionMode: s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_CLI,
	}
	payloadBytes, err := payload.MarshalVT()
	if err != nil {
		return "", time.Time{}, errors.Wrap(err, "marshal spacelink payload")
	}

	// Sign the payload and encode the ticket.
	sig, err := args.priv.Sign(payloadBytes)
	if err != nil {
		return "", time.Time{}, errors.Wrap(err, "sign spacelink payload")
	}
	ticketBytes, err := (&s4wave_provider_spacewave.SpaceLinkAuthTicket{
		Payload:        payloadBytes,
		AgentSignature: sig,
	}).MarshalVT()
	if err != nil {
		return "", time.Time{}, errors.Wrap(err, "marshal spacelink ticket")
	}
	return base64.StdEncoding.EncodeToString(ticketBytes), expiresAt, nil
}

func applyDeviceCompletion(record *deviceSetupRecord, encodedCompletion string, now time.Time) (*deviceSetupRecord, error) {
	// Reject a missing setup and require the completion nonce to match the ticket.
	if record == nil || record.SetupState == deviceSetupStateNotConfigured {
		return nil, errors.New("device setup must run before completion import")
	}
	if strings.HasPrefix(strings.TrimSpace(encodedCompletion), deviceLocalCompletionPrefix) {
		return applyLocalDeviceCompletion(record, encodedCompletion, now)
	}
	completion, err := decodeDeviceCompletion(encodedCompletion)
	if err != nil {
		return nil, err
	}
	payload, err := decodeDeviceStoredTicketPayload(record)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(completion.GetNonce(), payload.GetNonce()) {
		return nil, errors.New("device completion nonce does not match setup ticket")
	}

	// Copy the completion onto the setup record and fill a missing session ID.
	updated := *record
	updated.Completion = strings.TrimSpace(encodedCompletion)
	updated.CompletionAt = now.Unix()
	updated.CompletionStatus = deviceCompletionStatusLabel(completion.GetStatus())
	updated.FailureReason = ""
	if updated.SessionID == "" && record.PeerID != "" {
		pid, err := peer.IDB58Decode(record.PeerID)
		if err != nil {
			return nil, errors.Wrap(err, "parse setup peer id")
		}
		updated.SessionID = deviceSessionID(pid)
	}

	// Apply the completion status to the copied record.
	switch completion.GetStatus() {
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK:
		return applySuccessfulDeviceCompletion(&updated, completion, payload)
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_DENIED,
		s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_EXPIRED,
		s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_ERROR:
		updated.SetupState = deviceSetupStateFailed
		updated.FailureReason = deviceCompletionFailureReason(completion)
		updated.AccountID = ""
		updated.ResourceID = ""
		updated.SessionPeerID = ""
		return &updated, nil
	default:
		return nil, errors.New("unsupported device completion status")
	}
}

func applySuccessfulDeviceCompletion(
	record *deviceSetupRecord,
	completion *s4wave_provider_spacewave.SpaceLinkCallback,
	payload *s4wave_provider_spacewave.SpaceLinkAuthRequest,
) (*deviceSetupRecord, error) {
	// Require the account, resource, and session peer from a successful completion.
	if completion.GetAccountId() == "" {
		return nil, errors.New("device completion missing account id")
	}
	if len(completion.GetResourceId()) == 0 {
		return nil, errors.New("device completion missing resource id")
	}
	sessionPeerID, err := peer.IDFromBytes(completion.GetSessionPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "parse completion session peer id")
	}
	if !bytes.Equal(completion.GetSessionPeerId(), payload.GetAgentPeerId()) {
		return nil, errors.New("device completion session peer does not match setup ticket")
	}
	if record.PeerID != "" && sessionPeerID.String() != record.PeerID {
		return nil, errors.New("device completion session peer does not match setup state")
	}

	// Record the imported account, resource, and session peer.
	record.SetupState = deviceSetupStateImported
	record.AccountID = completion.GetAccountId()
	record.ResourceID = base64.StdEncoding.EncodeToString(completion.GetResourceId())
	if record.SessionID == "" {
		record.SessionID = deviceSessionID(sessionPeerID)
	}
	record.SessionPeerID = sessionPeerID.String()
	record.FailureReason = ""
	return record, nil
}

// applyLocalDeviceCompletion imports a local SpaceLink completion. The local
// completion carries a targeted invite instead of a cloud account ID; the
// Device creates its own local session from its durable key when the
// enrollment is activated.
func applyLocalDeviceCompletion(record *deviceSetupRecord, encodedCompletion string, now time.Time) (*deviceSetupRecord, error) {
	// Decode the local completion and require its nonce to match the setup ticket.
	if record == nil || record.SetupState == deviceSetupStateNotConfigured {
		return nil, errors.New("device setup must run before completion import")
	}
	encodedCompletion = strings.TrimSpace(encodedCompletion)
	completion, err := decodeDeviceLocalCompletion(encodedCompletion)
	if err != nil {
		return nil, err
	}
	payload, err := decodeDeviceStoredTicketPayload(record)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(completion.GetNonce(), payload.GetNonce()) {
		return nil, errors.New("device completion nonce does not match setup ticket")
	}

	// Require the session peer and resource to match the setup state.
	sessionPeerID, err := peer.IDFromBytes(completion.GetSessionPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "parse completion session peer id")
	}
	if !bytes.Equal(completion.GetSessionPeerId(), payload.GetAgentPeerId()) {
		return nil, errors.New("device completion session peer does not match setup ticket")
	}
	if record.PeerID != "" && sessionPeerID.String() != record.PeerID {
		return nil, errors.New("device completion session peer does not match setup state")
	}
	if len(completion.GetResourceId()) == 0 {
		return nil, errors.New("device completion missing resource id")
	}

	// Copy the local completion into an imported setup record.
	updated := *record
	updated.Completion = encodedCompletion
	updated.CompletionAt = now.Unix()
	updated.CompletionStatus = deviceCompletionStatusLabel(s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK)
	updated.SetupState = deviceSetupStateImported
	updated.FailureReason = ""
	updated.AccountID = ""
	updated.ResourceID = base64.StdEncoding.EncodeToString([]byte(completion.GetResourceId()))

	// Fill the session ID and peer from the completion.
	if updated.SessionID == "" {
		updated.SessionID = deviceSessionID(sessionPeerID)
	}
	updated.SessionPeerID = sessionPeerID.String()
	return &updated, nil
}

func openDeviceSession(
	ctx context.Context,
	client *sdkClient,
	statePath string,
	record *deviceSetupRecord,
) (*deviceSetupRecord, error) {
	// Reject a local completion or an identity that cannot open a linked session.
	if strings.HasPrefix(record.Completion, deviceLocalCompletionPrefix) {
		return openLocalDeviceSession(ctx, client, statePath, record)
	}
	if record.AccountID == "" {
		return nil, errors.New("device completion missing account id")
	}
	if record.SessionID == "" {
		return nil, errors.New("device session id is missing")
	}
	_, pid, pemData, err := loadDeviceIdentity(deviceIdentityKeyPath(statePath))
	if err != nil {
		return nil, err
	}
	if record.SessionPeerID != "" && pid.String() != record.SessionPeerID {
		return nil, errors.New("device identity does not match imported completion")
	}
	if record.PeerID != "" && pid.String() != record.PeerID {
		return nil, errors.New("device identity does not match setup state")
	}

	// Mount the linked device session.
	resp, err := deviceMountLinkedSession(ctx, client, &s4wave_provider_spacewave.MountLinkedDeviceSessionRequest{
		AccountId:            record.AccountID,
		SessionId:            record.SessionID,
		Label:                record.Label,
		SessionPemPrivateKey: pemData,
		SessionPeerId:        pid.String(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "mount linked device session")
	}
	entry := resp.GetSessionListEntry()
	if entry == nil {
		return nil, errors.New("mount linked device session returned no session entry")
	}

	// Mark the session ready and upsert the Device object.
	updated := *record
	updated.SetupState = deviceSetupStateSessionReady
	updated.SessionIndex = entry.GetSessionIndex()
	updated.SessionPeerID = pid.String()
	objectKey, err := deviceUpsertObject(ctx, client, statePath, &updated, nil)
	if err != nil {
		return nil, errors.Wrap(err, "create or update device object")
	}
	updated.DeviceObjectKey = objectKey
	return &updated, nil
}

// openLocalDeviceSession activates a local SpaceLink enrollment: the daemon's
// local provider creates or reopens the Device's own session from its durable
// key and joins the target Space through the one-use targeted invite. It then
// waits for the joined World to accept the Device object.
func openLocalDeviceSession(
	ctx context.Context,
	client *sdkClient,
	statePath string,
	record *deviceSetupRecord,
) (*deviceSetupRecord, error) {
	// Mount the local session and persist it as imported.
	updated, err := deviceMountLocalSession(ctx, client, statePath, record)
	if err != nil {
		return nil, err
	}
	updated.SetupState = deviceSetupStateImported
	updated.DeviceObjectKey = ""
	updated.FailureReason = ""
	if err := writeDeviceSetupRecord(statePath, updated); err != nil {
		return nil, errors.Wrap(err, "persist activated Device session")
	}

	// Project the Device object, reporting while the World is not writable.
	return projectDeviceEnrollment(ctx, client, statePath, updated, func(err error) {
		fmt.Fprintf(os.Stderr, "waiting for the Space World to accept the Device: %v\n", err)
	})
}

// projectDeviceEnrollment writes the session-ready Device object for an
// activated record and persists the result. onBlocked, when set, reports each
// failed write while the projection waits for the World to advance; without it
// the first failure returns. A failed first projection persists the record as
// imported with its reason; a failed repair of a projected record changes
// nothing. The result is not persisted when another completion replaced the
// record during the projection.
func projectDeviceEnrollment(
	ctx context.Context,
	client *sdkClient,
	statePath string,
	record *deviceSetupRecord,
	onBlocked func(error),
) (*deviceSetupRecord, error) {
	// Project the record as session-ready.
	ready := *record
	ready.SetupState = deviceSetupStateSessionReady
	ready.FailureReason = ""
	objectKey, err := deviceUpsertObject(ctx, client, statePath, &ready, onBlocked)

	// Keep a newer enrollment that replaced this record while projecting.
	current, readErr := readDeviceSetupRecord(statePath)
	if readErr != nil {
		return nil, readErr
	}
	if current.Completion != record.Completion {
		return nil, errors.New("device setup record was replaced during projection")
	}

	// Record a failed first projection as pending.
	if err != nil {
		if record.DeviceObjectKey != "" {
			return nil, err
		}
		pending := *record
		pending.SetupState = deviceSetupStateImported
		pending.FailureReason = "Device object projection pending: " + err.Error()
		if persistErr := writeDeviceSetupRecord(statePath, &pending); persistErr != nil {
			return nil, errors.Wrapf(persistErr, "persist pending Device projection after: %v", err)
		}
		return &pending, nil
	}

	// Persist the ready record with its object key.
	ready.DeviceObjectKey = objectKey
	if err := writeDeviceSetupRecord(statePath, &ready); err != nil {
		return nil, err
	}
	return &ready, nil
}

// restoreLocalDeviceEnrollment retains the persisted local Device session on
// daemon startup and reasserts its P2P enrollment without replaying the invite.
// It then projects the Device object in the background, waiting for the World
// to accept it, so an interrupted or stale projection is repaired. The
// returned release stops the projection and drops the session.
func restoreLocalDeviceEnrollment(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	mount func(uint32) (localSessionMount, error),
) (func(), error) {
	// Read the setup record and skip restore unless a local completion already has a session.
	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(record.Completion, deviceLocalCompletionPrefix) || record.SessionIndex == 0 {
		return nil, nil
	}

	// Remount the local session and save the restored record.
	sess, err := mount(record.SessionIndex)
	if err != nil {
		return nil, err
	}
	updated, err := deviceMountLocalSession(ctx, client, statePath, record)
	if err != nil {
		sess.Release()
		return nil, err
	}
	if updated.DeviceObjectKey == "" {
		updated.SetupState = deviceSetupStateImported
	}
	if err := writeDeviceSetupRecord(statePath, updated); err != nil {
		sess.Release()
		return nil, err
	}

	// Project the Device object while the session is retained.
	projectCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := projectDeviceEnrollment(projectCtx, client, statePath, updated, func(err error) {
			le.WithError(err).Warn("Device object projection waiting for the World")
		})
		if err != nil && projectCtx.Err() == nil {
			le.WithError(err).Warn("Device object projection failed")
		}
	}()
	return func() {
		cancel()
		<-done
		sess.Release()
	}, nil
}

// mountLocalDeviceSession restores the durable session and P2P enrollment
// without requiring the Space World to be writable. The local session keeper
// retains this mount while the daemon serves requests.
func mountLocalDeviceSession(
	ctx context.Context,
	client *sdkClient,
	statePath string,
	record *deviceSetupRecord,
) (*deviceSetupRecord, error) {
	// Decode the local completion and load the matching device identity.
	completion, err := decodeDeviceLocalCompletion(record.Completion)
	if err != nil {
		return nil, err
	}
	_, pid, pemData, err := loadDeviceIdentity(deviceIdentityKeyPath(statePath))
	if err != nil {
		return nil, err
	}
	if record.PeerID != "" && pid.String() != record.PeerID {
		return nil, errors.New("device identity does not match setup state")
	}
	if record.SessionPeerID != "" && pid.String() != record.SessionPeerID {
		return nil, errors.New("device identity does not match imported completion")
	}
	if len(completion.GetSessionPeerId()) != 0 && !bytes.Equal([]byte(pid), completion.GetSessionPeerId()) {
		return nil, errors.New("device identity does not match approval completion")
	}

	// Complete the Space Link enrollment with the local provider.
	prov, cleanup, err := client.lookupLocalProvider(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	resp, err := prov.CompleteSpaceLinkEnrollment(ctx, &s4wave_provider_local.CompleteSpaceLinkEnrollmentRequest{
		SessionPemPrivateKey: pemData,
		SessionPeerId:        pid.String(),
		Invite:               completion.GetInvite(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "complete local SpaceLink enrollment")
	}
	entry := resp.GetSessionListEntry()
	if entry == nil {
		return nil, errors.New("local SpaceLink enrollment returned no session entry")
	}

	// Record the ready session from the enrollment response.
	updated := *record
	updated.SetupState = deviceSetupStateSessionReady
	updated.SessionIndex = entry.GetSessionIndex()
	updated.SessionPeerID = pid.String()
	updated.AccountID = entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
	return &updated, nil
}

func upsertLinkedDeviceObject(
	ctx context.Context,
	client *sdkClient,
	statePath string,
	record *deviceSetupRecord,
	onBlocked func(error),
) (string, error) {
	// Decode the resource ID and require a session index.
	if record == nil {
		return "", errors.New("device setup state is required")
	}
	spaceID, err := decodeDeviceResourceID(record.ResourceID)
	if err != nil {
		return "", err
	}
	if record.SessionIndex == 0 {
		return "", errors.New("device session index is required")
	}

	// Mount the linked session.
	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return "", err
	}
	defer sess.Release()

	// Mount the Space service for the linked resource.
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return "", err
	}
	defer spaceCleanup()

	// Open the World engine through the Space service.
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return "", err
	}
	defer engineCleanup()

	// Read the device policy.
	policy, err := device_policy.ReadFile(statePath)
	if err != nil {
		return "", err
	}

	// Upsert the Device object.
	return upsertLinkedDeviceObjectInWorld(ctx, engine, record, policy, time.Now(), onBlocked)
}

// upsertLinkedDeviceObjectInWorld replays the Device projection from a fresh
// World snapshot when another writer advances the SharedObject root. When
// onBlocked is set, every other failed write is reported and retried once the
// World advances past the revision it was attempted against, such as after a
// block the replay waits for arrives. Without onBlocked it returns the error.
func upsertLinkedDeviceObjectInWorld(
	ctx context.Context,
	engine world.Engine,
	record *deviceSetupRecord,
	policy *device_policy.DevicePolicy,
	now time.Time,
	onBlocked func(error),
) (string, error) {
	objectKey := deviceObjectKey(record.PeerID)
	stale := 0
	for {
		// Note the revision this attempt writes against.
		seqno, err := engine.GetSeqno(ctx)
		if err != nil {
			return "", err
		}

		// Reopen the complete read/merge/write transaction after a stale base.
		err = upsertLinkedDeviceObjectAttempt(ctx, engine, objectKey, record, policy, now)
		if err == nil {
			return objectKey, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(err, coord.ErrStaleGeneration) {
			stale++
			if stale < 10 {
				continue
			}
		}
		if onBlocked == nil {
			return "", err
		}

		// Wait for the World to advance before the next attempt.
		onBlocked(err)
		stale = 0
		if _, err := engine.WaitSeqno(ctx, seqno+1); err != nil {
			return "", err
		}
	}
}

// upsertLinkedDeviceObjectAttempt reads and writes the Device in one World
// transaction, releasing every acquired object before the next attempt.
func upsertLinkedDeviceObjectAttempt(
	ctx context.Context,
	engine world.Engine,
	objectKey string,
	record *deviceSetupRecord,
	policy *device_policy.DevicePolicy,
	now time.Time,
) error {
	// Open a write transaction on the World engine.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	// Merge the setup record into the Device object and commit it.
	next := deviceObjectFromSetupRecord(record, now)
	existingState, found, err := tx.GetObject(ctx, objectKey)
	defer world.ReleaseObjectState(existingState)
	if err != nil {
		return err
	}
	if found {
		existing, err := readDeviceBlock(ctx, existingState)
		if err != nil {
			return err
		}
		if existing != nil && existing.GetPeerId() != "" && existing.GetPeerId() != record.PeerID {
			return errors.New("existing device object peer_id does not match setup state")
		}
		mergeDeviceObjectState(next, existing)
		projected, _, err := projectDevicePolicyOntoDevice(next, policy, now)
		if err != nil {
			return err
		}
		next = projected
		_, _, err = world.AccessObjectState(ctx, existingState, true, func(bcs *block.Cursor) error {
			bcs.SetBlock(next, true)
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		projected, _, err := projectDevicePolicyOntoDevice(next, policy, now)
		if err != nil {
			return err
		}
		next = projected
		var createdObject world.ObjectState
		createdObject, _, err = world.CreateWorldObject(ctx, tx, objectKey, func(bcs *block.Cursor) error {
			bcs.ClearAllRefs()
			bcs.SetBlock(next, true)
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return err
		}
		if err := world_types.SetObjectType(ctx, tx, objectKey, s4wave_device.DeviceTypeID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func decodeDeviceResourceID(encoded string) (string, error) {
	// Decode the resource ID from the setup record.
	if strings.TrimSpace(encoded) == "" {
		return "", errors.New("device completion resource id is missing")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", errors.Wrap(err, "decode device completion resource id")
	}
	if len(data) == 0 {
		return "", errors.New("device completion resource id is empty")
	}
	return string(data), nil
}

func readDeviceBlock(ctx context.Context, objState world.ObjectState) (*s4wave_device.Device, error) {
	var state *s4wave_device.Device
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = s4wave_device.UnmarshalDevice(ctx, bcs)
		return uerr
	})
	return state, err
}

func deviceObjectFromSetupRecord(record *deviceSetupRecord, now time.Time) *s4wave_device.Device {
	ts := timestamppb.New(now)
	return &s4wave_device.Device{
		PeerId:        record.PeerID,
		Label:         record.Label,
		Platform:      &s4wave_device.DevicePlatform{Os: runtime.GOOS, Arch: runtime.GOARCH},
		DaemonVersion: "unknown",
		SetupState:    deviceSetupStateProto(record.SetupState),
		UpdateState:   s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE,
		LastStatus: &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    "device session ready",
			ObservedAt: ts.CloneVT(),
		},
		CreatedAt: ts.CloneVT(),
		UpdatedAt: ts,
	}
}

func mergeDeviceObjectState(next *s4wave_device.Device, existing *s4wave_device.Device) {
	// Keep the existing created-at, status, update state, and capabilities.
	if next == nil || existing == nil {
		return
	}
	if existing.GetCreatedAt() != nil {
		next.CreatedAt = existing.GetCreatedAt().CloneVT()
	}
	if existing.GetLastStatus() != nil {
		next.LastStatus = existing.GetLastStatus().CloneVT()
	}
	if existing.GetUpdateState() != s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_UNKNOWN {
		next.UpdateState = existing.GetUpdateState()
	}
	if caps := existing.GetCapabilities(); len(caps) > 0 {
		next.Capabilities = make([]*s4wave_device.DeviceCapability, 0, len(caps))
		for _, cap := range caps {
			if cap == nil {
				continue
			}
			next.Capabilities = append(next.Capabilities, cap.CloneVT())
		}
	}
}

func deviceSetupStateProto(state string) s4wave_device.DeviceSetupState {
	switch state {
	case deviceSetupStateWaiting:
		return s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_WAITING_FOR_COMPLETION
	case deviceSetupStateImported:
		return s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_COMPLETION_IMPORTED
	case deviceSetupStateSessionReady:
		return s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY
	case deviceSetupStateFailed:
		return s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_FAILED
	default:
		return s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_UNKNOWN
	}
}

func deviceObjectKey(peerID string) string {
	sum := sha256.Sum256([]byte(peerID))
	return "devices/" + hex.EncodeToString(sum[:])[:32]
}

func decodeDeviceCompletion(encoded string) (*s4wave_provider_spacewave.SpaceLinkCallback, error) {
	// Decode the Space Link callback completion.
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, errors.New("device completion payload is required")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Wrap(err, "decode device completion")
	}
	completion := &s4wave_provider_spacewave.SpaceLinkCallback{}
	if err := completion.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "parse device completion")
	}
	return completion, nil
}

func decodeDeviceStoredTicketPayload(record *deviceSetupRecord) (*s4wave_provider_spacewave.SpaceLinkAuthRequest, error) {
	// Decode the stored Space Link ticket payload.
	if record == nil || strings.TrimSpace(record.Ticket) == "" {
		return nil, errors.New("device setup ticket is missing")
	}
	ticketBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(record.Ticket))
	if err != nil {
		return nil, errors.Wrap(err, "decode device setup ticket")
	}
	ticket := &s4wave_provider_spacewave.SpaceLinkAuthTicket{}
	if err := ticket.UnmarshalVT(ticketBytes); err != nil {
		return nil, errors.Wrap(err, "parse device setup ticket")
	}
	payload := &s4wave_provider_spacewave.SpaceLinkAuthRequest{}
	if err := payload.UnmarshalVT(ticket.GetPayload()); err != nil {
		return nil, errors.Wrap(err, "parse device setup ticket payload")
	}
	return payload, nil
}

func deviceCompletionStatusLabel(status s4wave_provider_spacewave.SpaceLinkCallbackStatus) string {
	switch status {
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK:
		return "ok"
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_DENIED:
		return "denied"
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_EXPIRED:
		return "expired"
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_ERROR:
		return "error"
	default:
		return "unknown"
	}
}

func deviceCompletionFailureReason(completion *s4wave_provider_spacewave.SpaceLinkCallback) string {
	if completion.GetErrorMessage() != "" {
		return completion.GetErrorMessage()
	}
	switch completion.GetStatus() {
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_DENIED:
		return "approval denied"
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_EXPIRED:
		return "approval expired"
	case s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_ERROR:
		return "approval failed"
	default:
		return "approval failed"
	}
}

func deviceIdentityKeyPath(statePath string) string {
	return filepath.Join(statePath, deviceSetupStateDir, deviceIdentityKeyPEMFile)
}

func deviceSessionID(pid peer.ID) string {
	sum := sha256.Sum256([]byte(pid))
	return "device-" + hex.EncodeToString(sum[:])[:32]
}

func marshalDeviceSetupRecord(record *deviceSetupRecord) []byte {
	// Start the setup record object with its identity fields.
	var arena fastjson.Arena
	obj := arena.NewObject()
	obj.Set("setupState", arena.NewString(record.SetupState))
	setDeviceJSONString(&arena, obj, "peerId", record.PeerID)
	setDeviceJSONString(&arena, obj, "label", record.Label)
	setDeviceJSONString(&arena, obj, "requestedRole", record.RequestedRole)
	setDeviceJSONString(&arena, obj, "targetHint", record.TargetHint)
	setDeviceJSONString(&arena, obj, "completionMode", record.CompletionMode)

	// Record the completion, account, and session fields.
	setDeviceJSONString(&arena, obj, "completion", record.Completion)
	setDeviceJSONInt64(&arena, obj, "completionAt", record.CompletionAt)
	setDeviceJSONString(&arena, obj, "completionStatus", record.CompletionStatus)
	setDeviceJSONString(&arena, obj, "accountId", record.AccountID)
	setDeviceJSONString(&arena, obj, "resourceId", record.ResourceID)
	setDeviceJSONString(&arena, obj, "sessionId", record.SessionID)
	setDeviceJSONUint32(&arena, obj, "sessionIndex", record.SessionIndex)
	setDeviceJSONString(&arena, obj, "sessionPeerId", record.SessionPeerID)

	// Record the Device object, failure, expiry, and ticket, then marshal the object.
	setDeviceJSONString(&arena, obj, "deviceObjectKey", record.DeviceObjectKey)
	setDeviceJSONString(&arena, obj, "failureReason", record.FailureReason)
	setDeviceJSONInt64(&arena, obj, "expiresAt", record.ExpiresAt)
	setDeviceJSONString(&arena, obj, "ticket", record.Ticket)
	return obj.MarshalTo(nil)
}

func parseDeviceSetupRecord(data []byte) (*deviceSetupRecord, error) {
	// Parse the setup record JSON.
	var parser fastjson.Parser
	v, err := parser.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	if v.Type() != fastjson.TypeObject {
		return nil, errors.New("device setup state must be object")
	}
	sessionIndex := v.GetUint("sessionIndex")
	if sessionIndex > math.MaxUint32 {
		return nil, errors.Errorf("device setup session index exceeds uint32 range: %d", sessionIndex)
	}
	return &deviceSetupRecord{
		SetupState:       string(v.GetStringBytes("setupState")),
		PeerID:           string(v.GetStringBytes("peerId")),
		Label:            string(v.GetStringBytes("label")),
		RequestedRole:    string(v.GetStringBytes("requestedRole")),
		TargetHint:       string(v.GetStringBytes("targetHint")),
		CompletionMode:   string(v.GetStringBytes("completionMode")),
		Completion:       string(v.GetStringBytes("completion")),
		CompletionAt:     v.GetInt64("completionAt"),
		CompletionStatus: string(v.GetStringBytes("completionStatus")),
		AccountID:        string(v.GetStringBytes("accountId")),
		ResourceID:       string(v.GetStringBytes("resourceId")),
		SessionID:        string(v.GetStringBytes("sessionId")),
		SessionIndex:     uint32(sessionIndex), //nolint:gosec // the MaxUint32 check above bounds persisted setup state.
		SessionPeerID:    string(v.GetStringBytes("sessionPeerId")),
		DeviceObjectKey:  string(v.GetStringBytes("deviceObjectKey")),
		FailureReason:    string(v.GetStringBytes("failureReason")),
		ExpiresAt:        v.GetInt64("expiresAt"),
		Ticket:           string(v.GetStringBytes("ticket")),
	}, nil
}

func marshalDeviceStatusOutput(out deviceStatusOutput) []byte {
	// Start the status object with the daemon path and Device label.
	var arena fastjson.Arena
	obj := arena.NewObject()
	obj.Set("daemonStatus", arena.NewString(out.DaemonStatus))
	obj.Set("setupState", arena.NewString(out.SetupState))
	obj.Set("statePath", arena.NewString(out.StatePath))
	obj.Set("socket", arena.NewString(out.Socket))
	setDeviceJSONString(&arena, obj, "peerId", out.PeerID)
	setDeviceJSONString(&arena, obj, "label", out.Label)

	// Record the requested role, completion, account, and session ID.
	setDeviceJSONString(&arena, obj, "requestedRole", out.RequestedRole)
	setDeviceJSONString(&arena, obj, "targetHint", out.TargetHint)
	setDeviceJSONString(&arena, obj, "completionMode", out.CompletionMode)
	setDeviceJSONInt64(&arena, obj, "completionAt", out.CompletionAt)
	setDeviceJSONString(&arena, obj, "completionStatus", out.CompletionStatus)
	setDeviceJSONString(&arena, obj, "accountId", out.AccountID)
	setDeviceJSONString(&arena, obj, "resourceId", out.ResourceID)
	setDeviceJSONString(&arena, obj, "sessionId", out.SessionID)

	// Record the session index, Device object, expiry, and identity flag, then marshal the object.
	setDeviceJSONUint32(&arena, obj, "sessionIndex", out.SessionIndex)
	setDeviceJSONString(&arena, obj, "sessionPeerId", out.SessionPeerID)
	setDeviceJSONString(&arena, obj, "deviceObjectKey", out.DeviceObjectKey)
	setDeviceJSONString(&arena, obj, "failureReason", out.FailureReason)
	setDeviceJSONInt64(&arena, obj, "expiresAt", out.ExpiresAt)
	setDeviceJSONString(&arena, obj, "ticket", out.Ticket)
	if out.IdentityCreated {
		obj.Set("identityCreated", arena.NewTrue())
	}
	return obj.MarshalTo(nil)
}

func marshalDeviceDockerSetupReport(report *deviceDockerSetupReport) []byte {
	// Start the Docker report with the label and state paths.
	var arena fastjson.Arena
	obj := arena.NewObject()
	obj.Set("label", arena.NewString(report.Label))
	obj.Set("statePath", arena.NewString(report.StatePath))
	obj.Set("socket", arena.NewString(report.Socket))
	obj.Set("containerStatePath", arena.NewString(report.ContainerStatePath))

	// Record the session type, role, completion, and ticket, then marshal the report.
	obj.Set("sessionType", arena.NewString(report.SessionType))
	obj.Set("requestedRole", arena.NewString(report.RequestedRole))
	obj.Set("completion", arena.NewString(report.Completion))
	obj.Set("enrollment", arena.NewString(report.Enrollment))
	obj.Set("ticket", arena.NewString(report.Ticket))
	return obj.MarshalTo(nil)
}

func setDeviceJSONString(arena *fastjson.Arena, obj *fastjson.Value, key, value string) {
	if value != "" {
		obj.Set(key, arena.NewString(value))
	}
}

func setDeviceJSONInt64(arena *fastjson.Arena, obj *fastjson.Value, key string, value int64) {
	if value != 0 {
		obj.Set(key, arena.NewNumberString(strconv.FormatInt(value, 10)))
	}
}

func setDeviceJSONUint32(arena *fastjson.Arena, obj *fastjson.Value, key string, value uint32) {
	if value != 0 {
		obj.Set(key, arena.NewNumberString(strconv.FormatUint(uint64(value), 10)))
	}
}

func writeDeviceSetupRecord(statePath string, record *deviceSetupRecord) error {
	// Write the setup record, defaulting an empty setup state.
	if record == nil {
		record = &deviceSetupRecord{SetupState: deviceSetupStateNotConfigured}
	}
	if record.SetupState == "" {
		record.SetupState = deviceSetupStateNotConfigured
	}
	data := marshalDeviceSetupRecord(record)
	data = append(data, '\n')
	path := deviceSetupRecordPath(statePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Wrap(err, "create device setup state directory")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return errors.Wrap(err, "write device setup state")
	}
	return nil
}

func readDeviceSetupRecord(statePath string) (*deviceSetupRecord, error) {
	// Read the setup record, treating a missing file as unconfigured.
	data, err := os.ReadFile(deviceSetupRecordPath(statePath))
	if os.IsNotExist(err) {
		return &deviceSetupRecord{SetupState: deviceSetupStateNotConfigured}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "read device setup state")
	}
	record, err := parseDeviceSetupRecord(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse device setup state")
	}
	if record.SetupState == "" {
		record.SetupState = deviceSetupStateNotConfigured
	}
	return record, nil
}

func deviceSetupRecordPath(statePath string) string {
	return filepath.Join(statePath, deviceSetupStateDir, deviceSetupStateFile)
}

func writeDeviceStatusOutput(out deviceStatusOutput, outputFormat string) error {
	switch outputFormat {
	case "json", "yaml":
		data := marshalDeviceStatusOutput(out)
		return formatOutput(data, outputFormat)
	case "text", "table":
		fields := [][2]string{
			{"Daemon", formatDeviceStatusLabel(out.DaemonStatus)},
			{"Setup", formatDeviceStatusLabel(out.SetupState)},
			{"State Path", out.StatePath},
			{"Socket", out.Socket},
		}
		if out.PeerID != "" {
			fields = append(fields, [2]string{"Peer", out.PeerID})
		}
		if out.Label != "" {
			fields = append(fields, [2]string{"Label", out.Label})
		}
		if out.RequestedRole != "" {
			fields = append(fields, [2]string{"Requested Role", out.RequestedRole})
		}
		if out.TargetHint != "" {
			fields = append(fields, [2]string{"Target Hint", out.TargetHint})
		}
		if out.CompletionMode != "" {
			fields = append(fields, [2]string{"Completion", formatDeviceStatusLabel(out.CompletionMode)})
		}
		if out.CompletionStatus != "" {
			fields = append(fields, [2]string{"Completion Status", formatDeviceStatusLabel(out.CompletionStatus)})
		}
		if out.AccountID != "" {
			fields = append(fields, [2]string{"Account", out.AccountID})
		}
		if out.ResourceID != "" {
			fields = append(fields, [2]string{"Resource", out.ResourceID})
		}
		if out.SessionID != "" {
			fields = append(fields, [2]string{"Session", out.SessionID})
		}
		if out.SessionIndex != 0 {
			fields = append(fields, [2]string{"Session Index", strconv.FormatUint(uint64(out.SessionIndex), 10)})
		}
		if out.SessionPeerID != "" {
			fields = append(fields, [2]string{"Session Peer", out.SessionPeerID})
		}
		if out.DeviceObjectKey != "" {
			fields = append(fields, [2]string{"Device Object", out.DeviceObjectKey})
		}
		if out.FailureReason != "" {
			fields = append(fields, [2]string{"Failure", out.FailureReason})
		}
		if out.CompletionAt != 0 {
			fields = append(fields, [2]string{"Completed At", time.Unix(out.CompletionAt, 0).Format(time.RFC3339)})
		}
		if out.ExpiresAt != 0 {
			fields = append(fields, [2]string{"Expires At", time.Unix(out.ExpiresAt, 0).Format(time.RFC3339)})
		}
		if out.Ticket != "" {
			fields = append(fields, [2]string{"Ticket", out.Ticket})
		}
		writeFields(os.Stdout, fields)
		return nil
	default:
		return formatOutput(nil, outputFormat)
	}
}

func writeDeviceDockerSetupReport(report *deviceDockerSetupReport, outputFormat string) error {
	switch outputFormat {
	case "json", "yaml":
		data := marshalDeviceDockerSetupReport(report)
		return formatOutput(data, outputFormat)
	case "text", "table":
		writeFields(os.Stdout, [][2]string{
			{"Label", report.Label},
			{"State Path", report.StatePath},
			{"Socket", report.Socket},
			{"Container State Path", report.ContainerStatePath},
			{"Session Type", report.SessionType},
			{"Requested Role", report.RequestedRole},
			{"Completion", report.Completion},
			{"Enrollment", report.Enrollment},
			{"Ticket", report.Ticket},
		})
		return nil
	default:
		return formatOutput(nil, outputFormat)
	}
}

func formatDeviceStatusLabel(value string) string {
	return strings.ReplaceAll(value, "_", " ")
}
