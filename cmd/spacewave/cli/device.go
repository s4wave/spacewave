//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
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

type deviceSetupRecord struct {
	SetupState       string `json:"setupState"`
	PeerID           string `json:"peerId,omitempty"`
	Label            string `json:"label,omitempty"`
	RequestedRole    string `json:"requestedRole,omitempty"`
	TargetHint       string `json:"targetHint,omitempty"`
	CompletionMode   string `json:"completionMode,omitempty"`
	Completion       string `json:"completion,omitempty"`
	CompletionAt     int64  `json:"completionAt,omitempty"`
	CompletionStatus string `json:"completionStatus,omitempty"`
	AccountID        string `json:"accountId,omitempty"`
	ResourceID       string `json:"resourceId,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	SessionIndex     uint32 `json:"sessionIndex,omitempty"`
	SessionPeerID    string `json:"sessionPeerId,omitempty"`
	DeviceObjectKey  string `json:"deviceObjectKey,omitempty"`
	FailureReason    string `json:"failureReason,omitempty"`
	ExpiresAt        int64  `json:"expiresAt,omitempty"`
	Ticket           string `json:"ticket,omitempty"`
}

type deviceStatusOutput struct {
	DaemonStatus     string `json:"daemonStatus"`
	SetupState       string `json:"setupState"`
	StatePath        string `json:"statePath"`
	Socket           string `json:"socket"`
	PeerID           string `json:"peerId,omitempty"`
	Label            string `json:"label,omitempty"`
	RequestedRole    string `json:"requestedRole,omitempty"`
	TargetHint       string `json:"targetHint,omitempty"`
	CompletionMode   string `json:"completionMode,omitempty"`
	CompletionAt     int64  `json:"completionAt,omitempty"`
	CompletionStatus string `json:"completionStatus,omitempty"`
	AccountID        string `json:"accountId,omitempty"`
	ResourceID       string `json:"resourceId,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	SessionIndex     uint32 `json:"sessionIndex,omitempty"`
	SessionPeerID    string `json:"sessionPeerId,omitempty"`
	DeviceObjectKey  string `json:"deviceObjectKey,omitempty"`
	FailureReason    string `json:"failureReason,omitempty"`
	ExpiresAt        int64  `json:"expiresAt,omitempty"`
	Ticket           string `json:"ticket,omitempty"`
	IdentityCreated  bool   `json:"identityCreated,omitempty"`
}

type deviceSetupArgs struct {
	statePath     string
	outputFormat  string
	label         string
	targetHint    string
	requestedRole string
	expiresIn     time.Duration
}

type deviceCompleteArgs struct {
	statePath    string
	outputFormat string
	completion   string
}

type deviceDockerSetupReport struct {
	Label              string `json:"label"`
	StatePath          string `json:"statePath"`
	Socket             string `json:"socket"`
	ContainerStatePath string `json:"containerStatePath"`
	SessionType        string `json:"sessionType"`
	RequestedRole      string `json:"requestedRole"`
	Completion         string `json:"completion"`
	Enrollment         string `json:"enrollment"`
	Ticket             string `json:"ticket"`
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

var deviceUpsertObject = upsertLinkedDeviceObject

func newDeviceCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	return &cli.Command{
		Name:    "device",
		Aliases: []string{"devices"},
		Usage:   "manage Spacewave-managed Device setup",
		Flags:   daemonClientFlags(&statePath),
		Subcommands: []*cli.Command{
			newDeviceSetupCommand(),
			newDeviceCompleteCommand(),
			newDeviceStatusCommand(),
		},
	}
}

func newDeviceSetupCommand() *cli.Command {
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
		Flags: append(daemonClientFlags(&statePath),
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
			deviceOutputFlag(),
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
		Flags: append(daemonClientFlags(&statePath),
			&cli.StringFlag{
				Name:        "label",
				Usage:       "managed Device label",
				Required:    true,
				Destination: &label,
			},
			deviceOutputFlag(),
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
		Flags: append(daemonClientFlags(&statePath),
			&cli.StringFlag{
				Name:        "completion",
				Usage:       "base64 SpaceLink completion payload",
				Destination: &completion,
			},
			deviceOutputFlag(),
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
		Flags: append(daemonClientFlags(&statePath), deviceOutputFlag()),
		Action: func(c *cli.Context) error {
			return runDeviceStatus(c, statePath, c.String("output"))
		},
	}
}

func deviceOutputFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "output",
		Usage: "output format (text/json/yaml)",
		Value: "text",
	}
}

func runDeviceSetup(c *cli.Context, args deviceSetupArgs) error {
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
	return writeDeviceStatusOutput(deviceStatusOutput{
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
		IdentityCreated:  identityCreated,
	}, args.outputFormat)
}

func runDeviceComplete(c *cli.Context, args deviceCompleteArgs) error {
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
	if updated.SetupState == deviceSetupStateImported {
		activated, err := openDeviceSession(ctx, client, resolvedStatePath, updated)
		if err != nil {
			return err
		}
		updated = activated
		if err := writeDeviceSetupRecord(resolvedStatePath, updated); err != nil {
			return err
		}
	}
	return writeDeviceStatusOutput(deviceStatusOutput{
		DaemonStatus:     "running",
		SetupState:       updated.SetupState,
		StatePath:        resolvedStatePath,
		Socket:           sockPath,
		PeerID:           updated.PeerID,
		Label:            updated.Label,
		RequestedRole:    updated.RequestedRole,
		TargetHint:       updated.TargetHint,
		CompletionMode:   updated.CompletionMode,
		CompletionAt:     updated.CompletionAt,
		CompletionStatus: updated.CompletionStatus,
		AccountID:        updated.AccountID,
		ResourceID:       updated.ResourceID,
		SessionID:        updated.SessionID,
		SessionIndex:     updated.SessionIndex,
		SessionPeerID:    updated.SessionPeerID,
		DeviceObjectKey:  updated.DeviceObjectKey,
		FailureReason:    updated.FailureReason,
		ExpiresAt:        updated.ExpiresAt,
		Ticket:           updated.Ticket,
	}, args.outputFormat)
}

func runDeviceStatus(c *cli.Context, statePath, outputFormat string) error {
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

	record, err := readDeviceSetupRecord(resolvedStatePath)
	if err != nil {
		return err
	}
	return writeDeviceStatusOutput(deviceStatusOutput{
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
	}, outputFormat)
}

func buildDeviceDockerSetupReport(c *cli.Context, statePath string, label string) (*deviceDockerSetupReport, error) {
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
	path := deviceIdentityKeyPath(statePath)
	priv, pid, _, err := loadDeviceIdentity(path)
	if err == nil {
		return priv, pid, false, nil
	}
	if !os.IsNotExist(err) {
		return nil, "", false, err
	}

	priv, _, err = crypto.GenerateEd25519Key(cryptorand.Reader)
	if err != nil {
		return nil, "", false, errors.Wrap(err, "generate device identity")
	}
	pemData, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return nil, "", false, errors.Wrap(err, "marshal device identity")
	}
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
	if args.priv == nil {
		return "", time.Time{}, errors.New("device identity private key is required")
	}
	if args.agentPeerID == "" {
		return "", time.Time{}, errors.New("device peer id is required")
	}
	if args.label == "" {
		return "", time.Time{}, errors.New("device label is required")
	}
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
	if record == nil || record.SetupState == deviceSetupStateNotConfigured {
		return nil, errors.New("device setup must run before completion import")
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

func openDeviceSession(
	ctx context.Context,
	client *sdkClient,
	statePath string,
	record *deviceSetupRecord,
) (*deviceSetupRecord, error) {
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

	updated := *record
	updated.SetupState = deviceSetupStateSessionReady
	updated.SessionIndex = entry.GetSessionIndex()
	updated.SessionPeerID = pid.String()
	objectKey, err := deviceUpsertObject(ctx, client, &updated)
	if err != nil {
		return nil, errors.Wrap(err, "create or update device object")
	}
	updated.DeviceObjectKey = objectKey
	return &updated, nil
}

func upsertLinkedDeviceObject(
	ctx context.Context,
	client *sdkClient,
	record *deviceSetupRecord,
) (string, error) {
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

	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return "", err
	}
	defer sess.Release()

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return "", err
	}
	defer spaceCleanup()

	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return "", err
	}
	defer engineCleanup()

	objectKey := deviceObjectKey(record.PeerID)
	now := time.Now()
	next := deviceObjectFromSetupRecord(record, now)

	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return "", errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	existingState, found, err := tx.GetObject(ctx, objectKey)
	if err != nil {
		return "", err
	}
	if found {
		existing, err := readDeviceBlock(ctx, existingState)
		if err != nil {
			return "", err
		}
		if existing != nil && existing.GetPeerId() != "" && existing.GetPeerId() != record.PeerID {
			return "", errors.New("existing device object peer_id does not match setup state")
		}
		mergeDeviceObjectState(next, existing)
		_, _, err = world.AccessObjectState(ctx, existingState, true, func(bcs *block.Cursor) error {
			bcs.SetBlock(next, true)
			return nil
		})
		if err != nil {
			return "", err
		}
	} else {
		_, _, err = world.CreateWorldObject(ctx, tx, objectKey, func(bcs *block.Cursor) error {
			bcs.ClearAllRefs()
			bcs.SetBlock(next, true)
			return nil
		})
		if err != nil {
			return "", err
		}
		if err := world_types.SetObjectType(ctx, tx, objectKey, s4wave_device.DeviceTypeID); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return objectKey, nil
}

func decodeDeviceResourceID(encoded string) (string, error) {
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

func writeDeviceSetupRecord(statePath string, record *deviceSetupRecord) error {
	if record == nil {
		record = &deviceSetupRecord{SetupState: deviceSetupStateNotConfigured}
	}
	if record.SetupState == "" {
		record.SetupState = deviceSetupStateNotConfigured
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return errors.Wrap(err, "marshal device setup state")
	}
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
	data, err := os.ReadFile(deviceSetupRecordPath(statePath))
	if os.IsNotExist(err) {
		return &deviceSetupRecord{SetupState: deviceSetupStateNotConfigured}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "read device setup state")
	}
	var record deviceSetupRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, errors.Wrap(err, "parse device setup state")
	}
	if record.SetupState == "" {
		record.SetupState = deviceSetupStateNotConfigured
	}
	return &record, nil
}

func deviceSetupRecordPath(statePath string) string {
	return filepath.Join(statePath, deviceSetupStateDir, deviceSetupStateFile)
}

func writeDeviceStatusOutput(out deviceStatusOutput, outputFormat string) error {
	switch outputFormat {
	case "json", "yaml":
		data, err := json.Marshal(out)
		if err != nil {
			return errors.Wrap(err, "marshal device status output")
		}
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
			fields = append(fields, [2]string{"Session Index", fmt.Sprintf("%d", out.SessionIndex)})
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
		data, err := json.Marshal(report)
		if err != nil {
			return errors.Wrap(err, "marshal device docker setup output")
		}
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
