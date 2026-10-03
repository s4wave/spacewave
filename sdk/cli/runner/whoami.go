package runner

import (
	"context"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
)

// RunWhoami executes the shared whoami command against the configured client factory.
func RunWhoami(config Config, c *cli.Context, outputFormat string, sessionIdx uint32) error {
	// Connect to the daemon using the configured client factory.
	config = config.defaults()
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := config.ClientFactory.NewClient(ctx, c)
	if err != nil {
		return err
	}
	defer client.Close()

	// Mount the requested session for identity readback.
	sess, err := client.MountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Read the mounted session's identifying information.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}

	// Read the session provider reference and lock state.
	ref := info.GetSessionRef().GetProviderResourceRef()
	lockStr := readLockState(ctx, sess, "unlocked (auto)")

	// Write the structured session identity report.
	if outputFormat == "json" || outputFormat == "yaml" {
		// Begin the identity object with the session identifier.
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionId")
		ms.WriteString(ref.GetId())

		// Write the session peer and provider identifiers.
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("peerId")
		ms.WriteString(info.GetPeerId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerId")
		ms.WriteString(ref.GetProviderId())

		// Write the provider account and session lock state, then finish the identity report.
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerAccountId")
		ms.WriteString(ref.GetProviderAccountId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("lock")
		ms.WriteString(lockStr)
		ms.WriteObjectEnd()
		return formatOutput(config.Stdout, buf.Bytes(), outputFormat)
	}

	// Write the text session identity report.
	writeFields(config.Stdout, [][2]string{
		{"Session", ref.GetId()},
		{"Peer", info.GetPeerId()},
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Lock", lockStr},
	})
	return nil
}
