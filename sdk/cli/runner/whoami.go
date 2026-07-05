package runner

import (
	"context"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
)

// RunWhoami executes the shared whoami command against the configured client factory.
func RunWhoami(config Config, c *cli.Context, outputFormat string, sessionIdx uint32) error {
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

	sess, err := client.MountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}

	ref := info.GetSessionRef().GetProviderResourceRef()
	lockStr := readLockState(ctx, sess, "unlocked (auto)")

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionId")
		ms.WriteString(ref.GetId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("peerId")
		ms.WriteString(info.GetPeerId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerId")
		ms.WriteString(ref.GetProviderId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerAccountId")
		ms.WriteString(ref.GetProviderAccountId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("lock")
		ms.WriteString(lockStr)
		ms.WriteObjectEnd()
		return formatOutput(config.Stdout, buf.Bytes(), outputFormat)
	}

	writeFields(config.Stdout, [][2]string{
		{"Session", ref.GetId()},
		{"Peer", info.GetPeerId()},
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Lock", lockStr},
	})
	return nil
}
