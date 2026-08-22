//go:build !js

package spacewave_cli

import (
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

type devicePolicyEnableShellArgs struct {
	statePath string
	disable   bool
}

type devicePolicyCheckoutRootAddArgs struct {
	statePath string
	write     bool
}

type devicePolicyCheckoutRootRemoveArgs struct {
	statePath string
}

type devicePolicySensorAddArgs struct {
	statePath string
	kind      string
	disable   bool
}

type devicePolicySensorRemoveArgs struct {
	statePath string
}

var devicePolicyReloadDaemon = requestDevicePolicyReload

func newDevicePolicyCommand() *cli.Command {
	return &cli.Command{
		Name:  "policy",
		Usage: "manage daemon-local Device policy",
		Subcommands: []*cli.Command{
			newDevicePolicyEnableShellCommand(),
			newDevicePolicyCheckoutRootCommand(),
			newDevicePolicySensorCommand(),
		},
	}
}

func newDevicePolicySensorCommand() *cli.Command {
	return &cli.Command{
		Name:  "sensor",
		Usage: "manage daemon-local sensor endpoint declarations",
		Subcommands: []*cli.Command{
			newDevicePolicySensorAddCommand(),
			newDevicePolicySensorRemoveCommand(),
		},
	}
}

func newDevicePolicySensorAddCommand() *cli.Command {
	args := &devicePolicySensorAddArgs{}
	return &cli.Command{
		Name:      "add",
		Usage:     "add or update a daemon-local sensor endpoint",
		ArgsUsage: "<id> <host:port>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

func newDevicePolicySensorRemoveCommand() *cli.Command {
	args := &devicePolicySensorRemoveArgs{}
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove a daemon-local sensor endpoint",
		ArgsUsage: "<id>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

func newDevicePolicyEnableShellCommand() *cli.Command {
	args := &devicePolicyEnableShellArgs{}
	return &cli.Command{
		Name:   "enable-shell",
		Usage:  "enable the daemon-local remote shell policy",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

func newDevicePolicyCheckoutRootCommand() *cli.Command {
	return &cli.Command{
		Name:  "checkout-root",
		Usage: "manage daemon-local checkout-root declarations",
		Subcommands: []*cli.Command{
			newDevicePolicyCheckoutRootAddCommand(),
			newDevicePolicyCheckoutRootRemoveCommand(),
		},
	}
}

func newDevicePolicyCheckoutRootAddCommand() *cli.Command {
	args := &devicePolicyCheckoutRootAddArgs{}
	return &cli.Command{
		Name:      "add",
		Usage:     "add a daemon-local checkout root",
		ArgsUsage: "<name> <path>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

func newDevicePolicyCheckoutRootRemoveCommand() *cli.Command {
	args := &devicePolicyCheckoutRootRemoveArgs{}
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove a daemon-local checkout root",
		ArgsUsage: "<name>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// BuildFlags returns flags for remote-shell policy mutation.
func (a *devicePolicyEnableShellArgs) BuildFlags() []cli.Flag {
	return append(
		daemonClientFlags(&a.statePath),
		&cli.BoolFlag{
			Name:        "disable",
			Usage:       "disable remote shell instead of enabling it",
			Destination: &a.disable,
		},
	)
}

// Run updates the remote-shell policy and signals the daemon.
func (a *devicePolicyEnableShellArgs) Run(c *cli.Context) error {
	return runDevicePolicyMutation(c, a.statePath, func(policy *device_policy.DevicePolicy) error {
		if policy.RemoteShell == nil {
			policy.RemoteShell = &device_policy.RemoteShellPolicy{}
		}
		policy.RemoteShell.Enabled = !a.disable
		if a.disable {
			policy.RemoteShell.Detail = "terminal disabled by local policy"
			return nil
		}
		policy.RemoteShell.Detail = "terminal enabled by local policy"
		return nil
	})
}

// BuildFlags returns flags for checkout-root add.
func (a *devicePolicyCheckoutRootAddArgs) BuildFlags() []cli.Flag {
	return append(
		daemonClientFlags(&a.statePath),
		&cli.BoolFlag{
			Name:        "write",
			Usage:       "allow write access after approval",
			Destination: &a.write,
		},
	)
}

// Run adds or replaces a checkout-root policy declaration.
func (a *devicePolicyCheckoutRootAddArgs) Run(c *cli.Context) error {
	if c.NArg() != 2 {
		return errors.New("checkout-root add requires <name> <path>")
	}
	name := strings.TrimSpace(c.Args().Get(0))
	if name == "" {
		return errors.New("checkout-root name is required")
	}
	rawPath := strings.TrimSpace(c.Args().Get(1))
	if rawPath == "" {
		return errors.New("checkout-root path is required")
	}
	path, err := filepath.Abs(rawPath)
	if err != nil {
		return errors.Wrap(err, "resolve checkout-root path")
	}
	access := s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY
	if a.write {
		access = s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE
	}
	return runDevicePolicyMutation(c, a.statePath, func(policy *device_policy.DevicePolicy) error {
		root := &device_policy.CheckoutRootPolicy{
			Name:      name,
			LocalPath: filepath.Clean(path),
			Access:    access,
		}
		for i, existing := range policy.GetCheckoutRoot() {
			if existing == nil {
				continue
			}
			if strings.TrimSpace(existing.GetName()) == name {
				policy.CheckoutRoot[i] = root
				return nil
			}
		}
		policy.CheckoutRoot = append(policy.CheckoutRoot, root)
		return nil
	})
}

// BuildFlags returns flags for checkout-root remove.
func (a *devicePolicyCheckoutRootRemoveArgs) BuildFlags() []cli.Flag {
	return daemonClientFlags(&a.statePath)
}

// Run removes a checkout-root policy declaration.
func (a *devicePolicyCheckoutRootRemoveArgs) Run(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("checkout-root remove requires <name>")
	}
	name := strings.TrimSpace(c.Args().Get(0))
	if name == "" {
		return errors.New("checkout-root name is required")
	}
	return runDevicePolicyMutation(c, a.statePath, func(policy *device_policy.DevicePolicy) error {
		for i, root := range policy.GetCheckoutRoot() {
			if root == nil || strings.TrimSpace(root.GetName()) != name {
				continue
			}
			policy.CheckoutRoot = append(policy.CheckoutRoot[:i], policy.CheckoutRoot[i+1:]...)
			return nil
		}
		return errors.Errorf("checkout-root %q is not declared", name)
	})
}

// parseSensorAdapterKind maps a CLI adapter-kind name onto its policy enum.
func parseSensorAdapterKind(raw string) (s4wave_device.SensorAdapterKind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "esphome":
		return s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME, nil
	default:
		return s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_UNKNOWN,
			errors.Errorf("unknown sensor adapter kind %q; supported: esphome", raw)
	}
}

// BuildFlags returns flags for sensor add.
func (a *devicePolicySensorAddArgs) BuildFlags() []cli.Flag {
	return append(
		daemonClientFlags(&a.statePath),
		&cli.StringFlag{
			Name:        "kind",
			Usage:       "sensor adapter kind (esphome)",
			Value:       "esphome",
			Destination: &a.kind,
		},
		&cli.BoolFlag{
			Name:        "disable",
			Usage:       "declare the endpoint but keep it disabled",
			Destination: &a.disable,
		},
	)
}

// Run adds or replaces one sensor endpoint declaration by stable ID. The
// address stays in daemon-local policy and never enters World state or
// command output.
func (a *devicePolicySensorAddArgs) Run(c *cli.Context) error {
	if c.NArg() != 2 {
		return errors.New("sensor add requires <id> <host:port>")
	}
	id := strings.TrimSpace(c.Args().Get(0))
	if id == "" {
		return errors.New("sensor endpoint id is required")
	}
	address := strings.TrimSpace(c.Args().Get(1))
	if address == "" {
		return errors.New("sensor endpoint address is required")
	}
	kind, err := parseSensorAdapterKind(a.kind)
	if err != nil {
		return err
	}
	return runDevicePolicyMutation(c, a.statePath, func(policy *device_policy.DevicePolicy) error {
		endpoint := &device_policy.SensorEndpointPolicy{
			Id:          id,
			Enabled:     !a.disable,
			AdapterKind: kind,
			Endpoint:    address,
		}
		for i, existing := range policy.GetSensorEndpoint() {
			if existing == nil {
				continue
			}
			if strings.TrimSpace(existing.GetId()) == id {
				policy.SensorEndpoint[i] = endpoint
				return nil
			}
		}
		policy.SensorEndpoint = append(policy.SensorEndpoint, endpoint)
		return nil
	})
}

// BuildFlags returns flags for sensor remove.
func (a *devicePolicySensorRemoveArgs) BuildFlags() []cli.Flag {
	return daemonClientFlags(&a.statePath)
}

// Run removes one sensor endpoint declaration by stable ID.
func (a *devicePolicySensorRemoveArgs) Run(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("sensor remove requires <id>")
	}
	id := strings.TrimSpace(c.Args().Get(0))
	if id == "" {
		return errors.New("sensor endpoint id is required")
	}
	return runDevicePolicyMutation(c, a.statePath, func(policy *device_policy.DevicePolicy) error {
		for i, endpoint := range policy.GetSensorEndpoint() {
			if endpoint == nil || strings.TrimSpace(endpoint.GetId()) != id {
				continue
			}
			policy.SensorEndpoint = append(policy.SensorEndpoint[:i], policy.SensorEndpoint[i+1:]...)
			return nil
		}
		return errors.Errorf("sensor endpoint %q is not declared", id)
	})
}

func runDevicePolicyMutation(
	c *cli.Context,
	statePath string,
	mutate func(*device_policy.DevicePolicy) error,
) error {
	ctx := c.Context
	resolvedStatePath, _, err := resolveDeviceDaemonPaths(c, statePath)
	if err != nil {
		return err
	}
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolvedStatePath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()

	policy, err := device_policy.ReadFile(resolvedStatePath)
	if err != nil {
		return err
	}
	if err := mutate(policy); err != nil {
		return err
	}
	policy.Revision++
	if err := device_policy.WriteFile(resolvedStatePath, policy); err != nil {
		return err
	}
	return devicePolicyReloadDaemon(ctx, client)
}
