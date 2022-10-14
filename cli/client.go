package cli

import (
	"context"
	"errors"
	"net"
	"strings"

	bifrost_cli "github.com/aperturerobotics/bifrost/cli"
	cbus_cli "github.com/aperturerobotics/controllerbus/cli"
	"github.com/aperturerobotics/hydra/bucket"
	api "github.com/aperturerobotics/hydra/daemon/api"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	ucli "github.com/urfave/cli/v2"
)

// ListBucketsConf is the list buckets request
type ListBucketsConf = volume.ListBucketsRequest

// ClientArgs contains the client arguments and functions.
type ClientArgs struct {
	// ListBucketsConf configures listing buckets.
	ListBucketsConf ListBucketsConf
	// BucketOpArgs are bucket operation arguments.
	BucketOpArgs bucket.BucketOpArgs
	// ObjectStoreOpReq configures object store operations.
	ObjectStoreOpReq api.ObjectStoreOpRequest
	// ApplyBucketConfigReq configures applying a bucket config.
	ApplyBucketConfigReq api.ApplyBucketConfigRequest
	// ApplyBucketConfigReqVolumeIDs is the list of volume IDs to apply to.
	ApplyBucketConfigReqVolumeIDs cli.StringSlice
	// ListBucketsRequest configures listing buckets.
	ListBucketsReq volume.ListBucketsRequest
	// CbusConf is the controller-bus configuration.
	CbusConf cbus_cli.ClientArgs
	// BifrostConf is the controller-bus configuration.
	BifrostConf bifrost_cli.ClientArgs

	// le is the logger entry
	le *logrus.Entry
	// ctx is the context
	ctx context.Context
	// client is the client instance
	client api.HydraDaemonClient

	// DialAddr is the address to dial.
	DialAddr string

	// RemotePeerIdsCsv are the set of remote peer IDs to connect to.
	RemotePeerIdsCsv string

	// BlockDataFile is the path to the file to load/store for blocks.
	BlockDataFile string
	// ObjectStoreFile is the path used for object store ops.
	ObjectStoreFile string
	// GetBlockRef is the block reference to get.
	GetBlockRef string
	// ApplyBucketConfigFile is the path used for bucket config.
	ApplyBucketConfigFile string
}

// ParseRemotePeerIdsCsv parses the RemotePeerIdsCsv field.
func (a *ClientArgs) ParseRemotePeerIdsCsv() []string {
	pts := strings.Split(a.RemotePeerIdsCsv, ",")
	var peerIds []string
	for _, pt := range pts {
		pt = strings.TrimSpace(pt)
		peerIds = append(peerIds, pt)
	}
	return peerIds
}

// BuildFlags attaches the flags to a flag set.
func (a *ClientArgs) BuildFlags() []ucli.Flag {
	return []ucli.Flag{
		&ucli.StringFlag{
			Name:        "dial-addr",
			Usage:       "address to dial API on",
			Destination: &a.DialAddr,
			Value:       "127.0.0.1:5110",
		},
	}
}

// SetClient sets the client instance.
func (a *ClientArgs) SetClient(client api.HydraDaemonClient) {
	a.client = client
}

// BuildClient builds the client or returns it if it has been set.
func (a *ClientArgs) BuildClient() (api.HydraDaemonClient, error) {
	if a.client != nil {
		return a.client, nil
	}

	if a.DialAddr == "" {
		return nil, errors.New("dial address is not set")
	}

	nconn, err := net.Dial("tcp", a.DialAddr)
	if err != nil {
		return nil, err
	}
	muxedConn, err := srpc.NewMuxedConn(nconn, false)
	if err != nil {
		return nil, err
	}
	client := srpc.NewClientWithMuxedConn(muxedConn)
	a.client = api.NewHydraDaemonClient(client)
	return a.client, nil
}

// BuildHydraCommand returns the hydra sub-command set.
func (a *ClientArgs) BuildHydraCommand() *cli.Command {
	hydraCmds := a.BuildCommands()
	return &cli.Command{
		Name:        "hydra",
		Usage:       "Hydra storage sub-commands.",
		Subcommands: hydraCmds,
	}
}

// BuildCommands attaches the commands.
func (a *ClientArgs) BuildCommands() []*ucli.Command {
	clientBlockCommands := []*ucli.Command{
		{
			Name:   "put",
			Usage:  "Puts a block into a bucket.",
			Action: a.RunPutBlock,
			Flags: []ucli.Flag{
				//  TODO: override put opts
				&ucli.StringFlag{
					Name:        "f, file",
					Usage:       "file to read the block data from, or - or empty for stdin",
					Destination: &a.BlockDataFile,
				},
			},
		},
		{
			Name:   "get",
			Usage:  "Gets a block from a bucket.",
			Action: a.RunGetBlock,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "ref",
					Usage:       "block reference to fetch",
					Destination: &a.GetBlockRef,
				},
			},
		},
		{
			Name:   "rm",
			Usage:  "Deletes a block from a bucket.",
			Action: a.RunRmBlock,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "ref",
					Usage:       "block reference to delete",
					Destination: &a.GetBlockRef,
				},
			},
		},
	}
	clientObjectStoreCommands := []*ucli.Command{
		{
			Name:   "get",
			Usage:  "gets a object from the store",
			Action: a.RunGetObject,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "key",
					Usage:       "key to get",
					Destination: &a.ObjectStoreOpReq.Key,
				},
			},
		},
		{
			Name:   "rm",
			Usage:  "deletes a object from the store",
			Action: a.RunRmObject,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "key",
					Usage:       "key to delete",
					Destination: &a.ObjectStoreOpReq.Key,
				},
			},
		},
		{
			Name:   "put",
			Usage:  "puts a object in the store",
			Action: a.RunPutObject,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "key",
					Usage:       "key to set",
					Destination: &a.ObjectStoreOpReq.Key,
				},
				&ucli.StringFlag{
					Name:        "f, file",
					Usage:       "file to set the value to, or - for stdin",
					Destination: &a.ObjectStoreFile,
				},
			},
		},
		{
			Name:   "list",
			Usage:  "lists keys in the object store",
			Action: a.RunListObjectKeys,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "prefix",
					Usage:       "prefix to list",
					Destination: &a.ObjectStoreOpReq.Key,
				},
			},
		},
	}
	clientBucketStoreCommands := []*ucli.Command{
		{
			Name:   "config",
			Usage:  "Apply a bucket conf to one or more volumes.",
			Action: a.RunApplyBucketConf,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "volume-regex",
					Usage:       "regex of volume IDs to apply to, if empty, applies to volumes that already have the bucket. cannot be set with volume-ids.",
					Destination: &a.ApplyBucketConfigReq.VolumeIdRe,
				},
				&ucli.StringSliceFlag{
					Name:        "volume-ids",
					Usage:       "list of volume IDs to apply to, if empty, uses volume-regex instead.",
					Value:       &a.ApplyBucketConfigReqVolumeIDs,
					Destination: &a.ApplyBucketConfigReqVolumeIDs,
				},
				&ucli.StringFlag{
					Name:        "f, file",
					Usage:       "file to read the configuration from",
					Destination: &a.ApplyBucketConfigFile,
				},
			},
		},
		{
			Name:   "list",
			Usage:  "Lists local bucket info across multiple volumes.",
			Action: a.RunListBuckets,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "bucket-id",
					Usage:       "limits information to a specific bucket",
					Destination: &a.ListBucketsReq.BucketId,
				},
				&ucli.StringFlag{
					Name:        "volume-id-re",
					Usage:       "limits information to a specific volume or set of volumes",
					Destination: &a.ListBucketsReq.VolumeRe,
				},
			},
		},
	}
	clientVolumeCommands := []*ucli.Command{{
		Name:   "list-volumes",
		Usage:  "Lists local attached volume info.",
		Action: a.RunListVolumes,
	}}
	return []*ucli.Command{
		{
			Name:        "block",
			Usage:       "volume bucket handle block sub-commands",
			Subcommands: clientBlockCommands,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "volume-id",
					Usage:       "volume ID to get the block from, optional",
					Destination: &a.BucketOpArgs.VolumeId,
				},
				&ucli.StringFlag{
					Name:        "bucket-id",
					Usage:       "bucket id to get the block from",
					Destination: &a.BucketOpArgs.BucketId,
				},
			},
		},
		{
			Name:        "bucket",
			Usage:       "bucket store sub-commands",
			Subcommands: clientBucketStoreCommands,
		},
		{
			Name:        "object",
			Usage:       "object store sub-commands",
			Subcommands: clientObjectStoreCommands,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "volume-id",
					Usage:       "volume ID to open the object store from",
					Destination: &a.ObjectStoreOpReq.VolumeId,
				},
				&ucli.StringFlag{
					Name:        "store-id",
					Usage:       "store ID to open",
					Destination: &a.ObjectStoreOpReq.StoreName,
				},
			},
		},
		{
			Name:        "volume",
			Usage:       "volume sub-commands",
			Subcommands: clientVolumeCommands,
		},
	}
}

// SetContext sets the context.
func (a *ClientArgs) SetContext(c context.Context) {
	a.ctx = c
}

// GetContext returns the context.
func (a *ClientArgs) GetContext() context.Context {
	if c := a.ctx; c != nil {
		return c
	}
	return context.TODO()
}

// SetLogger sets the root log entry.
func (a *ClientArgs) SetLogger(le *logrus.Entry) {
	a.le = le
}

// GetLogger returns the log entry
func (a *ClientArgs) GetLogger() *logrus.Entry {
	if le := a.le; le != nil {
		return le
	}
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	return logrus.NewEntry(log)
}
