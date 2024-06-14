package forge_lib_git_clone

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/bucket"
	git_world "github.com/aperturerobotics/hydra/git/world"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"
	transport_ssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	git_urls "github.com/whilp/git-urls"
	"golang.org/x/crypto/ssh"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

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
	// lookup the world engine
	sender := c.handle.GetPeerId()
	ts := c.handle.GetTimestamp()
	inWorld := c.inputVals[inputNameWorld]
	if inWorld == nil || inWorld.IsEmpty() {
		return errors.New("target world input must be set")
	}

	ipv, err := forge_target.InputValueToWorld(inWorld)
	if err != nil {
		return errors.Wrap(err, "world")
	}

	ws := ipv.GetWorldState()
	repoObjKey := c.conf.GetObjectKey()
	alreadyExistsObj, alreadyExists, err := ws.GetObject(ctx, repoObjKey)
	if err != nil {
		return err
	}

	cloneOpts := c.conf.GetCloneOpts()
	worktreeOpts := c.conf.GetWorktreeOpts().CloneVT()
	if worktreeOpts != nil {
		worktreeOpts.RepoObjectKey = repoObjKey
		worktreeOpts.Timestamp = ts
	}

	var repoRef *bucket.ObjectRef
	var repoRev uint64
	if alreadyExists {
		// TODO: should we do a "git fetch" here and add/or add/update the remote?
		// NOTE: in future we might configure a custom behavior here.
		repoRef, repoRev, err = alreadyExistsObj.GetRootRef(ctx)
		if err != nil {
			return err
		}
		c.le.Infof("repo already exists at rev %d: %s", repoRev, repoObjKey)

		if repoRev > 1 && !cloneOpts.GetDisableCheckout() && worktreeOpts.GetObjectKey() != "" {
			c.le.Info("initializing worktree from existing repo")
			_, err := worktreeOpts.ApplyWorldOp(ctx, c.le, ws, sender)
			if err != nil {
				return err
			}
		}
	} else {
		cloneURL := cloneOpts.GetUrl()
		authMethod, err := c.conf.GetAuthOpts().ResolveAuth(ctx, c.bus)
		if err != nil {
			return err
		}
		if sshMethod, ok := authMethod.(*transport_ssh.PublicKeys); ok {
			if signer := sshMethod.Signer; signer != nil {
				authorizedKey := ssh.MarshalAuthorizedKey(signer.PublicKey())
				c.le.Debugf("using public key for auth: %s", string(authorizedKey[:len(authorizedKey)-1]))
			}

			// parse the user from the url
			if sshMethod.User == "" {
				uri, err := git_urls.Parse(cloneURL)
				if err != nil {
					return err
				}
				sshMethod.User = uri.User.Username()
			}
		}

		// TODO: where to send progress?
		var progress sideband.Progress = os.Stderr
		c.le.Debugf(
			"git: clone %q to object %q worktree %q",
			cloneURL,
			repoObjKey,
			worktreeOpts.GetObjectKey(),
		)
		repoRef, err = git_world.GitClone(
			ctx,
			ws,
			repoObjKey,
			sender,
			cloneOpts,
			authMethod,
			progress,
			worktreeOpts,
		)
		if err != nil {
			return err
		}
		// repoRev = 1
	}

	// set the output
	outps := forge_value.ValueSlice{
		// output: repo
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
var _ forge_target.ExecController = ((*Controller)(nil))
