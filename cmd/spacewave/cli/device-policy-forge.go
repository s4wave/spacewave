//go:build !js

package spacewave_cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
)

type devicePolicyForgeWorkerSetArgs struct {
	statePath string
	milliCPU  uint64
	memory    uint64
	backends  *cli.StringSlice
}

type devicePolicyForgeWorkerClearArgs struct {
	statePath string
}

type devicePolicyForgeWorkerShowArgs struct {
	statePath string
}

var devicePolicyValidateForgeWorker = validateDevicePolicyForgeWorker

func newDevicePolicyForgeWorkerCommand() *cli.Command {
	return &cli.Command{
		Name:  "forge-worker",
		Usage: "manage the daemon-local Forge Worker declaration",
		Subcommands: []*cli.Command{
			newDevicePolicyForgeWorkerSetCommand(),
			newDevicePolicyForgeWorkerShowCommand(),
			newDevicePolicyForgeWorkerClearCommand(),
		},
	}
}

func newDevicePolicyForgeWorkerSetCommand() *cli.Command {
	args := &devicePolicyForgeWorkerSetArgs{backends: cli.NewStringSlice()}
	return &cli.Command{
		Name:      "set",
		Usage:     "declare the Forge Worker and capacity hosted by this Device",
		ArgsUsage: "<worker-object-key>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

func newDevicePolicyForgeWorkerShowCommand() *cli.Command {
	args := &devicePolicyForgeWorkerShowArgs{}
	return &cli.Command{
		Name:   "show",
		Usage:  "show the declared Forge Worker and capacity",
		Flags:  append([]cli.Flag{statePathFlag(&args.statePath)}, deviceOutputFlag()),
		Action: args.Run,
	}
}

func newDevicePolicyForgeWorkerClearCommand() *cli.Command {
	args := &devicePolicyForgeWorkerClearArgs{}
	return &cli.Command{
		Name:   "clear",
		Usage:  "drain and remove the Forge Worker declaration",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns flags for Forge Worker policy mutation.
func (a *devicePolicyForgeWorkerSetArgs) BuildFlags() []cli.Flag {
	return append(
		daemonClientFlags(&a.statePath),
		&cli.Uint64Flag{
			Name:        "milli-cpu",
			Usage:       "total CPU capacity in milli-cores",
			Required:    true,
			Destination: &a.milliCPU,
		},
		&cli.Uint64Flag{
			Name:        "memory-bytes",
			Usage:       "total memory capacity in bytes",
			Required:    true,
			Destination: &a.memory,
		},
		&cli.StringSliceFlag{
			Name:        "backend",
			Usage:       "supported execution backend (repeatable)",
			Required:    true,
			Destination: a.backends,
		},
	)
}

// Run validates and replaces the Forge Worker policy declaration.
func (a *devicePolicyForgeWorkerSetArgs) Run(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("forge-worker set requires <worker-object-key>")
	}
	workerObjectKey := strings.TrimSpace(c.Args().Get(0))
	if workerObjectKey == "" {
		return errors.New("forge-worker worker object key is required")
	}
	if a.milliCPU == 0 {
		return errors.New("forge-worker milli-cpu must be greater than zero")
	}
	if a.memory == 0 {
		return errors.New("forge-worker memory-bytes must be greater than zero")
	}
	backends, err := normalizedForgeWorkerBackends(a.backends.Value())
	if err != nil {
		return err
	}
	return runDevicePolicyMutationValidated(
		c,
		a.statePath,
		func(policy *device_policy.DevicePolicy) error {
			policy.ForgeWorker = &device_policy.ForgeWorkerPolicy{
				WorkerObjectKey: workerObjectKey,
				MilliCpu:        a.milliCPU,
				MemoryBytes:     a.memory,
				Backends:        backends,
			}
			return nil
		},
		func(client *sdkClient, statePath string, policy *device_policy.DevicePolicy) error {
			return devicePolicyValidateForgeWorker(c.Context, statePath, client, policy.GetForgeWorker())
		},
	)
}

// Run prints the exact local Forge Worker declaration.
func (a *devicePolicyForgeWorkerShowArgs) Run(c *cli.Context) error {
	if c.NArg() != 0 {
		return errors.New("forge-worker show does not accept arguments")
	}
	statePath, err := resolveStatePathFromContext(c, a.statePath)
	if err != nil {
		return err
	}
	policy, err := device_policy.ReadFile(statePath)
	if err != nil {
		return err
	}
	worker := policy.GetForgeWorker()
	outputFormat := c.String("output")
	if outputFormat == "json" || outputFormat == "yaml" {
		if worker == nil {
			return formatOutput([]byte("null"), outputFormat)
		}
		data, err := worker.MarshalJSON()
		if err != nil {
			return errors.Wrap(err, "marshal Forge Worker policy")
		}
		return formatOutput(data, outputFormat)
	}
	if outputFormat != "text" {
		return formatOutput(nil, outputFormat)
	}
	if worker == nil {
		_, err = fmt.Fprintln(os.Stdout, "Forge Worker: not declared")
		return err
	}
	_, err = fmt.Fprintf(
		os.Stdout,
		"Worker: %s\nCPU: %d milli-cores\nMemory: %d bytes\nBackends: %s\n",
		worker.GetWorkerObjectKey(),
		worker.GetMilliCpu(),
		worker.GetMemoryBytes(),
		strings.Join(worker.GetBackends(), ", "),
	)
	return err
}

// BuildFlags returns flags for Forge Worker policy removal.
func (a *devicePolicyForgeWorkerClearArgs) BuildFlags() []cli.Flag {
	return daemonClientFlags(&a.statePath)
}

// Run removes the Forge Worker declaration so the daemon drains its capacity.
func (a *devicePolicyForgeWorkerClearArgs) Run(c *cli.Context) error {
	if c.NArg() != 0 {
		return errors.New("forge-worker clear does not accept arguments")
	}
	return runDevicePolicyMutation(c, a.statePath, func(policy *device_policy.DevicePolicy) error {
		if policy.GetForgeWorker() == nil {
			return errors.New("forge-worker is not declared")
		}
		policy.ForgeWorker = nil
		return nil
	})
}

func normalizedForgeWorkerBackends(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	backends := make([]string, 0, len(values))
	for _, value := range values {
		backend := strings.TrimSpace(value)
		if backend == "" {
			return nil, errors.New("forge-worker backend must not be empty")
		}
		if strings.ContainsAny(backend, ", \t\r\n") {
			return nil, errors.Errorf("forge-worker backend %q cannot contain comma or whitespace", backend)
		}
		if _, ok := seen[backend]; ok {
			return nil, errors.Errorf("duplicate forge-worker backend %q", backend)
		}
		seen[backend] = struct{}{}
		backends = append(backends, backend)
	}
	if len(backends) == 0 {
		return nil, errors.New("forge-worker requires at least one backend")
	}
	slices.Sort(backends)
	return backends, nil
}

func validateDevicePolicyForgeWorker(
	ctx context.Context,
	statePath string,
	client *sdkClient,
	policy *device_policy.ForgeWorkerPolicy,
) error {
	if policy == nil {
		return errors.New("forge-worker policy is required")
	}
	record, ok, err := deviceLauncherProjectionTarget(statePath)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("completed Device session is required before declaring a Forge Worker")
	}
	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return err
	}
	defer sess.Release()
	spaceID, err := decodeDeviceResourceID(record.ResourceID)
	if err != nil {
		return err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	defer spaceCleanup()
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer engineCleanup()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	workerObjectKey := policy.GetWorkerObjectKey()
	if err := verifyForgeWorkerLink(ctx, tx, workerObjectKey); err != nil {
		return err
	}
	keypairs, _, err := forge_worker.CollectWorkerKeypairs(ctx, tx, workerObjectKey)
	if err != nil {
		return errors.Wrap(err, "collect Forge Worker keypairs")
	}
	var workerPeerIDs []string
	for _, keypair := range keypairs {
		if keypair == nil {
			continue
		}
		peerID, parseErr := keypair.ParsePeerID()
		if parseErr != nil {
			continue
		}
		workerPeerID := peerID.String()
		if workerPeerID == record.SessionPeerID {
			return nil
		}
		workerPeerIDs = append(workerPeerIDs, workerPeerID)
	}
	if len(workerPeerIDs) == 0 {
		return errors.Errorf("Forge Worker %q has no usable keypair", workerObjectKey)
	}
	return errors.Errorf(
		"Forge Worker %q peers %q do not include Device session peer %q",
		workerObjectKey,
		workerPeerIDs,
		record.SessionPeerID,
	)
}
