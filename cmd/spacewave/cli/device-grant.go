//go:build !js

package spacewave_cli

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/pkg/errors"
	os_image "github.com/s4wave/spacewave/os/image"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// deviceCompletionModeGrant marks a setup record whose completion the Device
// built from a bootstrap grant instead of importing an approval.
const deviceCompletionModeGrant = "grant"

// errDeviceEnrollmentPending reports a redemption the Space owner holds for
// approval. The Device redeems again with the same key until the owner
// admits or refuses the Device.
var errDeviceEnrollmentPending = errors.New("local SpaceLink enrollment is waiting for the Space owner's approval")

// deviceGrantRetryMin and deviceGrantRetryMax bound the wait between
// redemptions of a bootstrap grant. The wait doubles from the minimum while
// the owner has not decided or the owner is unreachable.
var (
	deviceGrantRetryMin = 2 * time.Second
	deviceGrantRetryMax = time.Minute
)

// runGrantEnrollment enrolls the Device through the bootstrap grant at
// grantPath, then holds the restored enrollment until ctx ends.
//
// The requester holds no link to the owner's decision, so a pending Device
// redeems the grant again after a doubling wait. The owner answers a repeat of
// a queued request without writing. A Device enrolled on an earlier boot
// skips the redemption and restores at once.
func runGrantEnrollment(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	grantPath string,
	client *sdkClient,
	mount func(sessionIdentity) (localSessionMount, error),
) {
	// Enroll and restore, waiting longer after each pending or failed attempt.
	var release func()
	for wait := deviceGrantRetryMin; ; wait = min(2*wait, deviceGrantRetryMax) {
		var err error
		release, err = enrollThroughGrant(ctx, le, statePath, grantPath, client, mount)
		if err == nil {
			break
		}
		if errors.Is(err, errDeviceEnrollmentPending) {
			le.Info("Device enrollment waiting for the Space owner's approval")
		} else {
			le.WithError(err).Warn("Device enrollment through the bootstrap grant failed")
		}
		select {
		case <-ctx.Done():
			return
		//gostyle:timer backoff
		case <-time.After(wait):
		}
	}

	// Hold the enrollment until the keeper exits.
	le.Info("Device enrolled; its Space session is restored")
	<-ctx.Done()
	release()
}

// enrollThroughGrant redeems the bootstrap grant when the Device has no
// session yet, then restores the enrollment as a later boot does.
func enrollThroughGrant(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	grantPath string,
	client *sdkClient,
	mount func(sessionIdentity) (localSessionMount, error),
) (func(), error) {
	// Build the grant's completion into the setup record on first use.
	record, err := prepareGrantEnrollment(statePath, grantPath, time.Now())
	if err != nil {
		return nil, err
	}

	// Redeem the grant until the owner admits the Device's key, recording why
	// each attempt has not finished.
	if _, ok := record.openedSession(); !ok {
		updated, err := deviceMountLocalSession(ctx, client, statePath, record)
		if err != nil {
			record.FailureReason = err.Error()
			if writeErr := writeDeviceSetupRecord(statePath, record); writeErr != nil {
				return nil, errors.Wrapf(err, "persist Device setup state: %v", writeErr)
			}
			return nil, err
		}
		updated.FailureReason = ""
		if err := writeDeviceSetupRecord(statePath, updated); err != nil {
			return nil, err
		}
	}

	// Restore the session and project the Device object.
	return restoreLocalDeviceEnrollment(ctx, le, statePath, client, mount)
}

// prepareGrantEnrollment returns the setup record that enrolls the Device
// through the bootstrap grant at grantPath, making the Device key on first use.
// A record that already holds a local completion is returned unchanged, so a
// later boot reuses the key and Space it enrolled with.
func prepareGrantEnrollment(statePath, grantPath string, now time.Time) (*deviceSetupRecord, error) {
	// Keep an existing local completion.
	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(record.Completion, deviceLocalCompletionPrefix) {
		return record, nil
	}

	// Read the grant and make the Device's own key.
	grant, err := os_image.ReadGrant(grantPath)
	if err != nil {
		return nil, err
	}
	_, pid, _, err := loadOrCreateDeviceIdentity(statePath)
	if err != nil {
		return nil, err
	}

	// Complete the setup with the grant as the Device's own approval.
	completion, err := (&s4wave_session.LocalSpaceLinkCompletion{
		ProviderId:    grant.GetProviderId(),
		ResourceId:    grant.GetSharedObjectId(),
		SessionPeerId: []byte(pid),
		Invite:        grant,
	}).MarshalVT()
	if err != nil {
		return nil, err
	}

	// Name the Device by its key.
	updated := *record
	updated.SetupState = deviceSetupStateImported
	updated.PeerID = pid.String()
	if updated.Label == "" {
		updated.Label = defaultDeviceSetupLabel()
	}

	// Complete the setup with the grant for the Space the grant selects.
	updated.CompletionMode = deviceCompletionModeGrant
	updated.Completion = deviceLocalCompletionPrefix + base64.StdEncoding.EncodeToString(completion)
	updated.CompletionAt = now.Unix()
	updated.ResourceID = base64.StdEncoding.EncodeToString([]byte(grant.GetSharedObjectId()))

	// Give the Device's session its key.
	updated.SessionID = deviceSessionID(pid)
	updated.SessionPeerID = pid.String()

	// Persist the completion so later boots redeem with the same key.
	if err := writeDeviceSetupRecord(statePath, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}
