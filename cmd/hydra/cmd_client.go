package main

import (
	"strings"
	"sync"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
)

var clientDialAddr string
var clientCommands []cli.Command

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

func init() {
	clientCommands = append(
		clientCommands,
		cli.Command{
			Name:   "list-volumes",
			Usage:  "Lists local attached volume info.",
			Action: runListVolumes,
		},
		cli.Command{
			Name:   "list-buckets",
			Usage:  "Lists local bucket info.",
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
			Name:   "apply-bucket-conf",
			Usage:  "Apply a bucket conf json file.",
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
			Name:   "put-block",
			Usage:  "Puts a block into a bucket.",
			Action: runPutBlock,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "volume-regex",
					Usage:       "regex to filter volumes to put the block into, if empty, applies to volumes that have the bucket",
					Destination: &putBlockVolumeRegex,
				},
				cli.StringFlag{
					Name:        "bucket-id",
					Usage:       "bucket id to put the block to",
					Destination: &putBlockBucketID,
				},
				//  TODO: override put opts
				cli.StringFlag{
					Name:        "f, file",
					Usage:       "file to read the block data from, or - or empty for stdin",
					Destination: &blockDataFile,
				},
			},
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
