package main

import (
	"strings"
	"sync"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
)

var clientDialAddr string
var clientCommands []cli.Command
var clientBlockCommands []cli.Command

var remotePeerIdsCsv string

func parseRemotePeerIdsCsv() []string {
	pts := strings.Split(remotePeerIdsCsv, ",")
	var peerIds []string
	for _, pt := range pts {
		pt = strings.TrimSpace(pt)
		peerIds = append(peerIds, pt)
	}
	return peerIds
}

var bucketOpArgs = &volume.BucketOpArgs{}

func init() {
	clientBlockCommands = append(
		clientBlockCommands,
		cli.Command{
			Name:   "put",
			Usage:  "Puts a block into a bucket.",
			Action: runPutBlock,
			Flags: []cli.Flag{
				//  TODO: override put opts
				cli.StringFlag{
					Name:        "f, file",
					Usage:       "file to read the block data from, or - or empty for stdin",
					Destination: &blockDataFile,
				},
			},
		},
		cli.Command{
			Name:   "get",
			Usage:  "Gets a block from a bucket.",
			Action: runGetBlock,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "ref",
					Usage:       "block reference to fetch",
					Destination: &getBlockRef,
				},
			},
		},
	)
	clientCommands = append(
		clientCommands,
		cli.Command{
			Name:        "block",
			Usage:       "volume bucket handle block sub-commands",
			Subcommands: clientBlockCommands,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "volume-id",
					Usage:       "volume ID to get the block from",
					Destination: &bucketOpArgs.VolumeId,
				},
				cli.StringFlag{
					Name:        "bucket-id",
					Usage:       "bucket id to get the block from",
					Destination: &bucketOpArgs.BucketId,
				},
			},
		},
		cli.Command{
			Name:   "apply-bucket-conf",
			Usage:  "Apply a bucket conf to one or more volumes.",
			Action: runApplyBucketConf,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "volume-regex",
					Usage:       "regex to filter volumes to apply the config to, if empty, applies to volumes that already have the bucket",
					Destination: &applyBucketConfVolumeRegex,
				},
				cli.StringFlag{
					Name:        "f, file",
					Usage:       "file to read the configuration from",
					Destination: &applyBucketConfFile,
				},
			},
		},
		cli.Command{
			Name:   "list-buckets",
			Usage:  "Lists local bucket info across multiple volumes.",
			Action: runListBuckets,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "bucket-id",
					Usage:       "limits information to a specific bucket",
					Destination: &listBucketRequest.BucketId,
				},
				cli.StringFlag{
					Name:        "volume-id-re",
					Usage:       "limits information to a specific volume or set of volumes",
					Destination: &listBucketRequest.VolumeRe,
				},
			},
		},
		cli.Command{
			Name:   "list-volumes",
			Usage:  "Lists local attached volume info.",
			Action: runListVolumes,
		},
	)
	commands = append(
		commands,
		cli.Command{
			Name:        "client",
			Usage:       "client sub-commands",
			After:       runCloseClient,
			Subcommands: clientCommands,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "dial-addr",
					Usage:       "address to dial API on",
					Destination: &clientDialAddr,
					Value:       "localhost:5110",
				},
			},
		},
	)
}

var clientMtx sync.Mutex
var client api.HydraDaemonServiceClient
var clientConn *grpc.ClientConn

// GetClient builds / returns the client.
func GetClient() (api.HydraDaemonServiceClient, error) {
	clientMtx.Lock()
	defer clientMtx.Unlock()

	if client != nil {
		return client, nil
	}

	var err error
	clientConn, err = grpc.Dial(clientDialAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	client = api.NewHydraDaemonServiceClient(clientConn)
	return client, nil
}

func runCloseClient(ctx *cli.Context) error {
	if clientConn != nil {
		clientConn.Close()
		clientConn = nil
	}
	return nil
}
