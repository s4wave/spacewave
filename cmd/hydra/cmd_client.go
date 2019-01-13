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
var clientObjectStoreCommands []cli.Command
var objectStoreFile string

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
var objectStoreOpArgs = &api.ObjectStoreOpRequest{}

func init() {
	clientObjectStoreCommands = append(
		clientObjectStoreCommands,
		cli.Command{
			Name:   "get",
			Usage:  "gets a object from the store",
			Action: runGetObject,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "key",
					Usage:       "key to get",
					Destination: &objectStoreOpArgs.Key,
				},
			},
		},
		cli.Command{
			Name:   "rm",
			Usage:  "deletes a object from the store",
			Action: runRmObject,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "key",
					Usage:       "key to delete",
					Destination: &objectStoreOpArgs.Key,
				},
			},
		},
		cli.Command{
			Name:   "put",
			Usage:  "puts a object in the store",
			Action: runPutObject,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "key",
					Usage:       "key to set",
					Destination: &objectStoreOpArgs.Key,
				},
				cli.StringFlag{
					Name:        "f, file",
					Usage:       "file to set the value to, or - for stdin",
					Destination: &objectStoreFile,
				},
			},
		},
		cli.Command{
			Name:   "list",
			Usage:  "lists keys in the object store",
			Action: runListObjectKeys,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "prefix",
					Usage:       "prefix to list",
					Destination: &objectStoreOpArgs.Key,
				},
			},
		},
	)
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
		cli.Command{
			Name:   "rm",
			Usage:  "Deletes a block from a bucket.",
			Action: runRmBlock,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "ref",
					Usage:       "block reference to delete",
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
					Usage:       "volume ID to get the block from, optional",
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
			Name:        "object",
			Usage:       "object store sub-commands",
			Subcommands: clientObjectStoreCommands,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "volume-id",
					Usage:       "volume ID to open the object store from",
					Destination: &objectStoreOpArgs.VolumeId,
				},
				cli.StringFlag{
					Name:        "store-id",
					Usage:       "store ID to open",
					Destination: &objectStoreOpArgs.StoreName,
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
