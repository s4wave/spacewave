//go:build !tinygo

package forge_lib_git_clone

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	transport_ssh "github.com/go-git/go-git/v6/plumbing/transport/ssh"
	"github.com/pkg/errors"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
	git_urls "github.com/whilp/git-urls"
	"golang.org/x/crypto/ssh"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/git/clone"

const (
	// inputNameWorld is the name of the Input for the target World.
	inputNameWorld = "world"
	// outputNameRepo is the name of the Output for the Repo snapshot.
	outputNameRepo = "repo"
)

// Controller implements the git clone controller.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// inputVals is the input values map
	inputVals forge_target.InputMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new git clone controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"git clone controller",
	)
}

// InitForgeExecController initializes the Forge execution controller.
// This is called before Execute().
// Any error returned cancels execution of the controller.
func (c *Controller) InitForgeExecController(
	ctx context.Context,
	inputVals forge_target.InputMap,
	handle forge_target.ExecControllerHandle,
) error {
	c.inputVals, c.handle = inputVals, handle
	return c.conf.Validate()
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// Read the sender and timestamp, and require the World input.
	sender := c.handle.GetPeerId()
	ts := c.handle.GetTimestamp()
	inWorld := c.inputVals[inputNameWorld]
	if inWorld == nil || inWorld.IsEmpty() {
		return errors.New("target world input must be set")
	}

	// Resolve the target World from the execution input.
	ipv, err := forge_target.InputValueToWorld(inWorld)
	if err != nil {
		return errors.Wrap(err, "world")
	}

	// Resolve the configured credentials, taking the SSH user from the URL.
	ws := ipv.GetWorldState()
	cloneURL := c.conf.GetCloneOpts().GetUrl()
	authMethod, err := c.conf.GetAuthOpts().ResolveAuth(ctx, c.bus)
	if err != nil {
		return err
	}
	if sshMethod, ok := authMethod.(*transport_ssh.PublicKeys); ok {
		if signer := sshMethod.Signer; signer != nil {
			authorizedKey := ssh.MarshalAuthorizedKey(signer.PublicKey())
			c.le.Debugf("using public key for auth: %s", string(authorizedKey[:len(authorizedKey)-1]))
		}
		if sshMethod.User == "" {
			uri, err := git_urls.Parse(cloneURL)
			if err != nil {
				return err
			}
			sshMethod.User = uri.User.Username()
		}
	}

	// Clone the repository, or fetch its branch when the World holds it.
	repoRef, err := c.conf.CloneOrFetch(ctx, c.le, ws, sender, ts, authMethod, os.Stderr)
	if err != nil {
		return err
	}

	// Publish the repository snapshot reference as the output.
	outps := forge_value.ValueSlice{
		forge_value.NewValueWithBucketRef(outputNameRepo, repoRef),
	}
	return c.handle.SetOutputs(ctx, outps, true)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ forge_target.ExecController = (*Controller)(nil)
