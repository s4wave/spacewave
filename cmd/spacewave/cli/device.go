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
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
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

func newDeviceCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "device",
		Usage: "manage Spacewave-managed Device setup",
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
		FailureReason:    record.FailureReason,
		ExpiresAt:        record.ExpiresAt,
		Ticket:           record.Ticket,
	}, outputFormat)
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
	return &updated, nil
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

func formatDeviceStatusLabel(value string) string {
	return strings.ReplaceAll(value, "_", " ")
}
