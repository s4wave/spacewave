package git_block

import "github.com/go-git/go-git/v5/config"

// ApplyConfigPatches applies mandatory modifications to a Git config.
func ApplyConfigPatches(c *config.Config) {
	e := config.Config{} // e is a empty config for dropping fields
	c.Core.IsBare = true
	c.Core.Worktree = ""
	c.User = e.User
	c.Author = e.Author
	c.Committer = e.Committer
}

// Config parses and returns the Git config associated with the repo.
func (r *Store) Config() (*config.Config, error) {
	conf := config.NewConfig()
	if err := conf.Unmarshal([]byte(r.root.GetGitConfig())); err != nil {
		return nil, err
	}
	ApplyConfigPatches(conf)
	return conf, nil
}

// SetConfig marshals and sets the Git config associated with the repo.
func (r *Store) SetConfig(c *config.Config) error {
	if c == nil {
		c = config.NewConfig()
	}
	ApplyConfigPatches(c)
	dat, err := c.Marshal()
	if err != nil {
		return err
	}
	nextConf := string(dat)
	if r.bcs != nil && nextConf != r.root.GetGitConfig() {
		r.root.GitConfig = nextConf
		r.bcs.SetBlock(r.root, true)
	}
	return nil
}

// _ is a type assertion
var _ config.ConfigStorer = (*Store)(nil)
