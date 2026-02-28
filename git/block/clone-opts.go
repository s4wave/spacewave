package git_block

import (
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

// IsEmpty checks if there are no clone URL set.
func (c *CloneOpts) IsEmpty() bool {
	return c.GetUrl() == ""
}

// Validate performs cursory checks of the config.
func (c *CloneOpts) Validate() error {
	if len(c.GetUrl()) == 0 {
		return ErrEmptyURL
	}
	return nil
}

// BuildCloneOpts constructs the go-git clone configuration.
// Note: the auth options are /not/ applied yet.
func (c *CloneOpts) BuildCloneOpts() *git.CloneOptions {
	recurseSubmodules := git.NoRecurseSubmodules
	if c.GetRecursive() {
		recurseSubmodules = git.DefaultSubmoduleRecursionDepth
		if d := c.GetRecursionDepth(); d != 0 {
			recurseSubmodules = git.SubmoduleRecursivity(d)
		}
	}

	tagMode := c.GetTagMode().ToGitTagMode()
	return &git.CloneOptions{
		URL:               c.GetUrl(),
		RemoteName:        c.GetRemoteName(),
		ReferenceName:     plumbing.ReferenceName(c.GetRef()),
		SingleBranch:      c.GetSingleBranch(),
		NoCheckout:        c.GetDisableCheckout(),
		Depth:             int(c.GetDepth()),
		RecurseSubmodules: recurseSubmodules,
		Tags:              tagMode,
	}
}
