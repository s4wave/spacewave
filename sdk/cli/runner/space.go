package runner

import (
	"context"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// RunSpaceList executes the shared space list command against the configured client factory.
func RunSpaceList(config Config, c *cli.Context, outputFormat string, sessionIdx uint32, watch bool) error {
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

	strm, err := sess.WatchResourcesList(ctx)
	if err != nil {
		return errors.Wrap(err, "watch resources list")
	}
	defer strm.Close()

	resp, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv resources list")
	}
	if err := writeSpacesList(config, outputFormat, resp); err != nil {
		return err
	}
	if !watch {
		return nil
	}

	for {
		resp, err = strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv resources list")
		}
		if _, err := config.Stdout.Write([]byte("\n--- " + config.Now().Format(time.RFC3339) + " ---\n")); err != nil {
			return err
		}
		if err := writeSpacesList(config, outputFormat, resp); err != nil {
			return err
		}
	}
}

func writeSpacesList(config Config, outputFormat string, resp *s4wave_session.WatchResourcesListResponse) error {
	switch outputFormat {
	case "json", "yaml":
		data, err := resp.MarshalJSON()
		if err != nil {
			return err
		}
		return formatOutput(config.Stdout, data, outputFormat)
	default:
		printSpacesList(config.Stdout, resp)
		return nil
	}
}

func printSpacesList(w interface{ Write([]byte) (int, error) }, resp *s4wave_session.WatchResourcesListResponse) {
	spaces := resp.GetSpacesList()
	if len(spaces) == 0 {
		w.Write([]byte("no spaces\n"))
		return
	}
	rows := [][]string{{"ID", "NAME"}}
	for _, sp := range spaces {
		rows = append(rows, []string{
			truncateID(sp.GetEntry().GetRef().GetProviderResourceRef().GetId(), 8),
			sp.GetSpaceMeta().GetName(),
		})
	}
	writeTable(w, "", rows)
}
